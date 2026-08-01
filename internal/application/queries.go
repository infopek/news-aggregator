package application

import (
	"time"

	"github.com/infopek/news-aggregator/internal/domain"
)

type FeedFilter struct {
	SourceIDs       []domain.SourceID
	Read            *bool
	Saved           *bool
	IncludeHidden   bool
	Text            string
	PublishedAfter  *time.Time
	PublishedBefore *time.Time
}

type FeedQuery struct {
	Filter FeedFilter
	Cursor string
	Limit  int
}

type RankedArticle struct {
	Article domain.Article
	Library domain.LibraryState
	Ranking domain.RankingResult
}

type FeedPage struct {
	Articles   []RankedArticle
	NextCursor string
}

type ArticleDetail struct {
	Article domain.Article
	Library domain.LibraryState
	Ranking domain.RankingResult
}
