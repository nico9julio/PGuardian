package config

const (
	AppName           = "PGuardian"
	ServiceName       = "PGuardianService"
	ServiceDisplay    = "PGuardian PostgreSQL Backup Service"
	ServiceDesc       = "Automated local and Google Drive backup service for PostgreSQL on Windows Server."
	PipeName          = `\\.\pipe\PGuardianIPC`
	DefaultGitHubRepo = "nico9julio/PGuardian"
)

// Version and Commit are injected at build time via -ldflags.
var (
	Version   = "0.1.1"
	Commit    = "dev"
	BuildDate = "unknown"
)
