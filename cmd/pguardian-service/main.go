package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nico9julio/PGuardian/internal/api"
	"github.com/nico9julio/PGuardian/internal/auth"
	"github.com/nico9julio/PGuardian/internal/backup"
	"github.com/nico9julio/PGuardian/internal/cloud"
	"github.com/nico9julio/PGuardian/internal/config"
	"github.com/nico9julio/PGuardian/internal/database"
	"github.com/nico9julio/PGuardian/internal/notify"
	"github.com/nico9julio/PGuardian/internal/scheduler"
	"github.com/nico9julio/PGuardian/internal/updater"
	"github.com/nico9julio/PGuardian/internal/winintegration"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func main() {
	if err := config.EnsureDirectories(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create data directories: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) > 1 {
		cmd := strings.ToLower(os.Args[1])
		switch cmd {
		case "install":
			if err := installWindowsService(); err != nil {
				fmt.Fprintf(os.Stderr, "install service failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("PGuardianService installed successfully.")
			return
		case "uninstall":
			if err := uninstallWindowsService(); err != nil {
				fmt.Fprintf(os.Stderr, "uninstall service failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("PGuardianService uninstalled successfully.")
			return
		case "console", "--console":
			runInteractiveConsole()
			return
		}
	}

	isSvc, err := svc.IsWindowsService()
	if err == nil && isSvc {
		logCloser := config.InitLogger("service.log", false)
		defer func() { _ = logCloser.Close() }()
		if err := svc.Run(config.ServiceName, &pguardianWinService{}); err != nil {
			slog.Error("windows service execution error", "error", err)
			os.Exit(1)
		}
		return
	}

	runInteractiveConsole()
}

type runtimeStack struct {
	store     *database.Store
	sched     *scheduler.Coordinator
	apiServer *api.Server
	ctxCancel context.CancelFunc
}

func startRuntimeStack() (*runtimeStack, error) {
	store, err := database.Open(config.DatabasePath())
	if err != nil {
		return nil, err
	}

	broadcaster := notify.NewBroadcaster()
	dispatcher := notify.NewDispatcher(store, broadcaster)
	cloudSvc := cloud.NewService(store)
	engine := backup.NewEngine(store, cloudSvc, dispatcher)
	sched := scheduler.NewCoordinator(store, engine, dispatcher)
	upd := updater.NewCoordinator(store, engine, dispatcher)
	authMgr := auth.NewManager()
	apiSrv := api.NewServer(store, authMgr, engine, cloudSvc, dispatcher, sched, upd)

	if err := apiSrv.StartNamedPipe(); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("listen named pipe: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := sched.Start(ctx); err != nil {
		slog.Warn("scheduler startup warning", "error", err)
	}

	// Ensure active RDP/Console sessions of enabled users have PGuardianTray.exe running (e.g. after service restart or update)
	config.SafeGo("ensure-active-trays", func() {
		time.Sleep(1500 * time.Millisecond)
		ensureTrayInActiveEnabledSessions(store)
	})

	slog.Info("PGuardianService started", "version", config.Version, "pipe", config.PipeName)
	return &runtimeStack{
		store:     store,
		sched:     sched,
		apiServer: apiSrv,
		ctxCancel: cancel,
	}, nil
}

func (rt *runtimeStack) Shutdown() {
	slog.Info("shutting down PGuardianService")
	rt.ctxCancel()
	rt.sched.Stop()
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = rt.apiServer.Stop(shutCtx)
	_ = rt.store.Close()
}

func ensureTrayInActiveEnabledSessions(store *database.Store) {
	enabledUsers, err := store.ListWindowsNotifyUsers()
	if err != nil || len(enabledUsers) == 0 {
		return
	}
	trayExe := filepath.Join(config.InstallDir(), "PGuardianTray.exe")
	if _, err := os.Stat(trayExe); err != nil {
		return
	}
	sessions, err := winintegration.ListActiveSessions()
	if err != nil {
		return
	}
	for _, s := range sessions {
		if s.ConnectionStat != "Active" {
			continue
		}
		for _, u := range enabledUsers {
			if u.Enabled && (strings.EqualFold(u.Username, s.Username) || strings.EqualFold(u.Username, s.ShortName)) {
				_ = winintegration.LaunchProcessInSession(s.SessionID, trayExe, "")
				break
			}
		}
	}
}

type pguardianWinService struct{}

func (m *pguardianWinService) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	rt, err := startRuntimeStack()
	if err != nil {
		slog.Error("failed to initialize runtime stack", "error", err)
		return false, 1
	}

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

loop:
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			rt.Shutdown()
			break loop
		}
	}
	return false, 0
}

func runInteractiveConsole() {
	logCloser := config.InitLogger("service.log", true)
	defer func() { _ = logCloser.Close() }()

	rt, err := startRuntimeStack()
	if err != nil {
		slog.Error("failed to start PGuardian console service", "error", err)
		os.Exit(1)
	}
	fmt.Printf("PGuardianService v%s running in console mode on %s. Press Ctrl+C to stop.\n", config.Version, config.PipeName)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	rt.Shutdown()
}

func installWindowsService() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()

	s, err := m.OpenService(config.ServiceName)
	if err == nil {
		_ = s.Close()
		return nil // Already installed
	}

	s, err = m.CreateService(config.ServiceName, exePath, mgr.Config{
		StartType:    mgr.StartAutomatic,
		DisplayName:  config.ServiceDisplay,
		Description:  config.ServiceDesc,
		DelayedAutoStart: false,
	})
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	// Configure automatic crash recovery in Windows SCM (restart after 5s, 15s, 30s)
	recoveryActions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 15 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}
	_ = s.SetRecoveryActions(recoveryActions, 86400)
	return nil
}

func uninstallWindowsService() error {
	// Clean up per-user tray scheduled tasks before removing service
	if store, err := database.Open(config.DatabasePath()); err == nil {
		users, _ := store.ListWindowsNotifyUsers()
		for _, u := range users {
			_ = winintegration.RemoveUserTrayAutostart(u.Username)
		}
		_ = store.Close()
	}

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

	_, _ = s.Control(svc.Stop)
	time.Sleep(1500 * time.Millisecond)
	return s.Delete()
}
