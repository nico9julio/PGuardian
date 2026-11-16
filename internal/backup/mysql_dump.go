package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/nico9julio/PGuardian/internal/config"
	"github.com/nico9julio/PGuardian/internal/database"
	"github.com/nico9julio/PGuardian/internal/mysql"
	"golang.org/x/sys/windows"
)

func runStreamingMySQLDump(ctx context.Context, binPath string, srv *database.PGServer, pwd, dbName, partPath string) (int64, string, error) {
	trimmedDB := strings.TrimSpace(dbName)
	if trimmedDB == "" || strings.HasPrefix(trimmedDB, "-") {
		return 0, "", errors.New("invalid MySQL/MariaDB database name")
	}

	if err := os.MkdirAll(config.TempDir(), 0750); err != nil {
		return 0, "", fmt.Errorf("create temp dir: %w", err)
	}
	cnfFile, err := os.CreateTemp(config.TempDir(), "mysql_*.cnf")
	if err != nil {
		return 0, "", fmt.Errorf("create temp cnf: %w", err)
	}
	cnfPath := cnfFile.Name()
	defer func() { _ = os.Remove(cnfPath) }()

	_ = cnfFile.Chmod(0600)
	if _, err := cnfFile.WriteString(mysql.FormatDefaultsExtraFile(srv.Username, pwd)); err != nil {
		_ = cnfFile.Close()
		return 0, "", err
	}
	if err := cnfFile.Close(); err != nil {
		return 0, "", err
	}

	host := strings.TrimSpace(srv.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	port := srv.Port
	if port <= 0 {
		port = 3306
	}

	args := []string{
		"--defaults-extra-file=" + cnfPath,
		"--protocol=TCP",
		"--host=" + host,
		"--port=" + strconv.Itoa(port),
		"--single-transaction",
		"--quick",
		"--routines",
		"--triggers",
		"--events",
		"--hex-blob",
		"--default-character-set=utf8mb4",
	}
	if mysql.NeedsColumnStatisticsZero(mysql.ReadDumpVersion(binPath)) {
		args = append(args, "--column-statistics=0")
	}
	args = append(args, trimmedDB)

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.BELOW_NORMAL_PRIORITY_CLASS | windows.CREATE_NO_WINDOW,
	}

	return streamCommandGzipToFile(cmd, "mysqldump", partPath)
}
