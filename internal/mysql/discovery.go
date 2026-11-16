package mysql

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

type DetectedMySQLDump struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

// DiscoverMySQLDumpBinaries scans standard Windows MariaDB/MySQL installation directories and PATH.
func DiscoverMySQLDumpBinaries() []DetectedMySQLDump {
	seen := make(map[string]bool)
	var candidates []string

	patterns := []string{
		`C:\Program Files\MariaDB *\bin\mariadb-dump.exe`,
		`C:\Program Files\MariaDB *\bin\mysqldump.exe`,
		`C:\Program Files (x86)\MariaDB *\bin\mariadb-dump.exe`,
		`C:\Program Files (x86)\MariaDB *\bin\mysqldump.exe`,
		`C:\Program Files\MySQL\*\bin\mysqldump.exe`,
		`C:\Program Files (x86)\MySQL\*\bin\mysqldump.exe`,
		`C:\xampp\mysql\bin\mysqldump.exe`,
		`C:\laragon\bin\mysql\*\bin\mysqldump.exe`,
		`C:\laragon\bin\mariadb\*\bin\mariadb-dump.exe`,
		`C:\laragon\bin\mariadb\*\bin\mysqldump.exe`,
		`C:\wamp64\bin\mysql\*\bin\mysqldump.exe`,
		`C:\wamp64\bin\mariadb\*\bin\mariadb-dump.exe`,
		`C:\wamp64\bin\mariadb\*\bin\mysqldump.exe`,
	}
	for _, pat := range patterns {
		matches, _ := filepath.Glob(pat)
		candidates = append(candidates, matches...)
	}
	for _, exeName := range []string{"mariadb-dump", "mysqldump"} {
		if p, err := exec.LookPath(exeName); err == nil {
			candidates = append(candidates, p)
		}
	}

	var results []DetectedMySQLDump
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
		ver := ReadDumpVersion(clean)
		results = append(results, DetectedMySQLDump{
			Path:    clean,
			Version: ver,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		iMaria := strings.EqualFold(filepath.Base(results[i].Path), "mariadb-dump.exe")
		jMaria := strings.EqualFold(filepath.Base(results[j].Path), "mariadb-dump.exe")
		if iMaria != jMaria {
			return iMaria
		}
		return results[i].Path > results[j].Path
	})
	return results
}

// ReadDumpVersion runs `<binPath> --version` hidden and returns the version string.
func ReadDumpVersion(binPath string) string {
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

func escapeMySQLOptionValue(val string) string {
	var b strings.Builder
	b.Grow(len(val) + 4)
	for _, r := range val {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// FormatDefaultsExtraFile formats a temporary [client] options file for --defaults-extra-file
// so passwords with special characters are passed safely without appearing on the command line.
func FormatDefaultsExtraFile(user, password string) string {
	var b strings.Builder
	b.WriteString("[client]\n")
	b.WriteString(`user="` + escapeMySQLOptionValue(user) + "\"\n")
	b.WriteString(`password="` + escapeMySQLOptionValue(password) + "\"\n")
	return b.String()
}

// NeedsColumnStatisticsZero returns true for Oracle MySQL 8.x/9.x mysqldump clients,
// which otherwise fail with Unknown table 'COLUMN_STATISTICS' when dumping MariaDB or MySQL 5.7.
func NeedsColumnStatisticsZero(binVersion string) bool {
	lower := strings.ToLower(strings.TrimSpace(binVersion))
	if lower == "" || lower == "unknown" || strings.Contains(lower, "mariadb") {
		return false
	}
	return strings.Contains(lower, " 8.") || strings.Contains(lower, " 9.")
}
