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
