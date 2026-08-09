package integration_test

import (
	"context"
	"encoding/base64"
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

func TestRankedFeedMatchesRankingTieOrderAndRejectsStructuralCursors(t *testing.T) {
	store, _ := openStore(t)
	defer store.Close()
	ctx := context.Background()
	at := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	source := serviceFeed("tie-source", "https://example.com/ties")
	must(t, store.Sources().Save(ctx, source))
	published := at.Add(-24 * time.Hour)
	articles := []domain.Article{
		{ID: "published", Fingerprint: "tie-p", SourceID: source.ID, CanonicalURL: "https://example.com/ties/p", Title: "Published", PublishedAt: &published, FetchedAt: published, ContentPermission: domain.ContentMetadataOnly},
		{ID: "missing-a", Fingerprint: "tie-a", SourceID: source.ID, CanonicalURL: "https://example.com/ties/a", Title: "Missing A", FetchedAt: at.Add(-time.Hour), ContentPermission: domain.ContentMetadataOnly},
		{ID: "missing-z", Fingerprint: "tie-z", SourceID: source.ID, CanonicalURL: "https://example.com/ties/z", Title: "Missing Z", FetchedAt: at, ContentPermission: domain.ContentMetadataOnly},
		{ID: "unranked", Fingerprint: "tie-u", SourceID: source.ID, CanonicalURL: "https://example.com/ties/u", Title: "Unranked", FetchedAt: at.Add(time.Hour), ContentPermission: domain.ContentMetadataOnly},
	}
	for _, article := range articles {
		_, err := store.Articles().Upsert(ctx, article)
		must(t, err)
	}
	for _, id := range []domain.ArticleID{"published", "missing-a", "missing-z"} {
		must(t, store.Rankings().SaveResults(ctx, []domain.RankingResult{{ArticleID: id, Score: .7, AlgorithmVersion: "test-v1", CalculatedAt: at, Contributions: []domain.ScoreContribution{{Signal: domain.SignalInterest, RawScore: .7, Weight: 1, WeightedScore: .7, ReasonCode: "TIE", ReasonValues: map[string]string{}}}}}))
	}
	service := appfeed.Service{Articles: store.Articles(), Library: store.Libraries(), Rankings: store.Rankings()}
	var got []domain.ArticleID
	cursor := ""
	for {
		page, err := service.GetFeed(ctx, application.FeedQuery{Limit: 1, Cursor: cursor})
		must(t, err)
		if len(page.Articles) == 0 {
			break
		}
		got = append(got, page.Articles[0].Article.ID)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	want := []domain.ArticleID{"published", "missing-a", "missing-z"}
	if len(got) != len(want) {
		t.Fatalf("tie order=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tie order=%v want=%v", got, want)
		}
	}
	for _, raw := range []string{
		`{"publicationPresent":false,"time":0,"id":"x"}`,
		`{"score":0,"time":0,"id":"x"}`,
		`{"score":0,"publicationPresent":false,"id":"x"}`,
		`{"score":0,"publicationPresent":false,"time":0}`,
		`{"score":"zero","publicationPresent":false,"time":0,"id":"x"}`,
		`{"score":0,"publicationPresent":false,"time":0,"id":"x","extra":true}`,
	} {
		encoded := base64.RawURLEncoding.EncodeToString([]byte(raw))
		if _, err := service.GetFeed(ctx, application.FeedQuery{Limit: 1, Cursor: encoded}); err != application.ErrInvalidInput {
			t.Fatalf("cursor %s error=%v", raw, err)
		}
	}
}

func TestRankedFeedCursorUsesOneReadSnapshot(t *testing.T) {
	store, path := openStore(t)
	defer store.Close()
	writer := reopenStore(t, path)
	defer writer.Close()
	ctx := context.Background()
	at := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	source := serviceFeed("snapshot-source", "https://example.com/snapshot")
	must(t, store.Sources().Save(ctx, source))
	for _, article := range []domain.Article{
		{ID: "snapshot-a", Fingerprint: "snapshot-fp-a", SourceID: source.ID, CanonicalURL: "https://example.com/snapshot/a", Title: "A", FetchedAt: at, ContentPermission: domain.ContentMetadataOnly},
		{ID: "snapshot-b", Fingerprint: "snapshot-fp-b", SourceID: source.ID, CanonicalURL: "https://example.com/snapshot/b", Title: "B", FetchedAt: at, ContentPermission: domain.ContentMetadataOnly},
	} {
		_, err := store.Articles().Upsert(ctx, article)
		must(t, err)
	}
	saveRank := func(repository application.RankingRepository, id domain.ArticleID, score float64) error {
		return repository.SaveResults(ctx, []domain.RankingResult{{ArticleID: id, Score: score, AlgorithmVersion: "snapshot-v1", CalculatedAt: at, Contributions: []domain.ScoreContribution{{Signal: domain.SignalInterest, RawScore: score, Weight: 1, WeightedScore: score, ReasonCode: "SNAPSHOT", ReasonValues: map[string]string{}}}}})
	}
	must(t, saveRank(store.Rankings(), "snapshot-a", .8))
	must(t, saveRank(store.Rankings(), "snapshot-b", .7))
	var cursor string
	err := store.WithinTransaction(ctx, func(txctx context.Context) error {
		first, err := store.Articles().QueryFeed(txctx, application.FeedQuery{Limit: 1})
		if err != nil {
			return err
		}
		if len(first.Articles) != 1 || first.Articles[0].Article.ID != "snapshot-a" || first.Articles[0].Ranking.Score != .8 || first.NextCursor == "" {
			t.Fatalf("first snapshot page=%+v", first)
		}
		cursor = first.NextCursor
		updated := make(chan error, 1)
		go func() { updated <- saveRank(writer.Rankings(), "snapshot-a", .1) }()
		if err := <-updated; err != nil {
			return err
		}
		second, err := store.Articles().QueryFeed(txctx, application.FeedQuery{Limit: 1, Cursor: cursor})
		if err != nil {
			return err
		}
		if len(second.Articles) != 1 || second.Articles[0].Article.ID != "snapshot-b" {
			t.Fatalf("snapshot continuation=%+v", second)
		}
		return nil
	})
	must(t, err)
	afterUpdate, err := store.Articles().QueryFeed(ctx, application.FeedQuery{Limit: 1, Cursor: cursor})
	must(t, err)
	if len(afterUpdate.Articles) != 1 || afterUpdate.Articles[0].Article.ID != "snapshot-b" {
		t.Fatalf("unchanged article omitted after update: %+v", afterUpdate)
	}
}
