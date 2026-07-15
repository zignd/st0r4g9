// Package store is the SQLite persistence layer for st0r4g9: API keys,
// buckets, objects, and their metadata/annotations. Object bytes live on the
// filesystem (see package storage); this package holds everything else.
package store

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed migrations.sql
var migrations string

// Sentinel errors returned by the store, mapped to HTTP statuses by the API layer.
var (
	// ErrNotFound is returned when a requested row does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict is returned when a uniqueness constraint would be violated.
	ErrConflict = errors.New("conflict")
)

// Store wraps a SQLite connection pool.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path, enables foreign
// keys and WAL, and applies the schema migrations.
func Open(path string) (*Store, error) {
	// _pragma params are honored per-connection by modernc.org/sqlite.
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if _, err := db.Exec(migrations); err != nil {
		return nil, fmt.Errorf("apply migrations: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }
