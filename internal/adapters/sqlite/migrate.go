package sqlite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

var migrationName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

var ErrNewerSchema = errors.New("database schema is newer than this application")
var ErrMigrationState = errors.New("invalid migration state")

type migration struct {
	version   int
	name, sql string
}

func (s *Store) Migrate(ctx context.Context, directory string) (int, error) {
	migrations, err := loadMigrations(directory)
	if err != nil {
		return 0, err
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at_ms INTEGER NOT NULL)`); err != nil {
		return 0, mapError(err)
	}
	applied := map[int]string{}
	rows, err := s.db.QueryContext(ctx, `SELECT version,name FROM schema_migrations ORDER BY version`)
	if err != nil {
		return 0, mapError(err)
	}
	for rows.Next() {
		var v int
		var n string
		if err := rows.Scan(&v, &n); err != nil {
			rows.Close()
			return 0, mapError(err)
		}
		applied[v] = n
	}
	if err := rows.Close(); err != nil {
		return 0, mapError(err)
	}
	max := 0
	if len(migrations) > 0 {
		max = migrations[len(migrations)-1].version
	}
	for version, name := range applied {
		if version > max {
			return 0, ErrNewerSchema
		}
		found := false
		for _, m := range migrations {
			if m.version == version {
				found = true
				if m.name != name {
					return 0, ErrMigrationState
				}
				break
			}
		}
		if !found {
			return 0, ErrMigrationState
		}
	}
	for _, m := range migrations {
		if _, ok := applied[m.version]; ok {
			continue
		}
		if err := s.applyMigration(ctx, m); err != nil {
			return 0, err
		}
	}
	return max, nil
}

func loadMigrations(directory string) ([]migration, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	var result []migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := migrationName.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		version, _ := strconv.Atoi(match[1])
		body, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		result = append(result, migration{version: version, name: entry.Name(), sql: string(body)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].version < result[j].version })
	for index, m := range result {
		if m.version != index+1 {
			return nil, ErrMigrationState
		}
		if index > 0 && result[index-1].version == m.version {
			return nil, ErrMigrationState
		}
	}
	return result, nil
}

func (s *Store) applyMigration(ctx context.Context, m migration) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return mapError(err)
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return mapError(err)
	}
	rollback := func() { _, _ = conn.ExecContext(context.Background(), "ROLLBACK") }
	if _, err = conn.ExecContext(ctx, m.sql); err != nil {
		rollback()
		return mapError(err)
	}
	if _, err = conn.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,applied_at_ms) VALUES(?,?,?)`, m.version, m.name, timeNowMillis()); err != nil {
		rollback()
		return mapError(err)
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		rollback()
		return mapError(err)
	}
	return nil
}

var timeNowMillis = func() int64 { return time.Now().UTC().UnixMilli() }
