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

type blockingLibraryRepository struct {
	application.LibraryRepository
	entered chan struct{}
	release chan struct{}
}

func (r *blockingLibraryRepository) Apply(ctx context.Context, id domain.ArticleID, patch domain.LibraryPatch, at time.Time) (domain.LibraryState, error) {
	state, err := r.LibraryRepository.Apply(ctx, id, patch, at)
	if err == nil && patch.Hidden != nil && !*patch.Hidden {
		close(r.entered)
		<-r.release
	}
	return state, err
}

func (r *flakyArticleRecompute) Article(ctx context.Context, id domain.ArticleID) error {
	if r.failures > 0 {
		r.failures--
		return application.ErrUnavailable
	}
	return r.base.Article(ctx, id)
}

func (r *flakyArticleRecompute) ArticleMutation(ctx context.Context, id domain.ArticleID, transactions application.TransactionManager, mutation func(context.Context) error) error {
	if r.failures > 0 {
		r.failures--
		return transactions.WithinTransaction(ctx, func(txctx context.Context) error {
			if err := mutation(txctx); err != nil {
				return err
			}
			return application.ErrUnavailable
		})
	}
	return r.base.ArticleMutation(ctx, id, transactions, mutation)
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
	service := applibrary.Service{Articles: store.Articles(), Library: store.Libraries(), Clock: clock, Recompute: recompute, Gate: gate, Transactions: store}
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
	if _, err := store.Rankings().GetResult(ctx, "library-a"); err != nil {
		t.Fatalf("hidden article lost its restorable ranking: %v", err)
	}
	hiddenPage, err := (appfeed.Service{Articles: store.Articles(), Library: store.Libraries(), Rankings: store.Rankings()}).GetFeed(ctx, application.FeedQuery{Limit: 30, Filter: application.FeedFilter{IncludeHidden: true}})
	must(t, err)
	if len(hiddenPage.Articles) != 2 || hiddenPage.Articles[1].Article.ID != "library-a" || hiddenPage.Articles[1].Library.HiddenAt == nil {
		t.Fatalf("hidden article is not available for restore: %+v", hiddenPage.Articles)
	}
	hiddenResult, err := store.Rankings().GetResult(ctx, "library-a")
	must(t, err)
	if hiddenResult.Score != 0 || hiddenResult.Contributions[0].ReasonCode != appranking.ReasonArticleHidden {
		t.Fatalf("hidden result is not an explicit current exclusion: %+v", hiddenResult)
	}
	clock.now = clock.now.Add(time.Hour)
	configuration.NormalizationVersion = "v2"
	must(t, store.Rankings().SaveConfiguration(ctx, configuration))
	must(t, recompute.Full(ctx))
	hiddenResult, err = store.Rankings().GetResult(ctx, "library-a")
	must(t, err)
	if hiddenResult.AlgorithmVersion != appranking.CombinedAlgorithmVersion+"+v2" || !hiddenResult.CalculatedAt.Equal(clock.now) {
		t.Fatalf("hidden result was not refreshed with current ranking inputs: %+v", hiddenResult)
	}
	hidden = false
	clock.now = clock.now.Add(time.Hour)
	state, err = service.UpdateLibraryState(ctx, application.UpdateLibraryStateCommand{ArticleID: "library-a", Patch: domain.LibraryPatch{Hidden: &hidden}})
	must(t, err)
	if state.HiddenAt != nil || state.SavedAt == nil {
		t.Fatalf("restore lost independent state: %+v", state)
	}
	restoredResult, err := store.Rankings().GetResult(ctx, "library-a")
	if err != nil {
		t.Fatalf("restored article was not reranked: %v", err)
	}
	if !restoredResult.CalculatedAt.Equal(clock.now) || restoredResult.Score == 0 || restoredResult.Contributions[0].ReasonCode == appranking.ReasonArticleHidden {
		t.Fatalf("restore exposed a stale hidden result: %+v", restoredResult)
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

func TestRestoreDoesNotDeadlockWithConcurrentFullRecompute(t *testing.T) {
	store, _ := openStore(t)
	defer store.Close()
	ctx := context.Background()
	clock := &rankClock{now: time.Now().UTC()}
	must(t, store.Profiles().Save(ctx, domain.UserProfile{ID: domain.LocalProfileID, UpdatedAt: clock.now}))
	must(t, store.Rankings().SaveConfiguration(ctx, domain.RankingConfiguration{Recency: domain.SignalWeight{Enabled: true, Weight: 1}, PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1"}))
	source := serviceFeed("concurrent-restore-source", "https://example.com/concurrent-restore")
	must(t, store.Sources().Save(ctx, source))
	article := domain.Article{ID: "concurrent-restore", Fingerprint: "concurrent-restore-fp", SourceID: source.ID, CanonicalURL: "https://example.com/concurrent-restore/article", Title: "Concurrent restore", PublishedAt: &clock.now, FetchedAt: clock.now, ContentPermission: domain.ContentMetadataOnly}
	_, err := store.Articles().Upsert(ctx, article)
	must(t, err)
	gate := &appranking.VersionGate{}
	blockedLibrary := &blockingLibraryRepository{LibraryRepository: store.Libraries(), entered: make(chan struct{}), release: make(chan struct{})}
	recompute := &appranking.Recomputer{Articles: store.Articles(), Library: blockedLibrary, Profiles: store.Profiles(), Rankings: store.Rankings(), Results: store.Rankings(), Clock: clock, Gate: gate}
	must(t, recompute.Full(ctx))
	service := applibrary.Service{Articles: store.Articles(), Library: blockedLibrary, Clock: clock, Recompute: recompute, Gate: gate, Transactions: store}
	hidden := true
	_, err = service.UpdateLibraryState(ctx, application.UpdateLibraryStateCommand{ArticleID: article.ID, Patch: domain.LibraryPatch{Hidden: &hidden}})
	must(t, err)

	restoreDone := make(chan error, 1)
	hidden = false
	go func() {
		_, restoreErr := service.UpdateLibraryState(ctx, application.UpdateLibraryStateCommand{ArticleID: article.ID, Patch: domain.LibraryPatch{Hidden: &hidden}})
		restoreDone <- restoreErr
	}()
	select {
	case <-blockedLibrary.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("restore did not reach its transaction")
	}
	fullDone := make(chan error, 1)
	go func() { fullDone <- recompute.Full(context.WithoutCancel(ctx)) }()
	close(blockedLibrary.release)
	for name, result := range map[string]<-chan error{"restore": restoreDone, "full recompute": fullDone} {
		select {
		case err := <-result:
			must(t, err)
		case <-time.After(2 * time.Second):
			t.Fatalf("%s deadlocked", name)
		}
	}
	state, err := store.Libraries().Get(ctx, article.ID)
	must(t, err)
	result, rankErr := store.Rankings().GetResult(ctx, article.ID)
	must(t, rankErr)
	if state.HiddenAt != nil || result.Score == 0 || result.Contributions[0].ReasonCode == appranking.ReasonArticleHidden {
		t.Fatalf("restore did not publish fresh eligible ranking: state=%+v result=%+v", state, result)
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
	status := &applibrary.RecomputeStatus{}
	service := applibrary.Service{Articles: store.Articles(), Library: store.Libraries(), Clock: clock, Recompute: flaky, Gate: gate, Status: status, Transactions: store}
	saved := true
	first, err := service.UpdateLibraryState(ctx, application.UpdateLibraryStateCommand{ArticleID: article.ID, Patch: domain.LibraryPatch{Saved: &saved}})
	must(t, err)
	if first.SavedAt == nil || !status.Failed() {
		t.Fatalf("mutation result=%+v recompute failed=%v", first, status.Failed())
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
	if status.Failed() {
		t.Fatal("successful retry did not clear recomputation failure status")
	}

	flaky.failures = 1
	response := httptest.NewRecorder()
	handler := httpapi.NewFeedHandler(httpapi.FeedAPI{Library: service})
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/v1/articles/"+string(article.ID)+"/library-state", bytes.NewBufferString(`{"read":true}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("recompute failure changed successful mutation response: status=%d body=%s", response.Code, response.Body.String())
	}
	if !status.Failed() {
		t.Fatal("HTTP mutation did not retain recomputation failure status")
	}

	flaky.failures = 0
	hidden := true
	state, err := service.UpdateLibraryState(ctx, application.UpdateLibraryStateCommand{ArticleID: article.ID, Patch: domain.LibraryPatch{Hidden: &hidden}})
	must(t, err)
	if state.HiddenAt == nil {
		t.Fatal("hide did not persist")
	}
	flaky.failures = 1
	hidden = false
	state, err = service.UpdateLibraryState(ctx, application.UpdateLibraryStateCommand{ArticleID: article.ID, Patch: domain.LibraryPatch{Hidden: &hidden}})
	must(t, err)
	if state.HiddenAt == nil || !status.Failed() {
		t.Fatalf("failed restore was exposed as eligible: state=%+v failed=%v", state, status.Failed())
	}
	persisted, err = store.Libraries().Get(ctx, article.ID)
	must(t, err)
	if persisted.HiddenAt == nil {
		t.Fatal("failed restore did not roll back hidden state")
	}
	page, err := (appfeed.Service{Articles: store.Articles(), Library: store.Libraries(), Rankings: store.Rankings()}).GetFeed(ctx, application.FeedQuery{Limit: 30})
	must(t, err)
	if len(page.Articles) != 0 {
		t.Fatalf("failed restore leaked article into normal feed: %+v", page.Articles)
	}
}
