package ingestion

import (
	"context"
	"errors"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

// Runner owns the durable adapter-to-repository boundary. Fetching happens
// before the transaction; article writes and cursor advancement commit
// together. A failed normalization, article write, or source save therefore
// leaves both articles and the previous cursor unchanged.
type Runner struct {
	Adapter      application.IngestionAdapter
	Sources      application.SourceIngestionRepository
	Articles     application.ArticleRepository
	Transactions application.TransactionManager
	Clock        application.Clock
	NewID        IDGenerator
}

type RunResult struct {
	Writes    []application.ArticleWriteResult
	Warnings  []string
	Unchanged bool
	Fetched   int
}

func (r Runner) Run(ctx context.Context, sourceID domain.SourceID) (RunResult, error) {
	if r.Adapter == nil || r.Sources == nil || r.Articles == nil || r.Transactions == nil || r.Clock == nil || r.NewID == nil || sourceID == "" {
		return RunResult{}, application.ErrInvalidInput
	}
	source, err := r.Sources.Get(ctx, sourceID)
	if err != nil {
		return RunResult{}, err
	}
	if source.Kind != r.Adapter.Kind() {
		return RunResult{}, application.ErrInvalidInput
	}
	result, err := r.Adapter.Fetch(ctx, source, application.FetchCursor{Value: source.RefreshCursor, ETag: source.RefreshETag, LastModified: source.RefreshLastModified})
	if err != nil {
		var rateLimit *application.RateLimitError
		if errors.As(err, &rateLimit) {
			retryAt := r.Clock.Now().UTC().Add(rateLimit.RetryAfter)
			state := ingestionState(source)
			state.LastError, state.RetryAfter = "rate_limited", &retryAt
			if saveErr := r.Sources.UpdateIngestionState(ctx, source.ID, state); saveErr != nil {
				return RunResult{}, saveErr
			}
		}
		return RunResult{}, err
	}
	var writes []application.ArticleWriteResult
	err = r.Transactions.WithinTransaction(ctx, func(txctx context.Context) error {
		if !result.Unchanged {
			writes, err = (Service{Articles: r.Articles, Clock: r.Clock, NewID: r.NewID}).Ingest(txctx, source, result.Items)
			if err != nil {
				return err
			}
		}
		now := r.Clock.Now().UTC()
		return r.Sources.UpdateIngestionState(txctx, source.ID, application.SourceIngestionState{
			RefreshCursor: result.NextCursor.Value, RefreshETag: result.NextCursor.ETag,
			RefreshLastModified: result.NextCursor.LastModified, LastSuccessAt: &now,
		})
	})
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{Writes: writes, Warnings: result.Warnings, Unchanged: result.Unchanged, Fetched: len(result.Items) + len(result.Warnings)}, nil
}

func ingestionState(source domain.Source) application.SourceIngestionState {
	return application.SourceIngestionState{
		RefreshCursor: source.RefreshCursor, RefreshETag: source.RefreshETag,
		RefreshLastModified: source.RefreshLastModified, LastSuccessAt: source.LastSuccessAt,
		LastError: source.LastError, RetryAfter: source.RetryAfter,
	}
}
