// Package httpfetch provides the sole bounded outbound HTTP boundary for ingestion adapters.
package httpfetch

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
)

const (
	defaultMaxBytes  = int64(4 << 20)
	absoluteMaxBytes = int64(32 << 20)
	defaultTimeout   = 15 * time.Second
	maxTimeout       = 60 * time.Second
	maxRedirects     = 5
	maxRetryAfter    = 15 * time.Minute
	maxPacing        = 5 * time.Minute
)

var (
	ErrInvalidURL       = errors.New("http fetch: invalid URL")
	ErrBlockedAddress   = errors.New("http fetch: destination address is not public")
	ErrResponseTooLarge = errors.New("http fetch: response exceeds byte limit")
	ErrContentType      = errors.New("http fetch: response content type is not allowed")
	ErrRedirect         = errors.New("http fetch: redirect policy rejected request")
)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type Dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type Config struct {
	UserAgent       string
	MaxBytes        int64
	MaxRawBytes     int64
	Timeout         time.Duration
	MaxRedirects    int
	MaxRetryAfter   time.Duration
	PerSourcePacing time.Duration
	Resolver        Resolver
	Dialer          Dialer
	Now             func() time.Time
	Sleep           func(context.Context, time.Duration) error
}

type Fetcher struct {
	client        *http.Client
	userAgent     string
	maxBytes      int64
	maxRawBytes   int64
	maxRetryAfter time.Duration
	pacing        time.Duration
	now           func() time.Time
	sleep         func(context.Context, time.Duration) error
	mu            sync.Mutex
	lastBySource  map[string]time.Time
}

func New(config Config) (*Fetcher, error) {
	if strings.TrimSpace(config.UserAgent) == "" {
		return nil, errors.New("http fetch: descriptive User-Agent is required")
	}
	if config.MaxBytes <= 0 {
		config.MaxBytes = defaultMaxBytes
	} else if config.MaxBytes > absoluteMaxBytes {
		config.MaxBytes = absoluteMaxBytes
	}
	if config.MaxRawBytes <= 0 {
		config.MaxRawBytes = config.MaxBytes
	} else if config.MaxRawBytes > absoluteMaxBytes {
		config.MaxRawBytes = absoluteMaxBytes
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultTimeout
	}
	if config.Timeout > maxTimeout {
		config.Timeout = maxTimeout
	}
	if config.MaxRedirects <= 0 || config.MaxRedirects > maxRedirects {
		config.MaxRedirects = maxRedirects
	}
	if config.MaxRetryAfter <= 0 || config.MaxRetryAfter > maxRetryAfter {
		config.MaxRetryAfter = maxRetryAfter
	}
	if config.PerSourcePacing < 0 {
		config.PerSourcePacing = 0
	} else if config.PerSourcePacing > maxPacing {
		config.PerSourcePacing = maxPacing
	}
	if config.Resolver == nil {
		config.Resolver = net.DefaultResolver
	}
	if config.Dialer == nil {
		config.Dialer = &net.Dialer{}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Sleep == nil {
		config.Sleep = sleepContext
	}

	f := &Fetcher{
		userAgent: config.UserAgent, maxBytes: config.MaxBytes, maxRawBytes: config.MaxRawBytes,
		maxRetryAfter: config.MaxRetryAfter, pacing: config.PerSourcePacing,
		now: config.Now, sleep: config.Sleep, lastBySource: make(map[string]time.Time),
	}
	transport := &http.Transport{
		Proxy: nil, DisableCompression: true,
		DialContext:         f.safeDial(config.Resolver, config.Dialer),
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: config.Timeout,
		MaxResponseHeaderBytes: 64 << 10,
	}
	f.client = &http.Client{Transport: transport, Timeout: config.Timeout}
	f.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= config.MaxRedirects {
			return fmt.Errorf("%w: redirect limit exceeded", ErrRedirect)
		}
		if err := validateURL(req.URL); err != nil {
			return err
		}
		previous := via[len(via)-1].URL
		if !sameOrigin(previous, req.URL) {
			req.Header.Del("Authorization")
			req.Header.Del("Proxy-Authorization")
			req.Header.Del("Cookie")
		}
		return nil
	}
	return f, nil
}

func (f *Fetcher) Fetch(ctx context.Context, request application.FetchRequest) (application.FetchResponse, error) {
	target, err := url.Parse(request.URL)
	if err != nil || validateURL(target) != nil {
		return application.FetchResponse{}, ErrInvalidURL
	}
	method := request.Method
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		return application.FetchResponse{}, errors.New("http fetch: unsupported method")
	}
	if err := f.pace(ctx, string(request.SourceID)); err != nil {
		return application.FetchResponse{}, err
	}
	if request.Timeout > 0 {
		timeout := request.Timeout
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), nil)
	if err != nil {
		return application.FetchResponse{}, ErrInvalidURL
	}
	copyHeaders(req.Header, request.Headers)
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept-Encoding", "gzip")
	if request.ETag != "" {
		req.Header.Set("If-None-Match", request.ETag)
	}
	if request.LastModified != "" {
		req.Header.Set("If-Modified-Since", request.LastModified)
	}

	response, err := f.client.Do(req)
	if err != nil {
		return application.FetchResponse{}, fmt.Errorf("http fetch %s: %w", safeTarget(target), sanitizeNetworkError(err))
	}
	defer response.Body.Close()

	if response.StatusCode >= 200 && response.StatusCode < 300 && response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusNotModified &&
		method != http.MethodHead && !contentTypeAllowed(response.Header.Get("Content-Type"), request.AllowedContentTypes) {
		return application.FetchResponse{}, fmt.Errorf("%w: %s", ErrContentType, safeTarget(target))
	}
	limit := request.MaxBytes
	if limit <= 0 || limit > f.maxBytes {
		limit = f.maxBytes
	}
	body, err := readBounded(response.Body, response.Header.Get("Content-Encoding"), f.maxRawBytes, limit)
	if err != nil {
		return application.FetchResponse{}, err
	}
	retryAfter, retryable := retryMetadata(response.StatusCode, response.Header.Get("Retry-After"), f.now(), f.maxRetryAfter)
	return application.FetchResponse{
		StatusCode: response.StatusCode, Headers: response.Header.Clone(), Body: io.NopCloser(bytes.NewReader(body)),
		FinalURL: response.Request.URL.String(), RetryAfter: retryAfter, Retryable: retryable,
	}, nil
}

func (f *Fetcher) safeDial(resolver Resolver, dialer Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, ErrInvalidURL
		}
		ips, err := resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve destination: %w", err)
		}
		if len(ips) == 0 {
			return nil, errors.New("resolve destination: no addresses")
		}
		for _, ip := range ips {
			if !isPublic(ip) {
				return nil, ErrBlockedAddress
			}
		}
		var lastErr error
		for _, ip := range ips {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		return nil, fmt.Errorf("connect destination: %w", lastErr)
	}
}

func (f *Fetcher) pace(ctx context.Context, source string) error {
	if source == "" || f.pacing <= 0 {
		return nil
	}
	f.mu.Lock()
	now := f.now()
	wait := f.lastBySource[source].Add(f.pacing).Sub(now)
	if wait < 0 {
		wait = 0
	}
	f.lastBySource[source] = now.Add(wait)
	f.mu.Unlock()
	return f.sleep(ctx, wait)
}

func validateURL(target *url.URL) error {
	if target == nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" || target.User != nil {
		return ErrInvalidURL
	}
	if target.Fragment != "" {
		return ErrInvalidURL
	}
	return nil
}

func isPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified() && ip.IsGlobalUnicast() &&
		!isReserved(ip)
}

func isReserved(ip netip.Addr) bool {
	for _, prefix := range reservedPrefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

var reservedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2001:2::/48"),
}

func copyHeaders(destination http.Header, source map[string][]string) {
	for name, values := range source {
		canonical := http.CanonicalHeaderKey(name)
		if canonical == "Host" {
			continue
		}
		for _, value := range values {
			destination.Add(canonical, value)
		}
	}
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}

func contentTypeAllowed(value string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	for _, candidate := range allowed {
		if mediaType == strings.ToLower(strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func readBounded(raw io.Reader, encoding string, rawLimit, decodedLimit int64) ([]byte, error) {
	rawReader := &limitReader{reader: raw, remaining: rawLimit + 1}
	var reader io.Reader = rawReader
	var closer io.Closer
	if strings.EqualFold(strings.TrimSpace(encoding), "gzip") {
		gzipReader, err := gzip.NewReader(rawReader)
		if err != nil {
			return nil, errors.New("http fetch: invalid gzip response")
		}
		reader, closer = gzipReader, gzipReader
	} else if strings.TrimSpace(encoding) != "" && !strings.EqualFold(strings.TrimSpace(encoding), "identity") {
		return nil, errors.New("http fetch: unsupported content encoding")
	}
	if closer != nil {
		defer closer.Close()
	}
	data, err := io.ReadAll(io.LimitReader(reader, decodedLimit+1))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		if errors.Is(err, ErrResponseTooLarge) {
			return nil, ErrResponseTooLarge
		}
		return nil, errors.New("http fetch: response read failed")
	}
	if int64(len(data)) > decodedLimit || rawReader.remaining <= 0 {
		return nil, ErrResponseTooLarge
	}
	return data, nil
}

type limitReader struct {
	reader    io.Reader
	remaining int64
}

func (reader *limitReader) Read(buffer []byte) (int, error) {
	if reader.remaining <= 0 {
		return 0, ErrResponseTooLarge
	}
	if int64(len(buffer)) > reader.remaining {
		buffer = buffer[:reader.remaining]
	}
	n, err := reader.reader.Read(buffer)
	reader.remaining -= int64(n)
	return n, err
}

func retryMetadata(status int, value string, now time.Time, cap time.Duration) (time.Duration, bool) {
	if status != http.StatusTooManyRequests && status != http.StatusRequestTimeout && (status < 500 || status > 599) {
		return 0, false
	}
	delay := time.Duration(0)
	if seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && seconds >= 0 {
		if seconds > int64(cap/time.Second) {
			delay = cap
		} else {
			delay = time.Duration(seconds) * time.Second
		}
	} else if when, err := http.ParseTime(value); err == nil && when.After(now) {
		delay = when.Sub(now)
	}
	if delay > cap {
		delay = cap
	}
	return delay, true
}

func safeTarget(target *url.URL) string { return target.Scheme + "://" + target.Host }

func sanitizeNetworkError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, ErrBlockedAddress) {
		return ErrBlockedAddress
	}
	if errors.Is(err, ErrRedirect) {
		return ErrRedirect
	}
	if errors.Is(err, ErrInvalidURL) {
		return ErrInvalidURL
	}
	return errors.New("network request failed")
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

var _ application.HTTPFetcher = (*Fetcher)(nil)
