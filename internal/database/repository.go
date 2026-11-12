package database

import (
	"errors"
	"time"

	"github.com/nico9julio/PGuardian/internal/crypto"
)

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// --- Settings ---

func (s *Store) GetSetting(key, defaultVal string) string {
	var val string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, key).Scan(&val)
	if err != nil {
		return defaultVal
	}
	return val
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO app_settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, nowISO(),
	)
	return err
}

func (s *Store) SetEncryptedSetting(key, plaintext string) error {
	if plaintext == "" {
		return s.SetSetting(key, "")
	}
	enc, err := crypto.ProtectString(plaintext)
	if err != nil {
		return err
	}
	return s.SetSetting(key, enc)
}

func (s *Store) GetEncryptedSetting(key string) (string, error) {
	enc := s.GetSetting(key, "")
	if enc == "" {
		return "", nil
	}
	return crypto.UnprotectString(enc)
}

func (s *Store) IsSetupCompleted() bool {
	return s.GetSetting(KeySetupCompleted, "false") == "true"
}

// --- PostgreSQL Servers ---

func (s *Store) ListPGServers() ([]PGServer, error) {
	rows, err := s.db.Query(`
		SELECT id, name, host, port, username, password_enc, pg_dump_path, is_default, created_at
		FROM pg_servers ORDER BY is_default DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PGServer
	for rows.Next() {
		var srv PGServer
		var pwdEnc string
		var isDef int
		if err := rows.Scan(&srv.ID, &srv.Name, &srv.Host, &srv.Port, &srv.Username, &pwdEnc, &srv.PGDumpPath, &isDef, &srv.CreatedAt); err != nil {
			return nil, err
		}
		srv.HasPassword = pwdEnc != ""
		srv.IsDefault = isDef == 1
		out = append(out, srv)
	}
	return out, rows.Err()
}

func (s *Store) GetPGServerWithSecret(id int64) (*PGServer, string, error) {
	var srv PGServer
	var pwdEnc string
	var isDef int
	err := s.db.QueryRow(`
		SELECT id, name, host, port, username, password_enc, pg_dump_path, is_default, created_at
		FROM pg_servers WHERE id = ?`, id).Scan(
		&srv.ID, &srv.Name, &srv.Host, &srv.Port, &srv.Username, &pwdEnc, &srv.PGDumpPath, &isDef, &srv.CreatedAt,
	)
	if err != nil {
		return nil, "", err
	}
	srv.HasPassword = pwdEnc != ""
	srv.IsDefault = isDef == 1
	pwd, err := crypto.UnprotectString(pwdEnc)
	if err != nil {
		return nil, "", err
	}
	return &srv, pwd, nil
}

func (s *Store) SavePGServer(srv PGServer) (int64, error) {
	var pwdEnc string
	var err error
	if srv.Password != "" {
		pwdEnc, err = crypto.ProtectString(srv.Password)
		if err != nil {
			return 0, err
		}
	} else if srv.ID > 0 {
		_ = s.db.QueryRow(`SELECT password_enc FROM pg_servers WHERE id = ?`, srv.ID).Scan(&pwdEnc)
	}

	isDef := 0
	if srv.IsDefault {
		isDef = 1
		_, _ = s.db.Exec(`UPDATE pg_servers SET is_default = 0`)
	}

	if srv.ID == 0 {
		res, err := s.db.Exec(`
			INSERT INTO pg_servers (name, host, port, username, password_enc, pg_dump_path, is_default, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			srv.Name, srv.Host, srv.Port, srv.Username, pwdEnc, srv.PGDumpPath, isDef, nowISO(),
		)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}

	_, err = s.db.Exec(`
		UPDATE pg_servers
		SET name = ?, host = ?, port = ?, username = ?, password_enc = ?, pg_dump_path = ?, is_default = ?
		WHERE id = ?`,
		srv.Name, srv.Host, srv.Port, srv.Username, pwdEnc, srv.PGDumpPath, isDef, srv.ID,
	)
	return srv.ID, err
}

func (s *Store) DeletePGServer(id int64) error {
	_, err := s.db.Exec(`DELETE FROM pg_servers WHERE id = ?`, id)
	return err
}

// --- Backup Profiles ---

func (s *Store) ListProfiles() ([]BackupProfile, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.name, p.pg_server_id, COALESCE(sv.name, ''), p.enabled, p.schedule_cron, p.schedule_label,
		       p.format, p.selection_mode, p.local_dir_override, p.local_retention_mode, p.local_retention_value,
		       p.drive_enabled, p.drive_folder_id, p.drive_folder_path, p.drive_retention_mode, p.drive_retention_value, p.created_at
		FROM backup_profiles p
		LEFT JOIN pg_servers sv ON sv.id = p.pg_server_id
		ORDER BY p.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []BackupProfile
	for rows.Next() {
		var p BackupProfile
		var en, drEn int
		if err := rows.Scan(
			&p.ID, &p.Name, &p.PGServerID, &p.ServerName, &en, &p.ScheduleCron, &p.ScheduleLabel,
			&p.Format, &p.SelectionMode, &p.LocalDirOverride, &p.LocalRetentionMode, &p.LocalRetentionValue,
			&drEn, &p.DriveFolderID, &p.DriveFolderPath, &p.DriveRetentionMode, &p.DriveRetentionValue, &p.CreatedAt,
		); err != nil {
			return nil, err
		}
		p.Enabled = en == 1
		p.DriveEnabled = drEn == 1
		p.IncludedDatabases = []string{}
		p.ExcludedDatabases = []string{}
		profiles = append(profiles, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range profiles {
		inc, exc, err := s.getProfileDBSelections(profiles[i].ID)
		if err != nil {
			return nil, err
		}
		profiles[i].IncludedDatabases = inc
		profiles[i].ExcludedDatabases = exc
	}
	return profiles, nil
}

func (s *Store) GetProfile(id int64) (*BackupProfile, error) {
	all, err := s.ListProfiles()
	if err != nil {
		return nil, err
	}
	for _, p := range all {
		if p.ID == id {
			cp := p
			return &cp, nil
		}
	}
	return nil, errors.New("profile not found")
}

func (s *Store) getProfileDBSelections(profileID int64) ([]string, []string, error) {
	rows, err := s.db.Query(`SELECT db_name, inclusion_type FROM profile_databases WHERE profile_id = ? ORDER BY db_name ASC`, profileID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	inc := []string{}
	exc := []string{}
	for rows.Next() {
		var dbName, kind string
		if err := rows.Scan(&dbName, &kind); err != nil {
			return nil, nil, err
		}
		if kind == "excluded" {
			exc = append(exc, dbName)
		} else {
			inc = append(inc, dbName)
		}
	}
	return inc, exc, rows.Err()
}

func (s *Store) SaveProfile(p BackupProfile) (int64, error) {
	en := 0
	if p.Enabled {
		en = 1
	}
	drEn := 0
	if p.DriveEnabled {
		drEn = 1
	}
	if p.Format == "" {
		p.Format = "custom"
	}
	if p.SelectionMode == "" {
		p.SelectionMode = "manual"
	}
	if p.LocalRetentionMode == "" {
		p.LocalRetentionMode = "count"
	}
	if p.DriveRetentionMode == "" {
		p.DriveRetentionMode = "count"
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	profileID := p.ID
	if profileID == 0 {
		res, err := tx.Exec(`
			INSERT INTO backup_profiles (
				name, pg_server_id, enabled, schedule_cron, schedule_label, format, selection_mode,
				local_dir_override, local_retention_mode, local_retention_value,
				drive_enabled, drive_folder_id, drive_folder_path, drive_retention_mode, drive_retention_value, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.Name, p.PGServerID, en, p.ScheduleCron, p.ScheduleLabel, p.Format, p.SelectionMode,
			p.LocalDirOverride, p.LocalRetentionMode, p.LocalRetentionValue,
			drEn, p.DriveFolderID, p.DriveFolderPath, p.DriveRetentionMode, p.DriveRetentionValue, nowISO(),
		)
		if err != nil {
			return 0, err
		}
		profileID, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else {
		_, err := tx.Exec(`
			UPDATE backup_profiles SET
				name = ?, pg_server_id = ?, enabled = ?, schedule_cron = ?, schedule_label = ?, format = ?, selection_mode = ?,
				local_dir_override = ?, local_retention_mode = ?, local_retention_value = ?,
				drive_enabled = ?, drive_folder_id = ?, drive_folder_path = ?, drive_retention_mode = ?, drive_retention_value = ?
			WHERE id = ?`,
			p.Name, p.PGServerID, en, p.ScheduleCron, p.ScheduleLabel, p.Format, p.SelectionMode,
			p.LocalDirOverride, p.LocalRetentionMode, p.LocalRetentionValue,
			drEn, p.DriveFolderID, p.DriveFolderPath, p.DriveRetentionMode, p.DriveRetentionValue, profileID,
		)
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`DELETE FROM profile_databases WHERE profile_id = ?`, profileID); err != nil {
			return 0, err
		}
	}

	for _, dbName := range p.IncludedDatabases {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO profile_databases (profile_id, db_name, inclusion_type) VALUES (?, ?, 'included')`, profileID, dbName); err != nil {
			return 0, err
		}
	}
	for _, dbName := range p.ExcludedDatabases {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO profile_databases (profile_id, db_name, inclusion_type) VALUES (?, ?, 'excluded')`, profileID, dbName); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return profileID, nil
}

func (s *Store) DeleteProfile(id int64) error {
	_, err := s.db.Exec(`DELETE FROM backup_profiles WHERE id = ?`, id)
	return err
}
