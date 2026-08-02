package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/adapters/credentials"
	"github.com/infopek/news-aggregator/internal/adapters/sqlite"
	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
	"github.com/infopek/news-aggregator/internal/httpapi"
)

type httpClock struct{}

func (httpClock) Now() time.Time { return time.Unix(100, 0).UTC() }

func TestConfigurationHTTPThroughTemporarySQLiteAndFakeVault(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, sqlite.Config{Path: filepath.Join(t.TempDir(), "api.db"), MigrationDir: migrations()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	vault := credentials.NewFake(func(_ context.Context, _ domain.CredentialID, use func([]byte) error) error {
		return use([]byte("opaque-test-secret"))
	})
	service := application.ConfigurationService{Profiles: store.Profiles(), Rankings: store.Rankings(), Sources: store.Sources(), Transactions: store, Credentials: vault, Clock: httpClock{}, CredentialReference: credentials.ReferenceForSource}
	ranking := domain.RankingConfiguration{Recency: domain.SignalWeight{Enabled: true, Weight: 1}, PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1"}
	if err := service.Initialize(ctx, domain.UserProfile{ID: domain.LocalProfileID}, ranking); err != nil {
		t.Fatal(err)
	}
	api := http.NewServeMux()
	api.Handle("GET /api/v1/health", httpapi.NewHealthHandler("test"))
	api.Handle("/api/v1/", httpapi.NewConfigurationHandler(httpapi.ConfigurationAPI{Profiles: service, Sources: service, NewID: func() string { return "integration-source" }}))
	server := httptest.NewServer(httpapi.NewLocalHandler(api, nil))
	defer server.Close()
	source := `{"name":"Integration feed","url":"https://example.com/feed","kind":"feed","enabled":true,"contentPermission":"metadata_only","adapterConfig":{"format":"auto"},"scraperPolicy":{"status":"not_applicable","termsUrl":null,"robotsUrl":null,"reviewedAt":null,"reviewNotes":null}}`
	for _, step := range []struct {
		method, path, body string
		status             int
	}{{"GET", "/api/v1/health", "", 200}, {"GET", "/api/v1/profile", "", 200}, {"GET", "/api/v1/ranking-config", "", 200}, {"POST", "/api/v1/sources", source, 201}, {"GET", "/api/v1/sources/integration-source", "", 200}, {"PUT", "/api/v1/sources/integration-source/credential", `{"secret":"HTTP-SECRET-SENTINEL"}`, 200}, {"DELETE", "/api/v1/sources/integration-source/credential", "", 200}, {"DELETE", "/api/v1/sources/integration-source", "", 204}, {"GET", "/api/v1/sources/integration-source", "", 404}, {"GET", "/api/v1/not-real", "", 404}} {
		req, err := http.NewRequest(step.method, server.URL+step.path, strings.NewReader(step.body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != step.status {
			res.Body.Close()
			t.Fatalf("%s %s = %d, want %d", step.method, step.path, res.StatusCode, step.status)
		}
		res.Body.Close()
	}
}
