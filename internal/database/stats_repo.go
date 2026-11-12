package database

import "time"

type DashboardMetrics struct {
	TotalExecutions24h int   `json:"totalExecutions24h"`
	Success24h         int   `json:"success24h"`
	Failed24h          int   `json:"failed24h"`
	PendingCloudCount  int   `json:"pendingCloudCount"`
	TotalLocalBytes    int64 `json:"totalLocalBytes"`
	ActiveProfiles     int   `json:"activeProfiles"`
	ConfiguredServers  int   `json:"configuredServers"`
}

func (s *Store) GetDashboardMetrics() (DashboardMetrics, error) {
	var m DashboardMetrics
	since := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)

	_ = s.db.QueryRow(`SELECT COUNT(1) FROM backup_executions WHERE started_at >= ?`, since).Scan(&m.TotalExecutions24h)
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM backup_executions WHERE started_at >= ? AND local_status = 'completed' AND (cloud_status = 'not_applicable' OR cloud_status = 'completed')`, since).Scan(&m.Success24h)
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM backup_executions WHERE started_at >= ? AND (local_status = 'failed' OR cloud_status = 'failed')`, since).Scan(&m.Failed24h)
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM backup_executions WHERE local_status = 'completed' AND deleted_local_at = '' AND cloud_status IN ('pending', 'retrying', 'uploading')`).Scan(&m.PendingCloudCount)
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(file_size_bytes), 0) FROM backup_executions WHERE local_status = 'completed' AND deleted_local_at = ''`).Scan(&m.TotalLocalBytes)
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM backup_profiles WHERE enabled = 1`).Scan(&m.ActiveProfiles)
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM pg_servers`).Scan(&m.ConfiguredServers)
	return m, nil
}

type DBBackupCounts struct {
	TotalCopies     int
	LocalCopies     int
	CloudCopies     int
	LastBackupAt    string
	LastLocalStatus string
	LastCloudStatus string
}

func (s *Store) GetDBBackupStatsMap(serverID int64) (map[string]DBBackupCounts, error) {
	rows, err := s.db.Query(`
		SELECT db_name,
		       COUNT(1) AS total_copies,
		       SUM(CASE WHEN local_status = 'completed' AND deleted_local_at = '' THEN 1 ELSE 0 END) AS local_copies,
		       SUM(CASE WHEN cloud_status = 'completed' AND deleted_cloud_at = '' THEN 1 ELSE 0 END) AS cloud_copies
		FROM backup_executions
		WHERE pg_server_id = ?
		GROUP BY db_name`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(map[string]DBBackupCounts)
	for rows.Next() {
		var dbName string
		var c DBBackupCounts
		if err := rows.Scan(&dbName, &c.TotalCopies, &c.LocalCopies, &c.CloudCopies); err != nil {
			return nil, err
		}
		res[dbName] = c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for dbName, c := range res {
		_ = s.db.QueryRow(`
			SELECT started_at, local_status, cloud_status
			FROM backup_executions
			WHERE pg_server_id = ? AND db_name = ?
			ORDER BY id DESC LIMIT 1`, serverID, dbName).Scan(&c.LastBackupAt, &c.LastLocalStatus, &c.LastCloudStatus)
		res[dbName] = c
	}
	return res, nil
}
