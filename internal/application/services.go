package application

import (
	"context"

	"github.com/infopek/news-aggregator/internal/domain"
)

type ProfileService interface {
	GetProfile(context.Context) (domain.UserProfile, error)
	UpdateProfile(context.Context, UpdateProfileCommand) (domain.UserProfile, error)
	GetRankingConfiguration(context.Context) (domain.RankingConfiguration, error)
	UpdateRankingConfiguration(context.Context, UpdateRankingConfigurationCommand) (domain.RankingConfiguration, error)
}

type SourceService interface {
	ListSources(context.Context) ([]domain.Source, error)
	GetSource(context.Context, domain.SourceID) (domain.Source, error)
	SaveSource(context.Context, SaveSourceCommand) (domain.Source, error)
	ImportStarterSources(context.Context, ImportStarterSourcesCommand) ([]domain.Source, error)
	DeleteSource(context.Context, DeleteSourceCommand) error
	ConfigureCredential(context.Context, ConfigureCredentialCommand) error
	DeleteCredential(context.Context, DeleteCredentialCommand) error
}

type RefreshService interface {
	StartRefresh(context.Context, StartRefreshCommand) (domain.RefreshRun, error)
	GetRefresh(context.Context, domain.RefreshRunID) (domain.RefreshRun, error)
}

type FeedService interface {
	GetFeed(context.Context, FeedQuery) (FeedPage, error)
	GetArticle(context.Context, domain.ArticleID) (ArticleDetail, error)
	UpdateLibraryState(context.Context, UpdateLibraryStateCommand) (domain.LibraryState, error)
}
