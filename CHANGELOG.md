# Changelog

All notable changes to **PGuardian** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-11-11

### Added
- **Windows Service Core (`PGuardianService.exe`):** Native Session 0 service in Go with Windows SCM automatic crash recovery, panic-isolated background workers, rotating JSON logs (`lumberjack`), and pure-Go embedded SQLite (`modernc.org/sqlite`) in WAL mode.
- **PostgreSQL Streaming Backup Engine:** Auto-detection of installed `pg_dump.exe` binaries, connection verification, live database size discovery, low-priority process execution (`BELOW_NORMAL_PRIORITY_CLASS`), pre-flight disk space checks, and streaming support for Custom (`-Fc .backup`) and Gzip SQL (`.sql.gz`) formats with `SHA-256` integrity verification.
- **Flexible Profiles & Retention Policies:** Cron-based backup profiles supporting manual database selection or automatic inclusion of all current and future databases (with exclusion list), plus independent Local and Google Drive retention (`count` or `days`) that protects unsynced local backups.
- **Google Drive OAuth 2.0 & Persistent Queue:** Bring-your-own OAuth `Client ID` & `Client Secret` configuration, browser login via Loopback + PKCE (`S256`), interactive visual Drive folder browser/creator, automatic per-database subfolders, 8 MB resumable chunked uploads, and exponential backoff offline retry queue.
- **Multi-Session RDP Notification System (`PGuardianTray.exe`):** Ultra-lightweight pure Win32 System Tray agent (~4 MB RAM, zero WebView2) with per-user Windows Task Scheduler logon registration strictly for enabled RDP/Console users, plus Telegram Bot API integration supporting multiple Chat IDs, 1-click chat auto-detection, and scheduled daily summaries.
- **Native Bilingual Admin Console (`PGuardian.exe`):** Wails v2 + React 19 + TypeScript + Tailwind CSS v4 interface featuring Spanish/English localization, adaptive System/Dark/Light themes, 6-step Initial Setup Wizard, Argon2id Master Password/PIN lock screen, and Windows Named Pipe (`\\.\pipe\PGuardianIPC`) communication.
- **Atomic Production Updater (`PGuardianUpdater.exe`):** Dual update workflow via 1-click GitHub Releases or local `.zip` bundle over RDP, including SHA-256 manifest verification, multi-session RDP tray drain, online SQLite `VACUUM INTO` snapshot, healthcheck verification, and automatic rollback on failure.
- **Windows Installer:** Inno Setup installer (`PGuardianSetup_v0.1.0.exe`) with language-neutral SID ACL hardening on `%ProgramData%\PGuardian`.

### Security
- Encrypted all stored credentials and API tokens at rest using machine-bound **Windows DPAPI (`CryptProtectData`)**.
- Eliminated local TCP listeners in favor of ACL-restricted Windows Named Pipes with caller token SID verification.

[Unreleased]: https://github.com/nico9julio/PGuardian/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/nico9julio/PGuardian/releases/tag/v0.1.0
