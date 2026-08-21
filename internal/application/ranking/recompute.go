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
	Gate     *VersionGate
	mu       sync.Mutex
}

type VersionGate struct {
	mu         sync.Mutex
	generation uint64
	active     uint64
	changed    chan struct{}
}

func (g *VersionGate) BeginMutation() func() {
	g.mu.Lock()
	g.generation++
	g.active++
	g.signalLocked()
	g.mu.Unlock()
	return func() {
		g.mu.Lock()
		g.generation++
		g.active--
		g.signalLocked()
		g.mu.Unlock()
	}
}

func (g *VersionGate) signalLocked() {
	if g.changed != nil {
		close(g.changed)
	}
	g.changed = make(chan struct{})
}

func (g *VersionGate) stable(ctx context.Context) (uint64, error) {
	for {
		g.mu.Lock()
		if g.active == 0 {
			version := g.generation
			g.mu.Unlock()
			return version, nil
		}
		if g.changed == nil {
			g.changed = make(chan struct{})
		}
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-changed:
		}
	}
}

func (g *VersionGate) current() uint64 { g.mu.Lock(); defer g.mu.Unlock(); return g.generation }
func (g *VersionGate) commit(version uint64, save func() error) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.generation != version || g.active != 0 {
		return false, nil
	}
	return true, save()
}

func (r *Recomputer) Full(ctx context.Context) error { return r.recompute(ctx, nil) }
func (r *Recomputer) Article(ctx context.Context, id domain.ArticleID) error {
	if id == "" {
		return application.ErrInvalidInput
	}
	return r.recompute(ctx, map[domain.ArticleID]struct{}{id: {}})
}

// ArticleMutation owns recomputation serialization before opening the
// mutation transaction. This lock order lets a caller publish a mutation and
// its targeted ranking atomically without holding SQLite while waiting for a
// concurrent full recomputation.
func (r *Recomputer) ArticleMutation(ctx context.Context, id domain.ArticleID, transactions application.TransactionManager, mutation func(context.Context) error) error {
	if id == "" || transactions == nil || mutation == nil || !r.ready() {
		return application.ErrUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return transactions.WithinTransaction(ctx, func(txctx context.Context) error {
		if err := mutation(txctx); err != nil {
			return err
		}
		return r.recomputeLocked(txctx, map[domain.ArticleID]struct{}{id: {}}, true)
	})
}

func (r *Recomputer) recompute(ctx context.Context, targets map[domain.ArticleID]struct{}) error {
	if !r.ready() {
		return application.ErrUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recomputeLocked(ctx, targets, false)
}

func (r *Recomputer) ready() bool {
	return r != nil && r.Articles != nil && r.Library != nil && r.Profiles != nil && r.Rankings != nil && r.Results != nil && r.Clock != nil && r.Gate != nil
}

func (r *Recomputer) recomputeLocked(ctx context.Context, targets map[domain.ArticleID]struct{}, transactionOwned bool) error {
	for {
		version := r.Gate.current()
		if !transactionOwned {
			var err error
			version, err = r.Gate.stable(ctx)
			if err != nil {
				return err
			}
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
		hiddenResults := make([]domain.RankingResult, 0)
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
				hiddenResults = append(hiddenResults, domain.RankingResult{
					ArticleID: article.ID, Score: 0, AlgorithmVersion: CombinedAlgorithmVersion + "+" + configuration.NormalizationVersion, CalculatedAt: r.Clock.Now(),
					Contributions: []domain.ScoreContribution{{Signal: domain.SignalBehavior, ReasonCode: ReasonArticleHidden, ReasonValues: map[string]string{"action": "hidden"}}},
				})
			}
			candidates = append(candidates, Candidate{ArticleID: article.ID, PublishedAt: article.PublishedAt, Age: ExplicitAgeSignal(configuration.Age.Enabled, profile.Age), Gender: ExplicitGenderSignal(configuration.Gender.Enabled, profile.Gender), Signals: []SignalResult{
				RecencySignal(configuration.Recency.Enabled, r.Clock.Now(), article.PublishedAt, DefaultRecencyWindow),
				InterestSignal(configuration.Interest.Enabled, profile.Interests, article.Topics),
				SourcePreferenceSignal(configuration.SourcePreference.Enabled, article.SourceID, profile.PreferredSources), behavior,
				LocationSignal(configuration.Location.Enabled, profile.Location, ExplicitLocationMetadata(article.Topics)), textByID[article.ID],
			}})
		}
		results, err := Aggregate(ctx, AggregateInput{Candidates: candidates, Configuration: configuration, CalculatedAt: r.Clock.Now()})
		if err != nil {
			return err
		}
		results = append(results, hiddenResults...)
		if err := ctx.Err(); err != nil {
			return err
		}
		save := func() error {
			// Hidden articles remain excluded by the feed's library-state filter.
			// Retain their last persisted result so the explicit include-hidden
			// library view can display and restore them; restoration recalculates
			// the article against current inputs before it rejoins the ranked feed.
			return r.Results.SaveResults(ctx, results)
		}
		if transactionOwned {
			// The caller owns the write transaction and recomputation mutex.
			// Other gate participants cannot publish SQLite changes until this
			// transaction releases, so waiting for them here would invert the
			// database/gate lock order. Their completion changes the generation
			// and schedules the recomputation for their subsequently committed state.
			return save()
		}
		committed, err := r.Gate.commit(version, save)
		if err != nil || committed {
			return err
		}
	}
}
