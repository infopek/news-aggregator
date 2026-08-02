package integration_test

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/infopek/news-aggregator/internal/adapters/httpfetch"
	"github.com/infopek/news-aggregator/internal/application"
)

// This integration-level matrix proves that the production transport rejects a
// blocked DNS result before its dialer can make any network connection.
func TestHTTPFetcherBlocksPrivateDNSResultBeforeDial(t *testing.T) {
	dialer := &neverDialer{}
	fetcher, err := httpfetch.New(httpfetch.Config{
		UserAgent: "NewsAggregator/1.0 (+local-app)",
		Resolver:  privateResolver{}, Dialer: dialer,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = fetcher.Fetch(context.Background(), application.FetchRequest{URL: "https://attacker-controlled.example/feed"})
	if !errors.Is(err, httpfetch.ErrBlockedAddress) {
		t.Fatalf("error = %v", err)
	}
	if dialer.called {
		t.Fatal("dialer called for blocked DNS result")
	}
}

type privateResolver struct{}

func (privateResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
}

type neverDialer struct{ called bool }

func (dialer *neverDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	dialer.called = true
	return nil, errors.New("must not dial")
}
