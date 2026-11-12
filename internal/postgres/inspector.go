package postgres

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
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

func buildConnConfig(p ConnectionParams) (*pgx.ConnConfig, error) {
	port := p.Port
	if port <= 0 {
		port = 5432
	}
	host := p.Host
	if host == "" {
		host = "127.0.0.1"
	}
	cfg, err := pgx.ParseConfig("postgres://postgres@127.0.0.1:5432/postgres")
	if err != nil {
		return nil, err
	}
	cfg.Host = host
	cfg.Port = uint16(port)
	cfg.User = p.Username
	cfg.Password = p.Password
	cfg.Database = "postgres"
	cfg.ConnectTimeout = 6 * time.Second
	cfg.DialFunc = (&net.Dialer{Timeout: 6 * time.Second}).DialContext
	return cfg, nil
}

// TestConnection verifies PostgreSQL credentials and returns the server version string.
func TestConnection(ctx context.Context, p ConnectionParams) (string, error) {
	tCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	cfg, err := buildConnConfig(p)
	if err != nil {
		return "", err
	}
	conn, err := pgx.ConnectConfig(tCtx, cfg)
	if err != nil {
		return "", fmt.Errorf("connect to %s:%s failed: %w", cfg.Host, strconv.Itoa(int(cfg.Port)), err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	var version string
	if err := conn.QueryRow(tCtx, `SELECT version()`).Scan(&version); err != nil {
		return "", err
	}
	return version, nil
}

// ListDatabases queries all non-template databases and their current disk sizes on the PostgreSQL server.
func ListDatabases(ctx context.Context, p ConnectionParams) ([]LiveDatabaseInfo, error) {
	tCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	cfg, err := buildConnConfig(p)
	if err != nil {
		return nil, err
	}
	conn, err := pgx.ConnectConfig(tCtx, cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close(context.Background()) }()

	rows, err := conn.Query(tCtx, `
		SELECT datname, COALESCE(pg_database_size(datname), 0)
		FROM pg_database
		WHERE datistemplate = false AND datallowconn = true
		ORDER BY datname ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []LiveDatabaseInfo{}
	for rows.Next() {
		var d LiveDatabaseInfo
		if err := rows.Scan(&d.Name, &d.SizeBytes); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
