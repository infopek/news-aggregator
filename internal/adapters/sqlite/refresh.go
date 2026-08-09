package sqlite

import (
	"context"
	"database/sql"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type RefreshRepository struct{ store *Store }

func (s *Store) Refreshes() *RefreshRepository { return &RefreshRepository{store: s} }
func (r *RefreshRepository) Create(ctx context.Context, v domain.RefreshRun) error {
	if !validRefresh(v) {
		return application.ErrInvalidInput
	}
	return r.store.WithinTransaction(ctx, func(ctx context.Context) error {
		_, err := r.store.q(ctx).ExecContext(ctx, `INSERT INTO refresh_runs(id,started_at_ms,finished_at_ms,status)VALUES(?,?,?,?)`, v.ID, millis(v.StartedAt), nullableMillis(v.FinishedAt), v.Status)
		if err != nil {
			return mapError(err)
		}
		if len(v.Outcomes) > 0 {
			return r.saveOutcomes(ctx, v)
		}
		return nil
	})
}
func (r *RefreshRepository) Save(ctx context.Context, v domain.RefreshRun) error {
	if !validRefresh(v) {
		return application.ErrInvalidInput
	}
	return r.store.WithinTransaction(ctx, func(ctx context.Context) error {
		result, err := r.store.q(ctx).ExecContext(ctx, `UPDATE refresh_runs SET finished_at_ms=?,status=? WHERE id=?`, nullableMillis(v.FinishedAt), v.Status, v.ID)
		if err != nil {
			return mapError(err)
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return application.ErrNotFound
		}
		return r.saveOutcomes(ctx, v)
	})
}
func (r *RefreshRepository) saveOutcomes(ctx context.Context, v domain.RefreshRun) error {
	if _, err := r.store.q(ctx).ExecContext(ctx, `DELETE FROM refresh_outcomes WHERE refresh_run_id=?`, v.ID); err != nil {
		return mapError(err)
	}
	for _, o := range v.Outcomes {
		if o.SourceID == "" || o.Fetched < 0 || o.Inserted < 0 || o.Updated < 0 || o.Skipped < 0 || o.Failed < 0 {
			return application.ErrInvalidInput
		}
		if _, err := r.store.q(ctx).ExecContext(ctx, `INSERT INTO refresh_outcomes(refresh_run_id,source_id,fetched,inserted,updated,skipped,failed,error_code,error_summary)VALUES(?,?,?,?,?,?,?,?,?)`, v.ID, o.SourceID, o.Fetched, o.Inserted, o.Updated, o.Skipped, o.Failed, o.ErrorCode, o.ErrorSummary); err != nil {
			return mapError(err)
		}
	}
	return nil
}
func (r *RefreshRepository) Get(ctx context.Context, id domain.RefreshRunID) (domain.RefreshRun, error) {
	return r.getWhere(ctx, ` WHERE id=?`, id)
}
func (r *RefreshRepository) Active(ctx context.Context) (*domain.RefreshRun, error) {
	v, err := r.getWhere(ctx, ` WHERE status='running'`)
	if err == application.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}
func (r *RefreshRepository) getWhere(ctx context.Context, where string, arg ...any) (domain.RefreshRun, error) {
	var v domain.RefreshRun
	var start int64
	var finish sql.NullInt64
	err := r.store.q(ctx).QueryRowContext(ctx, `SELECT id,started_at_ms,finished_at_ms,status FROM refresh_runs`+where, arg...).Scan(&v.ID, &start, &finish, &v.Status)
	if err == sql.ErrNoRows {
		return v, application.ErrNotFound
	}
	if err != nil {
		return v, mapError(err)
	}
	v.StartedAt = timeFromMillis(start)
	v.FinishedAt = timePtr(finish)
	rows, err := r.store.q(ctx).QueryContext(ctx, `SELECT source_id,fetched,inserted,updated,skipped,failed,error_code,error_summary FROM refresh_outcomes WHERE refresh_run_id=? ORDER BY source_id`, v.ID)
	if err != nil {
		return v, mapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var o domain.SourceRefreshOutcome
		if err := rows.Scan(&o.SourceID, &o.Fetched, &o.Inserted, &o.Updated, &o.Skipped, &o.Failed, &o.ErrorCode, &o.ErrorSummary); err != nil {
			return v, mapError(err)
		}
		v.Outcomes = append(v.Outcomes, o)
	}
	return v, mapError(rows.Err())
}
func validRefresh(v domain.RefreshRun) bool {
	if v.ID == "" || v.StartedAt.IsZero() {
		return false
	}
	if v.Status == domain.RefreshRunning {
		return v.FinishedAt == nil
	}
	return (v.Status == domain.RefreshSucceeded || v.Status == domain.RefreshPartialSuccess || v.Status == domain.RefreshFailed || v.Status == domain.RefreshCancelled) && v.FinishedAt != nil && !v.FinishedAt.Before(v.StartedAt)
}

var _ application.TransactionManager = (*Store)(nil)
var _ application.ProfileRepository = (*ProfileRepository)(nil)
var _ application.SourceRepository = (*SourceRepository)(nil)
var _ application.SourceIngestionRepository = (*SourceRepository)(nil)
var _ application.ArticleRepository = (*ArticleRepository)(nil)
var _ application.LibraryRepository = (*LibraryRepository)(nil)
var _ application.FeedFilterRepository = (*FeedFilterRepository)(nil)
var _ application.RankingRepository = (*RankingRepository)(nil)
var _ application.RefreshRepository = (*RefreshRepository)(nil)
