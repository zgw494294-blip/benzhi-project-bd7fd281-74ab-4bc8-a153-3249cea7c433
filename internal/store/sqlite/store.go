package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db           *sql.DB
	sharedMemory bool
	closeMu      sync.Mutex
	closed       bool
}

var memoryStorePool struct {
	sync.Mutex
	db       *sql.DB
	refCount int
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = ":memory:"
	}
	if path == ":memory:" {
		return openSharedMemoryStore()
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	s := &Store{db: db, sharedMemory: false}
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

// openSharedMemoryStore returns a Store backed by a single in-memory SQLite
// database shared across callers within the same process. The underlying
// *sql.DB is reference counted, so closing one Store does not affect other
// Stores still holding a reference; the database is only closed once the last
// reference is released.
func openSharedMemoryStore() (*Store, error) {
	memoryStorePool.Lock()
	defer memoryStorePool.Unlock()
	if memoryStorePool.db != nil {
		memoryStorePool.refCount++
		return &Store{db: memoryStorePool.db, sharedMemory: true}, nil
	}
	db, err := sql.Open("sqlite", "file:bioacoustic?mode=memory&cache=shared")
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	s := &Store{db: db, sharedMemory: true}
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
	memoryStorePool.db = db
	memoryStorePool.refCount = 1
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

func (s *Store) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.sharedMemory {
		memoryStorePool.Lock()
		db := memoryStorePool.db
		if memoryStorePool.refCount > 0 {
			memoryStorePool.refCount--
		}
		if memoryStorePool.refCount == 0 && db != nil {
			memoryStorePool.db = nil
		} else {
			db = nil
		}
		memoryStorePool.Unlock()
		if db != nil {
			return db.Close()
		}
		return nil
	}
	return s.db.Close()
}
