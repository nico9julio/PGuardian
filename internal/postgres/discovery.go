package postgres

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type DetectedPGDump struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

// DiscoverPGDumpBinaries scans standard Windows PostgreSQL installation directories and PATH.
func DiscoverPGDumpBinaries() []DetectedPGDump {
	seen := make(map[string]bool)
	var candidates []string

	patterns := []string{
		`C:\Program Files\PostgreSQL\*\bin\pg_dump.exe`,
		`C:\Program Files (x86)\PostgreSQL\*\bin\pg_dump.exe`,
	}
	for _, pat := range patterns {
		matches, _ := filepath.Glob(pat)
		candidates = append(candidates, matches...)
	}
	if p, err := exec.LookPath("pg_dump"); err == nil {
		candidates = append(candidates, p)
	}

	var results []DetectedPGDump
	for _, c := range candidates {
		clean := filepath.Clean(c)
		lower := strings.ToLower(clean)
		if seen[lower] {
			continue
		}
		info, err := os.Stat(clean)
		if err != nil || info.IsDir() {
			continue
		}
		seen[lower] = true
		ver := readPGDumpVersion(clean)
		results = append(results, DetectedPGDump{
			Path:    clean,
			Version: ver,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Path > results[j].Path
	})
	return results
}

func readPGDumpVersion(binPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "--version")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
