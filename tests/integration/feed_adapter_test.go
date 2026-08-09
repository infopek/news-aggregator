package integration_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/adapters/feeds"
	"github.com/infopek/news-aggregator/internal/adapters/httpfetch"
	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/application/ingestion"
	"github.com/infopek/news-aggregator/internal/domain"
)

type feedScript struct {
	bodies   []string
	calls    int
	requests []application.FetchRequest
}

type publicResolver struct{}

func (publicResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
}

type serverDialer struct{ address string }

func (d serverDialer) DialContext(ctx context.Context, network, _ string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, d.address)
}

func TestFeedAdapterControlledServerConditionalRequest(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			if r.Header.Get("If-None-Match") != `"server-v1"` || r.Header.Get("If-Modified-Since") != "Sun, 02 Aug 2026 12:00:00 GMT" {
				t.Errorf("conditional headers=%v", r.Header)
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("ETag", `"server-v1"`)
		w.Header().Set("Last-Modified", "Sun, 02 Aug 2026 12:00:00 GMT")
		_, _ = io.WriteString(w, `<rss version="2.0"><channel><item><guid>one</guid><title>One</title><link>/one</link></item></channel></rss>`)
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	must(t, err)
	fetcher, err := httpfetch.New(httpfetch.Config{UserAgent: "NewsAggregator integration test", Resolver: publicResolver{}, Dialer: serverDialer{address: parsed.Host}})
	must(t, err)
	adapter := feeds.Adapter{Fetcher: fetcher}
	source := feedSource("controlled-feed", "http://feed.example/feed")
	first, err := adapter.Fetch(context.Background(), source, application.FetchCursor{})
	must(t, err)
	second, err := adapter.Fetch(context.Background(), source, first.NextCursor)
	must(t, err)
	if calls != 2 || len(first.Items) != 1 || !second.Unchanged || len(second.Items) != 0 {
		t.Fatalf("calls=%d first=%+v second=%+v", calls, first, second)
	}
}

func (s *feedScript) Fetch(_ context.Context, request application.FetchRequest) (application.FetchResponse, error) {
	s.requests = append(s.requests, request)
	s.calls++
	if s.calls == 2 {
		return application.FetchResponse{StatusCode: http.StatusNotModified, Body: io.NopCloser(strings.NewReader("must not parse")), FinalURL: "https://publisher.example/feeds/rss.xml", Headers: http.Header{"Etag": {`"v1"`}}}, nil
	}
	return application.FetchResponse{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(s.bodies[0])), FinalURL: "https://publisher.example/feeds/rss.xml", Headers: http.Header{"Etag": {`"v1"`}, "Last-Modified": {"Sun, 02 Aug 2026 12:00:00 GMT"}}}, nil
}

func TestFeedAdapterToSQLiteConditionalLifecycle(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "feeds", "rss-namespaces.xml"))
	must(t, err)
	store, path := openStore(t)
	defer store.Close()
	ctx := context.Background()
	source := feedSource("rss-integration", "https://publisher.example/start")
	source.ContentPermission = domain.ContentMetadataOnly
	must(t, store.Sources().Save(ctx, source))
	script := &feedScript{bodies: []string{string(fixture)}}
	adapter := feeds.Adapter{Fetcher: script}
	runner := ingestion.Runner{Adapter: adapter, Sources: store.Sources(), Articles: store.Articles(), Transactions: store, Clock: ingestionClock{time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)}, NewID: func(fp string) domain.ArticleID { return domain.ArticleID("feed-" + fp[len(fp)-12:]) }}
	first, err := runner.Run(ctx, source.ID)
	must(t, err)
	if len(first.Writes) != 1 || !first.Writes[0].Inserted || len(first.Warnings) != 1 || first.Fetched != 2 {
		t.Fatalf("writes=%+v warnings=%v", first.Writes, first.Warnings)
	}
	persisted, err := store.Sources().Get(ctx, source.ID)
	must(t, err)
	if persisted.RefreshETag != `"v1"` || persisted.RefreshLastModified == "" {
		t.Fatalf("persisted cursor=%+v", persisted)
	}
	second, err := runner.Run(ctx, source.ID)
	must(t, err)
	if !second.Unchanged || len(second.Writes) != 0 {
		t.Fatalf("second=%+v", second)
	}
	if script.requests[1].ETag != `"v1"` || script.requests[1].LastModified == "" {
		t.Fatalf("conditional request=%+v", script.requests[1])
	}
	db := rawDB(t, path)
	defer db.Close()
	var count int
	must(t, db.QueryRow(`SELECT count(*) FROM articles`).Scan(&count))
	if count != 1 {
		t.Fatalf("304 performed article writes: count=%d", count)
	}
	stored, err := store.Articles().Get(ctx, first.Writes[0].ArticleID)
	must(t, err)
	if stored.CanonicalURL != "https://publisher.example/articles/rss-1" || stored.FullContent != "" || stored.Excerpt != "RSS summary" {
		t.Fatalf("normalized permission result=%+v", stored)
	}
}

type staticFeedAdapter struct{ result application.AdapterResult }

func (staticFeedAdapter) Kind() domain.SourceKind { return domain.SourceKindFeed }
func (a staticFeedAdapter) Fetch(context.Context, domain.Source, application.FetchCursor) (application.AdapterResult, error) {
	return a.result, nil
}

type blockingFeedAdapter struct {
	started chan struct{}
	release chan struct{}
	result  application.AdapterResult
}

func (blockingFeedAdapter) Kind() domain.SourceKind { return domain.SourceKindFeed }
func (a blockingFeedAdapter) Fetch(ctx context.Context, _ domain.Source, _ application.FetchCursor) (application.AdapterResult, error) {
	close(a.started)
	select {
	case <-a.release:
		return a.result, nil
	case <-ctx.Done():
		return application.AdapterResult{}, ctx.Err()
	}
}

func TestIngestionRunnerDoesNotResurrectOrOverwriteSource(t *testing.T) {
	t.Run("deleted during fetch", func(t *testing.T) {
		store, path := openStore(t)
		defer store.Close()
		ctx := context.Background()
		source := feedSource("delete-during-fetch", "https://publisher.example/feed")
		must(t, store.Sources().Save(ctx, source))
		adapter := blockingFeedAdapter{started: make(chan struct{}), release: make(chan struct{}), result: application.AdapterResult{Items: []application.AdapterItem{{ExternalID: "one", CanonicalURL: "/one", Title: "One"}}}}
		runner := ingestion.Runner{Adapter: adapter, Sources: store.Sources(), Articles: store.Articles(), Transactions: store, Clock: ingestionClock{time.Now().UTC()}, NewID: func(string) domain.ArticleID { return "deleted-article" }}
		done := make(chan error, 1)
		go func() { _, err := runner.Run(ctx, source.ID); done <- err }()
		<-adapter.started
		must(t, store.Sources().Delete(ctx, source.ID))
		close(adapter.release)
		if err := <-done; !errors.Is(err, application.ErrNotFound) {
			t.Fatalf("run error=%v, want not found", err)
		}
		if _, err := store.Sources().Get(ctx, source.ID); !errors.Is(err, application.ErrNotFound) {
			t.Fatalf("deleted source was restored: %v", err)
		}
		db := rawDB(t, path)
		defer db.Close()
		var articles int
		must(t, db.QueryRow(`SELECT count(*) FROM articles WHERE primary_source_id=?`, source.ID).Scan(&articles))
		if articles != 0 {
			t.Fatalf("articles committed for deleted source: %d", articles)
		}
	})

	t.Run("configuration edited during fetch", func(t *testing.T) {
		store, _ := openStore(t)
		defer store.Close()
		ctx := context.Background()
		source := feedSource("edit-during-fetch", "https://publisher.example/feed")
		must(t, store.Sources().Save(ctx, source))
		adapter := blockingFeedAdapter{started: make(chan struct{}), release: make(chan struct{}), result: application.AdapterResult{Unchanged: true, NextCursor: application.FetchCursor{ETag: `"new"`}}}
		runner := ingestion.Runner{Adapter: adapter, Sources: store.Sources(), Articles: store.Articles(), Transactions: store, Clock: ingestionClock{time.Now().UTC()}, NewID: func(string) domain.ArticleID { return "unused" }}
		done := make(chan error, 1)
		go func() { _, err := runner.Run(ctx, source.ID); done <- err }()
		<-adapter.started
		edited, err := store.Sources().Get(ctx, source.ID)
		must(t, err)
		edited.Name = "Edited while refreshing"
		must(t, store.Sources().Save(ctx, edited))
		close(adapter.release)
		must(t, <-done)
		persisted, err := store.Sources().Get(ctx, source.ID)
		must(t, err)
		if persisted.Name != edited.Name || persisted.RefreshETag != `"new"` {
			t.Fatalf("concurrent edit or ingestion state lost: %+v", persisted)
		}
	})
}

type failingSources struct {
	application.SourceIngestionRepository
}

func (f failingSources) UpdateIngestionState(context.Context, domain.SourceID, application.SourceIngestionState) error {
	return application.ErrUnavailable
}

type failingArticles struct {
	application.ArticleRepository
	calls *int
}

func (f failingArticles) Upsert(context.Context, domain.Article) (application.ArticleWriteResult, error) {
	*f.calls++
	return application.ArticleWriteResult{}, application.ErrUnavailable
}

func TestIngestionRunnerFailureAndTransactionSemantics(t *testing.T) {
	validItem := application.AdapterItem{ExternalID: "one", CanonicalURL: "/one", Title: "One", FullContent: "permitted"}
	for _, test := range []struct {
		name                    string
		item                    application.AdapterItem
		failArticle, failSource bool
	}{
		{"normalization failure", application.AdapterItem{CanonicalURL: "/invalid"}, false, false},
		{"article failure", validItem, true, false},
		{"source cursor save failure", validItem, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, path := openStore(t)
			defer store.Close()
			ctx := context.Background()
			source := feedSource(domain.SourceID("rollback-"+test.name), "https://publisher.example/feed")
			source.RefreshETag = `"old"`
			must(t, store.Sources().Save(ctx, source))
			var articleCalls int
			var articles application.ArticleRepository = store.Articles()
			if test.failArticle {
				articles = failingArticles{ArticleRepository: articles, calls: &articleCalls}
			}
			var sources application.SourceIngestionRepository = store.Sources()
			if test.failSource {
				sources = failingSources{SourceIngestionRepository: sources}
			}
			runner := ingestion.Runner{Adapter: staticFeedAdapter{result: application.AdapterResult{Items: []application.AdapterItem{test.item}, NextCursor: application.FetchCursor{ETag: `"new"`}}}, Sources: sources, Articles: articles, Transactions: store, Clock: ingestionClock{time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}, NewID: func(string) domain.ArticleID { return "rollback-article" }}
			if _, err := runner.Run(ctx, source.ID); err == nil {
				t.Fatal("failure path succeeded")
			}
			persisted, err := store.Sources().Get(ctx, source.ID)
			must(t, err)
			if persisted.RefreshETag != `"old"` {
				t.Fatalf("cursor advanced=%q", persisted.RefreshETag)
			}
			db := rawDB(t, path)
			defer db.Close()
			var count int
			must(t, db.QueryRow(`SELECT count(*) FROM articles`).Scan(&count))
			if count != 0 {
				t.Fatalf("articles committed=%d", count)
			}
		})
	}
	t.Run("304 skips article repository", func(t *testing.T) {
		store, _ := openStore(t)
		defer store.Close()
		ctx := context.Background()
		source := feedSource("unchanged-runner", "https://publisher.example/feed")
		must(t, store.Sources().Save(ctx, source))
		calls := 0
		runner := ingestion.Runner{Adapter: staticFeedAdapter{result: application.AdapterResult{Unchanged: true, NextCursor: application.FetchCursor{ETag: `"v2"`}}}, Sources: store.Sources(), Articles: failingArticles{ArticleRepository: store.Articles(), calls: &calls}, Transactions: store, Clock: ingestionClock{time.Now().UTC()}, NewID: func(string) domain.ArticleID { return "unused" }}
		result, err := runner.Run(ctx, source.ID)
		must(t, err)
		if !result.Unchanged || calls != 0 {
			t.Fatalf("result=%+v article calls=%d", result, calls)
		}
		persisted, err := store.Sources().Get(ctx, source.ID)
		must(t, err)
		if persisted.RefreshETag != `"v2"` {
			t.Fatalf("cursor=%q", persisted.RefreshETag)
		}
	})
}

func TestFeedFixtureCorpus(t *testing.T) {
	for _, test := range []struct {
		name      string
		want      int
		malformed bool
	}{{"rss-namespaces.xml", 1, false}, {"atom-namespaces.xml", 1, false}, {"malformed.xml", 0, true}} {
		t.Run(test.name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "feeds", test.name))
			must(t, err)
			script := &feedScript{bodies: []string{string(body)}}
			source := feedSource(domain.SourceID("fixture-"+test.name), "https://fixture.example/feed")
			source.AdapterConfig.Feed.Format = domain.FeedFormatAuto
			result, err := (feeds.Adapter{Fetcher: script}).Fetch(context.Background(), source, application.FetchCursor{})
			if test.malformed {
				if err == nil {
					t.Fatal("malformed fixture succeeded")
				}
				return
			}
			must(t, err)
			if len(result.Items) != test.want {
				t.Fatalf("items=%d", len(result.Items))
			}
		})
	}
}
