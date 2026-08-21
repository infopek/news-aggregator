package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
	"github.com/infopek/news-aggregator/internal/httpapi"
)

type filterFeedReader struct{}

func (filterFeedReader) GetFeed(context.Context, application.FeedQuery) (application.FeedPage, error) {
	return application.FeedPage{}, nil
}

func (filterFeedReader) GetArticle(context.Context, domain.ArticleID) (application.ArticleDetail, error) {
	return application.ArticleDetail{}, application.ErrNotFound
}

func TestFeedFilterHTTPPersistsAcrossProcessStoreRestart(t *testing.T) {
	store, path := openStore(t)
	now := time.Date(2026, 8, 21, 13, 0, 0, 0, time.UTC)
	must(t, store.Profiles().Save(context.Background(), domain.UserProfile{ID: domain.LocalProfileID, UpdatedAt: now}))
	must(t, store.Sources().Save(context.Background(), serviceFeed("source-a", "https://example.com/filter-feed")))
	request := httptest.NewRequest(http.MethodPut, "/api/v1/feed-filter", bytes.NewBufferString(`{"sourceId":"source-a","read":"unread","savedOnly":true,"includeHidden":false,"searchQuery":"science"}`))
	response := httptest.NewRecorder()
	httpapi.NewAPIHandlerWithFeed("test", httpapi.ConfigurationAPI{}, httpapi.RefreshAPI{}, httpapi.FeedAPI{Service: filterFeedReader{}, Filters: store.FeedFilters(), Clock: &rankClock{now}}).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", response.Code, response.Body.String())
	}
	store.Close()

	restarted := reopenStore(t, path)
	defer restarted.Close()
	response = httptest.NewRecorder()
	httpapi.NewAPIHandlerWithFeed("test", httpapi.ConfigurationAPI{}, httpapi.RefreshAPI{}, httpapi.FeedAPI{Service: filterFeedReader{}, Filters: restarted.FeedFilters(), Clock: &rankClock{now}}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/feed-filter", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("load status=%d body=%s", response.Code, response.Body.String())
	}
	var got struct {
		SourceID, Read, SearchQuery string
		SavedOnly, IncludeHidden    bool
		UpdatedAt                   time.Time
	}
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.SourceID != "source-a" || got.Read != "unread" || !got.SavedOnly || got.SearchQuery != "science" || !got.UpdatedAt.Equal(now) {
		t.Fatalf("restored filter=%+v", got)
	}
}
