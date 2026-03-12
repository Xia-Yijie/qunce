package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type localStore struct {
	db *sql.DB
}

func openLocalStore(workDir string) (*localStore, error) {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}

	statePath := resolveLocalStatePath(workDir)
	db, err := sql.Open("sqlite", statePath)
	if err != nil {
		return nil, fmt.Errorf("open local state: %w", err)
	}

	store := &localStore{db: db}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *localStore) init() error {
	if _, err := s.db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA busy_timeout = 5000;

		CREATE TABLE IF NOT EXISTS agent_config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS agent_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			payload TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
	`); err != nil {
		return fmt.Errorf("init local state schema: %w", err)
	}

	return nil
}

func resolveLocalStatePath(workDir string) string {
	preferred := filepath.Join(workDir, "agent-state")
	legacy := filepath.Join(workDir, "agent.db")
	if _, err := os.Stat(preferred); err == nil {
		return preferred
	}
	if _, err := os.Stat(legacy); err != nil {
		return preferred
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		legacyPath := legacy + suffix
		if _, err := os.Stat(legacyPath); err != nil {
			continue
		}
		if err := os.Rename(legacyPath, preferred+suffix); err != nil {
			return legacy
		}
	}
	return preferred
}

func (s *localStore) Close() error {
	return s.db.Close()
}

func (s *localStore) Get(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM agent_config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read config %q: %w", key, err)
	}
	return value, nil
}

func (s *localStore) Set(key, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	if _, err := s.db.Exec(`
		INSERT INTO agent_config(key, value)
		VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, trimmed); err != nil {
		return fmt.Errorf("write config %q: %w", key, err)
	}

	return nil
}

func (s *localStore) RecordEvent(eventType, payload string) error {
	if _, err := s.db.Exec(`
		INSERT INTO agent_events(event_type, payload, created_at)
		VALUES(?, ?, ?)
	`, eventType, payload, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("record event %q: %w", eventType, err)
	}

	return nil
}
