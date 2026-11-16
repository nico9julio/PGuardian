package database

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestStoreOpenMigrateAndSnapshot(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "pguardian_test.db")

	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	if st.IsSetupCompleted() {
		t.Fatal("expected setupCompleted to be false initially")
	}

	if err := st.SetEncryptedSetting(KeyTelegramBotTokenEnc, "mock-bot-token-for-test"); err != nil {
		t.Fatalf("SetEncryptedSetting failed: %v", err)
	}
	got, err := st.GetEncryptedSetting(KeyTelegramBotTokenEnc)
	if err != nil || got != "mock-bot-token-for-test" {
		t.Fatalf("GetEncryptedSetting mismatch: got %q, err %v", got, err)
	}

	srvID, err := st.SavePGServer(PGServer{
		Name:      "Local PG",
		Engine:    EnginePostgres,
		Host:      "127.0.0.1",
		Port:      5432,
		Username:  "postgres",
		Password:  "mock-pg-pass",
		IsDefault: true,
	})
	if err != nil || srvID <= 0 {
		t.Fatalf("SavePGServer failed: %v", err)
	}

	mariaID, err := st.SavePGServer(PGServer{
		Name:     "Local MariaDB",
		Engine:   EngineMariaDB,
		Host:     "127.0.0.1",
		Username: "root",
		Password: "mock-maria-pass",
	})
	if err != nil || mariaID <= 0 {
		t.Fatalf("SavePGServer (MariaDB) failed: %v", err)
	}
	mariaSrv, mariaPwd, err := st.GetPGServerWithSecret(mariaID)
	if err != nil || mariaSrv.Engine != EngineMariaDB || mariaSrv.Port != 3306 || mariaPwd != "mock-maria-pass" {
		t.Fatalf("GetPGServerWithSecret MariaDB mismatch: srv=%+v err=%v", mariaSrv, err)
	}

	profID, err := st.SaveProfile(BackupProfile{
		Name:                "Nightly",
		TargetType:          TargetDatabase,
		PGServerID:          srvID,
		Enabled:             true,
		ScheduleCron:        "0 2 * * *",
		Format:              "custom",
		SelectionMode:       "manual",
		IncludedDatabases:   []string{"erp_prod", "crm"},
		LocalRetentionMode:  "count",
		LocalRetentionValue: 7,
	})
	if err != nil || profID <= 0 {
		t.Fatalf("SaveProfile failed: %v", err)
	}

	dirID, err := st.SaveDirectory(BackupDirectory{
		Name:            "WebUploads",
		Path:            `C:\inetpub\uploads`,
		ExcludePatterns: []string{"*.tmp", "node_modules", "*.tmp"},
	})
	if err != nil || dirID <= 0 {
		t.Fatalf("SaveDirectory failed: %v", err)
	}
	dirGot, err := st.GetDirectory(dirID)
	if err != nil || len(dirGot.ExcludePatterns) != 2 {
		t.Fatalf("GetDirectory mismatch: %+v err=%v", dirGot, err)
	}

	dirProfID, err := st.SaveProfile(BackupProfile{
		Name:                "Daily Folders",
		TargetType:          TargetDirectory,
		Enabled:             true,
		ScheduleCron:        "0 3 * * *",
		Format:              "zip",
		SelectionMode:       "manual",
		IncludedDirectories: []int64{dirID},
		LocalRetentionMode:  "count",
		LocalRetentionValue: 5,
	})
	if err != nil || dirProfID <= 0 {
		t.Fatalf("SaveProfile (Directory) failed: %v", err)
	}

	loadedDirProf, err := st.GetProfile(dirProfID)
	if err != nil || loadedDirProf.TargetType != TargetDirectory || loadedDirProf.PGServerID != 0 || len(loadedDirProf.IncludedDirectories) != 1 || loadedDirProf.IncludedDirectories[0] != dirID {
		t.Fatalf("GetProfile directory mismatch: %+v err=%v", loadedDirProf, err)
	}

	exID, err := st.InsertExecution(BackupExecution{
		ProfileID:   &dirProfID,
		ProfileName: "Daily Folders",
		TargetType:  TargetDirectory,
		DirectoryID: dirID,
		DBName:      "WebUploads",
		TriggerType: "manual",
		Format:      "zip",
		StartedAt:   nowISO(),
		LocalStatus: "running",
		CloudStatus: "not_applicable",
	})
	if err != nil || exID <= 0 {
		t.Fatalf("InsertExecution directory failed: %v", err)
	}
	if err := st.CompleteLocalExecutionWithWarning(exID, "completed", `C:\backups\dirs\WebUploads\WebUploads_1.zip`, 1024, "abc123", 50, "not_applicable", "", "1 file skipped"); err != nil {
		t.Fatalf("CompleteLocalExecutionWithWarning failed: %v", err)
	}

	dirExecs, err := st.ListExecutions(0, "", dirID, 10)
	if err != nil || len(dirExecs) != 1 || dirExecs[0].WarningMessage != "1 file skipped" {
		t.Fatalf("ListExecutions directory mismatch: %+v err=%v", dirExecs, err)
	}

	retCopies, err := st.ListActiveLocalCopiesForRetention(dirProfID, TargetDirectory, dirID, "WebUploads")
	if err != nil || len(retCopies) != 1 {
		t.Fatalf("ListActiveLocalCopiesForRetention directory mismatch: %+v err=%v", retCopies, err)
	}

	metrics, err := st.GetDashboardMetrics()
	if err != nil || metrics.ConfiguredServers != 2 || metrics.ConfiguredDirectories != 1 {
		t.Fatalf("GetDashboardMetrics mismatch: %+v err=%v", metrics, err)
	}

	snapPath := filepath.Join(tmp, "snapshot.db")
	if err := st.SnapshotTo(snapPath); err != nil {
		t.Fatalf("SnapshotTo failed: %v", err)
	}
}

func TestMigration002OnPopulatedV1Database(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "legacy_v1.db")

	rawSQL, err := migrationFiles.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatalf("read 001_init.sql: %v", err)
	}
	dsn := "file:" + filepath.ToSlash(dbPath) + "?_pragma=foreign_keys(1)"
	rawDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	if _, err := rawDB.Exec(string(rawSQL)); err != nil {
		_ = rawDB.Close()
		t.Fatalf("exec 001_init.sql: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (1, '2026-01-01T00:00:00Z')`); err != nil {
		_ = rawDB.Close()
		t.Fatalf("insert schema_migrations: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO pg_servers (id, name, host, port, username, password_enc, pg_dump_path, is_default, created_at) VALUES (1, 'Prod PG', '127.0.0.1', 5432, 'postgres', '', '', 1, '2026-01-01T00:00:00Z')`); err != nil {
		_ = rawDB.Close()
		t.Fatalf("insert pg_server: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO backup_profiles (id, name, pg_server_id, enabled, schedule_cron, schedule_label, format, selection_mode, local_dir_override, local_retention_mode, local_retention_value, drive_enabled, drive_folder_id, drive_folder_path, drive_retention_mode, drive_retention_value, created_at) VALUES (1, 'Daily PG', 1, 1, '0 2 * * *', 'Daily', 'custom', 'manual', '', 'count', 7, 0, '', '', 'count', 14, '2026-01-01T00:00:00Z')`); err != nil {
		_ = rawDB.Close()
		t.Fatalf("insert backup_profile: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO profile_databases (profile_id, db_name, inclusion_type) VALUES (1, 'erp', 'included')`); err != nil {
		_ = rawDB.Close()
		t.Fatalf("insert profile_databases: %v", err)
	}
	_ = rawDB.Close()

	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open (migrating v1->v2) failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	profs, err := st.ListProfiles()
	if err != nil || len(profs) != 1 || profs[0].TargetType != TargetDatabase || profs[0].PGServerID != 1 || len(profs[0].IncludedDatabases) != 1 || profs[0].IncludedDatabases[0] != "erp" {
		t.Fatalf("migrated profile mismatch: %+v err=%v", profs, err)
	}
}
