package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	appfeed "github.com/infopek/news-aggregator/internal/application/feed"
	"github.com/infopek/news-aggregator/internal/domain"
	"github.com/infopek/news-aggregator/internal/httpapi"
)

func TestRankedFeedPaginationFiltersAndPermission(t *testing.T) {
	store, _ := openStore(t)
	defer store.Close()
	ctx := context.Background()
	at := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	for _, source := range []domain.Source{serviceFeed("source-a", "https://example.com/a"), serviceFeed("source-b", "https://example.com/b")} {
		must(t, store.Sources().Save(ctx, source))
	}
	articles := []domain.Article{
		{ID: "article-a", Fingerprint: "fp-a", SourceID: "source-a", CanonicalURL: "https://example.com/a/1", Title: "Alpha local news", FetchedAt: at, PublishedAt: &at, Excerpt: "first", ContentPermission: domain.ContentMetadataOnly},
		{ID: "article-b", Fingerprint: "fp-b", SourceID: "source-b", CanonicalURL: "https://example.com/b/1", Title: "Beta", FetchedAt: at, PublishedAt: &at, Excerpt: "second local", FullContent: "permitted body", ContentPermission: domain.ContentFullAllowed},
		{ID: "article-c", Fingerprint: "fp-c", SourceID: "source-a", CanonicalURL: "https://example.com/a/2", Title: "Gamma", FetchedAt: at.Add(-time.Hour), Excerpt: "third", ContentPermission: domain.ContentMetadataOnly},
	}
	for _, article := range articles {
		if _, err := store.Articles().Upsert(ctx, article); err != nil {
			t.Fatal(err)
		}
	}
	for id, score := range map[domain.ArticleID]float64{"article-a": .8, "article-b": .8, "article-c": .4} {
		must(t, store.Rankings().SaveResults(ctx, []domain.RankingResult{{ArticleID: id, Score: score, AlgorithmVersion: "test-v1", CalculatedAt: at, Contributions: []domain.ScoreContribution{{Signal: domain.SignalRecency, RawScore: score, Weight: 1, WeightedScore: score, ReasonCode: "RECENCY", ReasonValues: map[string]string{}}}}}))
	}
	mustState, err := store.Libraries().Apply(ctx, "article-b", domain.LibraryPatch{Saved: boolPointer(true)}, at)
	must(t, err)
	_ = mustState
	service := appfeed.Service{Articles: store.Articles(), Library: store.Libraries(), Rankings: store.Rankings()}
	first, err := service.GetFeed(ctx, application.FeedQuery{Limit: 1})
	must(t, err)
	if len(first.Articles) != 1 || first.Articles[0].Article.ID != "article-a" || first.NextCursor == "" {
		t.Fatalf("first=%+v", first)
	}
	second, err := service.GetFeed(ctx, application.FeedQuery{Limit: 1, Cursor: first.NextCursor})
	must(t, err)
	if len(second.Articles) != 1 || second.Articles[0].Article.ID != "article-b" || second.NextCursor == "" {
		t.Fatalf("second=%+v", second)
	}
	third, err := service.GetFeed(ctx, application.FeedQuery{Limit: 1, Cursor: second.NextCursor})
	must(t, err)
	if len(third.Articles) != 1 || third.Articles[0].Article.ID != "article-c" || third.NextCursor != "" {
		t.Fatalf("third=%+v", third)
	}
	if _, err := service.GetFeed(ctx, application.FeedQuery{Limit: 1, Cursor: "not-a-cursor"}); err != application.ErrInvalidInput {
		t.Fatalf("cursor error=%v", err)
	}
	saved := true
	filtered, err := service.GetFeed(ctx, application.FeedQuery{Limit: 30, Filter: application.FeedFilter{Saved: &saved, Text: "local"}})
	must(t, err)
	if len(filtered.Articles) != 1 || filtered.Articles[0].Article.ID != "article-b" {
		t.Fatalf("filtered=%+v", filtered)
	}
	_, err = store.Libraries().Apply(ctx, "article-c", domain.LibraryPatch{Read: boolPointer(true), Hidden: boolPointer(true)}, at)
	must(t, err)
	visible, err := service.GetFeed(ctx, application.FeedQuery{Limit: 30})
	must(t, err)
	if len(visible.Articles) != 2 {
		t.Fatalf("hidden article remained visible: %+v", visible)
	}
	read := true
	readHidden, err := service.GetFeed(ctx, application.FeedQuery{Limit: 30, Filter: application.FeedFilter{Read: &read, IncludeHidden: true}})
	must(t, err)
	if len(readHidden.Articles) != 1 || readHidden.Articles[0].Article.ID != "article-c" {
		t.Fatalf("read/hidden filter=%+v", readHidden)
	}
	after := at.Add(-30 * time.Minute)
	sourceAndDate, err := service.GetFeed(ctx, application.FeedQuery{Limit: 30, Filter: application.FeedFilter{SourceIDs: []domain.SourceID{"source-a"}, PublishedAfter: &after}})
	must(t, err)
	if len(sourceAndDate.Articles) != 1 || sourceAndDate.Articles[0].Article.ID != "article-a" {
		t.Fatalf("source/date filter=%+v", sourceAndDate)
	}

	handler := httpapi.NewFeedHandler(httpapi.FeedAPI{Service: service})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/articles/article-a", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("metadata status=%d body=%s", response.Code, response.Body.String())
	}
	var metadata struct {
		FullContent *string `json:"fullContent"`
	}
	must(t, json.Unmarshal(response.Body.Bytes(), &metadata))
	if metadata.FullContent != nil {
		t.Fatalf("metadata content leaked: %q", *metadata.FullContent)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/articles/article-b", nil))
	var full struct {
		FullContent *string `json:"fullContent"`
	}
	must(t, json.Unmarshal(response.Body.Bytes(), &full))
	if full.FullContent == nil || *full.FullContent != "permitted body" {
		t.Fatalf("full content=%v", full.FullContent)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/feed?limit=0&publishedAfter="+url.QueryEscape("bad"), nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid query status=%d", response.Code)
	}
}

func boolPointer(value bool) *bool { return &value }
