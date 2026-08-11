package ranking

import (
	"context"
	"errors"
	"sync"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type RecomputeRepository interface {
	SaveResults(context.Context, []domain.RankingResult) error
	DeleteResults(context.Context, []domain.ArticleID) error
}

// Recomputer serializes calculation and persistence so an older input snapshot
// cannot commit after a newer mutation in this process.
type Recomputer struct {
	Articles application.ArticleRepository
	Library  application.LibraryRepository
	Profiles application.ProfileRepository
	Rankings application.RankingRepository
	Results  RecomputeRepository
	Clock    application.Clock
	mu       sync.Mutex
}

func (r *Recomputer) Full(ctx context.Context) error { return r.recompute(ctx, nil) }
func (r *Recomputer) Article(ctx context.Context, id domain.ArticleID) error {
	if id == "" {
		return application.ErrInvalidInput
	}
	return r.recompute(ctx, map[domain.ArticleID]struct{}{id: {}})
}

func (r *Recomputer) recompute(ctx context.Context, targets map[domain.ArticleID]struct{}) error {
	if r == nil || r.Articles == nil || r.Library == nil || r.Profiles == nil || r.Rankings == nil || r.Results == nil || r.Clock == nil {
		return application.ErrUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	articles, err := r.Articles.ListForRanking(ctx)
	if err != nil {
		return err
	}
	profile, err := r.Profiles.Get(ctx, domain.LocalProfileID)
	if err != nil {
		return err
	}
	configuration, err := r.Rankings.GetConfiguration(ctx)
	if err != nil {
		return err
	}
	text := TextSimilaritySignals(configuration.TextSimilarity.Enabled, articles, profile.Interests)
	textByID := make(map[domain.ArticleID]SignalResult, len(text))
	for _, v := range text {
		textByID[v.ArticleID] = v.Result
	}
	candidates := make([]Candidate, 0, len(articles))
	hidden := make([]domain.ArticleID, 0)
	for _, article := range articles {
		if targets != nil {
			if _, ok := targets[article.ID]; !ok {
				continue
			}
		}
		state, e := r.Library.Get(ctx, article.ID)
		if errors.Is(e, application.ErrNotFound) {
			state = domain.LibraryState{ArticleID: article.ID}
		} else if e != nil {
			return e
		}
		behavior := BehaviorSignal(configuration.Behavior.Enabled, state)
		if behavior.Excluded {
			hidden = append(hidden, article.ID)
		}
		candidates = append(candidates, Candidate{ArticleID: article.ID, PublishedAt: article.PublishedAt, Signals: []SignalResult{
			RecencySignal(configuration.Recency.Enabled, r.Clock.Now(), article.PublishedAt, DefaultRecencyWindow),
			InterestSignal(configuration.Interest.Enabled, profile.Interests, article.Topics),
			SourcePreferenceSignal(configuration.SourcePreference.Enabled, article.SourceID, profile.PreferredSources), behavior,
			LocationSignal(configuration.Location.Enabled, profile.Location, CoarseLocationMetadata{}), textByID[article.ID],
		}})
	}
	results, err := Aggregate(ctx, AggregateInput{Candidates: candidates, Configuration: configuration, CalculatedAt: r.Clock.Now()})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.Results.SaveResults(ctx, results); err != nil {
		return err
	}
	return r.Results.DeleteResults(ctx, hidden)
}
