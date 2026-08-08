package ranking

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/infopek/news-aggregator/internal/domain"
)

const (
	CombinedAlgorithmVersion = "weighted-signals-v1"
	ReasonAgeAdjustment      = "explicit_age_adjustment"
	ReasonGenderAdjustment   = "explicit_gender_adjustment"
	ReasonNeutralDefault     = "neutral_default"
)

var ErrInvalidSignalInput = errors.New("invalid ranking signal input")

// Candidate contains already-extracted, local signal evidence. Age and gender
// are explicit optional inputs; this layer never infers either value.
type Candidate struct {
	ArticleID   domain.ArticleID
	PublishedAt *time.Time
	Signals     []SignalResult
	Age         SignalResult
	Gender      SignalResult
}

type AggregateInput struct {
	Candidates    []Candidate
	Configuration domain.RankingConfiguration
	CalculatedAt  time.Time
}

// ResultRepository is the persistence capability needed by RankingService.
type ResultRepository interface {
	SaveResults(context.Context, []domain.RankingResult) error
}

type RankingService struct {
	Repository ResultRepository
}

func (service RankingService) RankAndSave(ctx context.Context, input AggregateInput) ([]domain.RankingResult, error) {
	results, err := Aggregate(ctx, input)
	if err != nil {
		return nil, err
	}
	if service.Repository == nil {
		return nil, ErrInvalidSignalInput
	}
	if err := service.Repository.SaveResults(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

// Aggregate normalizes configured active weights, applies hard demographic
// caps, emits only truthful nonzero evidence (or an explicit neutral reason),
// and sorts deterministically by score, publication time, then article ID.
func Aggregate(ctx context.Context, input AggregateInput) ([]domain.RankingResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input.CalculatedAt.IsZero() || input.Configuration.Validate() != nil {
		return nil, domain.ErrInvalidRankingConfiguration
	}
	weights := configuredWeights(input.Configuration)
	primaryTotal := activeWeight(weights, primarySignals())
	if !finitePositive(primaryTotal) {
		return nil, domain.ErrInvalidRankingConfiguration
	}

	results := make([]rankedCandidate, 0, len(input.Candidates))
	seen := make(map[domain.ArticleID]struct{}, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if candidate.ArticleID == "" {
			return nil, ErrInvalidSignalInput
		}
		if _, duplicate := seen[candidate.ArticleID]; duplicate {
			return nil, ErrInvalidSignalInput
		}
		seen[candidate.ArticleID] = struct{}{}
		hidden, err := validateCandidate(candidate, weights)
		if err != nil {
			return nil, err
		}
		if hidden {
			continue
		}
		contributions, err := contributionsFor(candidate, weights, primaryTotal, input.Configuration)
		if err != nil {
			return nil, err
		}
		score := 0.0
		for _, contribution := range contributions {
			score += contribution.WeightedScore
		}
		score = clamp(score)
		if len(contributions) == 0 {
			contributions = []domain.ScoreContribution{neutralContribution(weights, primaryTotal)}
		}
		results = append(results, rankedCandidate{result: domain.RankingResult{
			ArticleID: candidate.ArticleID, Score: score, Contributions: contributions,
			AlgorithmVersion: CombinedAlgorithmVersion + "+" + input.Configuration.NormalizationVersion,
			CalculatedAt:     input.CalculatedAt.UTC(),
		}, publishedAt: candidate.PublishedAt})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].result.Score != results[j].result.Score {
			return results[i].result.Score > results[j].result.Score
		}
		left, right := results[i].publishedAt, results[j].publishedAt
		if left != nil || right != nil {
			if left == nil {
				return false
			}
			if right == nil {
				return true
			}
			if !left.Equal(*right) {
				return left.After(*right)
			}
		}
		return results[i].result.ArticleID < results[j].result.ArticleID
	})
	ordered := make([]domain.RankingResult, len(results))
	for i := range results {
		ordered[i] = results[i].result
	}
	return ordered, nil
}

type rankedCandidate struct {
	result      domain.RankingResult
	publishedAt *time.Time
}

func contributionsFor(candidate Candidate, weights map[domain.RankingSignal]domain.SignalWeight, primaryTotal float64, configuration domain.RankingConfiguration) ([]domain.ScoreContribution, error) {
	bySignal := make(map[domain.RankingSignal]SignalResult, len(candidate.Signals)+2)
	for _, result := range candidate.Signals {
		if result.Signal == domain.SignalAge || result.Signal == domain.SignalGender || result.Excluded || !validResult(result) {
			return nil, ErrInvalidSignalInput
		}
		if _, known := weights[result.Signal]; !known {
			return nil, ErrInvalidSignalInput
		}
		if _, duplicate := bySignal[result.Signal]; duplicate {
			return nil, ErrInvalidSignalInput
		}
		bySignal[result.Signal] = result
	}
	for signal, result := range map[domain.RankingSignal]SignalResult{domain.SignalAge: candidate.Age, domain.SignalGender: candidate.Gender} {
		if result.Signal == "" {
			result.Signal = signal
		}
		if result.Signal != signal || result.Excluded || !validResult(result) {
			return nil, ErrInvalidSignalInput
		}
		bySignal[signal] = result
	}

	demographicBudget := configuredDemographicBudget(weights, configuration)
	primaryBudget := 1 - demographicBudget
	order := primarySignals()
	contributions := make([]domain.ScoreContribution, 0, len(order)+2)
	for _, signal := range order {
		result, ok := bySignal[signal]
		weight := weights[signal]
		if !ok || !weight.Enabled || weight.Weight == 0 || result.Score == 0 {
			continue
		}
		effectiveWeight := weight.Weight / primaryTotal * primaryBudget
		contributions = append(contributions, contribution(result, effectiveWeight, result.Score*effectiveWeight))
	}

	primaryScore := 0.0
	for _, item := range contributions {
		primaryScore += item.WeightedScore
	}
	if primaryScore == 0 {
		return contributions, nil
	}
	demographic := make([]domain.ScoreContribution, 0, 3)
	for _, signal := range demographicSignals() {
		result, weight := bySignal[signal], weights[signal]
		if !weight.Enabled || weight.Weight == 0 || result.Score == 0 {
			continue
		}
		weighted := math.Min(result.Score*weight.Weight, configuration.PerDemographicCap)
		demographic = append(demographic, contribution(result, weighted/result.Score, weighted))
	}
	demographicTotal := 0.0
	for _, item := range demographic {
		demographicTotal += item.WeightedScore
	}
	allowedDemographic := math.Min(demographicBudget, primaryScore)
	if demographicTotal > allowedDemographic {
		factor := allowedDemographic / demographicTotal
		for i := range demographic {
			demographic[i].Weight *= factor
			demographic[i].WeightedScore *= factor
		}
	}
	return append(contributions, demographic...), nil
}

func contribution(result SignalResult, normalizedWeight, weighted float64) domain.ScoreContribution {
	values := make(map[string]string, len(result.ReasonValues))
	for key, value := range result.ReasonValues {
		values[key] = value
	}
	return domain.ScoreContribution{Signal: result.Signal, RawScore: result.Score, Weight: normalizedWeight, WeightedScore: weighted, ReasonCode: result.ReasonCode, ReasonValues: values}
}

func neutralContribution(weights map[domain.RankingSignal]domain.SignalWeight, primaryTotal float64) domain.ScoreContribution {
	for _, signal := range primarySignals() {
		if weight := weights[signal]; weight.Enabled {
			return domain.ScoreContribution{Signal: signal, Weight: weight.Weight / primaryTotal, ReasonCode: ReasonNeutralDefault, ReasonValues: map[string]string{}}
		}
	}
	panic("validated configuration has no active signal")
}

func validResult(result SignalResult) bool {
	if result.Signal == "" || !finiteUnit(result.Score) {
		return false
	}
	if result.Score == 0 {
		return result.ReasonCode == "" || result.ReasonCode == ReasonSignalDisabled || result.ReasonCode == ReasonSignalUnavailable
	}
	if result.Signal == domain.SignalBehavior {
		return result.ReasonCode == ReasonArticleRead || result.ReasonCode == ReasonArticleSaved
	}
	return result.ReasonCode == reasonFor(result.Signal)
}

func validateCandidate(candidate Candidate, weights map[domain.RankingSignal]domain.SignalWeight) (bool, error) {
	seen := make(map[domain.RankingSignal]struct{}, len(candidate.Signals))
	hidden := false
	for _, result := range candidate.Signals {
		if _, known := weights[result.Signal]; !known || result.Signal == domain.SignalAge || result.Signal == domain.SignalGender {
			return false, ErrInvalidSignalInput
		}
		if _, duplicate := seen[result.Signal]; duplicate {
			return false, ErrInvalidSignalInput
		}
		seen[result.Signal] = struct{}{}
		if result.Excluded {
			if result.Signal != domain.SignalBehavior || result.Score != 0 || result.ReasonCode != ReasonArticleHidden || result.ReasonValues["action"] != "hidden" {
				return false, ErrInvalidSignalInput
			}
			hidden = true
			continue
		}
		if !validResult(result) {
			return false, ErrInvalidSignalInput
		}
	}
	return hidden, nil
}

func configuredWeights(configuration domain.RankingConfiguration) map[domain.RankingSignal]domain.SignalWeight {
	return map[domain.RankingSignal]domain.SignalWeight{
		domain.SignalRecency: configuration.Recency, domain.SignalInterest: configuration.Interest,
		domain.SignalSource: configuration.SourcePreference, domain.SignalBehavior: configuration.Behavior,
		domain.SignalLocation: configuration.Location, domain.SignalAge: configuration.Age,
		domain.SignalGender: configuration.Gender, domain.SignalTextSimilarity: configuration.TextSimilarity,
	}
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func primarySignals() []domain.RankingSignal {
	return []domain.RankingSignal{domain.SignalRecency, domain.SignalInterest, domain.SignalSource, domain.SignalBehavior, domain.SignalTextSimilarity}
}

func demographicSignals() []domain.RankingSignal {
	return []domain.RankingSignal{domain.SignalLocation, domain.SignalAge, domain.SignalGender}
}

func activeWeight(weights map[domain.RankingSignal]domain.SignalWeight, signals []domain.RankingSignal) float64 {
	total := 0.0
	for _, signal := range signals {
		if weight := weights[signal]; weight.Enabled {
			total += weight.Weight
		}
	}
	return total
}

func configuredDemographicBudget(weights map[domain.RankingSignal]domain.SignalWeight, configuration domain.RankingConfiguration) float64 {
	total := 0.0
	for _, signal := range demographicSignals() {
		if weight := weights[signal]; weight.Enabled {
			total += math.Min(weight.Weight, configuration.PerDemographicCap)
		}
	}
	return math.Min(total, configuration.TotalDemographicCap)
}

func reasonFor(signal domain.RankingSignal) string {
	switch signal {
	case domain.SignalRecency:
		return ReasonRecencyFresh
	case domain.SignalInterest:
		return ReasonInterestMatch
	case domain.SignalSource:
		return ReasonPreferredSource
	case domain.SignalBehavior:
		return ReasonArticleSaved // read is handled as another truthful behavior reason below.
	case domain.SignalLocation:
		return ReasonLocationMatch
	case domain.SignalTextSimilarity:
		return ReasonLocalTextMatch
	case domain.SignalAge:
		return ReasonAgeAdjustment
	case domain.SignalGender:
		return ReasonGenderAdjustment
	default:
		return ""
	}
}
