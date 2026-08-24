package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db            *sql.DB
	authCacheMu   sync.RWMutex
	authReadCache map[string]cachedSession
}

type querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type txRepo struct {
	*DB
	q *sql.Tx
}

func Open(ctx context.Context, path string) (*DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("database path: required")
	}
	dsn := path
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		dsn = "file:" + url.PathEscape(path)
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	dsn += separator + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	raw.SetMaxOpenConns(8)
	raw.SetMaxIdleConns(4)
	raw.SetConnMaxLifetime(30 * time.Minute)
	store := &DB{db: raw, authReadCache: make(map[string]cachedSession)}
	if err := store.Ping(ctx); err != nil {
		raw.Close()
		return nil, err
	}
	if err := store.migrate(ctx); err != nil {
		raw.Close()
		return nil, err
	}
	return store, nil
}

func (d *DB) Ping(ctx context.Context) error {
	if err := d.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite: %w", err)
	}
	return nil
}

func (d *DB) Close() error { return d.db.Close() }

func (d *DB) queryer() querier     { return d.db }
func (t *txRepo) queryer() querier { return t.q }
