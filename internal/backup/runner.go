package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nico9julio/PGuardian/internal/cloud"
	"github.com/nico9julio/PGuardian/internal/config"
	"github.com/nico9julio/PGuardian/internal/database"
	"github.com/nico9julio/PGuardian/internal/dirbackup"
	"github.com/nico9julio/PGuardian/internal/mysql"
	"github.com/nico9julio/PGuardian/internal/notify"
	"github.com/nico9julio/PGuardian/internal/postgres"
	"golang.org/x/sys/windows"
)

var nonSafeChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type Engine struct {
	store      *database.Store
	cloudSvc   *cloud.Service
	dispatcher *notify.Dispatcher
	runMu      sync.Mutex // Serializes dump/archive executions so server I/O stays gentle
	activeJobs atomic.Int32
}

func NewEngine(store *database.Store, cloudSvc *cloud.Service, dispatcher *notify.Dispatcher) *Engine {
	e := &Engine{
		store:      store,
		cloudSvc:   cloudSvc,
		dispatcher: dispatcher,
	}
	e.CleanOrphanPartFiles()
	return e
}

func (e *Engine) ActiveJobs() int32 {
	return e.activeJobs.Load()
}

// CleanOrphanPartFiles removes incomplete .part temporary files and temp mysql cnf files left behind by a power loss.
func (e *Engine) CleanOrphanPartFiles() {
	root := e.store.GetSetting(database.KeyLocalBackupRoot, config.DefaultLocalBackupsDir())
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".part") {
			_ = os.Remove(path)
		}
		return nil
	})
	if tmpEntries, err := os.ReadDir(config.TempDir()); err == nil {
		for _, entry := range tmpEntries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "mysql_") && strings.HasSuffix(entry.Name(), ".cnf") {
				_ = os.Remove(filepath.Join(config.TempDir(), entry.Name()))
			}
		}
	}
}

// RunProfile executes backups for all databases or directories targeted by a BackupProfile.
func (e *Engine) RunProfile(ctx context.Context, profileID int64, triggerType string) error {
	prof, err := e.store.GetProfile(profileID)
	if err != nil {
		return err
	}

	batchCtx := notify.WithBatchTask(ctx)
	startedAt := time.Now()

	if prof.TargetType == database.TargetDirectory {
		targetDirs, err := e.resolveProfileDirectories(prof)
		if err != nil {
			e.dispatcher.NotifyBackupError("(directories)", prof.Name, err.Error(), 0)
			return err
		}
		items := make([]notify.TaskBackupItem, 0, len(targetDirs))
		for i := range targetDirs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			d := targetDirs[i]
			execID, execErr := e.ExecuteDirectoryBackup(batchCtx, prof, &d, triggerType)
			if execErr != nil {
				slog.Error("directory backup failed", "profile", prof.Name, "dir", d.Name, "error", execErr)
			}
			items = append(items, e.buildTaskBackupItem(d.Name, execID, execErr))
		}
		e.dispatcher.NotifyTaskSummary(notify.TaskSummary{
			ProfileName:  prof.Name,
			TriggerType:  triggerType,
			TargetType:   prof.TargetType,
			DriveEnabled: prof.DriveEnabled,
			Duration:     time.Since(startedAt),
			Items:        items,
		})
		return nil
	}

	srv, pwd, err := e.store.GetPGServerWithSecret(prof.PGServerID)
	if err != nil {
		return fmt.Errorf("load server credentials: %w", err)
	}

	targetDBs, err := e.resolveProfileDatabases(ctx, prof, srv, pwd)
	if err != nil {
		e.dispatcher.NotifyBackupError("(server)", prof.Name, err.Error(), 0)
		return err
	}

	items := make([]notify.TaskBackupItem, 0, len(targetDBs))
	for _, dbName := range targetDBs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		execID, execErr := e.ExecuteSingleBackup(batchCtx, prof, srv, pwd, dbName, triggerType)
		if execErr != nil {
			slog.Error("database backup failed", "profile", prof.Name, "db", dbName, "error", execErr)
		}
		items = append(items, e.buildTaskBackupItem(dbName, execID, execErr))
	}
	e.dispatcher.NotifyTaskSummary(notify.TaskSummary{
		ProfileName:  prof.Name,
		TriggerType:  triggerType,
		TargetType:   prof.TargetType,
		DriveEnabled: prof.DriveEnabled,
		Duration:     time.Since(startedAt),
		Items:        items,
	})
	return nil
}

func (e *Engine) buildTaskBackupItem(targetName string, execID int64, execErr error) notify.TaskBackupItem {
	item := notify.TaskBackupItem{
		Name:    targetName,
		Success: execErr == nil,
	}
	if execErr != nil {
		item.Error = execErr.Error()
	}
	if execID > 0 {
		if ex, err := e.store.GetExecution(execID); err == nil && ex != nil {
			item.SizeBytes = ex.FileSizeBytes
			item.Success = ex.LocalStatus == "completed"
			item.CloudUploaded = ex.CloudStatus == "completed"
			item.CloudPending = ex.CloudStatus == "retrying" || ex.CloudStatus == "pending"
			item.Warning = ex.WarningMessage
			if ex.ErrorMessage != "" && item.Error == "" {
				item.Error = ex.ErrorMessage
			}
		}
	}
	return item
}

// RunManualDatabase triggers an immediate ad-hoc backup for a single database (using its assigned profile if any, or sensible defaults).
func (e *Engine) RunManualDatabase(ctx context.Context, serverID int64, dbName string, profileID int64) (int64, error) {
	srv, pwd, err := e.store.GetPGServerWithSecret(serverID)
	if err != nil {
		return 0, err
	}

	var prof *database.BackupProfile
	if profileID > 0 {
		prof, _ = e.store.GetProfile(profileID)
	}
	if prof == nil {
		allProfs, _ := e.store.ListProfiles()
		for i := range allProfs {
			p := &allProfs[i]
			if p.TargetType == database.TargetDatabase && p.PGServerID == serverID && profileMatchesDB(p, dbName) {
				prof = p
				break
			}
		}
	}
	if prof == nil {
		driveStatus := e.cloudSvc.Status()
		defFormat := "custom"
		if database.IsMySQLFamily(srv.Engine) {
			defFormat = "plain_gz"
		}
		prof = &database.BackupProfile{
			Name:                "Manual",
			TargetType:          database.TargetDatabase,
			PGServerID:          serverID,
			Format:              defFormat,
			LocalRetentionMode:  "count",
			LocalRetentionValue: 10,
			DriveEnabled:        driveStatus.Connected,
			DriveFolderID:       driveStatus.RootFolderID,
			DriveRetentionMode:  "count",
			DriveRetentionValue: 14,
		}
	}
	return e.ExecuteSingleBackup(ctx, prof, srv, pwd, dbName, "manual")
}

// RunManualDirectory triggers an immediate ad-hoc backup for a single configured directory.
func (e *Engine) RunManualDirectory(ctx context.Context, directoryID int64, profileID int64) (int64, error) {
	dir, err := e.store.GetDirectory(directoryID)
	if err != nil {
		return 0, err
	}

	var prof *database.BackupProfile
	if profileID > 0 {
		prof, _ = e.store.GetProfile(profileID)
	}
	if prof == nil {
		allProfs, _ := e.store.ListProfiles()
		for i := range allProfs {
			p := &allProfs[i]
			if p.TargetType == database.TargetDirectory && profileMatchesDir(p, directoryID) {
				prof = p
				break
			}
		}
	}
	if prof == nil {
		driveStatus := e.cloudSvc.Status()
		prof = &database.BackupProfile{
			Name:                "Manual",
			TargetType:          database.TargetDirectory,
			Format:              "zip",
			LocalRetentionMode:  "count",
			LocalRetentionValue: 10,
			DriveEnabled:        driveStatus.Connected,
			DriveFolderID:       driveStatus.RootFolderID,
			DriveRetentionMode:  "count",
			DriveRetentionValue: 14,
		}
	}
	return e.ExecuteDirectoryBackup(ctx, prof, dir, "manual")
}

func profileMatchesDB(p *database.BackupProfile, dbName string) bool {
	if p.SelectionMode == "all_auto" {
		for _, ex := range p.ExcludedDatabases {
			if ex == dbName {
				return false
			}
		}
		return true
	}
	for _, in := range p.IncludedDatabases {
		if in == dbName {
			return true
		}
	}
	return false
}

func profileMatchesDir(p *database.BackupProfile, dirID int64) bool {
	if p.SelectionMode == "all_auto" {
		for _, ex := range p.ExcludedDirectories {
			if ex == dirID {
				return false
			}
		}
		return true
	}
	for _, in := range p.IncludedDirectories {
		if in == dirID {
			return true
		}
	}
	return false
}

func (e *Engine) resolveProfileDatabases(ctx context.Context, prof *database.BackupProfile, srv *database.PGServer, pwd string) ([]string, error) {
	if prof.SelectionMode == "manual" {
		if len(prof.IncludedDatabases) == 0 {
			return nil, nil
		}
		return prof.IncludedDatabases, nil
	}

	var liveDBs []postgres.LiveDatabaseInfo
	if database.IsMySQLFamily(srv.Engine) {
		mysqlDBs, err := mysql.ListDatabases(ctx, mysql.ConnectionParams{
			Host:     srv.Host,
			Port:     srv.Port,
			Username: srv.Username,
			Password: pwd,
		})
		if err != nil {
			return nil, err
		}
		for _, d := range mysqlDBs {
			liveDBs = append(liveDBs, postgres.LiveDatabaseInfo{Name: d.Name, SizeBytes: d.SizeBytes})
		}
	} else {
		pgDBs, err := postgres.ListDatabases(ctx, postgres.ConnectionParams{
			Host:     srv.Host,
			Port:     srv.Port,
			Username: srv.Username,
			Password: pwd,
		})
		if err != nil {
			return nil, err
		}
		liveDBs = pgDBs
	}

	excluded := make(map[string]bool, len(prof.ExcludedDatabases))
	for _, ex := range prof.ExcludedDatabases {
		excluded[ex] = true
	}
	var out []string
	for _, d := range liveDBs {
		if !excluded[d.Name] {
			out = append(out, d.Name)
		}
	}
	return out, nil
}

func (e *Engine) resolveProfileDirectories(prof *database.BackupProfile) ([]database.BackupDirectory, error) {
	allDirs, err := e.store.ListDirectories()
	if err != nil {
		return nil, err
	}
	if len(allDirs) == 0 {
		return nil, nil
	}

	if prof.SelectionMode == "manual" {
		wanted := make(map[int64]bool, len(prof.IncludedDirectories))
		for _, id := range prof.IncludedDirectories {
			wanted[id] = true
		}
		var out []database.BackupDirectory
		for _, d := range allDirs {
			if wanted[d.ID] {
				out = append(out, d)
			}
		}
		return out, nil
	}

	excluded := make(map[int64]bool, len(prof.ExcludedDirectories))
	for _, id := range prof.ExcludedDirectories {
		excluded[id] = true
	}
	var out []database.BackupDirectory
	for _, d := range allDirs {
		if !excluded[d.ID] {
			out = append(out, d)
		}
	}
	return out, nil
}

func (e *Engine) prepareLocalTarget(baseDirOverride string, subParts ...string) (string, string, error) {
	baseDir := strings.TrimSpace(baseDirOverride)
	if baseDir == "" {
		baseDir = e.store.GetSetting(database.KeyLocalBackupRoot, config.DefaultLocalBackupsDir())
	}
	parts := append([]string{baseDir}, subParts...)
	targetDir := filepath.Join(parts...)
	if err := os.MkdirAll(targetDir, 0750); err != nil {
		return "", "", fmt.Errorf("create backup folder: %w", err)
	}

	// Pre-flight disk space check
	minFreeGB, _ := strconv.ParseFloat(e.store.GetSetting(database.KeyMinFreeDiskGB, "5"), 64)
	if minFreeGB <= 0 {
		minFreeGB = 2
	}
	if disk, dErr := GetDiskSpace(targetDir); dErr == nil {
		if disk.FreeGB < minFreeGB {
			e.dispatcher.NotifyLowDisk(targetDir, disk.FreeGB, minFreeGB)
			if disk.FreeGB < 0.5 {
				return "", "", fmt.Errorf("aborted backup: critically low disk space (%.2f GB free)", disk.FreeGB)
			}
		}
	}
	return baseDir, targetDir, nil
}

func (e *Engine) ExecuteSingleBackup(ctx context.Context, prof *database.BackupProfile, srv *database.PGServer, pwd, dbName, triggerType string) (int64, error) {
	e.activeJobs.Add(1)
	defer e.activeJobs.Add(-1)

	e.runMu.Lock()
	defer e.runMu.Unlock()

	safeDB := nonSafeChars.ReplaceAllString(dbName, "_")
	if safeDB == "" || safeDB == "_" {
		safeDB = "db"
	}
	_, targetDir, err := e.prepareLocalTarget(prof.LocalDirOverride, safeDB)
	if err != nil {
		return 0, err
	}

	engine := database.NormalizeEngine(srv.Engine)
	format := prof.Format
	if database.IsMySQLFamily(engine) {
		format = "plain_gz"
	} else if format != "plain_gz" {
		format = "custom"
	}

	var profIDPtr *int64
	if prof.ID > 0 {
		idCopy := prof.ID
		profIDPtr = &idCopy
	}
	cloudInitial := "not_applicable"
	if prof.DriveEnabled {
		cloudInitial = "pending"
	}

	startTime := time.Now()
	execID, err := e.store.InsertExecution(database.BackupExecution{
		ProfileID:    profIDPtr,
		ProfileName:  prof.Name,
		TargetType:   database.TargetDatabase,
		ServerEngine: engine,
		PGServerID:   srv.ID,
		ServerName:   srv.Name,
		DBName:       dbName,
		TriggerType:  triggerType,
		Format:       format,
		StartedAt:    startTime.UTC().Format(time.RFC3339),
		LocalStatus:  "running",
		CloudStatus:  cloudInitial,
	})
	if err != nil {
		return 0, err
	}

	e.dispatcher.EmitLive(notify.Event{
		Type:        notify.EventBackupStarted,
		Title:       dbName,
		Message:     prof.Name,
		Severity:    "info",
		ExecutionID: execID,
		DBName:      dbName,
		Percent:     5,
	})

	ext := ".backup"
	if format == "plain_gz" {
		ext = ".sql.gz"
	}
	stamp := startTime.Format("20060102_150405")
	finalPath := filepath.Join(targetDir, fmt.Sprintf("%s_%s%s", safeDB, stamp, ext))
	partPath := finalPath + ".part"

	var sizeBytes int64
	var checksum string
	var dumpErr error

	if database.IsMySQLFamily(engine) {
		dumpBin := strings.TrimSpace(srv.PGDumpPath)
		if dumpBin == "" {
			found := mysql.DiscoverMySQLDumpBinaries()
			if len(found) > 0 {
				dumpBin = found[0].Path
			} else {
				dumpBin = "mysqldump"
			}
		}
		sizeBytes, checksum, dumpErr = runStreamingMySQLDump(ctx, dumpBin, srv, pwd, dbName, partPath)
	} else {
		pgDumpBin := strings.TrimSpace(srv.PGDumpPath)
		if pgDumpBin == "" {
			found := postgres.DiscoverPGDumpBinaries()
			if len(found) > 0 {
				pgDumpBin = found[0].Path
			} else {
				pgDumpBin = "pg_dump"
			}
		}
		sizeBytes, checksum, dumpErr = runStreamingPGDump(ctx, pgDumpBin, srv, pwd, dbName, format, partPath)
	}
	durationMs := time.Since(startTime).Milliseconds()

	if dumpErr != nil {
		_ = os.Remove(partPath)
		_ = e.store.CompleteLocalExecution(execID, "failed", "", 0, "", durationMs, "not_applicable", dumpErr.Error())
		e.dispatcher.NotifyBackupError(dbName, prof.Name, dumpErr.Error(), execID)
		return execID, dumpErr
	}

	if err := os.Rename(partPath, finalPath); err != nil {
		_ = os.Remove(partPath)
		_ = e.store.CompleteLocalExecution(execID, "failed", "", 0, "", durationMs, "not_applicable", err.Error())
		e.dispatcher.NotifyBackupError(dbName, prof.Name, err.Error(), execID)
		return execID, err
	}

	if err := e.store.CompleteLocalExecution(execID, "completed", finalPath, sizeBytes, checksum, durationMs, cloudInitial, ""); err != nil {
		return execID, err
	}

	if !prof.DriveEnabled {
		if prof.ID > 0 {
			e.ApplyLocalRetention(prof.ID, database.TargetDatabase, 0, dbName, prof.LocalRetentionMode, prof.LocalRetentionValue)
		}
		e.dispatcher.NotifyBackupSuccessCtx(ctx, dbName, prof.Name, sizeBytes, false, execID)
		return execID, nil
	}

	// Attempt immediate Google Drive upload; if network fails, leave in persistent retry queue without failing local backup!
	if err := e.UploadExecutionToCloud(ctx, execID); err != nil {
		slog.Warn("initial google drive upload deferred to retry queue", "execId", execID, "db", dbName, "error", err)
	}
	return execID, nil
}

func (e *Engine) ExecuteDirectoryBackup(ctx context.Context, prof *database.BackupProfile, dir *database.BackupDirectory, triggerType string) (int64, error) {
	e.activeJobs.Add(1)
	defer e.activeJobs.Add(-1)

	e.runMu.Lock()
	defer e.runMu.Unlock()

	safeDirName := nonSafeChars.ReplaceAllString(dir.Name, "_")
	if safeDirName == "" || safeDirName == "_" {
		safeDirName = fmt.Sprintf("dir_%d", dir.ID)
	}
	baseDir, targetDir, err := e.prepareLocalTarget(prof.LocalDirOverride, "dirs", safeDirName)
	if err != nil {
		return 0, err
	}

	var profIDPtr *int64
	if prof.ID > 0 {
		idCopy := prof.ID
		profIDPtr = &idCopy
	}
	cloudInitial := "not_applicable"
	if prof.DriveEnabled {
		cloudInitial = "pending"
	}

	startTime := time.Now()
	execID, err := e.store.InsertExecution(database.BackupExecution{
		ProfileID:   profIDPtr,
		ProfileName: prof.Name,
		TargetType:  database.TargetDirectory,
		DirectoryID: dir.ID,
		DBName:      dir.Name,
		TriggerType: triggerType,
		Format:      "zip",
		StartedAt:   startTime.UTC().Format(time.RFC3339),
		LocalStatus: "running",
		CloudStatus: cloudInitial,
	})
	if err != nil {
		return 0, err
	}

	e.dispatcher.EmitLive(notify.Event{
		Type:        notify.EventBackupStarted,
		Title:       dir.Name,
		Message:     prof.Name,
		Severity:    "info",
		ExecutionID: execID,
		DBName:      dir.Name,
		Percent:     5,
	})

	stamp := startTime.Format("20060102_150405")
	finalPath := filepath.Join(targetDir, fmt.Sprintf("%s_%s.zip", safeDirName, stamp))
	partPath := finalPath + ".part"

	archRes, archErr := dirbackup.ArchiveDirectoryToZip(ctx, dirbackup.ArchiveOptions{
		SourceDir:       dir.Path,
		DestPartPath:    partPath,
		ExcludePatterns: dir.ExcludePatterns,
		SkipSubtrees:    []string{baseDir, config.DefaultLocalBackupsDir()},
		OnProgress: func(filesDone int, bytesDone int64) {
			e.dispatcher.EmitLive(notify.Event{
				Type:        notify.EventBackupProgress,
				Title:       dir.Name,
				Message:     fmt.Sprintf("%d files (%.1f MB)", filesDone, float64(bytesDone)/(1024*1024)),
				Severity:    "info",
				ExecutionID: execID,
				DBName:      dir.Name,
				Percent:     50,
			})
		},
	})
	durationMs := time.Since(startTime).Milliseconds()

	if archErr != nil {
		_ = os.Remove(partPath)
		_ = e.store.CompleteLocalExecution(execID, "failed", "", 0, "", durationMs, "not_applicable", archErr.Error())
		e.dispatcher.NotifyBackupError(dir.Name, prof.Name, archErr.Error(), execID)
		return execID, archErr
	}

	if err := os.Rename(partPath, finalPath); err != nil {
		_ = os.Remove(partPath)
		_ = e.store.CompleteLocalExecution(execID, "failed", "", 0, "", durationMs, "not_applicable", err.Error())
		e.dispatcher.NotifyBackupError(dir.Name, prof.Name, err.Error(), execID)
		return execID, err
	}

	if err := e.store.CompleteLocalExecutionWithWarning(execID, "completed", finalPath, archRes.ArchiveSizeBytes, archRes.SHA256Checksum, durationMs, cloudInitial, "", archRes.WarningSummary); err != nil {
		return execID, err
	}

	if !prof.DriveEnabled {
		if prof.ID > 0 {
			e.ApplyLocalRetention(prof.ID, database.TargetDirectory, dir.ID, dir.Name, prof.LocalRetentionMode, prof.LocalRetentionValue)
		}
		e.dispatcher.NotifyBackupSuccessWithWarningCtx(ctx, dir.Name, prof.Name, archRes.ArchiveSizeBytes, false, archRes.WarningSummary, execID)
		return execID, nil
	}

	if err := e.UploadExecutionToCloud(ctx, execID); err != nil {
		slog.Warn("initial google drive upload deferred to retry queue", "execId", execID, "dir", dir.Name, "error", err)
	}
	return execID, nil
}

type byteCounter int64

func (b *byteCounter) Write(p []byte) (int, error) {
	*b += byteCounter(len(p))
	return len(p), nil
}

func streamCommandGzipToFile(cmd *exec.Cmd, toolName, partPath string) (int64, string, error) {
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return 0, "", err
	}
	outFile, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return 0, "", err
	}
	hasher := sha256.New()
	multiOut := io.MultiWriter(outFile, hasher)
	gzWriter, _ := gzip.NewWriterLevel(multiOut, gzip.DefaultCompression)
	var rawBytes byteCounter
	teeIn := io.TeeReader(stdoutPipe, &rawBytes)

	if err := cmd.Start(); err != nil {
		_ = outFile.Close()
		return 0, "", fmt.Errorf("start %s: %w", toolName, err)
	}

	buf := make([]byte, 64*1024)
	_, copyErr := io.CopyBuffer(gzWriter, teeIn, buf)
	gzCloseErr := gzWriter.Close()
	fileCloseErr := outFile.Close()
	waitErr := cmd.Wait()

	if waitErr != nil {
		return 0, "", fmt.Errorf("%s failed: %v (%s)", toolName, waitErr, strings.TrimSpace(stderrBuf.String()))
	}
	if copyErr != nil {
		return 0, "", copyErr
	}
	if gzCloseErr != nil {
		return 0, "", gzCloseErr
	}
	if fileCloseErr != nil {
		return 0, "", fileCloseErr
	}

	st, err := os.Stat(partPath)
	if err != nil || st.Size() == 0 || rawBytes == 0 {
		return 0, "", errors.New("generated .sql.gz archive is empty")
	}
	return st.Size(), hex.EncodeToString(hasher.Sum(nil)), nil
}

func runStreamingPGDump(ctx context.Context, binPath string, srv *database.PGServer, pwd, dbName, format, partPath string) (int64, string, error) {
	var args []string
	args = append(args,
		"--host", srv.Host,
		"--port", strconv.Itoa(srv.Port),
		"--username", srv.Username,
		"--no-password",
	)

	if format == "plain_gz" {
		args = append(args, "--format=plain", dbName)
	} else {
		args = append(args, "--format=custom", "--compress=6", "--file", partPath, dbName)
	}

	cmd := exec.CommandContext(ctx, binPath, args...)
	// Run pg_dump with BELOW_NORMAL_PRIORITY_CLASS and no console window so RDP users & Postgres stay responsive.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.BELOW_NORMAL_PRIORITY_CLASS | windows.CREATE_NO_WINDOW,
	}
	env := os.Environ()
	if pwd != "" {
		env = append(env, "PGPASSWORD="+pwd)
	}
	cmd.Env = env

	if format == "plain_gz" {
		return streamCommandGzipToFile(cmd, "pg_dump", partPath)
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	// Custom format (-Fc): pg_dump writes directly to partPath; then verify PGDMP magic header & compute SHA-256 stream.
	if err := cmd.Run(); err != nil {
		return 0, "", fmt.Errorf("pg_dump failed: %v (%s)", err, strings.TrimSpace(stderrBuf.String()))
	}

	f, err := os.Open(partPath)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	header := make([]byte, 5)
	if _, err := io.ReadFull(f, header); err != nil || string(header) != "PGDMP" {
		return 0, "", errors.New("invalid PostgreSQL custom dump header (expected PGDMP)")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, "", err
	}

	hasher := sha256.New()
	buf := make([]byte, 256*1024)
	n, err := io.CopyBuffer(hasher, f, buf)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(hasher.Sum(nil)), nil
}

// UploadExecutionToCloud uploads a completed local backup execution to Google Drive or schedules exponential backoff on failure.
func (e *Engine) UploadExecutionToCloud(ctx context.Context, execID int64) error {
	e.activeJobs.Add(1)
	defer e.activeJobs.Add(-1)

	ex, err := e.store.GetExecution(execID)
	if err != nil {
		return err
	}
	if ex.LocalStatus != "completed" || ex.LocalFilePath == "" {
		return errors.New("local backup file not available for cloud upload")
	}

	var folderID string
	var prof *database.BackupProfile
	if ex.ProfileID != nil {
		prof, _ = e.store.GetProfile(*ex.ProfileID)
		if prof != nil {
			folderID = prof.DriveFolderID
		}
	}

	_ = e.store.UpdateCloudStatus(ex.ID, "uploading", "", ex.RetryCount, "", "")

	driveID, upErr := e.cloudSvc.UploadBackupFile(ctx, folderID, ex.DBName, ex.LocalFilePath, func(pct int) {
		e.dispatcher.EmitLive(notify.Event{
			Type:        notify.EventBackupProgress,
			Title:       ex.DBName,
			Message:     fmt.Sprintf("Google Drive %d%%", pct),
			Severity:    "info",
			ExecutionID: ex.ID,
			DBName:      ex.DBName,
			Percent:     pct,
		})
	})

	if upErr != nil {
		nextRetryCount := ex.RetryCount + 1
		backoff := computeBackoff(nextRetryCount)
		nextAt := time.Now().UTC().Add(backoff)
		_ = e.store.UpdateCloudStatus(ex.ID, "retrying", "", nextRetryCount, nextAt.Format(time.RFC3339), upErr.Error())
		e.dispatcher.NotifyCloudRetry(ex.DBName, nextRetryCount, nextAt.Local().Format("15:04"), upErr.Error(), ex.ID)
		return upErr
	}

	if err := e.store.UpdateCloudStatus(ex.ID, "completed", driveID, ex.RetryCount, "", ""); err != nil {
		return err
	}

	if prof != nil && prof.ID > 0 {
		e.ApplyCloudRetention(ctx, prof.ID, ex.TargetType, ex.DirectoryID, ex.DBName, prof.DriveRetentionMode, prof.DriveRetentionValue)
		e.ApplyLocalRetention(prof.ID, ex.TargetType, ex.DirectoryID, ex.DBName, prof.LocalRetentionMode, prof.LocalRetentionValue)
	}

	e.dispatcher.NotifyBackupSuccessWithWarningCtx(ctx, ex.DBName, ex.ProfileName, ex.FileSizeBytes, true, ex.WarningMessage, ex.ID)
	return nil
}

func computeBackoff(retryCount int) time.Duration {
	switch {
	case retryCount <= 1:
		return 1 * time.Minute
	case retryCount == 2:
		return 5 * time.Minute
	case retryCount == 3:
		return 15 * time.Minute
	case retryCount == 4:
		return 1 * time.Hour
	default:
		return 3 * time.Hour
	}
}

func (e *Engine) ApplyLocalRetention(profileID int64, targetType string, directoryID int64, dbName, mode string, value int) {
	copies, err := e.store.ListActiveLocalCopiesForRetention(profileID, targetType, directoryID, dbName)
	if err != nil {
		return
	}
	toDel := SelectLocalDeletions(copies, mode, value, time.Now().UTC())
	for _, item := range toDel {
		if item.LocalFilePath != "" {
			if err := os.Remove(item.LocalFilePath); err == nil || os.IsNotExist(err) {
				_ = e.store.MarkLocalDeleted(item.ID)
			}
		}
	}
}

func (e *Engine) ApplyCloudRetention(ctx context.Context, profileID int64, targetType string, directoryID int64, dbName, mode string, value int) {
	copies, err := e.store.ListActiveCloudCopiesForRetention(profileID, targetType, directoryID, dbName)
	if err != nil {
		return
	}
	toDel := SelectCloudDeletions(copies, mode, value, time.Now().UTC())
	for _, item := range toDel {
		if item.DriveFileID != "" {
			if err := e.cloudSvc.DeleteFile(ctx, item.DriveFileID); err == nil {
				_ = e.store.MarkCloudDeleted(item.ID)
			} else {
				slog.Warn("failed to prune old Drive backup", "driveFileId", item.DriveFileID, "error", err)
			}
		}
	}
}
