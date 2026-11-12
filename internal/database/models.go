package database

// Setting keys persisted in app_settings.
const (
	KeySetupCompleted       = "setup_completed"
	KeyMasterPasswordHash   = "master_password_hash"
	KeyLocalBackupRoot      = "local_backup_root"
	KeyMinFreeDiskGB        = "min_free_disk_gb"
	KeyLanguage             = "language"
	KeyTheme                = "theme"
	KeyDriveClientID        = "drive_client_id"
	KeyDriveClientSecretEnc = "drive_client_secret_enc"
	KeyDriveTokenJSONEnc    = "drive_token_json_enc"
	KeyDriveAccountEmail    = "drive_account_email"
	KeyDriveRootFolderID    = "drive_root_folder_id"
	KeyDriveRootFolderName  = "drive_root_folder_name"
	KeyTelegramBotTokenEnc  = "telegram_bot_token_enc"
	KeyGitHubRepo           = "github_repo"
	KeyGitHubTokenEnc       = "github_token_enc"
	KeyLastDailySummaryDate = "last_daily_summary_date"
)

type PGServer struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"` // Input only; never returned in list responses
	HasPassword bool   `json:"hasPassword"`
	PGDumpPath  string `json:"pgDumpPath"`
	IsDefault   bool   `json:"isDefault"`
	CreatedAt   string `json:"createdAt"`
}

type BackupProfile struct {
	ID                  int64    `json:"id"`
	Name                string   `json:"name"`
	PGServerID          int64    `json:"pgServerId"`
	ServerName          string   `json:"serverName,omitempty"`
	Enabled             bool     `json:"enabled"`
	ScheduleCron        string   `json:"scheduleCron"`
	ScheduleLabel       string   `json:"scheduleLabel"`
	Format              string   `json:"format"`        // "custom" | "plain_gz"
	SelectionMode       string   `json:"selectionMode"` // "manual" | "all_auto"
	IncludedDatabases   []string `json:"includedDatabases"`
	ExcludedDatabases   []string `json:"excludedDatabases"`
	LocalDirOverride    string   `json:"localDirOverride"`
	LocalRetentionMode  string   `json:"localRetentionMode"` // "count" | "days"
	LocalRetentionValue int      `json:"localRetentionValue"`
	DriveEnabled        bool     `json:"driveEnabled"`
	DriveFolderID       string   `json:"driveFolderId"`
	DriveFolderPath     string   `json:"driveFolderPath"`
	DriveRetentionMode  string   `json:"driveRetentionMode"` // "count" | "days"
	DriveRetentionValue int      `json:"driveRetentionValue"`
	CreatedAt           string   `json:"createdAt"`
}

type BackupExecution struct {
	ID             int64  `json:"id"`
	ProfileID      *int64 `json:"profileId,omitempty"`
	ProfileName    string `json:"profileName"`
	PGServerID     int64  `json:"pgServerId"`
	ServerName     string `json:"serverName"`
	DBName         string `json:"dbName"`
	TriggerType    string `json:"triggerType"` // "scheduled" | "manual"
	Format         string `json:"format"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt"`
	DurationMs     int64  `json:"durationMs"`
	LocalStatus    string `json:"localStatus"` // "running" | "completed" | "failed"
	LocalFilePath  string `json:"localFilePath"`
	FileSizeBytes  int64  `json:"fileSizeBytes"`
	SHA256Checksum string `json:"sha256Checksum"`
	CloudStatus    string `json:"cloudStatus"` // "not_applicable" | "pending" | "uploading" | "retrying" | "completed" | "failed"
	DriveFileID    string `json:"driveFileId"`
	RetryCount     int    `json:"retryCount"`
	NextRetryAt    string `json:"nextRetryAt"`
	ErrorMessage   string `json:"errorMessage"`
	DeletedLocalAt string `json:"deletedLocalAt"`
	DeletedCloudAt string `json:"deletedCloudAt"`
}

type DatabaseBackupSummary struct {
	ServerID        int64             `json:"serverId"`
	ServerName      string            `json:"serverName"`
	DBName          string            `json:"dbName"`
	LiveSizeBytes   int64             `json:"liveSizeBytes"`
	AssignedProfiles []string         `json:"assignedProfiles"`
	TotalCopies     int               `json:"totalCopies"`
	LocalCopies     int               `json:"localCopies"`
	CloudCopies     int               `json:"cloudCopies"`
	LastBackupAt    string            `json:"lastBackupAt"`
	LastLocalStatus string            `json:"lastLocalStatus"`
	LastCloudStatus string            `json:"lastCloudStatus"`
	RecentBackups   []BackupExecution `json:"recentBackups,omitempty"`
}

type WindowsNotifyUser struct {
	Username string `json:"username"`
	SID      string `json:"sid"`
	Enabled  bool   `json:"enabled"`
}

type TelegramRecipient struct {
	ID        int64  `json:"id"`
	ChatID    string `json:"chatId"`
	Label     string `json:"label"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"createdAt"`
}

type NotificationRule struct {
	Channel          string `json:"channel"` // "windows" | "telegram"
	OnSuccess        bool   `json:"onSuccess"`
	OnError          bool   `json:"onError"`
	OnRetry          bool   `json:"onRetry"`
	OnLowDisk        bool   `json:"onLowDisk"`
	DailySummary     bool   `json:"dailySummary"`
	DailySummaryTime string `json:"dailySummaryTime"`
}
