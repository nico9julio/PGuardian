CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pg_servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    host TEXT NOT NULL DEFAULT '127.0.0.1',
    port INTEGER NOT NULL DEFAULT 5432,
    username TEXT NOT NULL DEFAULT 'postgres',
    password_enc TEXT NOT NULL DEFAULT '',
    pg_dump_path TEXT NOT NULL DEFAULT '',
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS backup_profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    pg_server_id INTEGER NOT NULL,
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

CREATE TABLE IF NOT EXISTS profile_databases (
    profile_id INTEGER NOT NULL,
    db_name TEXT NOT NULL,
    inclusion_type TEXT NOT NULL DEFAULT 'included',
    PRIMARY KEY (profile_id, db_name),
    FOREIGN KEY (profile_id) REFERENCES backup_profiles(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS backup_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id INTEGER,
    profile_name TEXT NOT NULL DEFAULT '',
    pg_server_id INTEGER NOT NULL,
    server_name TEXT NOT NULL DEFAULT '',
    db_name TEXT NOT NULL,
    trigger_type TEXT NOT NULL DEFAULT 'scheduled',
    format TEXT NOT NULL DEFAULT 'custom',
    started_at TEXT NOT NULL,
    finished_at TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    local_status TEXT NOT NULL DEFAULT 'running',
    local_file_path TEXT NOT NULL DEFAULT '',
    file_size_bytes INTEGER NOT NULL DEFAULT 0,
    sha256_checksum TEXT NOT NULL DEFAULT '',
    cloud_status TEXT NOT NULL DEFAULT 'not_applicable',
    drive_file_id TEXT NOT NULL DEFAULT '',
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    deleted_local_at TEXT NOT NULL DEFAULT '',
    deleted_cloud_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_executions_db ON backup_executions(pg_server_id, db_name, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_executions_cloud_queue ON backup_executions(cloud_status, next_retry_at);
CREATE INDEX IF NOT EXISTS idx_executions_started ON backup_executions(started_at DESC);

CREATE TABLE IF NOT EXISTS windows_notify_users (
    username TEXT PRIMARY KEY,
    sid TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS telegram_recipients (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS notification_rules (
    channel TEXT PRIMARY KEY,
    on_success INTEGER NOT NULL DEFAULT 1,
    on_error INTEGER NOT NULL DEFAULT 1,
    on_retry INTEGER NOT NULL DEFAULT 1,
    on_low_disk INTEGER NOT NULL DEFAULT 1,
    daily_summary INTEGER NOT NULL DEFAULT 0,
    daily_summary_time TEXT NOT NULL DEFAULT '08:00'
);

INSERT OR IGNORE INTO notification_rules (channel, on_success, on_error, on_retry, on_low_disk, daily_summary, daily_summary_time)
VALUES
    ('windows', 1, 1, 1, 1, 0, '08:00'),
    ('telegram', 0, 1, 1, 1, 1, '08:00');
