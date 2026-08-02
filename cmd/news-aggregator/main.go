package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/infopek/news-aggregator/internal/adapters/credentials"
	"github.com/infopek/news-aggregator/internal/adapters/sqlite"
	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
	"github.com/infopek/news-aggregator/internal/httpapi"
	"github.com/infopek/news-aggregator/internal/platform"
	"github.com/infopek/news-aggregator/internal/webassets"
)

const applicationVersion = "0.1.0"

func main() {
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	assets, err := fs.Sub(webassets.Files, "dist")
	if err != nil {
		return errors.New("compiled frontend assets are unavailable")
	}
	if _, err := fs.Stat(assets, "index.html"); err != nil {
		return errors.New("compiled frontend entrypoint is unavailable; build the web application first")
	}
	port, err := configuredPort(os.Getenv("NEWS_AGGREGATOR_PORT"))
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	configDir, err := os.UserConfigDir()
	if err != nil {
		return errors.New("local configuration directory is unavailable")
	}
	dataDir := filepath.Join(configDir, "NewsAggregator")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return errors.New("local data directory is unavailable")
	}
	store, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(dataDir, "news.db"), MigrationDir: "db/migrations"})
	if err != nil {
		return errors.New("local database is unavailable")
	}
	defer store.Close()
	vault := credentials.NewStore()
	configuration := application.ConfigurationService{Profiles: store.Profiles(), Rankings: store.Rankings(), Sources: store.Sources(), Transactions: store, Credentials: vault, Clock: systemClock{}, CredentialReference: credentials.ReferenceForSource}
	if err := configuration.Initialize(ctx, defaultProfile(), defaultRanking()); err != nil {
		return errors.New("local configuration could not be initialized")
	}

	api := http.NewServeMux()
	api.Handle("GET /api/v1/health", httpapi.NewHealthHandler(applicationVersion))
	api.Handle("/api/v1/", httpapi.NewConfigurationHandler(httpapi.ConfigurationAPI{Profiles: configuration, Sources: configuration, Starters: starterSources()}))
	host := platform.Host{
		Address: "127.0.0.1:" + strconv.Itoa(port),
		Handler: httpapi.NewLocalHandler(api, assets),
		Browser: platform.SystemBrowser{},
	}
	return host.Run(ctx)
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

func defaultProfile() domain.UserProfile { return domain.UserProfile{ID: domain.LocalProfileID} }
func defaultRanking() domain.RankingConfiguration {
	return domain.RankingConfiguration{
		Recency: domain.SignalWeight{Enabled: true, Weight: .25}, Interest: domain.SignalWeight{Enabled: true, Weight: .25}, SourcePreference: domain.SignalWeight{Enabled: true, Weight: .1}, Behavior: domain.SignalWeight{Enabled: true, Weight: .1},
		Location: domain.SignalWeight{Weight: .05}, Age: domain.SignalWeight{Weight: .05}, Gender: domain.SignalWeight{Weight: .05}, TextSimilarity: domain.SignalWeight{Enabled: true, Weight: .15}, PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1",
	}
}
func starterSources() []domain.Source {
	return []domain.Source{{ID: "starter-example-feed", Name: "Example Feed", URL: "https://example.com/feed.xml", Kind: domain.SourceKindFeed, Enabled: true, ContentPermission: domain.ContentMetadataOnly, AdapterConfig: domain.AdapterConfiguration{Feed: &domain.FeedConfiguration{Format: domain.FeedFormatAuto}}, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyNotApplicable}}}
}

func configuredPort(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("NEWS_AGGREGATOR_PORT must be an integer from 1 to 65535")
	}
	return port, nil
}
