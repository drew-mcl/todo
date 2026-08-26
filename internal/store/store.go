// Package store persists tasks in a local SQLite file.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go driver: no CGO, so the binary stays standalone
)

//go:embed schema.sql
var schemaSQL string

// migrations are applied in order; PRAGMA user_version records how far we got.
// Append new statements, never edit an applied one.
var migrations = []string{
	schemaSQL,

	// A batch is really a capture session -- usually a call. Naming it makes the
	// list a record of what was discussed and when, not just what is outstanding.
	`ALTER TABLE batches ADD COLUMN title TEXT NOT NULL DEFAULT '';`,
}

// Store owns the database handle.
type Store struct{ db *sql.DB }

// DefaultPath is where the database lives unless overridden by TODO_DB or --db.
func DefaultPath() string {
	if p := os.Getenv("TODO_DB"); p != "" {
		return p
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "todo.db"
	}
	return filepath.Join(dir, ".local", "share", "todo", "todo.db")
}

// Open connects to the database at path, creating and migrating it as needed.
// Pass ":memory:" for tests.
func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("creating database directory: %w", err)
		}
		dsn = path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	// One writer avoids "database is locked" entirely for a single-user tool.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("applying migration %d: %w", i+1, err)
		}
		// PRAGMA will not take a bind parameter.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
