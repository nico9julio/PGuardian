package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nico9julio/PGuardian/internal/auth"
	"github.com/nico9julio/PGuardian/internal/backup"
	"github.com/nico9julio/PGuardian/internal/config"
	"github.com/nico9julio/PGuardian/internal/database"
	"github.com/nico9julio/PGuardian/internal/notify"
	"github.com/nico9julio/PGuardian/internal/postgres"
	"github.com/nico9julio/PGuardian/internal/scheduler"
)

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Public / Bootstrap / Tray endpoints
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/setup/complete", s.handleSetupComplete)
	mux.HandleFunc("GET /api/v1/tray/stream", s.handleTrayStream)

	// Allowed during First-Run Wizard OR when authenticated
	mux.HandleFunc("GET /api/v1/postgres/discover", s.requireSetupOrAuth(s.handleDiscoverPG))
	mux.HandleFunc("POST /api/v1/postgres/test", s.requireSetupOrAuth(s.handleTestPG))
	mux.HandleFunc("POST /api/v1/postgres/databases-preview", s.requireSetupOrAuth(s.handlePreviewDatabases))
	mux.HandleFunc("GET /api/v1/cloud/status", s.requireSetupOrAuth(s.handleCloudStatus))
	mux.HandleFunc("POST /api/v1/cloud/credentials", s.requireSetupOrAuth(s.handleCloudCredentials))
	mux.HandleFunc("POST /api/v1/cloud/oauth-start", s.requireSetupOrAuth(s.handleCloudOAuthStart))
	mux.HandleFunc("GET /api/v1/cloud/folders", s.requireSetupOrAuth(s.handleCloudListFolders))
	mux.HandleFunc("POST /api/v1/cloud/folders", s.requireSetupOrAuth(s.handleCloudCreateFolder))
	mux.HandleFunc("POST /api/v1/cloud/root-folder", s.requireSetupOrAuth(s.handleCloudSetRoot))
	mux.HandleFunc("GET /api/v1/windows-users", s.requireSetupOrAuth(s.handleListWindowsUsers))
	mux.HandleFunc("POST /api/v1/notifications/telegram-detect", s.requireSetupOrAuth(s.handleTelegramDetect))

	// Authenticated Admin endpoints
	mux.HandleFunc("GET /api/v1/events/stream", s.requireAuth(s.handleAdminEventStream))
	mux.HandleFunc("GET /api/v1/dashboard", s.requireAuth(s.handleDashboard))
	mux.HandleFunc("GET /api/v1/logs", s.requireAuth(s.handleLogs))

	mux.HandleFunc("GET /api/v1/servers", s.requireAuth(s.handleListServers))
	mux.HandleFunc("POST /api/v1/servers", s.requireAuth(s.handleSaveServer))
	mux.HandleFunc("DELETE /api/v1/servers", s.requireAuth(s.handleDeleteServer))

	mux.HandleFunc("GET /api/v1/databases", s.requireAuth(s.handleListDatabasesCatalog))
	mux.HandleFunc("GET /api/v1/executions", s.requireAuth(s.handleListExecutions))
	mux.HandleFunc("POST /api/v1/backups/run-db", s.requireAuth(s.handleRunDatabaseBackup))
	mux.HandleFunc("POST /api/v1/backups/run-profile", s.requireAuth(s.handleRunProfileBackup))
	mux.HandleFunc("POST /api/v1/backups/retry-cloud", s.requireAuth(s.handleRetryCloudUpload))

	mux.HandleFunc("GET /api/v1/profiles", s.requireAuth(s.handleListProfiles))
	mux.HandleFunc("POST /api/v1/profiles", s.requireAuth(s.handleSaveProfile))
	mux.HandleFunc("DELETE /api/v1/profiles", s.requireAuth(s.handleDeleteProfile))

	mux.HandleFunc("POST /api/v1/cloud/disconnect", s.requireAuth(s.handleCloudDisconnect))

	mux.HandleFunc("GET /api/v1/notifications/config", s.requireAuth(s.handleGetNotificationConfig))
	mux.HandleFunc("POST /api/v1/notifications/rules", s.requireAuth(s.handleSaveNotificationRules))
	mux.HandleFunc("POST /api/v1/notifications/telegram-token", s.requireAuth(s.handleSaveTelegramToken))
	mux.HandleFunc("POST /api/v1/notifications/telegram-recipient", s.requireAuth(s.handleSaveTelegramRecipient))
	mux.HandleFunc("DELETE /api/v1/notifications/telegram-recipient", s.requireAuth(s.handleDeleteTelegramRecipient))
	mux.HandleFunc("POST /api/v1/notifications/windows-user", s.requireAuth(s.handleToggleWindowsUser))
	mux.HandleFunc("POST /api/v1/notifications/test", s.requireAuth(s.handleTestNotification))

	mux.HandleFunc("GET /api/v1/settings", s.requireAuth(s.handleGetSettings))
	mux.HandleFunc("POST /api/v1/settings", s.requireAuth(s.handleSaveSettings))
	mux.HandleFunc("POST /api/v1/settings/password", s.requireAuth(s.handleChangePassword))

	mux.HandleFunc("GET /api/v1/updates/check", s.requireAuth(s.handleUpdateCheck))
	mux.HandleFunc("POST /api/v1/updates/github-config", s.requireAuth(s.handleSaveGitHubConfig))
	mux.HandleFunc("POST /api/v1/updates/apply-github", s.requireAuth(s.handleApplyGitHubUpdate))
	mux.HandleFunc("POST /api/v1/updates/apply-zip", s.requireAuth(s.handleApplyZipUpdate))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"version":        config.Version,
		"setupCompleted": s.store.IsSetupCompleted(),
		"language":       s.store.GetSetting(database.KeyLanguage, "es"),
		"theme":          s.store.GetSetting(database.KeyTheme, "system"),
		"activeJobs":     s.engine.ActiveJobs(),
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	storedHash := s.store.GetSetting(database.KeyMasterPasswordHash, "")
	if storedHash == "" {
		writeError(w, http.StatusBadRequest, "initial setup has not been completed yet")
		return
	}
	tok, err := s.authMgr.Authenticate(req.Password, storedHash)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, auth.ErrRateLimited) {
			status = http.StatusTooManyRequests
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": tok})
}

func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	if s.store.IsSetupCompleted() {
		writeError(w, http.StatusForbidden, "setup already completed")
		return
	}
	var req struct {
		Language          string                 `json:"language"`
		Theme             string                 `json:"theme"`
		MasterPassword    string                 `json:"masterPassword"`
		LocalBackupRoot   string                 `json:"localBackupRoot"`
		MinFreeDiskGB     int                    `json:"minFreeDiskGb"`
		Server            database.PGServer      `json:"server"`
		InitialProfile    database.BackupProfile `json:"initialProfile"`
		TelegramBotToken  string                 `json:"telegramBotToken"`
		TelegramChatID    string                 `json:"telegramChatId"`
		TelegramChatLabel string                 `json:"telegramChatLabel"`
		WindowsUsers      []string               `json:"windowsUsers"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := auth.HashPassword(req.MasterPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.store.SetSetting(database.KeyMasterPasswordHash, hash)
	if req.Language != "" {
		_ = s.store.SetSetting(database.KeyLanguage, req.Language)
	}
	if req.Theme != "" {
		_ = s.store.SetSetting(database.KeyTheme, req.Theme)
	}
	localRoot := strings.TrimSpace(req.LocalBackupRoot)
	if localRoot == "" {
		localRoot = config.DefaultLocalBackupsDir()
	}
	_ = s.store.SetSetting(database.KeyLocalBackupRoot, localRoot)
	if req.MinFreeDiskGB <= 0 {
		req.MinFreeDiskGB = 5
	}
	_ = s.store.SetSetting(database.KeyMinFreeDiskGB, strconv.Itoa(req.MinFreeDiskGB))

	if req.Server.Host != "" {
		req.Server.IsDefault = true
		if req.Server.Name == "" {
			req.Server.Name = "PostgreSQL Local"
		}
		srvID, sErr := s.store.SavePGServer(req.Server)
		if sErr == nil && req.InitialProfile.Name != "" {
			req.InitialProfile.PGServerID = srvID
			_, _ = s.store.SaveProfile(req.InitialProfile)
		}
	}

	if strings.TrimSpace(req.TelegramBotToken) != "" {
		_ = s.store.SetEncryptedSetting(database.KeyTelegramBotTokenEnc, strings.TrimSpace(req.TelegramBotToken))
		if strings.TrimSpace(req.TelegramChatID) != "" {
			_ = s.store.SaveTelegramRecipient(database.TelegramRecipient{
				ChatID:  strings.TrimSpace(req.TelegramChatID),
				Label:   req.TelegramChatLabel,
				Enabled: true,
			})
		}
	}

	for _, u := range req.WindowsUsers {
		s.applyWindowsUserToggle(u, true)
	}

	_ = s.store.SetSetting(database.KeySetupCompleted, "true")
	_ = s.sched.Reload(context.Background())

	tok, err := s.authMgr.IssueInitialToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": tok})
}

func (s *Server) handleTrayStream(w http.ResponseWriter, r *http.Request) {
	caller := s.callerFromReq(r)
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	subID, ch := s.dispatcher.Broadcaster().Subscribe()
	defer s.dispatcher.Broadcaster().Unsubscribe(subID)

	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			_, _ = w.Write([]byte("{}\n"))
			flusher.Flush()
		case ev, open := <-ch:
			if !open {
				return
			}
			// Always forward lifecycle signals; filter user-facing Toasts strictly to enabled RDP/local users.
			if ev.Type != notify.EventSystemUpdating && ev.Type != notify.EventShutdownTray {
				if !s.isWindowsUserEnabled(caller.Username, caller.SID) {
					continue
				}
			}
			if ev.Type == notify.EventShutdownTray && ev.TargetUser != "" && !sameWindowsUser(ev.TargetUser, caller.Username) {
				continue
			}
			raw, _ := json.Marshal(ev)
			_, _ = w.Write(append(raw, '\n'))
			flusher.Flush()
		}
	}
}

func (s *Server) handleAdminEventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	subID, ch := s.dispatcher.Broadcaster().Subscribe()
	defer s.dispatcher.Broadcaster().Unsubscribe(subID)

	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			_, _ = w.Write([]byte("{}\n"))
			flusher.Flush()
		case ev, open := <-ch:
			if !open {
				return
			}
			raw, _ := json.Marshal(ev)
			_, _ = w.Write(append(raw, '\n'))
			flusher.Flush()
		}
	}
}

func (s *Server) isWindowsUserEnabled(username, sid string) bool {
	users, err := s.store.ListWindowsNotifyUsers()
	if err != nil {
		return false
	}
	for _, u := range users {
		if !u.Enabled {
			continue
		}
		if (sid != "" && u.SID != "" && strings.EqualFold(sid, u.SID)) || sameWindowsUser(u.Username, username) {
			return true
		}
	}
	return false
}

func sameWindowsUser(a, b string) bool {
	if strings.EqualFold(a, b) {
		return true
	}
	short := func(x string) string {
		if idx := strings.LastIndex(x, `\`); idx >= 0 {
			return x[idx+1:]
		}
		return x
	}
	return short(a) != "" && strings.EqualFold(short(a), short(b))
}

// --- Dashboard & Logs ---

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	metrics, _ := s.store.GetDashboardMetrics()
	localRoot := s.store.GetSetting(database.KeyLocalBackupRoot, config.DefaultLocalBackupsDir())
	disk, _ := backup.GetDiskSpace(localRoot)
	recent, _ := s.store.ListExecutions(0, "", 15)
	drive := s.cloudSvc.Status()

	writeJSON(w, http.StatusOK, map[string]any{
		"metrics":        metrics,
		"disk":           disk,
		"recentBackups":  recent,
		"driveStatus":    drive,
		"activeJobs":     s.engine.ActiveJobs(),
		"serviceVersion": config.Version,
	})
}

func (s *Server) handleLogs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, config.GlobalLogBuffer.Recent())
}

// --- PostgreSQL Discovery, Servers & Databases ---

func (s *Server) handleDiscoverPG(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, postgres.DiscoverPGDumpBinaries())
}

func (s *Server) handleTestPG(w http.ResponseWriter, r *http.Request) {
	var req database.PGServer
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pwd := req.Password
	if pwd == "" && req.ID > 0 {
		_, storedPwd, err := s.store.GetPGServerWithSecret(req.ID)
		if err == nil {
			pwd = storedPwd
		}
	}
	ver, err := postgres.TestConnection(r.Context(), postgres.ConnectionParams{
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: pwd,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"version": ver})
}

func (s *Server) handlePreviewDatabases(w http.ResponseWriter, r *http.Request) {
	var req database.PGServer
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pwd := req.Password
	if pwd == "" && req.ID > 0 {
		_, storedPwd, err := s.store.GetPGServerWithSecret(req.ID)
		if err == nil {
			pwd = storedPwd
		}
	}
	dbs, err := postgres.ListDatabases(r.Context(), postgres.ConnectionParams{
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: pwd,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dbs)
}

func (s *Server) handleListServers(w http.ResponseWriter, _ *http.Request) {
	srvs, err := s.store.ListPGServers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if srvs == nil {
		srvs = []database.PGServer{}
	}
	writeJSON(w, http.StatusOK, srvs)
}

func (s *Server) handleSaveServer(w http.ResponseWriter, r *http.Request) {
	var req database.PGServer
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := s.store.SavePGServer(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": id})
}

func (s *Server) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err := s.store.DeletePGServer(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.sched.Reload(context.Background())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleListDatabasesCatalog(w http.ResponseWriter, r *http.Request) {
	serverIDFilter, _ := strconv.ParseInt(r.URL.Query().Get("serverId"), 10, 64)
	servers, err := s.store.ListPGServers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	profiles, _ := s.store.ListProfiles()

	var summaries []database.DatabaseBackupSummary
	for _, srv := range servers {
		if serverIDFilter > 0 && srv.ID != serverIDFilter {
			continue
		}
		_, pwd, err := s.store.GetPGServerWithSecret(srv.ID)
		if err != nil {
			continue
		}
		liveDBs, err := postgres.ListDatabases(r.Context(), postgres.ConnectionParams{
			Host:     srv.Host,
			Port:     srv.Port,
			Username: srv.Username,
			Password: pwd,
		})
		if err != nil {
			continue
		}
		statsMap, _ := s.store.GetDBBackupStatsMap(srv.ID)

		for _, dbInfo := range liveDBs {
			var assigned []string
			for _, p := range profiles {
				if p.PGServerID != srv.ID || !p.Enabled {
					continue
				}
				if p.SelectionMode == "all_auto" {
					excluded := false
					for _, ex := range p.ExcludedDatabases {
						if ex == dbInfo.Name {
							excluded = true
							break
						}
					}
					if !excluded {
						assigned = append(assigned, p.Name)
					}
				} else {
					for _, inc := range p.IncludedDatabases {
						if inc == dbInfo.Name {
							assigned = append(assigned, p.Name)
							break
						}
					}
				}
			}
			if assigned == nil {
				assigned = []string{}
			}
			st := statsMap[dbInfo.Name]
			summaries = append(summaries, database.DatabaseBackupSummary{
				ServerID:         srv.ID,
				ServerName:       srv.Name,
				DBName:           dbInfo.Name,
				LiveSizeBytes:    dbInfo.SizeBytes,
				AssignedProfiles: assigned,
				TotalCopies:      st.TotalCopies,
				LocalCopies:      st.LocalCopies,
				CloudCopies:      st.CloudCopies,
				LastBackupAt:     st.LastBackupAt,
				LastLocalStatus:  st.LastLocalStatus,
				LastCloudStatus:  st.LastCloudStatus,
			})
		}
	}
	if summaries == nil {
		summaries = []database.DatabaseBackupSummary{}
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	srvID, _ := strconv.ParseInt(r.URL.Query().Get("serverId"), 10, 64)
	dbName := r.URL.Query().Get("dbName")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.store.ListExecutions(srvID, dbName, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleRunDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServerID  int64  `json:"serverId"`
		DBName    string `json:"dbName"`
		ProfileID int64  `json:"profileId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	config.SafeGo("manual-db-backup", func() {
		_, _ = s.engine.RunManualDatabase(context.Background(), req.ServerID, req.DBName, req.ProfileID)
	})
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

func (s *Server) handleRunProfileBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileID int64 `json:"profileId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	config.SafeGo("manual-profile-backup", func() {
		_ = s.engine.RunProfile(context.Background(), req.ProfileID, "manual")
	})
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

func (s *Server) handleRetryCloudUpload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExecutionID int64 `json:"executionId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	config.SafeGo("manual-cloud-retry", func() {
		_ = s.engine.UploadExecutionToCloud(context.Background(), req.ExecutionID)
	})
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

// --- Profiles ---

func (s *Server) handleListProfiles(w http.ResponseWriter, _ *http.Request) {
	profs, err := s.store.ListProfiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if profs == nil {
		profs = []database.BackupProfile{}
	}
	writeJSON(w, http.StatusOK, profs)
}

func (s *Server) handleSaveProfile(w http.ResponseWriter, r *http.Request) {
	var req database.BackupProfile
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := scheduler.ValidateCron(req.ScheduleCron); err != nil {
		writeError(w, http.StatusBadRequest, "invalid schedule cron expression: "+err.Error())
		return
	}
	id, err := s.store.SaveProfile(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.sched.Reload(context.Background())
	writeJSON(w, http.StatusOK, map[string]int64{"id": id})
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err := s.store.DeleteProfile(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.sched.Reload(context.Background())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
