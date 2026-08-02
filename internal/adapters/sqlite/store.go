package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	_ "modernc.org/sqlite"
)

const (
	DefaultBusyTimeout  = time.Second
	DefaultMaxOpenConns = 1
)

type Config struct {
	Path         string
	MigrationDir string
	BusyTimeout  time.Duration
	MaxOpenConns int
}

type Store struct{ db *sql.DB }

type txKey struct{}

func Open(ctx context.Context, config Config) (*Store, error) {
	if strings.TrimSpace(config.Path) == "" || strings.TrimSpace(config.MigrationDir) == "" {
		return nil, application.ErrInvalidInput
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = DefaultBusyTimeout
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = DefaultMaxOpenConns
	}
	path, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, application.ErrInvalidInput
	}
	params := url.Values{}
	params.Add("_pragma", "foreign_keys(1)")
	params.Add("_pragma", "journal_mode(WAL)")
	params.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", config.BusyTimeout.Milliseconds()))
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?"+params.Encode())
	if err != nil {
		return nil, mapError(err)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	db.SetConnMaxLifetime(0)
	store := &Store{db: db}
	if err := store.verifyConnection(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := store.Migrate(ctx, config.MigrationDir); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) verifyConnection(ctx context.Context) error {
	var enabled int
	if err := s.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		return mapError(err)
	}
	if enabled != 1 {
		return application.ErrUnavailable
	}
	return nil
}

func (s *Store) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return application.ErrInvalidInput
	}
	if _, nested := ctx.Value(txKey{}).(*sql.Tx); nested {
		return fn(ctx)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return mapError(err)
	}
	txctx := context.WithValue(ctx, txKey{}, tx)
	if err := fn(txctx); err != nil {
		_ = tx.Rollback()
		return mapError(err)
	}
	if err := ctx.Err(); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return mapError(err)
	}
	return nil
}

type dbtx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) q(ctx context.Context) dbtx {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx
	}
	return s.db
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, application.ErrNotFound) || errors.Is(err, application.ErrConflict) || errors.Is(err, application.ErrInvalidInput) || errors.Is(err, application.ErrUnavailable) {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "unique constraint"), strings.Contains(message, "constraint failed"), strings.Contains(message, "foreign key constraint"):
		return application.ErrConflict
	case strings.Contains(message, "database is locked"), strings.Contains(message, "database table is locked"), strings.Contains(message, "busy"):
		return application.ErrUnavailable
	default:
		return application.ErrUnavailable
	}
}

func millis(t time.Time) int64             { return t.UTC().UnixMilli() }
func timeFromMillis(value int64) time.Time { return time.UnixMilli(value).UTC() }
func nullableMillis(value *time.Time) any {
	if value == nil {
		return nil
	}
	return millis(*value)
}
func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
