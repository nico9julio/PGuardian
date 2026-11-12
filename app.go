package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nico9julio/PGuardian/internal/ipc"
	"github.com/nico9julio/PGuardian/internal/notify"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type APIResponse struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
	Error  string `json:"error,omitempty"`
}

type App struct {
	ctx          context.Context
	httpCli      *http.Client
	streamCli    *http.Client
	mu           sync.Mutex
	streamCancel context.CancelFunc
}

func NewApp() *App {
	return &App{
		httpCli:   ipc.NewHTTPClient(30 * time.Second),
		streamCli: ipc.NewHTTPClient(0),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(_ context.Context) {
	a.StopEventStream()
}

// CallService proxies a JSON request from the Wails UI over the Windows Named Pipe to PGuardianService.
func (a *App) CallService(method, path, token, bodyJSON string) APIResponse {
	var bodyReader io.Reader
	if bodyJSON != "" {
		bodyReader = bytes.NewBufferString(bodyJSON)
	}
	url := "http://pipe" + path
	req, err := http.NewRequestWithContext(a.ctx, method, url, bodyReader)
	if err != nil {
		return APIResponse{Status: 0, Error: err.Error()}
	}
	if bodyJSON != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := a.httpCli.Do(req)
	if err != nil {
		return APIResponse{
			Status: 503,
			Error:  "PGuardianService is not reachable on Named Pipe (" + err.Error() + ")",
		}
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	return APIResponse{
		Status: resp.StatusCode,
		Body:   string(raw),
	}
}

// StartEventStream subscribes to live service events and emits them to the React frontend.
func (a *App) StartEventStream(token string) {
	a.StopEventStream()
	if token == "" {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.mu.Lock()
	a.streamCancel = cancel
	a.mu.Unlock()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://pipe/api/v1/events/stream", nil)
			if err != nil {
				return
			}
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := a.streamCli.Do(req)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
					continue
				}
			}

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || line == "{}" {
					continue
				}
				var ev notify.Event
				if err := json.Unmarshal([]byte(line), &ev); err == nil && ev.Type != "" {
					if ev.Type == notify.EventSystemUpdating {
						wailsRuntime.EventsEmit(a.ctx, "pguardian:event", ev)
						time.Sleep(250 * time.Millisecond)
						wailsRuntime.Quit(a.ctx)
						_ = resp.Body.Close()
						return
					}
					wailsRuntime.EventsEmit(a.ctx, "pguardian:event", ev)
				}
			}
			_ = resp.Body.Close()
			time.Sleep(2 * time.Second)
		}
	}()
}

func (a *App) StopEventStream() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.streamCancel != nil {
		a.streamCancel()
		a.streamCancel = nil
	}
}

// PickDirectory opens a native Windows directory chooser dialog.
func (a *App) PickDirectory(title, defaultDir string) (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: defaultDir,
	})
}

// PickExeFile opens a native Windows file chooser filtered to pg_dump.exe.
func (a *App) PickExeFile(title string) (string, error) {
	return wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: title,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Executable (*.exe)", Pattern: "*.exe"},
		},
	})
}

// PickUpdateZip opens a native Windows file chooser filtered to update .zip bundles.
func (a *App) PickUpdateZip(title string) (string, error) {
	return wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: title,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "PGuardian Update Bundle (*.zip)", Pattern: "*.zip"},
		},
	})
}

// OpenExternalURL opens a URL (e.g. Google OAuth login or GitHub release) in the default system browser.
func (a *App) OpenExternalURL(targetURL string) {
	wailsRuntime.BrowserOpenURL(a.ctx, targetURL)
}

// RevealInExplorer opens Windows Explorer highlighting the specified backup file or folder.
func (a *App) RevealInExplorer(targetPath string) error {
	clean := filepath.Clean(targetPath)
	cmd := exec.Command("explorer.exe", fmt.Sprintf("/select,%s", clean))
	return cmd.Start()
}
