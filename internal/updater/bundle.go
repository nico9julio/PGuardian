package updater

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxExtractedFileBytes = 250 * 1024 * 1024 // 250 MB safety cap against zip bombs

type Manifest struct {
	Version   string            `json:"version"`
	Commit    string            `json:"commit"`
	BuildDate string            `json:"buildDate"`
	Files     map[string]string `json:"files"` // filename -> sha256 hex
}

type UpdatePlan struct {
	TargetVersion       string   `json:"targetVersion"`
	PreviousVersion     string   `json:"previousVersion"`
	InstallDir          string   `json:"installDir"`
	StagingDir          string   `json:"stagingDir"`
	RollbackDir         string   `json:"rollbackDir"`
	DatabasePath        string   `json:"databasePath"`
	DatabaseSnapshot    string   `json:"databaseSnapshot"`
	RelaunchUISessionID uint32   `json:"relaunchUiSessionId"`
	Files               []string `json:"files"`
}

// CompareSemVer compares two semantic version strings (with or without leading "v").
// Returns 1 if a > b, -1 if a < b, 0 if equal.
func CompareSemVer(a, b string) int {
	pa := parseVerParts(a)
	pb := parseVerParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] > pb[i] {
			return 1
		}
		if pa[i] < pb[i] {
			return -1
		}
	}
	return 0
}

func parseVerParts(v string) [3]int {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V"))
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		out[i] = n
	}
	return out
}

// ExtractAndVerifyBundle unpacks an update .zip into stagingBase/<version> and verifies every SHA-256 digest.
func ExtractAndVerifyBundle(zipPath, stagingBase string) (*Manifest, string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, "", fmt.Errorf("open update zip: %w", err)
	}
	defer zr.Close()

	var manifest *Manifest
	for _, zf := range zr.File {
		if filepath.Base(zf.Name) == "manifest.json" {
			rc, err := zf.Open()
			if err != nil {
				return nil, "", err
			}
			var m Manifest
			decErr := json.NewDecoder(io.LimitReader(rc, 1024*1024)).Decode(&m)
			_ = rc.Close()
			if decErr != nil {
				return nil, "", fmt.Errorf("invalid manifest.json: %w", decErr)
			}
			manifest = &m
			break
		}
	}
	if manifest == nil || manifest.Version == "" || len(manifest.Files) == 0 {
		return nil, "", errors.New("update bundle missing valid manifest.json")
	}

	destDir := filepath.Join(stagingBase, "v"+strings.TrimPrefix(manifest.Version, "v"))
	_ = os.RemoveAll(destDir)
	if err := os.MkdirAll(destDir, 0750); err != nil {
		return nil, "", err
	}

	cleanDestRoot := filepath.Clean(destDir) + string(os.PathSeparator)

	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		baseName := filepath.Base(zf.Name)
		outPath := filepath.Join(destDir, baseName)
		if !strings.HasPrefix(filepath.Clean(outPath), cleanDestRoot) {
			return nil, "", errors.New("zip slip path traversal rejected")
		}

		rc, err := zf.Open()
		if err != nil {
			return nil, "", err
		}
		outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0750)
		if err != nil {
			_ = rc.Close()
			return nil, "", err
		}

		_, copyErr := io.Copy(outFile, io.LimitReader(rc, maxExtractedFileBytes))
		_ = outFile.Close()
		_ = rc.Close()
		if copyErr != nil {
			return nil, "", copyErr
		}
	}

	// Cryptographically verify every binary listed in manifest.json
	for fileName, expectedHex := range manifest.Files {
		extractedPath := filepath.Join(destDir, filepath.Base(fileName))
		actualHex, err := ComputeFileSHA256(extractedPath)
		if err != nil {
			return nil, "", fmt.Errorf("missing expected file %s in bundle: %w", fileName, err)
		}
		if !strings.EqualFold(actualHex, strings.TrimSpace(expectedHex)) {
			return nil, "", fmt.Errorf("SHA-256 mismatch for %s (expected %s, got %s)", fileName, expectedHex, actualHex)
		}
	}

	return manifest, destDir, nil
}

func ComputeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 128*1024)
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
