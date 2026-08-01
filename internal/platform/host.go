// Package platform provides operating-system and process-lifecycle adapters.
package platform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodHead, localURL, nil)
		if err != nil {
			return fmt.Errorf("create readiness request: %w", err)
		}
		response, err := host.HTTPClient.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode < http.StatusInternalServerError {
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
