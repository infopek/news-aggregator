package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/application/ingestion"
	"github.com/infopek/news-aggregator/internal/domain"
)

type refreshRunner func(context.Context, domain.SourceID) (ingestion.RunResult, error)

func (f refreshRunner) Run(ctx context.Context, id domain.SourceID) (ingestion.RunResult, error) {
	return f(ctx, id)
}

type refreshClock struct{ value time.Time }

func (c refreshClock) Now() time.Time { return c.value }

type rateLimitedAdapter struct{}

func (rateLimitedAdapter) Kind() domain.SourceKind { return domain.SourceKindAPI }
func (rateLimitedAdapter) Fetch(context.Context, domain.Source, application.FetchCursor) (application.AdapterResult, error) {
	return application.AdapterResult{}, &application.RateLimitError{Retryable: true, RetryAfter: time.Minute}
}

func TestRefreshCoordinatorPersistsTerminalStatusAcrossRestart(t *testing.T) {
	store, path := openStore(t)
	source := domain.Source{ID: "feed", Name: "Fixture", URL: "https://fictional.invalid/feed", Kind: domain.SourceKindFeed, Enabled: true, ContentPermission: domain.ContentMetadataOnly, AdapterConfig: domain.AdapterConfiguration{Feed: &domain.FeedConfiguration{Format: domain.FeedFormatAuto}}, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyNotApplicable}}
	must(t, store.Sources().Save(context.Background(), source))
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	coordinator := &ingestion.Coordinator{Refreshes: store.Refreshes(), Sources: store.Sources(), Runners: map[domain.SourceKind]ingestion.SourceRunner{domain.SourceKindFeed: refreshRunner(func(context.Context, domain.SourceID) (ingestion.RunResult, error) {
		return ingestion.RunResult{Fetched: 3, Writes: []application.ArticleWriteResult{{Inserted: true}, {Updated: true}}}, nil
	})}, Clock: refreshClock{now}, NewID: func() domain.RefreshRunID { return "restart-run" }}
	run, err := coordinator.StartRefresh(context.Background(), application.StartRefreshCommand{})
	must(t, err)
	deadline := time.Now().Add(time.Second)
	for {
		run, err = store.Refreshes().Get(context.Background(), run.ID)
		must(t, err)
		if run.Status != domain.RefreshRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("refresh remained running")
		}
		time.Sleep(time.Millisecond)
	}
	if run.Status != domain.RefreshSucceeded || len(run.Outcomes) != 1 || run.Outcomes[0].Skipped != 1 {
		t.Fatalf("run=%+v", run)
	}
	store.Close()
	store = reopenStore(t, path)
	defer store.Close()
	persisted, err := store.Refreshes().Get(context.Background(), run.ID)
	must(t, err)
	if persisted.Status != domain.RefreshSucceeded || persisted.FinishedAt == nil || persisted.Outcomes[0].Fetched != 3 {
		t.Fatalf("persisted=%+v", persisted)
	}
}

func TestRefreshRateLimitPersistsStructuredSourceState(t *testing.T) {
	store, _ := openStore(t)
	defer store.Close()
	ctx := context.Background()
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	source := domain.Source{ID: "api", Name: "Fixture API", URL: "https://fictional.invalid/api", Kind: domain.SourceKindAPI, Enabled: true, ContentPermission: domain.ContentMetadataOnly, AdapterConfig: domain.AdapterConfiguration{API: &domain.APIConfiguration{Provider: "fictional", PageSize: 10}}, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyNotApplicable}}
	must(t, store.Sources().Save(ctx, source))
	runner := &ingestion.Runner{Adapter: rateLimitedAdapter{}, Sources: store.Sources(), Articles: store.Articles(), Transactions: store, Clock: refreshClock{now}, NewID: func(string) domain.ArticleID { return "unused" }}
	coordinator := &ingestion.Coordinator{Refreshes: store.Refreshes(), Sources: store.Sources(), Runners: map[domain.SourceKind]ingestion.SourceRunner{domain.SourceKindAPI: runner}, Clock: refreshClock{now}, NewID: func() domain.RefreshRunID { return "rate-run" }}
	run, err := coordinator.StartRefresh(ctx, application.StartRefreshCommand{})
	must(t, err)
	coordinator.Wait()
	run, err = store.Refreshes().Get(ctx, run.ID)
	must(t, err)
	if run.Status != domain.RefreshFailed || len(run.Outcomes) != 1 || run.Outcomes[0].ErrorCode != "rate_limited" {
		t.Fatalf("run=%+v", run)
	}
	persisted, err := store.Sources().Get(ctx, source.ID)
	must(t, err)
	if persisted.LastError != "rate_limited" || persisted.RetryAfter == nil || !persisted.RetryAfter.Equal(now.Add(time.Minute)) {
		t.Fatalf("source=%+v", persisted)
	}
}
