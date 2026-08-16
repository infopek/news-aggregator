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

type MutationArticleRecomputer interface {
	ArticleRecomputer
	ArticleMutation(context.Context, domain.ArticleID, application.TransactionManager, func(context.Context) error) error
}

type Service struct {
	Articles     application.ArticleRepository
	Library      application.LibraryRepository
	Clock        application.Clock
	Recompute    ArticleRecomputer
	Gate         interface{ BeginMutation() func() }
	Status       *RecomputeStatus
	Transactions application.TransactionManager
}

func (s Service) UpdateLibraryState(ctx context.Context, command application.UpdateLibraryStateCommand) (domain.LibraryState, error) {
	if s.Articles == nil || s.Library == nil || s.Clock == nil || s.Recompute == nil || s.Gate == nil || command.ArticleID == "" {
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
		s.record(s.Recompute.Article(ctx, command.ArticleID))
		return current, nil
	}
	if command.Patch.Hidden != nil && !*command.Patch.Hidden && current.HiddenAt != nil {
		return s.restore(ctx, command, current)
	}
	done := s.Gate.BeginMutation()
	updated, err := s.Library.Apply(ctx, command.ArticleID, command.Patch, s.Clock.Now())
	done()
	if err != nil {
		return domain.LibraryState{}, err
	}
	s.record(s.Recompute.Article(ctx, command.ArticleID))
	return updated, nil
}

func (s Service) restore(ctx context.Context, command application.UpdateLibraryStateCommand, current domain.LibraryState) (domain.LibraryState, error) {
	if s.Transactions == nil {
		return domain.LibraryState{}, application.ErrUnavailable
	}
	var updated domain.LibraryState
	atomic, ok := s.Recompute.(MutationArticleRecomputer)
	if !ok {
		return domain.LibraryState{}, application.ErrUnavailable
	}
	var mutationApplied bool
	err := atomic.ArticleMutation(ctx, command.ArticleID, s.Transactions, func(txctx context.Context) error {
		done := s.Gate.BeginMutation()
		defer done()
		var err error
		updated, err = s.Library.Apply(txctx, command.ArticleID, command.Patch, s.Clock.Now())
		if err == nil {
			mutationApplied = true
		}
		return err
	})
	if err != nil && mutationApplied {
		s.record(err)
		return current, nil
	}
	if err != nil {
		return domain.LibraryState{}, err
	}
	s.record(nil)
	return updated, nil
}

func (s Service) record(err error) {
	if s.Status != nil {
		s.Status.record(err)
	}
}

func unchanged(state domain.LibraryState, patch domain.LibraryPatch) bool {
	return (patch.Read == nil || *patch.Read == (state.ReadAt != nil)) && (patch.Saved == nil || *patch.Saved == (state.SavedAt != nil)) && (patch.Hidden == nil || *patch.Hidden == (state.HiddenAt != nil))
}
