package application

import (
	"context"
	"io"
	"time"

	"github.com/infopek/news-aggregator/internal/domain"
)

type Clock interface {
	Now() time.Time
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

type ProfileRepository interface {
	Get(context.Context, domain.ProfileID) (domain.UserProfile, error)
	Save(context.Context, domain.UserProfile) error
}

type SourceRepository interface {
	List(context.Context) ([]domain.Source, error)
	Get(context.Context, domain.SourceID) (domain.Source, error)
	Save(context.Context, domain.Source) error
	Delete(context.Context, domain.SourceID) error
}

type ArticleRepository interface {
	Get(context.Context, domain.ArticleID) (domain.Article, error)
	Upsert(context.Context, domain.Article) (ArticleWriteResult, error)
	ListForRanking(context.Context) ([]domain.Article, error)
	QueryFeed(context.Context, FeedQuery) (FeedPage, error)
}

type ArticleWriteResult struct {
	ArticleID domain.ArticleID
	Inserted  bool
	Updated   bool
}

type LibraryRepository interface {
	Get(context.Context, domain.ArticleID) (domain.LibraryState, error)
	Apply(context.Context, domain.ArticleID, domain.LibraryPatch, time.Time) (domain.LibraryState, error)
}

type RankingRepository interface {
	GetConfiguration(context.Context) (domain.RankingConfiguration, error)
	SaveConfiguration(context.Context, domain.RankingConfiguration) error
	SaveResults(context.Context, []domain.RankingResult) error
	GetResult(context.Context, domain.ArticleID) (domain.RankingResult, error)
}

type RefreshRepository interface {
	Create(context.Context, domain.RefreshRun) error
	Get(context.Context, domain.RefreshRunID) (domain.RefreshRun, error)
	Save(context.Context, domain.RefreshRun) error
	Active(context.Context) (*domain.RefreshRun, error)
}

type FetchRequest struct {
	URL      string
	Method   string
	Headers  map[string][]string
	MaxBytes int64
	Timeout  time.Duration
}

type FetchResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       io.ReadCloser
	FinalURL   string
}

type HTTPFetcher interface {
	Fetch(context.Context, FetchRequest) (FetchResponse, error)
}

// CredentialStore never exposes a general read method. Trusted adapters may
// resolve a credential only inside the callback scope.
type CredentialStore interface {
	Store(context.Context, domain.CredentialID, []byte) error
	Delete(context.Context, domain.CredentialID) error
	WithSecret(context.Context, domain.CredentialID, func([]byte) error) error
}

type FetchCursor struct {
	Value        string
	ETag         string
	LastModified string
}

type AdapterItem struct {
	ExternalID   string
	CanonicalURL string
	Title        string
	Author       string
	PublishedAt  *time.Time
	Excerpt      string
	FullContent  string
	Language     string
	Topics       []string
}

type AdapterResult struct {
	Items      []AdapterItem
	NextCursor FetchCursor
	Unchanged  bool
	Warnings   []string
}

type IngestionAdapter interface {
	Kind() domain.SourceKind
	Fetch(context.Context, domain.Source, FetchCursor) (AdapterResult, error)
}

type RankingInput struct {
	Articles      []domain.Article
	LibraryStates map[domain.ArticleID]domain.LibraryState
	Profile       domain.UserProfile
	Configuration domain.RankingConfiguration
}

type Ranker interface {
	Rank(context.Context, RankingInput) ([]domain.RankingResult, error)
}
