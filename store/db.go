// Package store provides a lightweight SQL-backed store for browser credentials
// captured by the opentalon-chrome plugin.  SQLite and PostgreSQL are supported.
package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps a SQL database connection.
type DB struct {
	db     *sql.DB
	driver string // "sqlite" or "postgres"
}

// OpenSQLite opens (or creates) state.db in dataDir and runs pending migrations.
func OpenSQLite(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("store: create data dir: %w", err)
	}
	raw, err := sql.Open("sqlite", filepath.Join(dataDir, "state.db")+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("store: open db: %w", err)
	}
	if _, err := raw.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("store: busy_timeout: %w", err)
	}
	return openDB(raw, "sqlite")
}

// OpenPostgres opens a PostgreSQL connection at dsn and runs pending migrations.
// dsn is a lib/pq connection string, e.g. "postgres://user:pass@host/db?sslmode=disable".
func OpenPostgres(dsn string) (*DB, error) {
	raw, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open db: %w", err)
	}
	if err := raw.Ping(); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("store: ping postgres: %w", err)
	}
	return openDB(raw, "postgres")
}

// openDB attaches driver metadata and runs migrations on an already-open connection.
func openDB(raw *sql.DB, driver string) (*DB, error) {
	d := &DB{db: raw, driver: driver}
	if err := d.runMigrations(); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return d, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error { return d.db.Close() }

// SQLDB returns the raw *sql.DB for use by the Store.
func (d *DB) SQLDB() *sql.DB { return d.db }

// q rewrites ? → $N for Postgres. Only handles bare placeholders;
// does not parse quoted strings. All queries in this package use
// simple ? params so this is sufficient.
func (d *DB) q(query string) string {
	if d.driver != "postgres" {
		return query
	}
	var b strings.Builder
	n := 0
	for _, ch := range query {
		if ch == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
		} else {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func (d *DB) runMigrations() error {
	if _, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL PRIMARY KEY)`); err != nil {
		return fmt.Errorf("migrations: create schema_version: %w", err)
	}

	var current int
	var v sql.NullInt64
	switch err := d.db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&v); {
	case errors.Is(err, sql.ErrNoRows):
		// first run — current stays 0
	case err != nil:
		return fmt.Errorf("migrations: read version: %w", err)
	case v.Valid:
		current = int(v.Int64)
	}

	names, err := migrationNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		n, err := migrationNumber(name)
		if err != nil || n <= current {
			continue
		}
		sqlStr, err := fs.ReadFile(migrationsFS, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("migration %s: read: %w", name, err)
		}
		tx, err := d.db.Begin()
		if err != nil {
			return fmt.Errorf("migration %s: begin: %w", name, err)
		}
		if _, err := tx.Exec(string(sqlStr)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s: exec: %w", name, err)
		}
		if _, err := tx.Exec(`DELETE FROM schema_version`); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s: clear version: %w", name, err)
		}
		if _, err := tx.Exec(d.q(`INSERT INTO schema_version (version) VALUES (?)`), n); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s: set version: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migration %s: commit: %w", name, err)
		}
		current = n
	}
	return nil
}

func migrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func migrationNumber(name string) (int, error) {
	base := strings.TrimSuffix(name, ".sql")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid migration name: %s", name)
	}
	return strconv.Atoi(parts[0])
}
