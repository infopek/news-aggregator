package httpfetch

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type staticResolver struct{ addresses []netip.Addr }

func (resolver staticResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return append([]netip.Addr(nil), resolver.addresses...), nil
}

type serverDialer struct{ address string }

func (dialer serverDialer) DialContext(ctx context.Context, network, _ string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, dialer.address)
}

func newServerFetcher(t *testing.T, handler http.Handler, mutate func(*Config)) (*Fetcher, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	config := Config{
		UserAgent: "NewsAggregator/1.0 (+local-app)", MaxBytes: 1024, MaxRawBytes: 1024,
		Resolver: staticResolver{[]netip.Addr{netip.MustParseAddr("93.184.216.34")}},
		Dialer:   serverDialer{strings.TrimPrefix(server.URL, "http://")},
	}
	if mutate != nil {
		mutate(&config)
	}
	fetcher, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return fetcher, server
}

func TestFetchConditionalHeadersUserAgentAndContentType(t *testing.T) {
	fetcher, _ := newServerFetcher(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("User-Agent"); got != "NewsAggregator/1.0 (+local-app)" {
			t.Errorf("User-Agent = %q", got)
		}
		if request.Header.Get("If-None-Match") != `"etag"` || request.Header.Get("If-Modified-Since") == "" {
			t.Error("conditional headers missing")
		}
		if request.Header.Get("Authorization") != "Bearer SENTINEL_AUTH" || request.Header.Get("Cookie") != "SENTINEL_COOKIE" {
			t.Error("direct-request credentials missing")
		}
		writer.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = io.WriteString(writer, "feed")
	}), nil)

	response, err := fetcher.Fetch(context.Background(), application.FetchRequest{
		URL: "http://news.example/feed?token=SENTINEL_QUERY", ETag: `"etag"`,
		LastModified: "Wed, 21 Oct 2015 07:28:00 GMT", AllowedContentTypes: []string{"application/rss+xml"},
		Headers: map[string][]string{"Authorization": {"Bearer SENTINEL_AUTH"}, "Cookie": {"SENTINEL_COOKIE"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	if string(body) != "feed" || response.FinalURL == "" {
		t.Fatalf("unexpected response: body=%q URL=%q", body, response.FinalURL)
	}
}

func TestCrossOriginRedirectStripsCredentials(t *testing.T) {
	var requestNumber int
	fetcher, _ := newServerFetcher(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestNumber++
		if requestNumber == 1 {
			http.Redirect(writer, request, "http://redirected.example/feed", http.StatusFound)
			return
		}
		if request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" {
			t.Error("credentials forwarded across origins")
		}
		_, _ = io.WriteString(writer, "ok")
	}), nil)
	_, err := fetcher.Fetch(context.Background(), application.FetchRequest{
		URL: "http://public.example/feed",
		Headers: map[string][]string{
			"Authorization": {"Bearer SENTINEL_AUTH"},
			"Cookie":        {"SENTINEL_COOKIE"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBlockedTargetMatrix(t *testing.T) {
	data, err := os.ReadFile("../../../test/fixtures/http/blocked-targets.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Address  string `json:"address"`
		Category string `json:"category"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		t.Run(test.Category+"/"+test.Address, func(t *testing.T) {
			if isPublic(netip.MustParseAddr(test.Address)) {
				t.Fatalf("%s classified public", test.Address)
			}
		})
	}
	if !isPublic(netip.MustParseAddr("8.8.8.8")) || !isPublic(netip.MustParseAddr("2606:4700:4700::1111")) {
		t.Fatal("known public addresses were blocked")
	}
}

func TestURLValidationRejectsUnsafeForms(t *testing.T) {
	fetcher, err := New(Config{UserAgent: "NewsAggregator/1.0", Resolver: staticResolver{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"file:///etc/passwd", "http://user:secret@example.com/", "http://", "//example.com", "http://example.com/#secret"} {
		_, gotErr := fetcher.Fetch(context.Background(), application.FetchRequest{URL: target})
		if !errors.Is(gotErr, ErrInvalidURL) {
			t.Errorf("Fetch(%q) error = %v", target, gotErr)
		}
	}
}

func TestConfigurationIsClampedToSafetyCaps(t *testing.T) {
	fetcher, err := New(Config{
		UserAgent: "NewsAggregator/1.0", MaxBytes: absoluteMaxBytes + 1,
		MaxRawBytes: absoluteMaxBytes + 1, PerSourcePacing: maxPacing + time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.maxBytes != absoluteMaxBytes || fetcher.maxRawBytes != absoluteMaxBytes || fetcher.pacing != maxPacing {
		t.Fatalf("caps = decoded:%d raw:%d pacing:%v", fetcher.maxBytes, fetcher.maxRawBytes, fetcher.pacing)
	}
}

func TestDialRejectsAnyNonPublicResolutionBeforeDial(t *testing.T) {
	dialer := &recordingDialer{}
	fetcher, err := New(Config{
		UserAgent: "NewsAggregator/1.0", Resolver: staticResolver{[]netip.Addr{
			netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("127.0.0.1"),
		}}, Dialer: dialer,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, gotErr := fetcher.Fetch(context.Background(), application.FetchRequest{URL: "http://rebind.example/"})
	if !errors.Is(gotErr, ErrBlockedAddress) || dialer.called {
		t.Fatalf("error=%v dialed=%v", gotErr, dialer.called)
	}
}

type recordingDialer struct{ called bool }

func (dialer *recordingDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	dialer.called = true
	return nil, errors.New("unexpected dial")
}

func TestRedirectToPrivateAndRedirectLoopAreBlocked(t *testing.T) {
	t.Run("private", func(t *testing.T) {
		var lookups int
		resolver := resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			lookups++
			if lookups == 1 {
				return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		})
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, "http://private.example/secret", http.StatusFound)
		}))
		defer server.Close()
		fetcher, _ := New(Config{UserAgent: "NewsAggregator/1.0", Resolver: resolver, Dialer: serverDialer{strings.TrimPrefix(server.URL, "http://")}})
		_, err := fetcher.Fetch(context.Background(), application.FetchRequest{URL: "http://public.example/"})
		if !errors.Is(err, ErrBlockedAddress) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("loop", func(t *testing.T) {
		fetcher, _ := newServerFetcher(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, "http://public.example/loop", http.StatusFound)
		}), func(config *Config) { config.MaxRedirects = 2 })
		_, err := fetcher.Fetch(context.Background(), application.FetchRequest{URL: "http://public.example/loop"})
		if !errors.Is(err, ErrRedirect) {
			t.Fatalf("error = %v", err)
		}
	})
}

type resolverFunc func(context.Context, string, string) ([]netip.Addr, error)

func (fn resolverFunc) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return fn(ctx, network, host)
}

func TestRawAndExpandedResponseLimits(t *testing.T) {
	tests := []struct {
		name       string
		compressed bool
		payload    []byte
		rawLimit   int64
		maxBytes   int64
	}{
		{name: "raw", payload: bytes.Repeat([]byte("x"), 65), rawLimit: 64, maxBytes: 128},
		{name: "expanded gzip", compressed: true, payload: bytes.Repeat([]byte("x"), 1024), rawLimit: 256, maxBytes: 64},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fetcher, _ := newServerFetcher(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/xml")
				if test.compressed {
					writer.Header().Set("Content-Encoding", "gzip")
					zipper := gzip.NewWriter(writer)
					_, _ = zipper.Write(test.payload)
					_ = zipper.Close()
					return
				}
				_, _ = writer.Write(test.payload)
			}), func(config *Config) { config.MaxRawBytes, config.MaxBytes = test.rawLimit, test.maxBytes })
			_, err := fetcher.Fetch(context.Background(), application.FetchRequest{URL: "http://public.example/", AllowedContentTypes: []string{"application/xml"}})
			if !errors.Is(err, ErrResponseTooLarge) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestTimeoutAndCancellationDuringRead(t *testing.T) {
	for _, test := range []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
	}{
		{name: "timeout", ctx: func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), 20*time.Millisecond)
		}},
		{name: "cancel", ctx: func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			started := make(chan struct{})
			fetcher, _ := newServerFetcher(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusOK)
				if flusher, ok := writer.(http.Flusher); ok {
					flusher.Flush()
				}
				close(started)
				<-time.After(200 * time.Millisecond)
				_, _ = io.WriteString(writer, "late")
			}), nil)
			ctx, cancel := test.ctx()
			defer cancel()
			if test.name == "cancel" {
				go func() { <-started; cancel() }()
			}
			_, err := fetcher.Fetch(ctx, application.FetchRequest{URL: "http://public.example/"})
			if err == nil || (!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestRetryAfterMetadataIsBounded(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		value string
		want  time.Duration
	}{
		{"120", 30 * time.Second},
		{"9223372036854775807", 30 * time.Second},
		{now.Add(10 * time.Second).Format(http.TimeFormat), 10 * time.Second},
		{"malformed", 0},
	} {
		got, retryable := retryMetadata(http.StatusTooManyRequests, test.value, now, 30*time.Second)
		if !retryable || got != test.want {
			t.Errorf("retryMetadata(%q) = %v, %v", test.value, got, retryable)
		}
	}
	if _, retryable := retryMetadata(http.StatusBadRequest, "10", now, time.Minute); retryable {
		t.Error("400 marked retryable")
	}
}

func TestFetchReturnsRetryMetadataWithoutRequiringSuccessContentType(t *testing.T) {
	fetcher, _ := newServerFetcher(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Retry-After", "120")
		writer.WriteHeader(http.StatusTooManyRequests)
	}), func(config *Config) { config.MaxRetryAfter = 30 * time.Second })
	response, err := fetcher.Fetch(context.Background(), application.FetchRequest{
		URL: "http://public.example/", AllowedContentTypes: []string{"application/rss+xml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !response.Retryable || response.RetryAfter != 30*time.Second {
		t.Fatalf("retry metadata = %v, %v", response.Retryable, response.RetryAfter)
	}
}

func TestPerSourcePacingIsConcurrentSafeAndIndependent(t *testing.T) {
	var mu sync.Mutex
	var waits []time.Duration
	now := time.Unix(100, 0)
	fetcher, _ := newServerFetcher(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(writer, "ok") }), func(config *Config) {
		config.PerSourcePacing = time.Second
		config.Now = func() time.Time { return now }
		config.Sleep = func(_ context.Context, delay time.Duration) error {
			mu.Lock()
			waits = append(waits, delay)
			mu.Unlock()
			return nil
		}
	})
	var group sync.WaitGroup
	for _, source := range []domain.SourceID{"a", "a", "b"} {
		group.Add(1)
		go func(source domain.SourceID) {
			defer group.Done()
			_, err := fetcher.Fetch(context.Background(), application.FetchRequest{URL: "http://public.example/", SourceID: source})
			if err != nil {
				t.Errorf("Fetch() error = %v", err)
			}
		}(source)
	}
	group.Wait()
	mu.Lock()
	defer mu.Unlock()
	var delayed int
	for _, wait := range waits {
		if wait == time.Second {
			delayed++
		}
	}
	if len(waits) != 3 || delayed != 1 {
		t.Fatalf("waits = %v", waits)
	}
}

func TestDiagnosticsDoNotLeakSecretsOrBodies(t *testing.T) {
	fetcher, _ := newServerFetcher(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(writer, "SENTINEL_RESPONSE_BODY")
	}), nil)
	_, err := fetcher.Fetch(context.Background(), application.FetchRequest{
		URL: "http://public.example/path?api_key=SENTINEL_QUERY", AllowedContentTypes: []string{"application/json"},
		Headers: map[string][]string{"Authorization": {"SENTINEL_AUTH"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	message := err.Error()
	for _, secret := range []string{"SENTINEL_QUERY", "SENTINEL_AUTH", "SENTINEL_RESPONSE_BODY", "api_key", "/path"} {
		if strings.Contains(message, secret) {
			t.Fatalf("error leaked %q: %s", secret, message)
		}
	}
}
