// Package platform provides operating-system and process-lifecycle adapters.
package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultShutdownTimeout = 5 * time.Second
	defaultReadinessDelay  = 10 * time.Millisecond
)

type ListenerFactory func(network, address string) (net.Listener, error)

type BrowserLauncher interface {
	Open(string) error
}

type Clock interface {
	After(time.Duration) <-chan time.Time
}

type systemClock struct{}

func (systemClock) After(delay time.Duration) <-chan time.Time { return time.After(delay) }

// Host owns the local HTTP server lifecycle. Its dependencies are injectable
// so lifecycle tests never open a real browser or rely on process signals.
type Host struct {
	Address         string
	Handler         http.Handler
	Listen          ListenerFactory
	Browser         BrowserLauncher
	Clock           Clock
	HTTPClient      *http.Client
	Logger          *slog.Logger
	ShutdownTimeout time.Duration
	ReadinessDelay  time.Duration
}

// Run serves until ctx is cancelled or the server fails.
func (host Host) Run(ctx context.Context) error {
	if host.Listen == nil {
		host.Listen = net.Listen
	}
	if host.Clock == nil {
		host.Clock = systemClock{}
	}
	if host.HTTPClient == nil {
		host.HTTPClient = &http.Client{Timeout: time.Second}
	}
	if host.Logger == nil {
		host.Logger = slog.Default()
	}
	if host.Handler == nil {
		host.Handler = http.NotFoundHandler()
	}
	if host.ShutdownTimeout <= 0 {
		host.ShutdownTimeout = defaultShutdownTimeout
	}
	if host.ReadinessDelay <= 0 {
		host.ReadinessDelay = defaultReadinessDelay
	}

	if !isLoopbackAddress(host.Address) {
		return fmt.Errorf("refusing non-loopback listen address %q", host.Address)
	}
	listener, err := host.Listen("tcp", host.Address)
	if err != nil {
		return fmt.Errorf("listen on local address: %w", err)
	}
	if !isLoopback(listener.Addr()) {
		_ = listener.Close()
		return fmt.Errorf("refusing non-loopback listener %q", listener.Addr().String())
	}

	server := &http.Server{
		Handler:           host.Handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()

	localURL := listenerURL(listener.Addr())
	host.Logger.Info("local server listening", "url", localURL)
	if err := host.waitUntilReady(ctx, localURL, serveResult); err != nil {
		_ = server.Close()
		return err
	}
	if host.Browser != nil {
		if err := host.Browser.Open(localURL); err != nil {
			host.Logger.Warn("default browser unavailable", "error", "browser launch failed")
		}
	}

	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("local HTTP server stopped: %w", err)
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), host.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			_ = server.Close()
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		err := <-serveResult
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("local HTTP server stopped during shutdown: %w", err)
		}
		return nil
	}
}

func (host Host) waitUntilReady(ctx context.Context, localURL string, serveResult <-chan error) error {
	readinessURL, err := url.JoinPath(localURL, "/api/v1/health")
	if err != nil {
		return fmt.Errorf("create readiness URL: %w", err)
	}
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, readinessURL, nil)
		if err != nil {
			return fmt.Errorf("create readiness request: %w", err)
		}
		response, err := host.HTTPClient.Do(request)
		if err == nil {
			if isReady(response) {
				return nil
			}
		}
		select {
		case serverError := <-serveResult:
			return fmt.Errorf("local HTTP server failed before readiness: %w", serverError)
		case <-ctx.Done():
			return fmt.Errorf("startup cancelled: %w", ctx.Err())
		case <-host.Clock.After(host.ReadinessDelay):
		}
	}
}

func isReady(response *http.Response) bool {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return false
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4097))
	decoder.DisallowUnknownFields()
	var health struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := decoder.Decode(&health); err != nil || health.Status != "ready" || health.Version == "" {
		return false
	}
	return decoder.Decode(&struct{}{}) == io.EOF
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isLoopback(address net.Addr) bool {
	host, _, err := net.SplitHostPort(address.String())
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func listenerURL(address net.Addr) string {
	return (&url.URL{Scheme: "http", Host: address.String(), Path: "/"}).String()
}
