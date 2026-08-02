package application

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/domain"
)

var (
	_ Clock              = fixedClock{}
	_ HTTPFetcher        = fetcherStub{}
	_ CredentialStore    = credentialStoreStub{}
	_ IngestionAdapter   = ingestionAdapterStub{}
	_ Ranker             = rankerStub{}
	_ ProfileRepository  = profileRepositoryStub{}
	_ SourceRepository   = sourceRepositoryStub{}
	_ ArticleRepository  = articleRepositoryStub{}
	_ LibraryRepository  = libraryRepositoryStub{}
	_ RankingRepository  = rankingRepositoryStub{}
	_ RefreshRepository  = refreshRepositoryStub{}
	_ TransactionManager = transactionManagerStub{}
)

func TestCredentialResolutionIsCallbackScoped(t *testing.T) {
	store := credentialStoreStub{secret: []byte("sentinel")}
	called := false
	err := store.WithSecret(context.Background(), "credential-1", func(secret []byte) error {
		called = true
		if string(secret) != "sentinel" {
			t.Fatalf("secret = %q, want sentinel", string(secret))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithSecret() error = %v", err)
	}
	if !called {
		t.Fatal("WithSecret() did not invoke callback")
	}
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(0, 0).UTC() }

type fetcherStub struct{}

func (fetcherStub) Fetch(context.Context, FetchRequest) (FetchResponse, error) {
	return FetchResponse{StatusCode: 200, Body: io.NopCloser(strings.NewReader("fixture"))}, nil
}

type credentialStoreStub struct{ secret []byte }

func (credentialStoreStub) Configured(context.Context, domain.CredentialID) (bool, error) {
	return true, nil
}
func (credentialStoreStub) Store(context.Context, domain.CredentialID, []byte) error { return nil }
func (credentialStoreStub) Delete(context.Context, domain.CredentialID) error        { return nil }
func (store credentialStoreStub) WithSecret(_ context.Context, _ domain.CredentialID, use func([]byte) error) error {
	return use(store.secret)
}

type ingestionAdapterStub struct{}

func (ingestionAdapterStub) Kind() domain.SourceKind { return domain.SourceKindFeed }
func (ingestionAdapterStub) Fetch(context.Context, domain.Source, FetchCursor) (AdapterResult, error) {
	return AdapterResult{}, nil
}

type rankerStub struct{}

func (rankerStub) Rank(context.Context, RankingInput) ([]domain.RankingResult, error) {
	return nil, nil
}

type transactionManagerStub struct{}

func (transactionManagerStub) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}

type profileRepositoryStub struct{}

func (profileRepositoryStub) Get(context.Context, domain.ProfileID) (domain.UserProfile, error) {
	return domain.UserProfile{}, nil
}
func (profileRepositoryStub) Save(context.Context, domain.UserProfile) error { return nil }

type sourceRepositoryStub struct{}

func (sourceRepositoryStub) List(context.Context) ([]domain.Source, error) { return nil, nil }
func (sourceRepositoryStub) Get(context.Context, domain.SourceID) (domain.Source, error) {
	return domain.Source{}, nil
}
func (sourceRepositoryStub) Save(context.Context, domain.Source) error     { return nil }
func (sourceRepositoryStub) Delete(context.Context, domain.SourceID) error { return nil }

type articleRepositoryStub struct{}

func (articleRepositoryStub) Get(context.Context, domain.ArticleID) (domain.Article, error) {
	return domain.Article{}, nil
}
func (articleRepositoryStub) Upsert(context.Context, domain.Article) (ArticleWriteResult, error) {
	return ArticleWriteResult{}, nil
}
func (articleRepositoryStub) ListForRanking(context.Context) ([]domain.Article, error) {
	return nil, nil
}
func (articleRepositoryStub) QueryFeed(context.Context, FeedQuery) (FeedPage, error) {
	return FeedPage{}, nil
}

type libraryRepositoryStub struct{}

func (libraryRepositoryStub) Get(context.Context, domain.ArticleID) (domain.LibraryState, error) {
	return domain.LibraryState{}, nil
}
func (libraryRepositoryStub) Apply(context.Context, domain.ArticleID, domain.LibraryPatch, time.Time) (domain.LibraryState, error) {
	return domain.LibraryState{}, nil
}

type rankingRepositoryStub struct{}

func (rankingRepositoryStub) GetConfiguration(context.Context) (domain.RankingConfiguration, error) {
	return domain.RankingConfiguration{}, nil
}
func (rankingRepositoryStub) SaveConfiguration(context.Context, domain.RankingConfiguration) error {
	return nil
}
func (rankingRepositoryStub) SaveResults(context.Context, []domain.RankingResult) error { return nil }
func (rankingRepositoryStub) GetResult(context.Context, domain.ArticleID) (domain.RankingResult, error) {
	return domain.RankingResult{}, nil
}

type refreshRepositoryStub struct{}

func (refreshRepositoryStub) Create(context.Context, domain.RefreshRun) error { return nil }
func (refreshRepositoryStub) Get(context.Context, domain.RefreshRunID) (domain.RefreshRun, error) {
	return domain.RefreshRun{}, nil
}
func (refreshRepositoryStub) Save(context.Context, domain.RefreshRun) error { return nil }
func (refreshRepositoryStub) Active(context.Context) (*domain.RefreshRun, error) {
	return nil, nil
}
