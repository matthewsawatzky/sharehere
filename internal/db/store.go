package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	path := filepath.Join(dataDir, "sharehere.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	stmts := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("sqlite pragma failed: %w", err)
		}
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.ensureDefaultSettings(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL,
			disabled INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			user_id INTEGER NULL,
			csrf_token TEXT NOT NULL,
			remember INTEGER NOT NULL DEFAULT 0,
			ip TEXT,
			user_agent TEXT,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS share_links (
			token TEXT PRIMARY KEY,
			path TEXT NOT NULL,
			mode TEXT NOT NULL,
			created_by INTEGER NULL,
			expires_at DATETIME NOT NULL,
			revoked INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_accessed_at DATETIME NULL,
			FOREIGN KEY(created_by) REFERENCES users(id) ON DELETE SET NULL
		);`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_user_id INTEGER NULL,
			action TEXT NOT NULL,
			target TEXT NOT NULL,
			metadata TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(actor_user_id) REFERENCES users(id) ON DELETE SET NULL
		);`,
		`CREATE TABLE IF NOT EXISTS login_attempts (
			key TEXT PRIMARY KEY,
			failed_count INTEGER NOT NULL DEFAULT 0,
			locked_until DATETIME NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_share_links_expiry ON share_links(expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_logs(created_at);`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate failed: %w", err)
		}
	}
	return nil
}
