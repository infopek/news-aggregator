package platform

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type immediateClock struct {
	calls atomic.Int32
}

func (clock *immediateClock) After(time.Duration) <-chan time.Time {
	clock.calls.Add(1)
	ready := make(chan time.Time, 1)
	ready <- time.Time{}
	return ready
}

type browserFunc func(string) error

func (function browserFunc) Open(target string) error { return function(target) }

type failOnceTransport struct {
	failed atomic.Bool
}

func (transport *failOnceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport.failed.CompareAndSwap(false, true) {
		return nil, errors.New("not ready yet")
	}
	return http.DefaultTransport.RoundTrip(request)
}

func TestHostBindsLoopbackWaitsForReadinessAndShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock := &immediateClock{}
	var requestedAddress string
	var requests atomic.Int32
	var browserCalls atomic.Int32
	host := Host{
		Address: "127.0.0.1:0",
		Listen: func(network, address string) (net.Listener, error) {
			requestedAddress = address
			listener, err := net.Listen(network, address)
			if err == nil && !isLoopback(listener.Addr()) {
				t.Fatalf("listener address %q is not loopback", listener.Addr())
			}
			return listener, err
		},
		Handler: http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			writeReady(response)
		}),
		Clock:      clock,
		HTTPClient: &http.Client{Transport: &failOnceTransport{}, Timeout: time.Second},
		Browser: browserFunc(func(target string) error {
			browserCalls.Add(1)
			if requests.Load() == 0 {
				t.Error("browser opened before a readiness response")
			}
			if !strings.HasPrefix(target, "http://127.0.0.1:") {
				t.Errorf("browser target %q is not loopback", target)
			}
			cancel()
			return errors.New("browser unavailable")
		}),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if err := host.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if requestedAddress != "127.0.0.1:0" {
		t.Fatalf("requested listen address = %q", requestedAddress)
	}
	if browserCalls.Load() != 1 {
		t.Fatalf("browser calls = %d, want 1", browserCalls.Load())
	}
	if clock.calls.Load() == 0 {
		t.Fatal("injected clock was not used for readiness retry")
	}
}

func TestHostRejectsNonLoopbackListener(t *testing.T) {
	host := Host{
		Address: "127.0.0.1:0",
		Listen: func(_, _ string) (net.Listener, error) {
			return net.Listen("tcp", "0.0.0.0:0")
		},
		Handler: http.NotFoundHandler(),
	}
	err := host.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "refusing non-loopback") {
		t.Fatalf("Run error = %v, want non-loopback rejection", err)
	}
}

func TestHostRejectsRequestedNonLoopbackBeforeListening(t *testing.T) {
	listenCalls := 0
	host := Host{
		Address: "0.0.0.0:8080",
		Listen: func(_, _ string) (net.Listener, error) {
			listenCalls++
			return nil, errors.New("must not be called")
		},
	}
	err := host.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "refusing non-loopback listen address") {
		t.Fatalf("Run error = %v, want requested-address rejection", err)
	}
	if listenCalls != 0 {
		t.Fatalf("listener factory calls = %d, want 0", listenCalls)
	}
}

func TestHostReturnsActionableListenError(t *testing.T) {
	host := Host{
		Address: "127.0.0.1:8080",
		Listen: func(_, _ string) (net.Listener, error) {
			return nil, errors.New("address already in use")
		},
	}
	err := host.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "listen on local address") || !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("Run error = %v", err)
	}
}

func TestHostGracefullyWaitsForInflightRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var once sync.Once
	host := Host{
		Address: "127.0.0.1:0",
		Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/api/v1/health" {
				writeReady(response)
				return
			}
			once.Do(func() { close(requestStarted) })
			<-releaseRequest
			response.WriteHeader(http.StatusNoContent)
		}),
		Browser: browserFunc(func(target string) error {
			go func() {
				response, err := http.Get(target)
				if err == nil {
					_ = response.Body.Close()
				}
			}()
			<-requestStarted
			cancel()
			go func() {
				time.Sleep(10 * time.Millisecond)
				close(releaseRequest)
			}()
			return nil
		}),
		ShutdownTimeout: time.Second,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if err := host.Run(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestHostDoesNotLaunchBrowserUntilCanonicalHealthIsReady(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "not found", status: http.StatusNotFound, body: `{"status":"ready","version":"0.1.0"}`},
		{name: "other 4xx", status: http.StatusForbidden, body: `{"status":"ready","version":"0.1.0"}`},
		{name: "malformed body", status: http.StatusOK, body: `{"status":`},
		{name: "wrong status", status: http.StatusOK, body: `{"status":"starting","version":"0.1.0"}`},
		{name: "missing version", status: http.StatusOK, body: `{"status":"ready"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			var browserCalls atomic.Int32
			var wrongPath atomic.Bool
			host := Host{
				Address: "127.0.0.1:0",
				Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
					if request.Method != http.MethodGet || request.URL.Path != "/api/v1/health" {
						wrongPath.Store(true)
					}
					response.Header().Set("Content-Type", "application/json")
					response.WriteHeader(test.status)
					_, _ = response.Write([]byte(test.body))
				}),
				Clock: &immediateClock{},
				Browser: browserFunc(func(string) error {
					browserCalls.Add(1)
					return nil
				}),
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			err := host.Run(ctx)
			if err == nil || !strings.Contains(err.Error(), "startup cancelled") {
				t.Fatalf("Run error = %v, want startup cancellation", err)
			}
			if browserCalls.Load() != 0 {
				t.Fatalf("browser calls = %d, want 0", browserCalls.Load())
			}
			if wrongPath.Load() {
				t.Fatal("readiness probe did not use GET /api/v1/health")
			}
		})
	}
}

func writeReady(response http.ResponseWriter) {
	response.Header().Set("Content-Type", "application/json")
	_, _ = response.Write([]byte(`{"status":"ready","version":"0.1.0"}`))
}
