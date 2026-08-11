package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/infopek/news-aggregator/internal/adapters/credentials"
	"github.com/infopek/news-aggregator/internal/adapters/feeds"
	"github.com/infopek/news-aggregator/internal/adapters/httpfetch"
	"github.com/infopek/news-aggregator/internal/adapters/newsapi"
	"github.com/infopek/news-aggregator/internal/adapters/scraper"
	"github.com/infopek/news-aggregator/internal/adapters/sqlite"
	"github.com/infopek/news-aggregator/internal/application"
	appfeed "github.com/infopek/news-aggregator/internal/application/feed"
	"github.com/infopek/news-aggregator/internal/application/ingestion"
	applibrary "github.com/infopek/news-aggregator/internal/application/library"
	appranking "github.com/infopek/news-aggregator/internal/application/ranking"
	"github.com/infopek/news-aggregator/internal/domain"
	"github.com/infopek/news-aggregator/internal/httpapi"
	"github.com/infopek/news-aggregator/internal/platform"
	"github.com/infopek/news-aggregator/internal/webassets"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

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
	migrationDir, err := materializeMigrations(dataDir)
	if err != nil {
		return errors.New("database migrations are unavailable")
	}
	store, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(dataDir, "news.db"), MigrationDir: migrationDir})
	if err != nil {
		return errors.New("local database is unavailable")
	}
	defer store.Close()
	vault := credentials.NewStore()
	configuration := application.ConfigurationService{Profiles: store.Profiles(), Rankings: store.Rankings(), Sources: store.Sources(), Transactions: store, Credentials: vault, Clock: systemClock{}, CredentialReference: credentials.ReferenceForSource}
	if err := configuration.Initialize(ctx, defaultProfile(), defaultRanking()); err != nil {
		return errors.New("local configuration could not be initialized")
	}
	fetcher, err := httpfetch.New(httpfetch.Config{UserAgent: "NewsAggregator/" + applicationVersion + " (local Windows application)"})
	if err != nil {
		return errors.New("ingestion fetcher could not be initialized")
	}
	recompute := &appranking.Recomputer{Articles: store.Articles(), Library: store.Libraries(), Profiles: store.Profiles(), Rankings: store.Rankings(), Results: store.Rankings(), Clock: systemClock{}}
	if err := recompute.Full(ctx); err != nil {
		return errors.New("ranking state could not be refreshed")
	}
	newRunner := func(adapter application.IngestionAdapter) *ingestion.Runner {
		return &ingestion.Runner{Adapter: adapter, Sources: store.Sources(), Articles: store.Articles(), Transactions: store, Clock: systemClock{}, NewID: articleID}
	}
	refresh := &ingestion.Coordinator{Refreshes: store.Refreshes(), Sources: store.Sources(), Clock: systemClock{}, NewID: refreshID, ProcessContext: ctx, MaxConcurrency: 4, Runners: map[domain.SourceKind]ingestion.SourceRunner{
		domain.SourceKindFeed:    applibrary.Runner{Base: newRunner(feeds.Adapter{Fetcher: fetcher}), Recompute: recompute},
		domain.SourceKindAPI:     applibrary.Runner{Base: newRunner(newsapi.Adapter{Fetcher: fetcher, Credentials: vault}), Recompute: recompute},
		domain.SourceKindScraper: applibrary.Runner{Base: newRunner(scraper.Adapter{Fetcher: fetcher}), Recompute: recompute},
	}}

	feedQueries := appfeed.Service{Articles: store.Articles(), Library: store.Libraries(), Rankings: store.Rankings()}
	libraryActions := applibrary.Service{Articles: store.Articles(), Library: store.Libraries(), Clock: systemClock{}, Recompute: recompute}
	rankingConfiguration := applibrary.Configuration{Base: configuration, Recompute: recompute}
	api := httpapi.NewAPIHandlerWithFeed(applicationVersion, httpapi.ConfigurationAPI{Profiles: rankingConfiguration, Sources: configuration, Starters: starterSources()}, httpapi.RefreshAPI{Service: refresh}, httpapi.FeedAPI{Service: feedQueries, Library: libraryActions})
	host := platform.Host{
		Address: "127.0.0.1:" + strconv.Itoa(port),
		Handler: httpapi.NewLocalHandler(api, assets),
		Browser: platform.SystemBrowser{},
	}
	err = host.Run(ctx)
	finalizeCtx, cancelFinalize := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFinalize()
	if finalizeErr := refresh.Finalize(finalizeCtx); finalizeErr != nil {
		return errors.Join(err, errors.New("refresh state could not be finalized"))
	}
	return err
}

func materializeMigrations(dataDir string) (string, error) {
	dir := filepath.Join(dataDir, "migrations")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	entries, err := fs.ReadDir(embeddedMigrations, "migrations")
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		contents, err := embeddedMigrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return "", err
		}
		target := filepath.Join(dir, entry.Name())
		existing, readErr := os.ReadFile(target)
		if readErr == nil && string(existing) == string(contents) {
			continue
		}
		if readErr == nil {
			return "", errors.New("installed migration differs from embedded migration")
		}
		if readErr != nil && !os.IsNotExist(readErr) {
			return "", readErr
		}
		if err := installMigrationAtomically(dir, target, contents); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func installMigrationAtomically(dir, target string, contents []byte) error {
	temporary, err := os.CreateTemp(dir, ".migration-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, target); err == nil {
		return nil
	}
	// On Windows rename does not replace an existing target. A concurrent
	// process may have won the race; accept only the identical complete file.
	existing, readErr := os.ReadFile(target)
	if readErr == nil && bytes.Equal(existing, contents) {
		return nil
	}
	return errors.New("migration installation failed")
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
func articleID(fingerprint string) domain.ArticleID {
	sum := sha256.Sum256([]byte(fingerprint))
	return domain.ArticleID("article-" + hex.EncodeToString(sum[:16]))
}
func refreshID() domain.RefreshRunID {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return ""
	}
	return domain.RefreshRunID("refresh-" + hex.EncodeToString(value[:]))
}

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
