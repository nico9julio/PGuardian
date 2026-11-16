package config

const (
	AppName           = "PGuardian"
	ServiceName       = "PGuardianService"
	ServiceDisplay    = "PGuardian Backup Service"
	ServiceDesc       = "Automated local and Google Drive backup service for PostgreSQL, MariaDB/MySQL, and Directories on Windows Server."
	PipeName          = `\\.\pipe\PGuardianIPC`
	DefaultGitHubRepo = "nico9julio/PGuardian"
)

// Version and Commit are injected at build time via -ldflags.
var (
	Version   = "0.2.0"
	Commit    = "dev"
	BuildDate = "unknown"
)
