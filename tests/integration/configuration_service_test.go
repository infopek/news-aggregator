package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/adapters/sqlite"
	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type serviceClock struct{ now time.Time }

func (clock serviceClock) Now() time.Time { return clock.now }

func TestConfigurationServiceSurvivesRestartAndRetainsArticlesOnSourceDelete(t *testing.T) {
	ctx := context.Background()
	store, path := openStore(t)
	now := time.Date(2026, 8, 2, 15, 0, 0, 0, time.UTC)
	service := configurationService(store, now)
	ranking := domain.RankingConfiguration{
		Recency:           domain.SignalWeight{Enabled: true, Weight: .5},
		Interest:          domain.SignalWeight{Enabled: true, Weight: .5},
		PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1",
	}
	profile := domain.UserProfile{ID: domain.LocalProfileID,
		Location: domain.OptionalSignal[domain.Location]{Present: true, Enabled: false, Value: domain.Location{Country: "HU", Region: "Budapest"}},
		Age:      domain.OptionalSignal[int]{Present: true, Enabled: false, Value: 40},
	}
	if err := service.Initialize(ctx, profile, ranking); err != nil {
		t.Fatal(err)
	}
	// Initialization is deliberately idempotent and cannot erase later edits.
	profile.Age.Value = 18
	if err := service.Initialize(ctx, profile, ranking); err != nil {
		t.Fatal(err)
	}

	starter := serviceFeed("starter", "https://example.com/feed")
	if _, err := service.ImportStarterSources(ctx, application.ImportStarterSourcesCommand{Sources: []domain.Source{starter}}); err != nil {
		t.Fatal(err)
	}
	starter.Name = "User-edited starter"
	if _, err := service.SaveSource(ctx, application.SaveSourceCommand{Source: starter}); err != nil {
		t.Fatal(err)
	}
	starter.Name = "Catalog name"
	if _, err := service.ImportStarterSources(ctx, application.ImportStarterSourcesCommand{Sources: []domain.Source{starter}}); err != nil {
		t.Fatal(err)
	}

	article := domain.Article{ID: "retained", Fingerprint: "retained-fp", SourceID: starter.ID, CanonicalURL: "https://example.com/article", Title: "Retained", FetchedAt: now, ContentPermission: domain.ContentMetadataOnly}
	if _, err := store.Articles().Upsert(ctx, article); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = reopenStore(t, path)
	defer store.Close()
	service = configurationService(store, now.Add(time.Hour))
	gotProfile, err := service.GetProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !gotProfile.Age.Present || gotProfile.Age.Enabled || gotProfile.Age.Value != 40 || gotProfile.Location.Enabled {
		t.Fatalf("optional state did not survive restart: %+v", gotProfile)
	}
	sources, err := service.ListSources(ctx)
	if err != nil || len(sources) != 1 || sources[0].Name != "User-edited starter" {
		t.Fatalf("starter edit did not survive restart: %+v, %v", sources, err)
	}
	if err := service.DeleteSource(ctx, application.DeleteSourceCommand{SourceID: starter.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Articles().Get(ctx, article.ID); err != nil {
		t.Fatalf("article was not retained after source deletion: %v", err)
	}
}

func configurationService(store *sqlite.Store, now time.Time) application.ConfigurationService {
	return application.ConfigurationService{Profiles: store.Profiles(), Rankings: store.Rankings(), Sources: store.Sources(), Transactions: store, Clock: serviceClock{now: now}}
}

func serviceFeed(id domain.SourceID, rawURL string) domain.Source {
	return domain.Source{ID: id, Name: "Starter", URL: rawURL, Kind: domain.SourceKindFeed, Enabled: true,
		AdapterConfig:     domain.AdapterConfiguration{Feed: &domain.FeedConfiguration{Format: domain.FeedFormatRSS}},
		ContentPermission: domain.ContentMetadataOnly, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyNotApplicable}}
}
