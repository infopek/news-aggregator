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
	readSet, savedSet, hiddenSet := patch.Read != nil, patch.Saved != nil, patch.Hidden != nil
	var read, saved, hidden sql.NullInt64
	err := r.store.q(ctx).QueryRowContext(ctx, `
		INSERT INTO library_states(article_id,read_at_ms,saved_at_ms,hidden_at_ms)
		SELECT ?,?,?,? WHERE EXISTS(SELECT 1 FROM articles WHERE id=?)
		ON CONFLICT(article_id) DO UPDATE SET
			read_at_ms=CASE WHEN ? THEN excluded.read_at_ms ELSE library_states.read_at_ms END,
			saved_at_ms=CASE WHEN ? THEN excluded.saved_at_ms ELSE library_states.saved_at_ms END,
			hidden_at_ms=CASE WHEN ? THEN excluded.hidden_at_ms ELSE library_states.hidden_at_ms END
		RETURNING read_at_ms,saved_at_ms,hidden_at_ms`,
		id, patchTime(patch.Read, at), patchTime(patch.Saved, at), patchTime(patch.Hidden, at), id,
		readSet, savedSet, hiddenSet).Scan(&read, &saved, &hidden)
	if err == sql.ErrNoRows {
		return domain.LibraryState{ArticleID: id}, application.ErrNotFound
	}
	if err != nil {
		return domain.LibraryState{ArticleID: id}, mapError(err)
	}
	return domain.LibraryState{ArticleID: id, ReadAt: timePtr(read), SavedAt: timePtr(saved), HiddenAt: timePtr(hidden)}, nil
}

func patchTime(enabled *bool, at time.Time) any {
	if enabled == nil || !*enabled {
		return nil
	}
	return at.UTC().UnixMilli()
}

func timePtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := timeFromMillis(v.Int64)
	return &t
}
