package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

type LiveDatabaseInfo struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
}

type ConnectionParams struct {
	Host     string
	Port     int
	Username string
	Password string
}

var systemSchemas = map[string]bool{
	"information_schema": true,
	"performance_schema": true,
	"sys":                true,
	"mysql":              true,
}

func buildConfig(p ConnectionParams) *mysqlDriver.Config {
	port := p.Port
	if port <= 0 {
		port = 3306
	}
	host := strings.TrimSpace(p.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	cfg := mysqlDriver.NewConfig()
	cfg.User = p.Username
	cfg.Passwd = p.Password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(host, strconv.Itoa(port))
	cfg.AllowNativePasswords = true
	cfg.Timeout = 6 * time.Second
	cfg.ReadTimeout = 15 * time.Second
	cfg.WriteTimeout = 10 * time.Second
	return cfg
}

func openConn(ctx context.Context, p ConnectionParams) (*sql.DB, string, error) {
	cfg := buildConfig(p)
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, cfg.Addr, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, cfg.Addr, err
	}
	return db, cfg.Addr, nil
}

// TestConnection verifies MariaDB/MySQL credentials and returns the server version string.
func TestConnection(ctx context.Context, p ConnectionParams) (string, error) {
	tCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	db, addr, err := openConn(tCtx, p)
	if err != nil {
		return "", fmt.Errorf("connect to %s failed: %w", addr, err)
	}
	defer func() { _ = db.Close() }()

	var version string
	if err := db.QueryRowContext(tCtx, `SELECT VERSION()`).Scan(&version); err != nil {
		return "", err
	}
	return version, nil
}

// ListDatabases queries all non-system schemas and their estimated disk sizes on the MariaDB/MySQL server.
func ListDatabases(ctx context.Context, p ConnectionParams) ([]LiveDatabaseInfo, error) {
	tCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	db, addr, err := openConn(tCtx, p)
	if err != nil {
		return nil, fmt.Errorf("connect to %s failed: %w", addr, err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.QueryContext(tCtx, `
		SELECT s.SCHEMA_NAME, COALESCE(SUM(COALESCE(t.DATA_LENGTH, 0) + COALESCE(t.INDEX_LENGTH, 0)), 0) AS size_bytes
		FROM information_schema.SCHEMATA s
		LEFT JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = s.SCHEMA_NAME
		WHERE s.SCHEMA_NAME NOT IN ('information_schema', 'performance_schema', 'sys', 'mysql')
		GROUP BY s.SCHEMA_NAME
		ORDER BY s.SCHEMA_NAME ASC`)
	if err == nil {
		defer rows.Close()
		out := []LiveDatabaseInfo{}
		for rows.Next() {
			var d LiveDatabaseInfo
			if scanErr := rows.Scan(&d.Name, &d.SizeBytes); scanErr != nil {
				return nil, scanErr
			}
			out = append(out, d)
		}
		if rowsErr := rows.Err(); rowsErr == nil {
			return out, nil
		}
	}

	// Fallback to SHOW DATABASES for restricted users without information_schema.TABLES access
	fbRows, fbErr := db.QueryContext(tCtx, `SHOW DATABASES`)
	if fbErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, fbErr
	}
	defer fbRows.Close()

	out := []LiveDatabaseInfo{}
	for fbRows.Next() {
		var name string
		if scanErr := fbRows.Scan(&name); scanErr != nil {
			return nil, scanErr
		}
		if systemSchemas[strings.ToLower(name)] {
			continue
		}
		out = append(out, LiveDatabaseInfo{Name: name, SizeBytes: 0})
	}
	if fbRows.Err() != nil {
		return nil, fbRows.Err()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
