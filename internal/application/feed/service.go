package feed

import (
	"context"
	"errors"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type Service struct {
	Articles application.ArticleRepository
	Library  application.LibraryRepository
	Rankings application.RankingRepository
}

func (s Service) GetFeed(ctx context.Context, query application.FeedQuery) (application.FeedPage, error) {
	if s.Articles == nil {
		return application.FeedPage{}, application.ErrUnavailable
	}
	if query.Limit == 0 {
		query.Limit = 30
	}
	if query.Limit < 1 || query.Limit > 100 || len(query.Filter.Text) > 200 {
		return application.FeedPage{}, application.ErrInvalidInput
	}
	return s.Articles.QueryFeed(ctx, query)
}

func (s Service) GetArticle(ctx context.Context, id domain.ArticleID) (application.ArticleDetail, error) {
	if s.Articles == nil || s.Library == nil || s.Rankings == nil || id == "" {
		return application.ArticleDetail{}, application.ErrInvalidInput
	}
	article, err := s.Articles.Get(ctx, id)
	if err != nil {
		return application.ArticleDetail{}, err
	}
	library, err := s.Library.Get(ctx, id)
	if errors.Is(err, application.ErrNotFound) {
		library = domain.LibraryState{ArticleID: id}
	} else if err != nil {
		return application.ArticleDetail{}, err
	}
	ranking, err := s.Rankings.GetResult(ctx, id)
	if err != nil {
		return application.ArticleDetail{}, err
	}
	if article.ContentPermission != domain.ContentFullAllowed {
		article.FullContent = ""
	}
	return application.ArticleDetail{Article: article, Library: library, Ranking: ranking}, nil
}
