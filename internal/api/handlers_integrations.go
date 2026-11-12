package api

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nico9julio/PGuardian/internal/auth"
	"github.com/nico9julio/PGuardian/internal/config"
	"github.com/nico9julio/PGuardian/internal/database"
	"github.com/nico9julio/PGuardian/internal/notify"
	"github.com/nico9julio/PGuardian/internal/winintegration"
)

// --- Google Drive Handlers ---

func (s *Server) handleCloudStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.cloudSvc.Status())
}

func (s *Server) handleCloudCredentials(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.cloudSvc.SaveCredentials(req.ClientID, req.ClientSecret); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.cloudSvc.Status())
}

func (s *Server) handleCloudOAuthStart(w http.ResponseWriter, _ *http.Request) {
	authURL, err := s.cloudSvc.StartOAuthFlow(func(email string) {
		s.dispatcher.EmitLive(notify.Event{
			Type:     notify.EventCloudCompleted,
			Title:    "Google Drive",
			Message:  email,
			Severity: "success",
		})
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authUrl": authURL})
}

func (s *Server) handleCloudDisconnect(w http.ResponseWriter, _ *http.Request) {
	_ = s.cloudSvc.Disconnect()
	writeJSON(w, http.StatusOK, s.cloudSvc.Status())
}

func (s *Server) handleCloudListFolders(w http.ResponseWriter, r *http.Request) {
	parentID := r.URL.Query().Get("parentId")
	folders, err := s.cloudSvc.ListFolders(r.Context(), parentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, folders)
}

func (s *Server) handleCloudCreateFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ParentID string `json:"parentId"`
		Name     string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	folder, err := s.cloudSvc.CreateFolder(r.Context(), req.ParentID, req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, folder)
}

func (s *Server) handleCloudSetRoot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FolderID   string `json:"folderId"`
		FolderName string `json:"folderName"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.store.SetSetting(database.KeyDriveRootFolderID, req.FolderID)
	_ = s.store.SetSetting(database.KeyDriveRootFolderName, req.FolderName)
	writeJSON(w, http.StatusOK, s.cloudSvc.Status())
}

// --- Notifications & RDP Users ---

func (s *Server) handleListWindowsUsers(w http.ResponseWriter, _ *http.Request) {
	stored, _ := s.store.ListWindowsNotifyUsers()
	enabledMap := make(map[string]bool)
	for _, u := range stored {
		if u.Enabled {
			enabledMap[strings.ToLower(u.Username)] = true
		}
	}
	users, err := winintegration.ListAllServerUsers(enabledMap)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) handleGetNotificationConfig(w http.ResponseWriter, _ *http.Request) {
	rules, _ := s.store.GetNotificationRules()
	recipients, _ := s.store.ListTelegramRecipients()
	hasToken := s.store.GetSetting(database.KeyTelegramBotTokenEnc, "") != ""
	storedUsers, _ := s.store.ListWindowsNotifyUsers()
	enabledMap := make(map[string]bool)
	for _, u := range storedUsers {
		if u.Enabled {
			enabledMap[strings.ToLower(u.Username)] = true
		}
	}
	winUsers, _ := winintegration.ListAllServerUsers(enabledMap)

	writeJSON(w, http.StatusOK, map[string]any{
		"rules":              rules,
		"hasTelegramToken":   hasToken,
		"telegramRecipients": recipients,
		"windowsUsers":       winUsers,
	})
}

func (s *Server) handleSaveNotificationRules(w http.ResponseWriter, r *http.Request) {
	var rules []database.NotificationRule
	if err := decodeJSON(r, &rules); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, rule := range rules {
		if err := s.store.SaveNotificationRule(rule); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSaveTelegramToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BotToken string `json:"botToken"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetEncryptedSetting(database.KeyTelegramBotTokenEnc, strings.TrimSpace(req.BotToken)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleTelegramDetect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BotToken string `json:"botToken"`
	}
	_ = decodeJSON(r, &req)
	tok := strings.TrimSpace(req.BotToken)
	if tok == "" {
		tok, _ = s.store.GetEncryptedSetting(database.KeyTelegramBotTokenEnc)
	}
	chats, err := s.dispatcher.Telegram().DetectChats(r.Context(), tok)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if chats == nil {
		chats = []notify.DetectedTelegramChat{}
	}
	writeJSON(w, http.StatusOK, chats)
}

func (s *Server) handleSaveTelegramRecipient(w http.ResponseWriter, r *http.Request) {
	var req database.TelegramRecipient
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SaveTelegramRecipient(req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteTelegramRecipient(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	_ = s.store.DeleteTelegramRecipient(id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleToggleWindowsUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Enabled  bool   `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.applyWindowsUserToggle(req.Username, req.Enabled)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) applyWindowsUserToggle(username string, enabled bool) {
	sid := winintegration.LookupSID(username)
	_ = s.store.SetWindowsNotifyUser(username, sid, enabled)

	trayExe := filepath.Join(config.InstallDir(), "PGuardianTray.exe")
	if enabled {
		_ = winintegration.RegisterUserTrayAutostart(username, trayExe)
		// If user currently has an active RDP/Console session, launch PGuardianTray.exe live!
		sessions, _ := winintegration.ListActiveSessions()
		for _, sess := range sessions {
			if sameWindowsUser(sess.Username, username) && sess.ConnectionStat == "Active" {
				_ = winintegration.LaunchProcessInSession(sess.SessionID, trayExe, "")
			}
		}
	} else {
		_ = winintegration.RemoveUserTrayAutostart(username)
		s.dispatcher.EmitLive(notify.Event{
			Type:       notify.EventShutdownTray,
			TargetUser: username,
		})
	}
}

func (s *Server) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.dispatcher.SendTestAlert(req.Channel); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- General Settings & Master Password ---

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	minDisk, _ := strconv.Atoi(s.store.GetSetting(database.KeyMinFreeDiskGB, "5"))
	writeJSON(w, http.StatusOK, map[string]any{
		"language":        s.store.GetSetting(database.KeyLanguage, "es"),
		"theme":           s.store.GetSetting(database.KeyTheme, "system"),
		"localBackupRoot": s.store.GetSetting(database.KeyLocalBackupRoot, config.DefaultLocalBackupsDir()),
		"minFreeDiskGb":   minDisk,
		"githubRepo":      s.store.GetSetting(database.KeyGitHubRepo, config.DefaultGitHubRepo),
		"hasGithubToken":  s.store.GetSetting(database.KeyGitHubTokenEnc, "") != "",
		"version":         config.Version,
	})
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Language        string `json:"language"`
		Theme           string `json:"theme"`
		LocalBackupRoot string `json:"localBackupRoot"`
		MinFreeDiskGB   int    `json:"minFreeDiskGb"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Language != "" {
		_ = s.store.SetSetting(database.KeyLanguage, req.Language)
	}
	if req.Theme != "" {
		_ = s.store.SetSetting(database.KeyTheme, req.Theme)
	}
	if strings.TrimSpace(req.LocalBackupRoot) != "" {
		_ = s.store.SetSetting(database.KeyLocalBackupRoot, strings.TrimSpace(req.LocalBackupRoot))
	}
	if req.MinFreeDiskGB > 0 {
		_ = s.store.SetSetting(database.KeyMinFreeDiskGB, strconv.Itoa(req.MinFreeDiskGB))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	storedHash := s.store.GetSetting(database.KeyMasterPasswordHash, "")
	ok, err := auth.VerifyPassword(req.CurrentPassword, storedHash)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, auth.ErrInvalidCredentials.Error())
		return
	}
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = s.store.SetSetting(database.KeyMasterPasswordHash, newHash)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- Atomic Self-Update Endpoints ---

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	res, err := s.upd.CheckLatest(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleSaveGitHubConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repo  string `json:"repo"`
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Repo) != "" {
		_ = s.store.SetSetting(database.KeyGitHubRepo, strings.TrimSpace(req.Repo))
	}
	if req.Token == "__CLEAR__" {
		_ = s.store.SetSetting(database.KeyGitHubTokenEnc, "")
	} else if strings.TrimSpace(req.Token) != "" {
		_ = s.store.SetEncryptedSetting(database.KeyGitHubTokenEnc, strings.TrimSpace(req.Token))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleApplyGitHubUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Force bool `json:"force"`
	}
	_ = decodeJSON(r, &req)
	caller := s.callerFromReq(r)
	ver, err := s.upd.ApplyFromGitHub(r.Context(), caller.SessionID, req.Force)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"updatingTo": ver})
}

func (s *Server) handleApplyZipUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ZipPath string `json:"zipPath"`
		Force   bool   `json:"force"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	caller := s.callerFromReq(r)
	ver, err := s.upd.ApplyFromLocalZip(r.Context(), req.ZipPath, caller.SessionID, req.Force)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"updatingTo": ver})
}
