package ranking

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type retryArticles struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	calls   int
}

func (r *retryArticles) ListForRanking(context.Context) ([]domain.Article, error) {
	r.calls++
	r.once.Do(func() {
		close(r.started)
		<-r.release
	})
	return nil, nil
}
func (*retryArticles) Get(context.Context, domain.ArticleID) (domain.Article, error) {
	return domain.Article{}, application.ErrNotFound
}
func (*retryArticles) Upsert(context.Context, domain.Article) (application.ArticleWriteResult, error) {
	return application.ArticleWriteResult{}, nil
}
func (*retryArticles) QueryFeed(context.Context, application.FeedQuery) (application.FeedPage, error) {
	return application.FeedPage{}, nil
}

type retryProfiles struct{}

func (retryProfiles) Get(context.Context, domain.ProfileID) (domain.UserProfile, error) {
	return domain.UserProfile{ID: domain.LocalProfileID}, nil
}
func (retryProfiles) Save(context.Context, domain.UserProfile) error { return nil }

type retryLibrary struct{}

func (retryLibrary) Get(context.Context, domain.ArticleID) (domain.LibraryState, error) {
	return domain.LibraryState{}, application.ErrNotFound
}
func (retryLibrary) Apply(context.Context, domain.ArticleID, domain.LibraryPatch, time.Time) (domain.LibraryState, error) {
	return domain.LibraryState{}, nil
}

type retryRankings struct{ saves int }

func (r *retryRankings) GetConfiguration(context.Context) (domain.RankingConfiguration, error) {
	return domain.RankingConfiguration{Recency: domain.SignalWeight{Enabled: true, Weight: 1}, PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1"}, nil
}
func (*retryRankings) SaveConfiguration(context.Context, domain.RankingConfiguration) error {
	return nil
}
func (r *retryRankings) SaveResults(context.Context, []domain.RankingResult) error {
	r.saves++
	return nil
}
func (*retryRankings) GetResult(context.Context, domain.ArticleID) (domain.RankingResult, error) {
	return domain.RankingResult{}, application.ErrNotFound
}
func (*retryRankings) DeleteResults(context.Context, []domain.ArticleID) error { return nil }

type retryClock struct{}

func (retryClock) Now() time.Time { return time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC) }

func TestVersionGateRejectsCommitAfterMutation(t *testing.T) {
	gate := &VersionGate{}
	stale := gate.current()
	done := gate.BeginMutation()
	done()
	called := false
	committed, err := gate.commit(stale, func() error { called = true; return nil })
	if err != nil || committed || called {
		t.Fatalf("stale commit executed: committed=%v called=%v error=%v", committed, called, err)
	}
	current := gate.current()
	committed, err = gate.commit(current, func() error { called = true; return nil })
	if err != nil || !committed || !called {
		t.Fatalf("current commit rejected: committed=%v called=%v error=%v", committed, called, err)
	}
}

func TestVersionGateRejectsCommitWhileMutationIsActive(t *testing.T) {
	gate := &VersionGate{}
	done := gate.BeginMutation()
	version := gate.current()
	called := false
	committed, err := gate.commit(version, func() error { called = true; return nil })
	done()
	if err != nil || committed || called {
		t.Fatalf("committed=%v called=%v error=%v", committed, called, err)
	}
}

func TestRecomputerRetriesWhenFailedMutationInvalidatesCalculation(t *testing.T) {
	gate := &VersionGate{}
	articles := &retryArticles{started: make(chan struct{}), release: make(chan struct{})}
	rankings := &retryRankings{}
	recomputer := &Recomputer{Articles: articles, Library: retryLibrary{}, Profiles: retryProfiles{}, Rankings: rankings, Results: rankings, Clock: retryClock{}, Gate: gate}
	done := make(chan error, 1)
	go func() { done <- recomputer.Full(context.Background()) }()
	<-articles.started

	finishFailedMutation := gate.BeginMutation()
	finishFailedMutation()
	close(articles.release)

	if err := <-done; err != nil {
		t.Fatalf("recompute error=%v", err)
	}
	if articles.calls != 2 || rankings.saves != 1 {
		t.Fatalf("article snapshots=%d saves=%d", articles.calls, rankings.saves)
	}
}
