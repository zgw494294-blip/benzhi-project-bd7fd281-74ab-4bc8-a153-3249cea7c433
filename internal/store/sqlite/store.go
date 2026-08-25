package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	if path == "" {
		path = ":memory:"
	}
	dsn := path
	if path == ":memory:" {
		dsn = "file:bioacoustic?mode=memory&cache=shared"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	s := &Store{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.configure(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.VerifyIntegrity(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) configure(ctx context.Context) error {
	statements := []string{"PRAGMA foreign_keys = ON", "PRAGMA journal_mode = WAL", "PRAGMA synchronous = NORMAL", "PRAGMA busy_timeout = 5000"}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("配置 SQLite: %w", err)
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }
