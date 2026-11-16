package database

import "strings"

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

const (
	EnginePostgres = "postgres"
	EngineMariaDB  = "mariadb"
	EngineMySQL    = "mysql"

	TargetDatabase  = "database"
	TargetDirectory = "directory"
)

func NormalizeEngine(eng string) string {
	switch strings.ToLower(strings.TrimSpace(eng)) {
	case EngineMariaDB:
		return EngineMariaDB
	case EngineMySQL:
		return EngineMySQL
	default:
		return EnginePostgres
	}
}

func IsMySQLFamily(eng string) bool {
	n := NormalizeEngine(eng)
	return n == EngineMariaDB || n == EngineMySQL
}

func NormalizeTargetType(t string) string {
	if strings.EqualFold(strings.TrimSpace(t), TargetDirectory) {
		return TargetDirectory
	}
	return TargetDatabase
}

type PGServer struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Engine      string `json:"engine"` // "postgres" | "mariadb" | "mysql"
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"` // Input only; never returned in list responses
	HasPassword bool   `json:"hasPassword"`
	PGDumpPath  string `json:"pgDumpPath"` // Dump binary path (pg_dump.exe / mariadb-dump.exe / mysqldump.exe)
	IsDefault   bool   `json:"isDefault"`
	CreatedAt   string `json:"createdAt"`
}

type BackupDirectory struct {
	ID              int64    `json:"id"`
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	ExcludePatterns []string `json:"excludePatterns"`
	CreatedAt       string   `json:"createdAt"`
}

type DirectoryBackupSummary struct {
	DirectoryID      int64             `json:"directoryId"`
	Name             string            `json:"name"`
	Path             string            `json:"path"`
	ExcludePatterns  []string          `json:"excludePatterns"`
	Exists           bool              `json:"exists"`
	LiveSizeBytes    int64             `json:"liveSizeBytes"`
	LiveFileCount    int               `json:"liveFileCount"`
	AssignedProfiles []string          `json:"assignedProfiles"`
	TotalCopies      int               `json:"totalCopies"`
	LocalCopies      int               `json:"localCopies"`
	CloudCopies      int               `json:"cloudCopies"`
	LastBackupAt     string            `json:"lastBackupAt"`
	LastLocalStatus  string            `json:"lastLocalStatus"`
	LastCloudStatus  string            `json:"lastCloudStatus"`
	RecentBackups    []BackupExecution `json:"recentBackups,omitempty"`
}

type BackupProfile struct {
	ID                  int64    `json:"id"`
	Name                string   `json:"name"`
	TargetType          string   `json:"targetType"` // "database" | "directory"
	PGServerID          int64    `json:"pgServerId"`
	ServerName          string   `json:"serverName,omitempty"`
	ServerEngine        string   `json:"serverEngine,omitempty"`
	Enabled             bool     `json:"enabled"`
	ScheduleCron        string   `json:"scheduleCron"`
	ScheduleLabel       string   `json:"scheduleLabel"`
	Format              string   `json:"format"`        // "custom" | "plain_gz" | "zip"
	SelectionMode       string   `json:"selectionMode"` // "manual" | "all_auto"
	IncludedDatabases   []string `json:"includedDatabases"`
	ExcludedDatabases   []string `json:"excludedDatabases"`
	IncludedDirectories []int64  `json:"includedDirectories"`
	ExcludedDirectories []int64  `json:"excludedDirectories"`
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
	TargetType     string `json:"targetType"`   // "database" | "directory"
	ServerEngine   string `json:"serverEngine"` // "postgres" | "mariadb" | "mysql" | ""
	PGServerID     int64  `json:"pgServerId"`
	ServerName     string `json:"serverName"`
	DirectoryID    int64  `json:"directoryId"`
	DBName         string `json:"dbName"` // Database name or directory alias
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
	WarningMessage string `json:"warningMessage"`
	DeletedLocalAt string `json:"deletedLocalAt"`
	DeletedCloudAt string `json:"deletedCloudAt"`
}

type DatabaseBackupSummary struct {
	ServerID         int64             `json:"serverId"`
	ServerName       string            `json:"serverName"`
	ServerEngine     string            `json:"serverEngine"`
	DBName           string            `json:"dbName"`
	LiveSizeBytes    int64             `json:"liveSizeBytes"`
	AssignedProfiles []string          `json:"assignedProfiles"`
	TotalCopies      int               `json:"totalCopies"`
	LocalCopies      int               `json:"localCopies"`
	CloudCopies      int               `json:"cloudCopies"`
	LastBackupAt     string            `json:"lastBackupAt"`
	LastLocalStatus  string            `json:"lastLocalStatus"`
	LastCloudStatus  string            `json:"lastCloudStatus"`
	RecentBackups    []BackupExecution `json:"recentBackups,omitempty"`
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
