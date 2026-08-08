package newsapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type fetcherFunc func(context.Context, application.FetchRequest) (application.FetchResponse, error)

func (f fetcherFunc) Fetch(ctx context.Context, request application.FetchRequest) (application.FetchResponse, error) {
	return f(ctx, request)
}

type resolverFunc func(context.Context, domain.CredentialID, func([]byte) error) error

func (f resolverFunc) WithSecret(ctx context.Context, id domain.CredentialID, use func([]byte) error) error {
	return f(ctx, id, use)
}

func TestFixturePaginationAndScopedCredential(t *testing.T) {
	body, err := os.ReadFile("../../../test/fixtures/api-providers/page-1.json")
	if err != nil {
		t.Fatal(err)
	}
	sentinel := "sentinel-secret-never-returned"
	ref := domain.CredentialID("opaque-ref")
	adapter := Adapter{Credentials: resolverFunc(func(_ context.Context, got domain.CredentialID, use func([]byte) error) error {
		if got != ref {
			t.Fatal(got)
		}
		return use([]byte(sentinel))
	}), Fetcher: fetcherFunc(func(_ context.Context, request application.FetchRequest) (application.FetchResponse, error) {
		if !strings.Contains(request.URL, "cursor=cursor-001") || !strings.Contains(request.URL, "page_size=25") {
			t.Fatalf("URL=%s", request.URL)
		}
		if got := request.Headers["Authorization"]; len(got) != 1 || got[0] != "Bearer "+sentinel {
			t.Fatalf("authorization=%v", got)
		}
		return response(http.StatusOK, body), nil
	})}
	result, err := adapter.Fetch(context.Background(), apiSource(&ref), application.FetchCursor{Value: "cursor-001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "Fixture API item" || result.NextCursor.Value != "cursor-002" {
		t.Fatalf("result=%+v", result)
	}
}

func TestStatusShapeCursorAndCancellationFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		adapter := Adapter{Fetcher: fetcherFunc(func(context.Context, application.FetchRequest) (application.FetchResponse, error) {
			return response(status, nil), nil
		})}
		if _, err := adapter.Fetch(context.Background(), apiSource(nil), application.FetchCursor{}); err == nil {
			t.Fatalf("status %d accepted", status)
		}
	}
	adapter := Adapter{Fetcher: fetcherFunc(func(context.Context, application.FetchRequest) (application.FetchResponse, error) {
		value := response(http.StatusTooManyRequests, nil)
		value.Retryable, value.RetryAfter = true, 45*time.Second
		return value, nil
	})}
	_, err := adapter.Fetch(context.Background(), apiSource(nil), application.FetchCursor{})
	var rateLimit *RateLimitError
	if !errors.As(err, &rateLimit) || !rateLimit.Retryable || rateLimit.RetryAfter != 45*time.Second {
		t.Fatalf("rate limit error=%v", err)
	}
	for _, body := range []string{`{"provider":"wrong","items":[]}`, `{"provider":"fictional-official-api","nextCursor":"same","items":[]}`, `{"provider":"fictional-official-api","items":[],"unknown":true}`} {
		adapter := Adapter{Fetcher: fetcherFunc(func(context.Context, application.FetchRequest) (application.FetchResponse, error) {
			return response(http.StatusOK, []byte(body)), nil
		})}
		cursor := application.FetchCursor{}
		if strings.Contains(body, `"same"`) {
			cursor.Value = "same"
		}
		if _, err := adapter.Fetch(context.Background(), apiSource(nil), cursor); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("body %s error=%v", body, err)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	adapter = Adapter{Fetcher: fetcherFunc(func(ctx context.Context, _ application.FetchRequest) (application.FetchResponse, error) {
		return application.FetchResponse{}, ctx.Err()
	})}
	if _, err := adapter.Fetch(cancelled, apiSource(nil), application.FetchCursor{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
}

func TestSecretCannotEscapeFetcherError(t *testing.T) {
	const sentinel = "sentinel-secret-never-returned"
	ref := domain.CredentialID("opaque-ref")
	adapter := Adapter{
		Credentials: resolverFunc(func(_ context.Context, _ domain.CredentialID, use func([]byte) error) error {
			return use([]byte(sentinel))
		}),
		Fetcher: fetcherFunc(func(context.Context, application.FetchRequest) (application.FetchResponse, error) {
			return application.FetchResponse{}, errors.New(sentinel)
		}),
	}
	_, err := adapter.Fetch(context.Background(), apiSource(&ref), application.FetchCursor{})
	if err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("unsafe error=%v", err)
	}
}

func apiSource(ref *domain.CredentialID) domain.Source {
	return domain.Source{ID: "api", Name: "Fictional", URL: "https://fictional.invalid/v1/articles", Kind: domain.SourceKindAPI, Enabled: true, ContentPermission: domain.ContentMetadataOnly, CredentialRef: ref, AdapterConfig: domain.AdapterConfiguration{API: &domain.APIConfiguration{Provider: "fictional-official-api", PageSize: 25}}, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyNotApplicable}}
}
func response(status int, body []byte) application.FetchResponse {
	return application.FetchResponse{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(body))), FinalURL: "https://fictional.invalid/v1/articles"}
}
