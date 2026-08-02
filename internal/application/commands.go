package application

import "github.com/infopek/news-aggregator/internal/domain"

type UpdateProfileCommand struct {
	Profile domain.UserProfile
}

type UpdateRankingConfigurationCommand struct {
	Configuration domain.RankingConfiguration
}

type SaveSourceCommand struct {
	Source domain.Source
}

type ImportStarterSourcesCommand struct {
	Sources []domain.Source
}

type DeleteSourceCommand struct {
	SourceID domain.SourceID
}

// ConfigureCredentialCommand is intentionally write-only. Secret must not be
// copied into source/domain records or returned in a result.
type ConfigureCredentialCommand struct {
	SourceID domain.SourceID
	Secret   []byte
}

type DeleteCredentialCommand struct {
	SourceID domain.SourceID
}

type StartRefreshCommand struct{}

type UpdateLibraryStateCommand struct {
	ArticleID domain.ArticleID
	Patch     domain.LibraryPatch
}
