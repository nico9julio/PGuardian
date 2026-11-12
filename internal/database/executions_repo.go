package database

import (
	"database/sql"
	"time"
)

func (s *Store) InsertExecution(ex BackupExecution) (int64, error) {
	var profID sql.NullInt64
	if ex.ProfileID != nil && *ex.ProfileID > 0 {
		profID = sql.NullInt64{Int64: *ex.ProfileID, Valid: true}
	}
	res, err := s.db.Exec(`
		INSERT INTO backup_executions (
			profile_id, profile_name, pg_server_id, server_name, db_name, trigger_type, format,
			started_at, local_status, cloud_status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		profID, ex.ProfileName, ex.PGServerID, ex.ServerName, ex.DBName, ex.TriggerType, ex.Format,
		ex.StartedAt, ex.LocalStatus, ex.CloudStatus,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) CompleteLocalExecution(id int64, status, filePath string, sizeBytes int64, sha256Sum string, durationMs int64, cloudStatus, errMsg string) error {
	_, err := s.db.Exec(`
		UPDATE backup_executions SET
			finished_at = ?, duration_ms = ?, local_status = ?, local_file_path = ?,
			file_size_bytes = ?, sha256_checksum = ?, cloud_status = ?, error_message = ?
		WHERE id = ?`,
		nowISO(), durationMs, status, filePath, sizeBytes, sha256Sum, cloudStatus, errMsg, id,
	)
	return err
}

func (s *Store) UpdateCloudStatus(id int64, cloudStatus, driveFileID string, retryCount int, nextRetryAt, errMsg string) error {
	_, err := s.db.Exec(`
		UPDATE backup_executions SET
			cloud_status = ?, drive_file_id = ?, retry_count = ?, next_retry_at = ?, error_message = ?
		WHERE id = ?`,
		cloudStatus, driveFileID, retryCount, nextRetryAt, errMsg, id,
	)
	return err
}

func (s *Store) MarkLocalDeleted(id int64) error {
	_, err := s.db.Exec(`UPDATE backup_executions SET deleted_local_at = ? WHERE id = ?`, nowISO(), id)
	return err
}

func (s *Store) MarkCloudDeleted(id int64) error {
	_, err := s.db.Exec(`UPDATE backup_executions SET deleted_cloud_at = ? WHERE id = ?`, nowISO(), id)
	return err
}

func (s *Store) GetExecution(id int64) (*BackupExecution, error) {
	row := s.db.QueryRow(`
		SELECT id, profile_id, profile_name, pg_server_id, server_name, db_name, trigger_type, format,
		       started_at, finished_at, duration_ms, local_status, local_file_path, file_size_bytes,
		       sha256_checksum, cloud_status, drive_file_id, retry_count, next_retry_at, error_message,
		       deleted_local_at, deleted_cloud_at
		FROM backup_executions WHERE id = ?`, id)
	ex, err := scanExecutionRow(row)
	if err != nil {
		return nil, err
	}
	return &ex, nil
}

func (s *Store) ListExecutions(serverID int64, dbName string, limit int) ([]BackupExecution, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
		SELECT id, profile_id, profile_name, pg_server_id, server_name, db_name, trigger_type, format,
		       started_at, finished_at, duration_ms, local_status, local_file_path, file_size_bytes,
		       sha256_checksum, cloud_status, drive_file_id, retry_count, next_retry_at, error_message,
		       deleted_local_at, deleted_cloud_at
		FROM backup_executions WHERE 1=1`
	var args []any
	if serverID > 0 {
		query += ` AND pg_server_id = ?`
		args = append(args, serverID)
	}
	if dbName != "" {
		query += ` AND db_name = ?`
		args = append(args, dbName)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []BackupExecution{}
	for rows.Next() {
		ex, err := scanExecutionRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ex)
	}
	return out, rows.Err()
}

// ListDueCloudUploads returns executions that succeeded locally and are pending or due for cloud retry.
func (s *Store) ListDueCloudUploads(now time.Time) ([]BackupExecution, error) {
	nowStr := now.UTC().Format(time.RFC3339)
	rows, err := s.db.Query(`
		SELECT id, profile_id, profile_name, pg_server_id, server_name, db_name, trigger_type, format,
		       started_at, finished_at, duration_ms, local_status, local_file_path, file_size_bytes,
		       sha256_checksum, cloud_status, drive_file_id, retry_count, next_retry_at, error_message,
		       deleted_local_at, deleted_cloud_at
		FROM backup_executions
		WHERE local_status = 'completed'
		  AND deleted_local_at = ''
		  AND (cloud_status = 'pending' OR (cloud_status = 'retrying' AND (next_retry_at = '' OR next_retry_at <= ?)))
		ORDER BY id ASC LIMIT 25`, nowStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []BackupExecution{}
	for rows.Next() {
		ex, err := scanExecutionRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ex)
	}
	return out, rows.Err()
}

// ListActiveLocalCopiesForRetention lists non-deleted completed local backups for a profile+db, newest first.
func (s *Store) ListActiveLocalCopiesForRetention(profileID int64, dbName string) ([]BackupExecution, error) {
	rows, err := s.db.Query(`
		SELECT id, profile_id, profile_name, pg_server_id, server_name, db_name, trigger_type, format,
		       started_at, finished_at, duration_ms, local_status, local_file_path, file_size_bytes,
		       sha256_checksum, cloud_status, drive_file_id, retry_count, next_retry_at, error_message,
		       deleted_local_at, deleted_cloud_at
		FROM backup_executions
		WHERE profile_id = ? AND db_name = ? AND local_status = 'completed' AND deleted_local_at = ''
		ORDER BY started_at DESC, id DESC`, profileID, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BackupExecution
	for rows.Next() {
		ex, err := scanExecutionRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ex)
	}
	return out, rows.Err()
}

// ListActiveCloudCopiesForRetention lists non-deleted completed Drive backups for a profile+db, newest first.
func (s *Store) ListActiveCloudCopiesForRetention(profileID int64, dbName string) ([]BackupExecution, error) {
	rows, err := s.db.Query(`
		SELECT id, profile_id, profile_name, pg_server_id, server_name, db_name, trigger_type, format,
		       started_at, finished_at, duration_ms, local_status, local_file_path, file_size_bytes,
		       sha256_checksum, cloud_status, drive_file_id, retry_count, next_retry_at, error_message,
		       deleted_local_at, deleted_cloud_at
		FROM backup_executions
		WHERE profile_id = ? AND db_name = ? AND cloud_status = 'completed' AND deleted_cloud_at = '' AND drive_file_id != ''
		ORDER BY started_at DESC, id DESC`, profileID, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BackupExecution
	for rows.Next() {
		ex, err := scanExecutionRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ex)
	}
	return out, rows.Err()
}

// --- Notifications & RDP Users ---

func (s *Store) ListWindowsNotifyUsers() ([]WindowsNotifyUser, error) {
	rows, err := s.db.Query(`SELECT username, sid, enabled FROM windows_notify_users ORDER BY username ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WindowsNotifyUser{}
	for rows.Next() {
		var u WindowsNotifyUser
		var en int
		if err := rows.Scan(&u.Username, &u.SID, &en); err != nil {
			return nil, err
		}
		u.Enabled = en == 1
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) SetWindowsNotifyUser(username, sid string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO windows_notify_users (username, sid, enabled, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET sid = excluded.sid, enabled = excluded.enabled, updated_at = excluded.updated_at`,
		username, sid, en, nowISO(),
	)
	return err
}

func (s *Store) ListTelegramRecipients() ([]TelegramRecipient, error) {
	rows, err := s.db.Query(`SELECT id, chat_id, label, enabled, created_at FROM telegram_recipients ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TelegramRecipient{}
	for rows.Next() {
		var r TelegramRecipient
		var en int
		if err := rows.Scan(&r.ID, &r.ChatID, &r.Label, &en, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Enabled = en == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) SaveTelegramRecipient(r TelegramRecipient) error {
	en := 0
	if r.Enabled {
		en = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO telegram_recipients (chat_id, label, enabled, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET label = excluded.label, enabled = excluded.enabled`,
		r.ChatID, r.Label, en, nowISO(),
	)
	return err
}

func (s *Store) DeleteTelegramRecipient(id int64) error {
	_, err := s.db.Exec(`DELETE FROM telegram_recipients WHERE id = ?`, id)
	return err
}

func (s *Store) GetNotificationRules() ([]NotificationRule, error) {
	rows, err := s.db.Query(`SELECT channel, on_success, on_error, on_retry, on_low_disk, daily_summary, daily_summary_time FROM notification_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NotificationRule{}
	for rows.Next() {
		var r NotificationRule
		var s1, e1, r1, l1, d1 int
		if err := rows.Scan(&r.Channel, &s1, &e1, &r1, &l1, &d1, &r.DailySummaryTime); err != nil {
			return nil, err
		}
		r.OnSuccess = s1 == 1
		r.OnError = e1 == 1
		r.OnRetry = r1 == 1
		r.OnLowDisk = l1 == 1
		r.DailySummary = d1 == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) SaveNotificationRule(r NotificationRule) error {
	b2i := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	_, err := s.db.Exec(`
		INSERT INTO notification_rules (channel, on_success, on_error, on_retry, on_low_disk, daily_summary, daily_summary_time)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(channel) DO UPDATE SET
			on_success = excluded.on_success, on_error = excluded.on_error, on_retry = excluded.on_retry,
			on_low_disk = excluded.on_low_disk, daily_summary = excluded.daily_summary, daily_summary_time = excluded.daily_summary_time`,
		r.Channel, b2i(r.OnSuccess), b2i(r.OnError), b2i(r.OnRetry), b2i(r.OnLowDisk), b2i(r.DailySummary), r.DailySummaryTime,
	)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanExecutionRow(r rowScanner) (BackupExecution, error) {
	var ex BackupExecution
	var profID sql.NullInt64
	err := r.Scan(
		&ex.ID, &profID, &ex.ProfileName, &ex.PGServerID, &ex.ServerName, &ex.DBName, &ex.TriggerType, &ex.Format,
		&ex.StartedAt, &ex.FinishedAt, &ex.DurationMs, &ex.LocalStatus, &ex.LocalFilePath, &ex.FileSizeBytes,
		&ex.SHA256Checksum, &ex.CloudStatus, &ex.DriveFileID, &ex.RetryCount, &ex.NextRetryAt, &ex.ErrorMessage,
		&ex.DeletedLocalAt, &ex.DeletedCloudAt,
	)
	if profID.Valid {
		v := profID.Int64
		ex.ProfileID = &v
	}
	return ex, err
}

func scanExecutionRows(rows *sql.Rows) (BackupExecution, error) {
	return scanExecutionRow(rows)
}
