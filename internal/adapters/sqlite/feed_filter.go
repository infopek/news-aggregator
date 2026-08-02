package sqlite

import (
	"context"
	"database/sql"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type FeedFilterRepository struct{ store *Store }

func (s *Store) FeedFilters() *FeedFilterRepository { return &FeedFilterRepository{store: s} }

func (r *FeedFilterRepository) Save(ctx context.Context, state domain.FeedFilterState) error {
	if !state.Valid() || state.UpdatedAt.IsZero() {
		return application.ErrInvalidInput
	}
	var source any
	if state.SourceID != "" {
		source = state.SourceID
	}
	_, err := r.store.q(ctx).ExecContext(ctx, `INSERT INTO feed_filter_state(profile_id,source_id,read_filter,saved_only,include_hidden,search_query,updated_at_ms) VALUES(?,?,?,?,?,?,?) ON CONFLICT(profile_id) DO UPDATE SET source_id=excluded.source_id,read_filter=excluded.read_filter,saved_only=excluded.saved_only,include_hidden=excluded.include_hidden,search_query=excluded.search_query,updated_at_ms=excluded.updated_at_ms`, state.ProfileID, source, state.Read, state.SavedOnly, state.IncludeHidden, state.SearchQuery, millis(state.UpdatedAt))
	return mapError(err)
}

func (r *FeedFilterRepository) Get(ctx context.Context, id domain.ProfileID) (domain.FeedFilterState, error) {
	var state domain.FeedFilterState
	var source sql.NullString
	var updated int64
	err := r.store.q(ctx).QueryRowContext(ctx, `SELECT profile_id,source_id,read_filter,saved_only,include_hidden,search_query,updated_at_ms FROM feed_filter_state WHERE profile_id=?`, id).Scan(&state.ProfileID, &source, &state.Read, &state.SavedOnly, &state.IncludeHidden, &state.SearchQuery, &updated)
	if err == sql.ErrNoRows {
		return state, application.ErrNotFound
	}
	if err != nil {
		return state, mapError(err)
	}
	state.SourceID = domain.SourceID(source.String)
	state.UpdatedAt = timeFromMillis(updated)
	return state, nil
}
