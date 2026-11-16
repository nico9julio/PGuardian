ALTER TABLE pg_servers ADD COLUMN engine TEXT NOT NULL DEFAULT 'postgres';

CREATE TABLE IF NOT EXISTS backup_directories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    exclude_patterns TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL
);

CREATE TEMP TABLE _tmp_profile_databases AS
SELECT profile_id, db_name, inclusion_type FROM profile_databases;

DROP TABLE profile_databases;

CREATE TABLE backup_profiles_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    target_type TEXT NOT NULL DEFAULT 'database',
    pg_server_id INTEGER,
    enabled INTEGER NOT NULL DEFAULT 1,
    schedule_cron TEXT NOT NULL DEFAULT '0 2 * * *',
    schedule_label TEXT NOT NULL DEFAULT 'Daily 02:00',
    format TEXT NOT NULL DEFAULT 'custom',
    selection_mode TEXT NOT NULL DEFAULT 'manual',
    local_dir_override TEXT NOT NULL DEFAULT '',
    local_retention_mode TEXT NOT NULL DEFAULT 'count',
    local_retention_value INTEGER NOT NULL DEFAULT 7,
    drive_enabled INTEGER NOT NULL DEFAULT 0,
    drive_folder_id TEXT NOT NULL DEFAULT '',
    drive_folder_path TEXT NOT NULL DEFAULT '',
    drive_retention_mode TEXT NOT NULL DEFAULT 'count',
    drive_retention_value INTEGER NOT NULL DEFAULT 14,
    created_at TEXT NOT NULL,
    FOREIGN KEY (pg_server_id) REFERENCES pg_servers(id) ON DELETE CASCADE
);

INSERT INTO backup_profiles_new (
    id, name, target_type, pg_server_id, enabled, schedule_cron, schedule_label,
    format, selection_mode, local_dir_override, local_retention_mode, local_retention_value,
    drive_enabled, drive_folder_id, drive_folder_path, drive_retention_mode, drive_retention_value, created_at
)
SELECT
    id, name, 'database',
    CASE WHEN pg_server_id > 0 THEN pg_server_id ELSE NULL END,
    enabled, schedule_cron, schedule_label,
    format, selection_mode, local_dir_override, local_retention_mode, local_retention_value,
    drive_enabled, drive_folder_id, drive_folder_path, drive_retention_mode, drive_retention_value, created_at
FROM backup_profiles;

DROP TABLE backup_profiles;
ALTER TABLE backup_profiles_new RENAME TO backup_profiles;

CREATE TABLE profile_databases (
    profile_id INTEGER NOT NULL,
    db_name TEXT NOT NULL,
    inclusion_type TEXT NOT NULL DEFAULT 'included',
    PRIMARY KEY (profile_id, db_name),
    FOREIGN KEY (profile_id) REFERENCES backup_profiles(id) ON DELETE CASCADE
);

INSERT INTO profile_databases (profile_id, db_name, inclusion_type)
SELECT profile_id, db_name, inclusion_type FROM _tmp_profile_databases;

DROP TABLE _tmp_profile_databases;

CREATE TABLE IF NOT EXISTS profile_directories (
    profile_id INTEGER NOT NULL,
    directory_id INTEGER NOT NULL,
    inclusion_type TEXT NOT NULL DEFAULT 'included',
    PRIMARY KEY (profile_id, directory_id),
    FOREIGN KEY (profile_id) REFERENCES backup_profiles(id) ON DELETE CASCADE,
    FOREIGN KEY (directory_id) REFERENCES backup_directories(id) ON DELETE CASCADE
);

ALTER TABLE backup_executions ADD COLUMN target_type TEXT NOT NULL DEFAULT 'database';
ALTER TABLE backup_executions ADD COLUMN server_engine TEXT NOT NULL DEFAULT 'postgres';
ALTER TABLE backup_executions ADD COLUMN directory_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE backup_executions ADD COLUMN warning_message TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_executions_dir ON backup_executions(target_type, directory_id, started_at DESC);
