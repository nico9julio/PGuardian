package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nico9julio/PGuardian/internal/cloud"
	"github.com/nico9julio/PGuardian/internal/database"
	"github.com/nico9julio/PGuardian/internal/mysql"
	"github.com/nico9julio/PGuardian/internal/notify"
	"golang.org/x/sys/windows"
)

func TestMySQLDiscoveryAndConfigHelpers(t *testing.T) {
	bins := mysql.DiscoverMySQLDumpBinaries()
	for _, b := range bins {
		if b.Path == "" {
			t.Fatal("discovered binary has empty path")
		}
		t.Logf("Discovered MySQL/MariaDB dump: %s (%s)", b.Path, b.Version)
	}

	raw := mysql.FormatDefaultsExtraFile("backup_user", `p@ss"w\o#r;d' $1`)
	if !strings.HasPrefix(raw, "[client]\n") {
		t.Fatalf("missing [client] header: %q", raw)
	}
	if !strings.Contains(raw, `user="backup_user"`) {
		t.Fatalf("unexpected user line: %q", raw)
	}
	wantPwd := `password="p@ss\"w\\o#r;d' $1"`
	if !strings.Contains(raw, wantPwd) {
		t.Fatalf("expected %q in %q", wantPwd, raw)
	}

	cases := []struct {
		ver  string
		want bool
	}{
		{"mysqldump  Ver 8.0.36 for Win64 on x86_64 (MySQL Community Server - GPL)", true},
		{"mysqldump  Ver 8.4.2 for Win64 on x86_64", true},
		{"mysqldump  Ver 9.0.1 for Win64 on x86_64", true},
		{"mariadb-dump from 11.4.2-MariaDB, client 10.19 for Win64 (AMD64)", false},
		{"mysqldump  Ver 10.19 Distrib 10.6.16-MariaDB, for Win64 (AMD64)", false},
		{"mysqldump  Ver 10.13 Distrib 5.7.44, for Win64 (x86_64)", false},
		{"unknown", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := mysql.NeedsColumnStatisticsZero(tc.ver); got != tc.want {
			t.Errorf("NeedsColumnStatisticsZero(%q) = %v, want %v", tc.ver, got, tc.want)
		}
	}
}

func TestEngineDirectoryBackupAndLockedFileWarning(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("PGUARDIAN_DATA_DIR", dataDir)

	backupRoot := filepath.Join(t.TempDir(), "backups")
	srcDir := filepath.Join(t.TempDir(), "ProjectFiles")
	if err := os.MkdirAll(srcDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "readme.txt"), []byte("hello"), 0640); err != nil {
		t.Fatal(err)
	}
	lockedPath := filepath.Join(srcDir, "busy.db")
	if err := os.WriteFile(lockedPath, []byte("locked"), 0640); err != nil {
		t.Fatal(err)
	}
	p16, err := windows.UTF16PtrFromString(lockedPath)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p16, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("lock CreateFile failed: %v", err)
	}
	defer windows.CloseHandle(h)

	store, err := database.Open(filepath.Join(dataDir, "data", "pguardian.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	_ = store.SetSetting(database.KeyLocalBackupRoot, backupRoot)

	dirID, err := store.SaveDirectory(database.BackupDirectory{
		Name:            "ProjectFiles",
		Path:            srcDir,
		ExcludePatterns: []string{"*.tmp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	profID, err := store.SaveProfile(database.BackupProfile{
		Name:                "Folders",
		TargetType:          database.TargetDirectory,
		Enabled:             true,
		ScheduleCron:        "0 3 * * *",
		Format:              "zip",
		SelectionMode:       "manual",
		IncludedDirectories: []int64{dirID},
		LocalRetentionMode:  "count",
		LocalRetentionValue: 2,
		DriveEnabled:        false,
	})
	if err != nil {
		t.Fatal(err)
	}

	broadcaster := notify.NewBroadcaster()
	dispatcher := notify.NewDispatcher(store, broadcaster)
	cloudSvc := cloud.NewService(store)
	engine := NewEngine(store, cloudSvc, dispatcher)

	execID, err := engine.RunManualDirectory(context.Background(), dirID, profID)
	if err != nil {
		t.Fatalf("RunManualDirectory failed: %v", err)
	}
	ex, err := store.GetExecution(execID)
	if err != nil {
		t.Fatal(err)
	}
	if ex.LocalStatus != "completed" || ex.Format != "zip" || ex.TargetType != database.TargetDirectory {
		t.Fatalf("unexpected execution state: %+v", ex)
	}
	if !strings.Contains(ex.WarningMessage, "busy.db") {
		t.Fatalf("expected warning about locked busy.db, got %q", ex.WarningMessage)
	}
	if _, statErr := os.Stat(ex.LocalFilePath); statErr != nil {
		t.Fatalf("expected local zip file to exist at %s: %v", ex.LocalFilePath, statErr)
	}
}
