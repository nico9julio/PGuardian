package database

import (
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
		Host:      "127.0.0.1",
		Port:      5432,
		Username:  "postgres",
		Password:  "mock-pg-pass",
		IsDefault: true,
	})
	if err != nil || srvID <= 0 {
		t.Fatalf("SavePGServer failed: %v", err)
	}

	profID, err := st.SaveProfile(BackupProfile{
		Name:                "Nightly",
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

	snapPath := filepath.Join(tmp, "snapshot.db")
	if err := st.SnapshotTo(snapPath); err != nil {
		t.Fatalf("SnapshotTo failed: %v", err)
	}
}
