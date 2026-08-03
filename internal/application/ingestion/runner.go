package ingestion

import (
	"context"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

// Runner owns the durable adapter-to-repository boundary. Fetching happens
// before the transaction; article writes and cursor advancement commit
// together. A failed normalization, article write, or source save therefore
// leaves both articles and the previous cursor unchanged.
type Runner struct {
	Adapter      application.IngestionAdapter
	Sources      application.SourceRepository
	Articles     application.ArticleRepository
	Transactions application.TransactionManager
	Clock        application.Clock
	NewID        IDGenerator
}

type RunResult struct {
	Writes    []application.ArticleWriteResult
	Warnings  []string
	Unchanged bool
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
		source.RefreshCursor = result.NextCursor.Value
		source.RefreshETag = result.NextCursor.ETag
		source.RefreshLastModified = result.NextCursor.LastModified
		now := r.Clock.Now().UTC()
		source.LastSuccessAt, source.LastError, source.RetryAfter = &now, "", nil
		return r.Sources.Save(txctx, source)
	})
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{Writes: writes, Warnings: result.Warnings, Unchanged: result.Unchanged}, nil
}
