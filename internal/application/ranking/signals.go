package ranking

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/infopek/news-aggregator/internal/domain"
)

const (
	PrimarySignalsVersion   = "primary-signals-v1"
	ReasonRecencyFresh      = "recency_fresh"
	ReasonInterestMatch     = "explicit_interest_match"
	ReasonPreferredSource   = "explicit_source_preference"
	ReasonArticleRead       = "article_read"
	ReasonArticleSaved      = "article_saved"
	ReasonArticleHidden     = "article_hidden"
	ReasonLocationMatch     = "explicit_location_match"
	ReasonLocalTextMatch    = "local_text_match"
	ReasonSignalUnavailable = "signal_unavailable"
	ReasonSignalDisabled    = "signal_disabled"
	DefaultRecencyWindow    = 7 * 24 * time.Hour
	ReadBehaviorScore       = 0.25
	SavedBehaviorScore      = 1.0
)

// SignalResult is the unweighted result of one primary extractor. Score is
// always finite and in [0,1]. Excluded is used only for hidden articles: a
// hidden article is removed from the feed rather than assigned a penalty.
type SignalResult struct {
	Signal       domain.RankingSignal
	Score        float64
	ReasonCode   string
	ReasonValues map[string]string
	Excluded     bool
}

type ArticleSignalResult struct {
	ArticleID domain.ArticleID
	Result    SignalResult
}

// CoarseLocationMetadata is explicit stored article or source metadata. It is
// deliberately a value supplied by the caller: extraction has no browser, IP,
// geolocation, or network capability.
type CoarseLocationMetadata struct {
	Country string
	Region  string
	City    string
}

// Clock is satisfied by application.Clock and kept capability-minimal here.
type Clock interface {
	Now() time.Time
}

type RecencyExtractor struct {
	Clock  Clock
	Window time.Duration
}

func (extractor RecencyExtractor) Extract(enabled bool, publishedAt *time.Time) SignalResult {
	if extractor.Clock == nil {
		return zeroSignal(domain.SignalRecency, enabled)
	}
	return RecencySignal(enabled, extractor.Clock.Now(), publishedAt, extractor.Window)
}

// RecencySignal decays linearly from 1 at now to 0 at window age. Future
// timestamps are safely treated as current. Missing timestamps or invalid
// windows produce zero without claiming freshness evidence.
func RecencySignal(enabled bool, now time.Time, publishedAt *time.Time, window time.Duration) SignalResult {
	result := zeroSignal(domain.SignalRecency, enabled)
	if !enabled || publishedAt == nil || now.IsZero() || window <= 0 {
		return result
	}
	age := now.Sub(*publishedAt)
	if age < 0 {
		age = 0
	}
	score := 1 - float64(age)/float64(window)
	result.Score = bounded(score)
	if result.Score == 0 {
		return result
	}
	result.ReasonCode = ReasonRecencyFresh
	result.ReasonValues = map[string]string{
		"age_seconds":    strconv.FormatInt(int64(age/time.Second), 10),
		"window_seconds": strconv.FormatInt(int64(window/time.Second), 10),
	}
	return result
}

// InterestSignal matches only explicit weighted interests against normalized
// article topics. It does not inspect forbidden full content or infer topics.
func InterestSignal(enabled bool, interests []domain.WeightedInterest, topics []string) SignalResult {
	result := zeroSignal(domain.SignalInterest, enabled)
	if !enabled {
		return result
	}
	topicSet := normalizedSet(topics)
	if len(topicSet) == 0 {
		return result
	}
	matches := make([]string, 0)
	var matched, total float64
	seen := make(map[string]struct{})
	for _, interest := range interests {
		name := normalizeLabel(interest.Name)
		if name == "" || !finiteUnit(interest.Weight) || interest.Weight == 0 {
			continue
		}
		total += interest.Weight
		if _, ok := topicSet[name]; ok {
			matched += interest.Weight
			if _, duplicate := seen[name]; !duplicate {
				matches = append(matches, name)
				seen[name] = struct{}{}
			}
		}
	}
	if total == 0 || matched == 0 {
		return result
	}
	sort.Strings(matches)
	result.Score = bounded(matched / total)
	result.ReasonCode = ReasonInterestMatch
	result.ReasonValues = map[string]string{"matched_interests": strings.Join(matches, ",")}
	return result
}

// TextSimilaritySignals integrates the RANK-001 local similarity extractor as
// an unweighted primary signal. It preserves RANK-001's permission boundary:
// FullContent is indexed only when ContentFullAllowed is explicit.
func TextSimilaritySignals(enabled bool, articles []domain.Article, interests []domain.WeightedInterest) []ArticleSignalResult {
	index := NewIndex(articles)
	query := queryTerms(interests)
	results := make([]ArticleSignalResult, 0, len(index.documents))
	for _, document := range index.documents {
		result := zeroSignal(domain.SignalTextSimilarity, enabled)
		if enabled {
			score, matches := index.score(document, query)
			result.Score = bounded(score)
			if result.Score > 0 {
				result.ReasonCode = ReasonLocalTextMatch
				result.ReasonValues = map[string]string{
					"algorithm_version":    AlgorithmVersion,
					"tokenization_version": TokenizationVersion,
					"matched_terms":        strings.Join(matches, ","),
				}
			}
		}
		results = append(results, ArticleSignalResult{ArticleID: document.id, Result: result})
	}
	return results
}

// SourcePreferenceSignal rewards only a source explicitly selected by the
// user. Merely being enabled, configured, or available is not a preference.
func SourcePreferenceSignal(enabled bool, sourceID domain.SourceID, preferred []domain.SourceID) SignalResult {
	result := zeroSignal(domain.SignalSource, enabled)
	if !enabled || sourceID == "" {
		return result
	}
	for _, candidate := range preferred {
		if candidate != "" && candidate == sourceID {
			result.Score = 1
			result.ReasonCode = ReasonPreferredSource
			result.ReasonValues = map[string]string{"source_id": string(sourceID)}
			return result
		}
	}
	return result
}

// BehaviorSignal uses only explicit current library actions. Saved dominates
// read but remains bounded. Hidden means exclusion, never a negative score.
// Clearing HiddenAt restores eligibility and allows remaining actions to apply.
func BehaviorSignal(enabled bool, state domain.LibraryState) SignalResult {
	result := zeroSignal(domain.SignalBehavior, enabled)
	if state.HiddenAt != nil {
		result.Excluded = true
		result.ReasonCode = ReasonArticleHidden
		result.ReasonValues = map[string]string{"action": "hidden"}
		return result
	}
	if !enabled {
		return result
	}
	if state.SavedAt != nil {
		result.Score = SavedBehaviorScore
		result.ReasonCode = ReasonArticleSaved
		result.ReasonValues = map[string]string{"action": "saved"}
		return result
	}
	if state.ReadAt != nil {
		result.Score = ReadBehaviorScore
		result.ReasonCode = ReasonArticleRead
		result.ReasonValues = map[string]string{"action": "read"}
	}
	return result
}

// LocationSignal compares an explicitly enabled manual profile location with
// explicit coarse article/source metadata. Country, region, and optional city
// matches score 1/3, 2/3, and 1 respectively. Matching stops at the first
// mismatch so a city name alone cannot fabricate regional relevance.
func LocationSignal(enabled bool, profile domain.OptionalSignal[domain.Location], metadata CoarseLocationMetadata) SignalResult {
	result := zeroSignal(domain.SignalLocation, enabled)
	if !enabled || !profile.Present || !profile.Enabled {
		return result
	}
	wantCountry, wantRegion := normalizeLabel(profile.Value.Country), normalizeLabel(profile.Value.Region)
	gotCountry, gotRegion := normalizeLabel(metadata.Country), normalizeLabel(metadata.Region)
	if wantCountry == "" || wantRegion == "" || gotCountry == "" || wantCountry != gotCountry {
		return result
	}
	result.Score = 1.0 / 3.0
	level := "country"
	if gotRegion != "" && wantRegion == gotRegion {
		result.Score = 2.0 / 3.0
		level = "region"
		city := profile.Value.City
		if city.Present && city.Enabled {
			wantCity, gotCity := normalizeLabel(city.Value), normalizeLabel(metadata.City)
			if wantCity != "" && gotCity != "" && wantCity == gotCity {
				result.Score = 1
				level = "city"
			}
		}
	}
	result.ReasonCode = ReasonLocationMatch
	result.ReasonValues = map[string]string{"match_level": level}
	return result
}

func zeroSignal(signal domain.RankingSignal, enabled bool) SignalResult {
	reason := ReasonSignalUnavailable
	if !enabled {
		reason = ReasonSignalDisabled
	}
	return SignalResult{Signal: signal, ReasonCode: reason, ReasonValues: map[string]string{}}
}

func normalizedSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if normalized := normalizeLabel(value); normalized != "" {
			result[normalized] = struct{}{}
		}
	}
	return result
}

func normalizeLabel(value string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsNumber(r))
	}), " ")
}

func bounded(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0
	}
	if value >= 1 {
		return 1
	}
	return value
}
