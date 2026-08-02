package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type LibraryRepository struct{ store *Store }

func (s *Store) Libraries() *LibraryRepository { return &LibraryRepository{store: s} }
func (r *LibraryRepository) Get(ctx context.Context, id domain.ArticleID) (domain.LibraryState, error) {
	var v domain.LibraryState
	v.ArticleID = id
	var read, saved, hidden sql.NullInt64
	err := r.store.q(ctx).QueryRowContext(ctx, `SELECT read_at_ms,saved_at_ms,hidden_at_ms FROM library_states WHERE article_id=?`, id).Scan(&read, &saved, &hidden)
	if err == sql.ErrNoRows {
		return v, application.ErrNotFound
	}
	if err != nil {
		return v, mapError(err)
	}
	v.ReadAt = timePtr(read)
	v.SavedAt = timePtr(saved)
	v.HiddenAt = timePtr(hidden)
	return v, nil
}
func (r *LibraryRepository) Apply(ctx context.Context, id domain.ArticleID, patch domain.LibraryPatch, at time.Time) (domain.LibraryState, error) {
	v, err := r.Get(ctx, id)
	if err == application.ErrNotFound {
		var exists int
		if e := r.store.q(ctx).QueryRowContext(ctx, `SELECT 1 FROM articles WHERE id=?`, id).Scan(&exists); e == sql.ErrNoRows {
			return v, application.ErrNotFound
		} else if e != nil {
			return v, mapError(e)
		}
		err = nil
	}
	if err != nil {
		return v, err
	}
	v.ArticleID = id
	if patch.Read != nil {
		v.ReadAt = patchedTime(*patch.Read, at)
	}
	if patch.Saved != nil {
		v.SavedAt = patchedTime(*patch.Saved, at)
	}
	if patch.Hidden != nil {
		v.HiddenAt = patchedTime(*patch.Hidden, at)
	}
	_, err = r.store.q(ctx).ExecContext(ctx, `INSERT INTO library_states(article_id,read_at_ms,saved_at_ms,hidden_at_ms)VALUES(?,?,?,?) ON CONFLICT(article_id)DO UPDATE SET read_at_ms=excluded.read_at_ms,saved_at_ms=excluded.saved_at_ms,hidden_at_ms=excluded.hidden_at_ms`, id, nullableMillis(v.ReadAt), nullableMillis(v.SavedAt), nullableMillis(v.HiddenAt))
	return v, mapError(err)
}
func patchedTime(enabled bool, at time.Time) *time.Time {
	if !enabled {
		return nil
	}
	t := at.UTC()
	return &t
}
func timePtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := timeFromMillis(v.Int64)
	return &t
}
