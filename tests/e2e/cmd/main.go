package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
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

//go:embed fixture/*
var fixtureFiles embed.FS

type clock struct{}

func (clock) Now() time.Time { return time.Now().UTC() }

type publicResolver struct{}

func (publicResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
}

type fixtureDialer struct{ address string }

func (d fixtureDialer) DialContext(ctx context.Context, network, _ string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, d.address)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	root, dataDir, port := os.Getenv("NEWS_AGGREGATOR_E2E_ROOT"), os.Getenv("NEWS_AGGREGATOR_E2E_DATA"), os.Getenv("NEWS_AGGREGATOR_PORT")
	if root == "" || dataDir == "" || port == "" {
		return errors.New("NEWS_AGGREGATOR_E2E_ROOT, NEWS_AGGREGATOR_E2E_DATA, and NEWS_AGGREGATOR_PORT are required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fixtureListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	fixtureServer := &http.Server{Handler: fixtureHandler()}
	go fixtureServer.Serve(fixtureListener)
	defer fixtureServer.Shutdown(context.Background())

	store, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(dataDir, "news.db"), MigrationDir: filepath.Join(root, "cmd", "news-aggregator", "migrations")})
	if err != nil {
		return err
	}
	defer store.Close()
	vault := credentials.NewStore()
	configuration := application.ConfigurationService{Profiles: store.Profiles(), Rankings: store.Rankings(), Sources: store.Sources(), Transactions: store, Credentials: vault, Clock: clock{}, CredentialReference: credentials.ReferenceForSource}
	if err := configuration.Initialize(ctx, domain.UserProfile{ID: domain.LocalProfileID}, defaultRanking()); err != nil {
		return err
	}
	fetcher, err := httpfetch.New(httpfetch.Config{UserAgent: "NewsAggregator/e2e", Resolver: publicResolver{}, Dialer: fixtureDialer{fixtureListener.Addr().String()}})
	if err != nil {
		return err
	}
	gate, status := &appranking.VersionGate{}, &applibrary.RecomputeStatus{}
	recompute := &appranking.Recomputer{Articles: store.Articles(), Library: store.Libraries(), Profiles: store.Profiles(), Rankings: store.Rankings(), Results: store.Rankings(), Clock: clock{}, Gate: gate}
	if err := recompute.Full(ctx); err != nil {
		return err
	}
	newRunner := func(adapter application.IngestionAdapter) *ingestion.Runner {
		return &ingestion.Runner{Adapter: adapter, Sources: store.Sources(), Articles: store.Articles(), Transactions: store, Clock: clock{}, NewID: articleID}
	}
	refresh := &ingestion.Coordinator{Refreshes: store.Refreshes(), Sources: store.Sources(), Clock: clock{}, NewID: func() domain.RefreshRunID {
		return domain.RefreshRunID(fmt.Sprintf("refresh-%d", time.Now().UnixNano()))
	}, ProcessContext: ctx, MaxConcurrency: 4, Runners: map[domain.SourceKind]ingestion.SourceRunner{
		domain.SourceKindFeed:    applibrary.Runner{Base: newRunner(feeds.Adapter{Fetcher: fetcher}), Recompute: recompute, Gate: gate, Status: status},
		domain.SourceKindAPI:     applibrary.Runner{Base: newRunner(newsapi.Adapter{Fetcher: fetcher, Credentials: vault}), Recompute: recompute, Gate: gate, Status: status},
		domain.SourceKindScraper: applibrary.Runner{Base: newRunner(scraper.Adapter{Fetcher: fetcher}), Recompute: recompute, Gate: gate, Status: status},
	}}
	queries := appfeed.Service{Articles: store.Articles(), Library: store.Libraries(), Rankings: store.Rankings()}
	actions := applibrary.Service{Articles: store.Articles(), Library: store.Libraries(), Clock: clock{}, Recompute: recompute, Gate: gate, Status: status}
	rankingConfiguration := applibrary.Configuration{Base: configuration, Recompute: recompute, Gate: gate, Status: status}
	ids, nextID := []string{"starter-failure", "metadata-source", "full-source"}, 0
	newSourceID := func() string { id := ids[nextID]; nextID++; return id }
	api := httpapi.NewAPIHandlerWithFeed("e2e", httpapi.ConfigurationAPI{Profiles: rankingConfiguration, Sources: configuration, Starters: starterSources(), NewID: newSourceID}, httpapi.RefreshAPI{Service: refresh}, httpapi.FeedAPI{Service: queries, Library: actions})
	assets, err := fs.Sub(webassets.Files, "dist")
	if err != nil {
		return err
	}
	host := platform.Host{Address: "127.0.0.1:" + port, Handler: httpapi.NewLocalHandler(api, assets)}
	err = host.Run(ctx)
	finalize, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return errors.Join(err, refresh.Finalize(finalize))
}

func fixtureHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "failure.fixture.test" {
			http.Error(w, "fixture outage", http.StatusServiceUnavailable)
			return
		}
		name := "fixture/metadata.xml"
		if r.Host == "full.fixture.test" {
			name = "fixture/full.xml"
		}
		if r.URL.Path != "/feed.xml" {
			http.NotFound(w, r)
			return
		}
		body, err := fixtureFiles.ReadFile(name)
		if err != nil {
			http.Error(w, "fixture unavailable", 500)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		w.Header().Set("ETag", `"e2e-v1"`)
		_, _ = w.Write(body)
	})
}

func articleID(fingerprint string) domain.ArticleID {
	sum := sha256.Sum256([]byte(fingerprint))
	return domain.ArticleID("article-" + hex.EncodeToString(sum[:16]))
}
func starterSources() []domain.Source {
	return []domain.Source{{ID: "starter-failure", Name: "Unavailable fixture", URL: "http://failure.fixture.test/feed.xml", Kind: domain.SourceKindFeed, Enabled: true, ContentPermission: domain.ContentMetadataOnly, AdapterConfig: domain.AdapterConfiguration{Feed: &domain.FeedConfiguration{Format: domain.FeedFormatRSS}}, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyNotApplicable}}}
}
func defaultRanking() domain.RankingConfiguration {
	return domain.RankingConfiguration{Recency: domain.SignalWeight{Enabled: true, Weight: .25}, Interest: domain.SignalWeight{Enabled: true, Weight: .25}, SourcePreference: domain.SignalWeight{Enabled: true, Weight: .1}, Behavior: domain.SignalWeight{Enabled: true, Weight: .1}, Location: domain.SignalWeight{Weight: .05}, Age: domain.SignalWeight{Weight: .05}, Gender: domain.SignalWeight{Weight: .05}, TextSimilarity: domain.SignalWeight{Enabled: true, Weight: .15}, PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1"}
}
