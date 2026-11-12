package database

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Store struct {
	db   *sql.DB
	path string
}

// Open initializes the pure-Go SQLite database in WAL mode with serialized access
// to guarantee zero SQLITE_BUSY errors and minimal memory footprint.
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0750); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(8000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)", filepath.ToSlash(dbPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	s := &Store{db: db, path: dbPath}
	if err := s.runMigrations(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	if err := s.recoverInterruptedJobs(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("recover interrupted jobs: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	if s.db != nil {
		_, _ = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE);`)
		return s.db.Close()
	}
	return nil
}

func (s *Store) Path() string {
	return s.path
}

// SnapshotTo creates a consistent atomic backup of the live WAL SQLite database using VACUUM INTO.
func (s *Store) SnapshotTo(destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0750); err != nil {
		return err
	}
	_ = os.Remove(destPath)
	_, err := s.db.Exec(`VACUUM INTO ?`, filepath.ToSlash(destPath))
	return err
}

func (s *Store) runMigrations() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);`)
	if err != nil {
		return err
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}

	type mig struct {
		version int
		name    string
	}
	var migs []mig
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		prefix := strings.SplitN(e.Name(), "_", 2)[0]
		v, err := strconv.Atoi(prefix)
		if err != nil {
			continue
		}
		migs = append(migs, mig{version: v, name: e.Name()})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })

	for _, m := range migs {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, m.version).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		rawSQL, err := migrationFiles.ReadFile("migrations/" + m.name)
		if err != nil {
			return err
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(rawSQL)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s failed: %w", m.name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, m.version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) recoverInterruptedJobs() error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE backup_executions
		SET local_status = 'failed', finished_at = ?, error_message = 'Interrupted by unexpected service or system restart'
		WHERE local_status = 'running'`, now)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		UPDATE backup_executions
		SET cloud_status = 'retrying', next_retry_at = ?
		WHERE cloud_status = 'uploading' AND local_status = 'completed'`, now)
	return err
}
