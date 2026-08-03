package integration_test

import (
	"context"
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
	result, err := adapter.Fetch(ctx, source, application.FetchCursor{})
	must(t, err)
	service := ingestion.Service{Articles: store.Articles(), Clock: ingestionClock{time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)}, NewID: func(fp string) domain.ArticleID { return domain.ArticleID("feed-" + fp[len(fp)-12:]) }}
	writes, err := service.Ingest(ctx, source, result.Items)
	must(t, err)
	if len(writes) != 1 || !writes[0].Inserted || len(result.Warnings) != 1 {
		t.Fatalf("writes=%+v warnings=%v", writes, result.Warnings)
	}
	source.RefreshETag, source.RefreshLastModified = result.NextCursor.ETag, result.NextCursor.LastModified
	must(t, store.Sources().Save(ctx, source))
	persisted, err := store.Sources().Get(ctx, source.ID)
	must(t, err)
	second, err := adapter.Fetch(ctx, persisted, application.FetchCursor{ETag: persisted.RefreshETag, LastModified: persisted.RefreshLastModified})
	must(t, err)
	if !second.Unchanged || len(second.Items) != 0 {
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
	stored, err := store.Articles().Get(ctx, writes[0].ArticleID)
	must(t, err)
	if stored.CanonicalURL != "https://publisher.example/articles/rss-1" || stored.FullContent != "" || stored.Excerpt != "RSS summary" {
		t.Fatalf("normalized permission result=%+v", stored)
	}
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
