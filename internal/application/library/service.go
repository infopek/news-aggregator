package library

import (
	"context"
	"errors"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type ArticleRecomputer interface {
	Article(context.Context, domain.ArticleID) error
}

type Service struct {
	Articles  application.ArticleRepository
	Library   application.LibraryRepository
	Clock     application.Clock
	Recompute ArticleRecomputer
}

func (s Service) UpdateLibraryState(ctx context.Context, command application.UpdateLibraryStateCommand) (domain.LibraryState, error) {
	if s.Articles == nil || s.Library == nil || s.Clock == nil || s.Recompute == nil || command.ArticleID == "" {
		return domain.LibraryState{}, application.ErrInvalidInput
	}
	if _, err := s.Articles.Get(ctx, command.ArticleID); err != nil {
		return domain.LibraryState{}, err
	}
	current, err := s.Library.Get(ctx, command.ArticleID)
	if errors.Is(err, application.ErrNotFound) {
		current = domain.LibraryState{ArticleID: command.ArticleID}
	} else if err != nil {
		return domain.LibraryState{}, err
	}
	if command.Patch.Read == nil && command.Patch.Saved == nil && command.Patch.Hidden == nil {
		return domain.LibraryState{}, application.ErrInvalidInput
	}
	if unchanged(current, command.Patch) {
		return current, s.Recompute.Article(ctx, command.ArticleID)
	}
	updated, err := s.Library.Apply(ctx, command.ArticleID, command.Patch, s.Clock.Now())
	if err != nil {
		return domain.LibraryState{}, err
	}
	if err := s.Recompute.Article(ctx, command.ArticleID); err != nil {
		return updated, err
	}
	return updated, nil
}

func unchanged(state domain.LibraryState, patch domain.LibraryPatch) bool {
	return (patch.Read == nil || *patch.Read == (state.ReadAt != nil)) && (patch.Saved == nil || *patch.Saved == (state.SavedAt != nil)) && (patch.Hidden == nil || *patch.Hidden == (state.HiddenAt != nil))
}
