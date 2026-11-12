package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/nico9julio/PGuardian/internal/config"
	"github.com/nico9julio/PGuardian/internal/ipc"
	"github.com/nico9julio/PGuardian/internal/updater"
	"github.com/nico9julio/PGuardian/internal/winintegration"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func main() {
	planPath := flag.String("plan", "", "Path to update_plan.json")
	flag.Parse()

	logCloser := config.InitLogger("updater.log", false)
	defer func() { _ = logCloser.Close() }()

	if *planPath == "" {
		slog.Error("updater invoked without --plan")
		os.Exit(1)
	}

	raw, err := os.ReadFile(*planPath)
	if err != nil {
		slog.Error("failed to read update_plan.json", "error", err)
		os.Exit(1)
	}
	var plan updater.UpdatePlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		slog.Error("failed to parse update_plan.json", "error", err)
		os.Exit(1)
	}

	slog.Info("starting atomic update transaction", "from", plan.PreviousVersion, "to", plan.TargetVersion)

	if err := applyTransaction(plan); err != nil {
		slog.Error("update transaction failed; initiating automatic rollback", "error", err)
		rollbackTransaction(plan)
		os.Exit(1)
	}

	slog.Info("atomic update completed successfully", "version", plan.TargetVersion)
	if plan.RelaunchUISessionID > 0 {
		uiExe := filepath.Join(plan.InstallDir, "PGuardian.exe")
		_ = winintegration.LaunchProcessInSession(plan.RelaunchUISessionID, uiExe, "")
	}
}

func applyTransaction(plan updater.UpdatePlan) error {
	if err := stopService(20 * time.Second); err != nil {
		slog.Warn("stopService warning", "error", err)
	}
	// Give trays/UI a brief moment to release executable image handles
	time.Sleep(1200 * time.Millisecond)

	_ = os.RemoveAll(plan.RollbackDir)
	if err := os.MkdirAll(plan.RollbackDir, 0750); err != nil {
		return err
	}

	// Backup current binaries to rollback/ and install staged binaries
	for _, name := range plan.Files {
		srcNew := filepath.Join(plan.StagingDir, name)
		destProd := filepath.Join(plan.InstallDir, name)
		bakPath := filepath.Join(plan.RollbackDir, name+".bak")

		if _, err := os.Stat(destProd); err == nil {
			// Rename works on Windows even if a lingering handle exists, and preserves clean rollback copy
			if err := os.Rename(destProd, bakPath); err != nil {
				if copyErr := copyFile(destProd, bakPath); copyErr != nil {
					return fmt.Errorf("backup %s failed: %w", name, err)
				}
			}
		}
		if err := copyFile(srcNew, destProd); err != nil {
			return fmt.Errorf("install new binary %s failed: %w", name, err)
		}
	}

	if err := startService(); err != nil {
		return fmt.Errorf("start new service failed: %w", err)
	}

	// Healthcheck new version over Named Pipe
	cli := ipc.NewHTTPClient(2 * time.Second)
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := cli.Get("http://pipe/api/v1/health")
		if err == nil {
			var h struct {
				Status  string `json:"status"`
				Version string `json:"version"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&h)
			_ = resp.Body.Close()
			if h.Status == "ok" {
				return nil
			}
		}
		time.Sleep(600 * time.Millisecond)
	}
	return fmt.Errorf("new service version %s failed healthcheck within 12s", plan.TargetVersion)
}

func rollbackTransaction(plan updater.UpdatePlan) {
	_ = stopService(10 * time.Second)
	time.Sleep(800 * time.Millisecond)

	for _, name := range plan.Files {
		bakPath := filepath.Join(plan.RollbackDir, name+".bak")
		destProd := filepath.Join(plan.InstallDir, name)
		if _, err := os.Stat(bakPath); err == nil {
			_ = os.Remove(destProd)
			_ = copyFile(bakPath, destProd)
		}
	}

	if plan.DatabaseSnapshot != "" && plan.DatabasePath != "" {
		if _, err := os.Stat(plan.DatabaseSnapshot); err == nil {
			_ = os.Remove(plan.DatabasePath + "-wal")
			_ = os.Remove(plan.DatabasePath + "-shm")
			_ = copyFile(plan.DatabaseSnapshot, plan.DatabasePath)
		}
	}

	_ = startService()
	slog.Info("automatic rollback restored previous version", "restoredVersion", plan.PreviousVersion)
}

func stopService(timeout time.Duration) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()

	s, err := m.OpenService(config.ServiceName)
	if err != nil {
		return nil
	}
	defer func() { _ = s.Close() }()

	status, err := s.Query()
	if err == nil && status.State == svc.Stopped {
		return nil
	}

	_, _ = s.Control(svc.Stop)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, qErr := s.Query()
		if qErr == nil && st.State == svc.Stopped {
			return nil
		}
		time.Sleep(400 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s to stop", config.ServiceName)
}

func startService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()

	s, err := m.OpenService(config.ServiceName)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	return s.Start()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	buf := make([]byte, 128*1024)
	_, err = io.CopyBuffer(out, in, buf)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
