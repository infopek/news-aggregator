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
	appfeed "github.com/infopek/news-aggregator/internal/application/feed"
	applibrary "github.com/infopek/news-aggregator/internal/application/library"
	appranking "github.com/infopek/news-aggregator/internal/application/ranking"
	"github.com/infopek/news-aggregator/internal/domain"
	"github.com/infopek/news-aggregator/internal/httpapi"
)

type rankClock struct{ now time.Time }

func (c *rankClock) Now() time.Time { return c.now }

type flakyArticleRecompute struct {
	base     *appranking.Recomputer
	failures int
}

func (r *flakyArticleRecompute) Article(ctx context.Context, id domain.ArticleID) error {
	if r.failures > 0 {
		r.failures--
		return application.ErrUnavailable
	}
	return r.base.Article(ctx, id)
}

func TestLibraryTransitionsRecomputeTargetAndSurviveRestart(t *testing.T) {
	store, path := openStore(t)
	ctx := context.Background()
	clock := &rankClock{now: time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)}
	profile := domain.UserProfile{ID: domain.LocalProfileID, UpdatedAt: clock.now}
	must(t, store.Profiles().Save(ctx, profile))
	configuration := domain.RankingConfiguration{Recency: domain.SignalWeight{Enabled: true, Weight: .5}, Behavior: domain.SignalWeight{Enabled: true, Weight: .5}, PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1"}
	must(t, store.Rankings().SaveConfiguration(ctx, configuration))
	source := serviceFeed("library-source", "https://example.com/library")
	must(t, store.Sources().Save(ctx, source))
	for _, id := range []domain.ArticleID{"library-a", "library-b"} {
		article := domain.Article{ID: id, Fingerprint: "fp-" + string(id), SourceID: source.ID, CanonicalURL: "https://example.com/" + string(id), Title: string(id), PublishedAt: &clock.now, FetchedAt: clock.now, ContentPermission: domain.ContentMetadataOnly}
		_, err := store.Articles().Upsert(ctx, article)
		must(t, err)
	}
	gate := &appranking.VersionGate{}
	recompute := &appranking.Recomputer{Articles: store.Articles(), Library: store.Libraries(), Profiles: store.Profiles(), Rankings: store.Rankings(), Results: store.Rankings(), Clock: clock, Gate: gate}
	must(t, recompute.Full(ctx))
	baselineB, err := store.Rankings().GetResult(ctx, "library-b")
	must(t, err)
	service := applibrary.Service{Articles: store.Articles(), Library: store.Libraries(), Clock: clock, Recompute: recompute, Gate: gate}
	clock.now = clock.now.Add(time.Hour)
	read := true
	first, err := service.UpdateLibraryState(ctx, application.UpdateLibraryStateCommand{ArticleID: "library-a", Patch: domain.LibraryPatch{Read: &read}})
	must(t, err)
	if first.ReadAt == nil {
		t.Fatal("read action was not persisted")
	}
	repeated, err := service.UpdateLibraryState(ctx, application.UpdateLibraryStateCommand{ArticleID: "library-a", Patch: domain.LibraryPatch{Read: &read}})
	must(t, err)
	if repeated.ReadAt == nil || !repeated.ReadAt.Equal(*first.ReadAt) {
		t.Fatalf("repeated action changed timestamp: first=%v repeated=%v", first.ReadAt, repeated.ReadAt)
	}
	afterB, err := store.Rankings().GetResult(ctx, "library-b")
	must(t, err)
	if !afterB.CalculatedAt.Equal(baselineB.CalculatedAt) {
		t.Fatalf("targeted update recalculated unrelated article: before=%v after=%v", baselineB.CalculatedAt, afterB.CalculatedAt)
	}
	saved, hidden := true, true
	state, err := service.UpdateLibraryState(ctx, application.UpdateLibraryStateCommand{ArticleID: "library-a", Patch: domain.LibraryPatch{Saved: &saved, Hidden: &hidden}})
	must(t, err)
	if state.SavedAt == nil || state.HiddenAt == nil {
		t.Fatalf("hide then save state=%+v", state)
	}
	if _, err := store.Rankings().GetResult(ctx, "library-a"); err != application.ErrNotFound {
		t.Fatalf("hidden ranking remained: %v", err)
	}
	hidden = false
	state, err = service.UpdateLibraryState(ctx, application.UpdateLibraryStateCommand{ArticleID: "library-a", Patch: domain.LibraryPatch{Hidden: &hidden}})
	must(t, err)
	if state.HiddenAt != nil || state.SavedAt == nil {
		t.Fatalf("restore lost independent state: %+v", state)
	}
	if _, err := store.Rankings().GetResult(ctx, "library-a"); err != nil {
		t.Fatalf("restored article was not reranked: %v", err)
	}

	handler := httpapi.NewFeedHandler(httpapi.FeedAPI{Service: appfeed.Service{Articles: store.Articles(), Library: store.Libraries(), Rankings: store.Rankings()}, Library: service})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/v1/articles/library-a/library-state", bytes.NewBufferString(`{"read":false}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		ReadAt  *time.Time `json:"readAt"`
		SavedAt *time.Time `json:"savedAt"`
	}
	must(t, json.Unmarshal(response.Body.Bytes(), &body))
	if body.ReadAt != nil || body.SavedAt == nil {
		t.Fatalf("patch response=%+v", body)
	}
	store.Close()
	store = reopenStore(t, path)
	defer store.Close()
	persisted, err := store.Libraries().Get(ctx, "library-a")
	must(t, err)
	if persisted.ReadAt != nil || persisted.SavedAt == nil || persisted.HiddenAt != nil {
		t.Fatalf("restart state=%+v", persisted)
	}
}

func TestCancelledRecomputePreservesPriorResult(t *testing.T) {
	store, _ := openStore(t)
	defer store.Close()
	ctx := context.Background()
	clock := &rankClock{now: time.Now().UTC()}
	must(t, store.Profiles().Save(ctx, domain.UserProfile{ID: domain.LocalProfileID, UpdatedAt: clock.now}))
	must(t, store.Rankings().SaveConfiguration(ctx, domain.RankingConfiguration{Recency: domain.SignalWeight{Enabled: true, Weight: 1}, PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1"}))
	source := serviceFeed("cancel-rank-source", "https://example.com/cancel-rank")
	must(t, store.Sources().Save(ctx, source))
	article := domain.Article{ID: "cancel-rank", Fingerprint: "cancel-rank-fp", SourceID: source.ID, CanonicalURL: "https://example.com/cancel-rank/article", Title: "Cancel", PublishedAt: &clock.now, FetchedAt: clock.now, ContentPermission: domain.ContentMetadataOnly}
	_, err := store.Articles().Upsert(ctx, article)
	must(t, err)
	r := &appranking.Recomputer{Articles: store.Articles(), Library: store.Libraries(), Profiles: store.Profiles(), Rankings: store.Rankings(), Results: store.Rankings(), Clock: clock, Gate: &appranking.VersionGate{}}
	must(t, r.Full(ctx))
	before, err := store.Rankings().GetResult(ctx, article.ID)
	must(t, err)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := r.Article(cancelled, article.ID); err != context.Canceled {
		t.Fatalf("cancel error=%v", err)
	}
	after, err := store.Rankings().GetResult(ctx, article.ID)
	must(t, err)
	if !after.CalculatedAt.Equal(before.CalculatedAt) || after.Score != before.Score {
		t.Fatalf("cancelled recompute overwrote result: before=%+v after=%+v", before, after)
	}
}

func TestLibraryMutationRecomputeFailureIsIdempotentlyRetryable(t *testing.T) {
	store, _ := openStore(t)
	defer store.Close()
	ctx := context.Background()
	clock := &rankClock{now: time.Now().UTC()}
	must(t, store.Profiles().Save(ctx, domain.UserProfile{ID: domain.LocalProfileID, UpdatedAt: clock.now}))
	must(t, store.Rankings().SaveConfiguration(ctx, domain.RankingConfiguration{Recency: domain.SignalWeight{Enabled: true, Weight: 1}, PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1"}))
	source := serviceFeed("retry-source", "https://example.com/retry")
	must(t, store.Sources().Save(ctx, source))
	article := domain.Article{ID: "retry-article", Fingerprint: "retry-fp", SourceID: source.ID, CanonicalURL: "https://example.com/retry/article", Title: "Retry", PublishedAt: &clock.now, FetchedAt: clock.now, ContentPermission: domain.ContentMetadataOnly}
	_, err := store.Articles().Upsert(ctx, article)
	must(t, err)
	gate := &appranking.VersionGate{}
	base := &appranking.Recomputer{Articles: store.Articles(), Library: store.Libraries(), Profiles: store.Profiles(), Rankings: store.Rankings(), Results: store.Rankings(), Clock: clock, Gate: gate}
	must(t, base.Full(ctx))
	flaky := &flakyArticleRecompute{base: base, failures: 1}
	service := applibrary.Service{Articles: store.Articles(), Library: store.Libraries(), Clock: clock, Recompute: flaky, Gate: gate}
	saved := true
	if _, err := service.UpdateLibraryState(ctx, application.UpdateLibraryStateCommand{ArticleID: article.ID, Patch: domain.LibraryPatch{Saved: &saved}}); err != application.ErrUnavailable {
		t.Fatalf("first error=%v", err)
	}
	persisted, err := store.Libraries().Get(ctx, article.ID)
	must(t, err)
	if persisted.SavedAt == nil {
		t.Fatal("mutation was lost with recompute failure")
	}
	retried, err := service.UpdateLibraryState(ctx, application.UpdateLibraryStateCommand{ArticleID: article.ID, Patch: domain.LibraryPatch{Saved: &saved}})
	must(t, err)
	if retried.SavedAt == nil || !retried.SavedAt.Equal(*persisted.SavedAt) {
		t.Fatalf("retry was not idempotent: before=%+v after=%+v", persisted, retried)
	}
}
