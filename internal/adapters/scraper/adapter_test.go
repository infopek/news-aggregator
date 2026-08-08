package scraper

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

func TestFixtureExtractionAndPermission(t *testing.T) {
	body, err := os.ReadFile("../../../test/fixtures/scrapers/approved.html")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	requests := 0
	adapter := Adapter{Now: func() time.Time { return now }, Fetcher: fetcherFunc(func(context.Context, application.FetchRequest) (application.FetchResponse, error) {
		requests++
		return page(http.StatusOK, body, "https://fictional.invalid/news"), nil
	})}
	source := scraperSource(now.Add(-time.Hour), domain.ContentMetadataOnly)
	result, err := adapter.Fetch(context.Background(), source, application.FetchCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || len(result.Items) != 1 || result.Items[0].CanonicalURL != "https://fictional.invalid/articles/scraper-001" || result.Items[0].FullContent != "" {
		t.Fatalf("result=%+v requests=%d", result, requests)
	}
	source.ContentPermission = domain.ContentFullAllowed
	result, err = adapter.Fetch(context.Background(), source, application.FetchCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Items[0].FullContent, "Permitted full content") || strings.Contains(result.Items[0].FullContent, "<script") || strings.Contains(result.Items[0].FullContent, "malicious()") {
		t.Fatalf("content=%q", result.Items[0].FullContent)
	}
}

func TestPolicyAndRedirectFailClosedWithoutFetch(t *testing.T) {
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	calls := 0
	adapter := Adapter{Now: func() time.Time { return now }, Fetcher: fetcherFunc(func(context.Context, application.FetchRequest) (application.FetchResponse, error) {
		calls++
		return page(http.StatusOK, []byte("<html></html>"), "https://other.invalid/news"), nil
	})}
	for _, source := range []domain.Source{scraperSource(now.Add(-181*24*time.Hour), domain.ContentMetadataOnly), scraperSource(now.Add(time.Hour), domain.ContentMetadataOnly)} {
		if _, err := adapter.Fetch(context.Background(), source, application.FetchCursor{}); !errors.Is(err, ErrPolicyRefused) {
			t.Fatalf("error=%v", err)
		}
	}
	if calls != 0 {
		t.Fatalf("policy refusal made %d requests", calls)
	}
	if _, err := adapter.Fetch(context.Background(), scraperSource(now.Add(-time.Hour), domain.ContentMetadataOnly), application.FetchCursor{}); !errors.Is(err, ErrPolicyRefused) {
		t.Fatalf("redirect error=%v", err)
	}
}

func TestSelectorAndMissingStructureRejected(t *testing.T) {
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	adapter := Adapter{Now: func() time.Time { return now }, Fetcher: fetcherFunc(func(context.Context, application.FetchRequest) (application.FetchResponse, error) {
		return page(http.StatusOK, []byte("<html><p>article text only</p></html>"), "https://fictional.invalid/news"), nil
	})}
	if _, err := adapter.Fetch(context.Background(), scraperSource(now.Add(-time.Hour), domain.ContentMetadataOnly), application.FetchCursor{}); !errors.Is(err, ErrInvalidPage) {
		t.Fatalf("error=%v", err)
	}
	for _, invalid := range []string{"article > a", ".", "article."} {
		source := scraperSource(now.Add(-time.Hour), domain.ContentMetadataOnly)
		source.AdapterConfig.Scraper.ArticleSelector = invalid
		if _, err := adapter.Fetch(context.Background(), source, application.FetchCursor{}); !errors.Is(err, application.ErrInvalidInput) {
			t.Fatalf("selector %q error=%v", invalid, err)
		}
	}
}

func scraperSource(reviewed time.Time, permission domain.ContentPermission) domain.Source {
	return domain.Source{ID: "scraper", Name: "Fictional public page", URL: "https://fictional.invalid/news", Kind: domain.SourceKindScraper, Enabled: true, ContentPermission: permission, AdapterConfig: domain.AdapterConfiguration{Scraper: &domain.ScraperConfiguration{ArticleSelector: "article", TitleSelector: "h1", ExcerptSelector: ".excerpt", ContentSelector: ".content"}}, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyApproved, TermsURL: "https://fictional.invalid/terms", RobotsURL: "https://fictional.invalid/robots.txt", ReviewedAt: &reviewed, ReviewNotes: "Fixture-only review; public content allowed."}}
}
func page(status int, body []byte, final string) application.FetchResponse {
	return application.FetchResponse{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(body))), FinalURL: final, Headers: http.Header{}}
}
