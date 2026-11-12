package config

import (
	"os"
	"path/filepath"
)

// DataRoot returns the persistent machine-wide directory for PGuardian state.
// Supports PGUARDIAN_DATA_DIR override for isolated unit tests and local development.
func DataRoot() string {
	if custom := os.Getenv("PGUARDIAN_DATA_DIR"); custom != "" {
		return custom
	}
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, AppName)
}

func DatabasePath() string {
	return filepath.Join(DataRoot(), "data", "pguardian.db")
}

func LogsDir() string {
	return filepath.Join(DataRoot(), "logs")
}

func DefaultLocalBackupsDir() string {
	return filepath.Join(DataRoot(), "backups")
}

func UpdatesDir() string {
	return filepath.Join(DataRoot(), "updates")
}

func StagingDir() string {
	return filepath.Join(UpdatesDir(), "staging")
}

func RollbackDir() string {
	return filepath.Join(UpdatesDir(), "rollback")
}

func DBSnapshotsDir() string {
	return filepath.Join(UpdatesDir(), "db_snapshots")
}

// EnsureDirectories creates all required application data subdirectories.
func EnsureDirectories() error {
	dirs := []string{
		filepath.Join(DataRoot(), "data"),
		LogsDir(),
		DefaultLocalBackupsDir(),
		StagingDir(),
		RollbackDir(),
		DBSnapshotsDir(),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0750); err != nil {
			return err
		}
	}
	return nil
}

// InstallDir returns the directory containing the running executable.
func InstallDir() string {
	exe, err := os.Executable()
	if err != nil {
		return `C:\Program Files\PGuardian`
	}
	return filepath.Dir(exe)
}
