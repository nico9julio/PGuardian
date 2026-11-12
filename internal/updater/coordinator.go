package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nico9julio/PGuardian/internal/backup"
	"github.com/nico9julio/PGuardian/internal/config"
	"github.com/nico9julio/PGuardian/internal/database"
	"github.com/nico9julio/PGuardian/internal/notify"
	"golang.org/x/sys/windows"
)

type Coordinator struct {
	store      *database.Store
	engine     *backup.Engine
	dispatcher *notify.Dispatcher
	gh         *GitHubClient
}

func NewCoordinator(store *database.Store, engine *backup.Engine, dispatcher *notify.Dispatcher) *Coordinator {
	return &Coordinator{
		store:      store,
		engine:     engine,
		dispatcher: dispatcher,
		gh:         NewGitHubClient(),
	}
}

func (c *Coordinator) CheckLatest(ctx context.Context) (ReleaseCheckResult, error) {
	repo := c.store.GetSetting(database.KeyGitHubRepo, config.DefaultGitHubRepo)
	tok, _ := c.store.GetEncryptedSetting(database.KeyGitHubTokenEnc)
	return c.gh.CheckLatestRelease(ctx, repo, tok)
}

func (c *Coordinator) ApplyFromGitHub(ctx context.Context, callerSessionID uint32, force bool) (string, error) {
	repo := c.store.GetSetting(database.KeyGitHubRepo, config.DefaultGitHubRepo)
	tok, _ := c.store.GetEncryptedSetting(database.KeyGitHubTokenEnc)

	check, err := c.gh.CheckLatestRelease(ctx, repo, tok)
	if err != nil {
		return "", err
	}
	if check.AssetURL == "" {
		return "", errors.New("no PGuardianUpdate.zip asset found in the latest GitHub release")
	}

	zipDest := filepath.Join(config.UpdatesDir(), fmt.Sprintf("github_release_%s.zip", check.LatestVersion))
	if err := c.gh.DownloadBundle(ctx, check.AssetURL, tok, zipDest); err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(zipDest) }()

	return c.ApplyFromLocalZip(ctx, zipDest, callerSessionID, force)
}

func (c *Coordinator) ApplyFromLocalZip(ctx context.Context, zipPath string, callerSessionID uint32, force bool) (string, error) {
	if !force && c.engine.ActiveJobs() > 0 {
		// Wait briefly for quick dumps to finish
		deadline := time.Now().Add(20 * time.Second)
		for c.engine.ActiveJobs() > 0 && time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
		if c.engine.ActiveJobs() > 0 {
			return "", errors.New("a backup or cloud upload is currently in progress; wait for it to finish or use Force Update")
		}
	}

	manifest, stagedDir, err := ExtractAndVerifyBundle(zipPath, config.StagingDir())
	if err != nil {
		return "", err
	}

	// Pre-update atomic SQLite snapshot
	snapPath := filepath.Join(config.DBSnapshotsDir(), fmt.Sprintf("pguardian_pre_%s_%d.db", manifest.Version, time.Now().Unix()))
	if err := c.store.SnapshotTo(snapPath); err != nil {
		return "", fmt.Errorf("snapshot sqlite before update: %w", err)
	}

	var fileList []string
	for name := range manifest.Files {
		fileList = append(fileList, filepath.Base(name))
	}

	plan := UpdatePlan{
		TargetVersion:       manifest.Version,
		PreviousVersion:     config.Version,
		InstallDir:          config.InstallDir(),
		StagingDir:          stagedDir,
		RollbackDir:         config.RollbackDir(),
		DatabasePath:        c.store.Path(),
		DatabaseSnapshot:    snapPath,
		RelaunchUISessionID: callerSessionID,
		Files:               fileList,
	}

	planPath := filepath.Join(stagedDir, "update_plan.json")
	planBytes, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(planPath, planBytes, 0600); err != nil {
		return "", err
	}

	updaterExe := filepath.Join(stagedDir, "PGuardianUpdater.exe")
	if _, err := os.Stat(updaterExe); err != nil {
		// Fallback to installed updater if bundle omitted it
		updaterExe = filepath.Join(config.InstallDir(), "PGuardianUpdater.exe")
	}

	// Broadcast graceful drain to all RDP System Tray agents
	c.dispatcher.EmitLive(notify.Event{
		Type:     notify.EventSystemUpdating,
		Title:    "PGuardian Updating",
		Message:  "Applying version " + manifest.Version,
		Severity: "info",
	})

	// Launch detached transactional updater after 400ms so HTTP response returns cleanly to UI first
	config.SafeGo("launch-updater", func() {
		time.Sleep(400 * time.Millisecond)
		cmd := exec.Command(updaterExe, "--plan", planPath)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS | windows.CREATE_NO_WINDOW,
		}
		if err := cmd.Start(); err != nil {
			slog.Error("failed to spawn PGuardianUpdater.exe", "error", err)
			return
		}
		_ = cmd.Process.Release()
	})

	return manifest.Version, nil
}
