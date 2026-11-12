package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nico9julio/PGuardian/internal/config"
)

type ReleaseCheckResult struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseName     string `json:"releaseName"`
	ReleaseNotes    string `json:"releaseNotes"`
	PublishedAt     string `json:"publishedAt"`
	AssetName       string `json:"assetName"`
	AssetURL        string `json:"assetUrl"`
	AssetSizeBytes  int64  `json:"assetSizeBytes"`
	HTMLURL         string `json:"htmlUrl"`
}

type GitHubClient struct {
	httpClient *http.Client
}

func NewGitHubClient() *GitHubClient {
	return &GitHubClient{
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          4,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 25 * time.Second,
			},
		},
	}
}

func (g *GitHubClient) CheckLatestRelease(ctx context.Context, repo, token string) (ReleaseCheckResult, error) {
	if strings.TrimSpace(repo) == "" {
		repo = config.DefaultGitHubRepo
	}
	res := ReleaseCheckResult{
		CurrentVersion: config.Version,
		LatestVersion:  config.Version,
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", strings.TrimSpace(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return res, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "PGuardian-Updater/"+config.Version)
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return res, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return res, nil // No releases published yet
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return res, fmt.Errorf("github releases API HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var ghRel struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		HTMLURL     string `json:"html_url"`
		Assets      []struct {
			Name               string `json:"name"`
			URL                string `json:"url"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&ghRel); err != nil {
		return res, err
	}

	res.LatestVersion = strings.TrimPrefix(ghRel.TagName, "v")
	res.UpdateAvailable = CompareSemVer(ghRel.TagName, config.Version) > 0
	res.ReleaseName = ghRel.Name
	res.ReleaseNotes = ghRel.Body
	res.PublishedAt = ghRel.PublishedAt
	res.HTMLURL = ghRel.HTMLURL

	for _, a := range ghRel.Assets {
		lower := strings.ToLower(a.Name)
		if strings.HasPrefix(lower, "pguardianupdate") && strings.HasSuffix(lower, ".zip") {
			res.AssetName = a.Name
			if strings.TrimSpace(token) != "" {
				res.AssetURL = a.URL // Private repo asset download endpoint
			} else {
				res.AssetURL = a.BrowserDownloadURL
			}
			res.AssetSizeBytes = a.Size
			break
		}
	}
	return res, nil
}

func (g *GitHubClient) DownloadBundle(ctx context.Context, assetURL, token, destZipPath string) error {
	if assetURL == "" {
		return errors.New("no PGuardianUpdate.zip asset found in GitHub Release")
	}
	if err := os.MkdirAll(filepath.Dir(destZipPath), 0750); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "PGuardian-Updater/"+config.Version)
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
		req.Header.Set("Accept", "application/octet-stream")
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download bundle HTTP %d", resp.StatusCode)
	}

	f, err := os.OpenFile(destZipPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	buf := make([]byte, 128*1024)
	_, copyErr := io.CopyBuffer(f, resp.Body, buf)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
