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
			response.WriteHeader(http.StatusNoContent)
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
			if request.Method == http.MethodHead {
				response.WriteHeader(http.StatusNoContent)
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
