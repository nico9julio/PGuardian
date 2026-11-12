package cloud

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nico9julio/PGuardian/internal/config"
	"github.com/nico9julio/PGuardian/internal/database"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const chunkSize = 8 * 1024 * 1024 // 8 MB resumable upload chunk (multiple of 256 KB required by Google Drive)

type DriveFolder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DriveStatus struct {
	Configured   bool   `json:"configured"`
	Connected    bool   `json:"connected"`
	ClientID     string `json:"clientId"`
	HasSecret    bool   `json:"hasSecret"`
	AccountEmail string `json:"accountEmail"`
	RootFolderID string `json:"rootFolderId"`
	RootFolder   string `json:"rootFolderName"`
}

type Service struct {
	store       *database.Store
	mu          sync.Mutex
	folderCache map[string]string // key: parentID + "/" + folderName -> folderID
	authCancel  context.CancelFunc
	baseTrans   *http.Transport
}

func NewService(store *database.Store) *Service {
	return &Service{
		store:       store,
		folderCache: make(map[string]string),
		baseTrans: &http.Transport{
			MaxIdleConns:          6,
			IdleConnTimeout:       45 * time.Second,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 45 * time.Second,
		},
	}
}

func (s *Service) Status() DriveStatus {
	clientID := s.store.GetSetting(database.KeyDriveClientID, "")
	secEnc := s.store.GetSetting(database.KeyDriveClientSecretEnc, "")
	tokEnc := s.store.GetSetting(database.KeyDriveTokenJSONEnc, "")
	return DriveStatus{
		Configured:   clientID != "" && secEnc != "",
		Connected:    tokEnc != "",
		ClientID:     clientID,
		HasSecret:    secEnc != "",
		AccountEmail: s.store.GetSetting(database.KeyDriveAccountEmail, ""),
		RootFolderID: s.store.GetSetting(database.KeyDriveRootFolderID, ""),
		RootFolder:   s.store.GetSetting(database.KeyDriveRootFolderName, ""),
	}
}

func (s *Service) SaveCredentials(clientID, clientSecret string) error {
	if strings.TrimSpace(clientID) == "" {
		return errors.New("Google OAuth Client ID is required")
	}
	if err := s.store.SetSetting(database.KeyDriveClientID, strings.TrimSpace(clientID)); err != nil {
		return err
	}
	if strings.TrimSpace(clientSecret) != "" {
		if err := s.store.SetEncryptedSetting(database.KeyDriveClientSecretEnc, strings.TrimSpace(clientSecret)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Disconnect() error {
	s.mu.Lock()
	s.folderCache = make(map[string]string)
	s.mu.Unlock()
	_ = s.store.SetSetting(database.KeyDriveTokenJSONEnc, "")
	_ = s.store.SetSetting(database.KeyDriveAccountEmail, "")
	return nil
}

// StartOAuthFlow launches an ephemeral loopback listener with PKCE and returns the browser authorization URL.
func (s *Service) StartOAuthFlow(onSuccess func(email string)) (string, error) {
	clientID := s.store.GetSetting(database.KeyDriveClientID, "")
	clientSecret, err := s.store.GetEncryptedSetting(database.KeyDriveClientSecretEnc)
	if err != nil || clientID == "" || clientSecret == "" {
		return "", errors.New("configure Google OAuth Client ID and Client Secret first")
	}

	s.mu.Lock()
	if s.authCancel != nil {
		s.authCancel()
		s.authCancel = nil
	}
	s.mu.Unlock()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("open loopback listener: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/oauth2/callback", port)

	oauthCfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirectURI,
		Scopes:       []string{"https://www.googleapis.com/auth/drive"},
	}

	verifierBytes := make([]byte, 32)
	_, _ = rand.Read(verifierBytes)
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	sum := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(sum[:])

	stateBytes := make([]byte, 16)
	_, _ = rand.Read(stateBytes)
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	authURL := oauthCfg.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	s.mu.Lock()
	s.authCancel = cancel
	s.mu.Unlock()

	mux := http.NewServeMux()
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	mux.HandleFunc("/oauth2/callback", func(w http.ResponseWriter, r *http.Request) {
		defer cancel()
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Authorization cancelled or missing code", http.StatusBadRequest)
			return
		}

		exCtx, exCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer exCancel()
		exCtx = context.WithValue(exCtx, oauth2.HTTPClient, &http.Client{Timeout: 20 * time.Second, Transport: s.baseTrans})

		tok, err := oauthCfg.Exchange(exCtx, code, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
		if err != nil {
			slog.Error("google oauth token exchange failed", "error", err)
			http.Error(w, "Token exchange failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		rawTok, err := json.Marshal(tok)
		if err != nil {
			http.Error(w, "Token serialization failed", http.StatusInternalServerError)
			return
		}
		if err := s.store.SetEncryptedSetting(database.KeyDriveTokenJSONEnc, string(rawTok)); err != nil {
			http.Error(w, "Failed to persist encrypted token", http.StatusInternalServerError)
			return
		}

		email, _ := s.fetchAccountEmail(exCtx)
		if email != "" {
			_ = s.store.SetSetting(database.KeyDriveAccountEmail, email)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>PGuardian</title>
<style>body{background:#0b1120;color:#f8fafc;font-family:system-ui,sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.card{background:#111827;border:1px solid #1e293b;border-radius:16px;padding:36px;max-width:440px;text-align:center;box-shadow:0 20px 50px rgba(0,0,0,.5)}
h2{color:#06b6d4;margin-top:0}</style></head>
<body><div class="card"><h2>✓ Google Drive Conectado</h2><p>Cuenta vinculada: <b>%s</b></p><p>Ya puedes cerrar esta pestaña y volver a <b>PGuardian</b>.</p></div></body></html>`, email)

		if onSuccess != nil {
			onSuccess(email)
		}
	})

	config.SafeGo("oauth-loopback", func() {
		go func() {
			<-ctx.Done()
			_ = srv.Close()
			_ = ln.Close()
		}()
		_ = srv.Serve(ln)
	})

	return authURL, nil
}

func (s *Service) authorizedClient(ctx context.Context) (*http.Client, error) {
	clientID := s.store.GetSetting(database.KeyDriveClientID, "")
	clientSecret, err := s.store.GetEncryptedSetting(database.KeyDriveClientSecretEnc)
	if err != nil || clientID == "" || clientSecret == "" {
		return nil, errors.New("google drive credentials not configured")
	}
	tokJSON, err := s.store.GetEncryptedSetting(database.KeyDriveTokenJSONEnc)
	if err != nil || tokJSON == "" {
		return nil, errors.New("google drive account not connected")
	}
	var tok oauth2.Token
	if err := json.Unmarshal([]byte(tokJSON), &tok); err != nil {
		return nil, err
	}

	oauthCfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{"https://www.googleapis.com/auth/drive"},
	}

	baseCtx := context.WithValue(ctx, oauth2.HTTPClient, &http.Client{
		Timeout:   60 * time.Second,
		Transport: s.baseTrans,
	})
	ts := oauthCfg.TokenSource(baseCtx, &tok)
	freshTok, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("refresh google drive token: %w", err)
	}
	if freshTok.AccessToken != tok.AccessToken {
		if raw, mErr := json.Marshal(freshTok); mErr == nil {
			_ = s.store.SetEncryptedSetting(database.KeyDriveTokenJSONEnc, string(raw))
		}
	}

	return &http.Client{
		Transport: &oauth2.Transport{
			Source: ts,
			Base:   s.baseTrans,
		},
	}, nil
}

func (s *Service) fetchAccountEmail(ctx context.Context) (string, error) {
	cli, err := s.authorizedClient(ctx)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/drive/v3/about?fields=user(emailAddress)", nil)
	if err != nil {
		return "", err
	}
	resp, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	var out struct {
		User struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.User.EmailAddress, nil
}

// ListFolders lists child folders inside parentID ("root" if empty) for the visual folder picker.
func (s *Service) ListFolders(ctx context.Context, parentID string) ([]DriveFolder, error) {
	if parentID == "" {
		parentID = "root"
	}
	cli, err := s.authorizedClient(ctx)
	if err != nil {
		return nil, err
	}

	escapedParent := strings.ReplaceAll(parentID, "'", "\\'")
	q := fmt.Sprintf("'%s' in parents and mimeType = 'application/vnd.google-apps.folder' and trashed = false", escapedParent)
	u := "https://www.googleapis.com/drive/v3/files?pageSize=200&orderBy=name&fields=files(id,name)&q=" + url.QueryEscape(q)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("drive list folders HTTP %d: %s", resp.StatusCode, string(b))
	}

	var parsed struct {
		Files []DriveFolder `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Files == nil {
		return []DriveFolder{}, nil
	}
	return parsed.Files, nil
}

// CreateFolder creates a new folder under parentID (or returns existing if already present).
func (s *Service) CreateFolder(ctx context.Context, parentID, name string) (DriveFolder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return DriveFolder{}, errors.New("folder name cannot be empty")
	}
	if parentID == "" {
		parentID = "root"
	}

	cacheKey := parentID + "/" + name
	s.mu.Lock()
	if id, ok := s.folderCache[cacheKey]; ok {
		s.mu.Unlock()
		return DriveFolder{ID: id, Name: name}, nil
	}
	s.mu.Unlock()

	existing, err := s.ListFolders(ctx, parentID)
	if err == nil {
		for _, f := range existing {
			if strings.EqualFold(f.Name, name) {
				s.mu.Lock()
				if len(s.folderCache) > 500 {
					s.folderCache = make(map[string]string)
				}
				s.folderCache[cacheKey] = f.ID
				s.mu.Unlock()
				return f, nil
			}
		}
	}

	cli, err := s.authorizedClient(ctx)
	if err != nil {
		return DriveFolder{}, err
	}

	meta := map[string]any{
		"name":     name,
		"mimeType": "application/vnd.google-apps.folder",
		"parents":  []string{parentID},
	}
	body, _ := json.Marshal(meta)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.googleapis.com/drive/v3/files?fields=id,name", bytes.NewReader(body))
	if err != nil {
		return DriveFolder{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return DriveFolder{}, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return DriveFolder{}, fmt.Errorf("drive create folder HTTP %d: %s", resp.StatusCode, string(b))
	}

	var created DriveFolder
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return DriveFolder{}, err
	}
	s.mu.Lock()
	s.folderCache[cacheKey] = created.ID
	s.mu.Unlock()
	return created, nil
}

// UploadBackupFile uploads a local backup file into <baseFolderID>/<dbName>/<filename> using 8 MB resumable chunks.
func (s *Service) UploadBackupFile(ctx context.Context, baseFolderID, dbName, localPath string, onProgress func(percent int)) (string, error) {
	if baseFolderID == "" {
		baseFolderID = s.store.GetSetting(database.KeyDriveRootFolderID, "root")
	}
	if baseFolderID == "" {
		baseFolderID = "root"
	}

	dbFolder, err := s.CreateFolder(ctx, baseFolderID, dbName)
	if err != nil {
		return "", fmt.Errorf("ensure database folder in Drive: %w", err)
	}

	f, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", err
	}
	totalSize := stat.Size()
	fileName := filepath.Base(localPath)

	cli, err := s.authorizedClient(ctx)
	if err != nil {
		return "", err
	}

	// Step 1: Initiate resumable upload session
	meta := map[string]any{
		"name":    fileName,
		"parents": []string{dbFolder.ID},
	}
	metaBytes, _ := json.Marshal(meta)
	initReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.googleapis.com/upload/drive/v3/files?uploadType=resumable&fields=id", bytes.NewReader(metaBytes))
	if err != nil {
		return "", err
	}
	initReq.Header.Set("Content-Type", "application/json; charset=UTF-8")
	initReq.Header.Set("X-Upload-Content-Length", fmt.Sprintf("%d", totalSize))
	initReq.Header.Set("X-Upload-Content-Type", "application/octet-stream")

	initResp, err := cli.Do(initReq)
	if err != nil {
		return "", fmt.Errorf("initiate resumable upload: %w", err)
	}
	_, _ = io.Copy(io.Discard, initResp.Body)
	_ = initResp.Body.Close()

	if initResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("drive resumable init returned HTTP %d", initResp.StatusCode)
	}
	sessionURI := initResp.Header.Get("Location")
	if sessionURI == "" {
		return "", errors.New("missing resumable session URI from Google Drive")
	}

	// Step 2: Stream fixed 8 MB chunks (constant memory usage even for 100+ GB files)
	buf := make([]byte, chunkSize)
	var offset int64
	for offset < totalSize || totalSize == 0 {
		n, readErr := io.ReadFull(f, buf)
		if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
			return "", readErr
		}
		chunk := buf[:n]
		endByte := offset + int64(n) - 1

		putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionURI, bytes.NewReader(chunk))
		if err != nil {
			return "", err
		}
		putReq.ContentLength = int64(n)
		if totalSize > 0 {
			putReq.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, endByte, totalSize))
		} else {
			putReq.Header.Set("Content-Range", "bytes */0")
		}

		putResp, err := cli.Do(putReq)
		if err != nil {
			return "", fmt.Errorf("upload chunk at %d: %w", offset, err)
		}
		respBytes, _ := io.ReadAll(io.LimitReader(putResp.Body, 16*1024))
		_ = putResp.Body.Close()

		if putResp.StatusCode == http.StatusOK || putResp.StatusCode == http.StatusCreated {
			var uploaded struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(respBytes, &uploaded)
			if onProgress != nil {
				onProgress(100)
			}
			return uploaded.ID, nil
		}
		if putResp.StatusCode != 308 { // 308 Resume Incomplete
			return "", fmt.Errorf("drive chunk upload HTTP %d: %s", putResp.StatusCode, string(respBytes))
		}

		offset += int64(n)
		if onProgress != nil && totalSize > 0 {
			pct := int((offset * 100) / totalSize)
			onProgress(pct)
		}
		if totalSize == 0 {
			break
		}
	}

	return "", errors.New("upload ended without file ID confirmation")
}

// DeleteFile deletes an old backup file from Google Drive during retention cleanup.
func (s *Service) DeleteFile(ctx context.Context, fileID string) error {
	if strings.TrimSpace(fileID) == "" {
		return nil
	}
	cli, err := s.authorizedClient(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "https://www.googleapis.com/drive/v3/files/"+url.PathEscape(fileID), nil)
	if err != nil {
		return err
	}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	return fmt.Errorf("drive delete HTTP %d", resp.StatusCode)
}
