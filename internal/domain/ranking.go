package domain

import (
	"errors"
	"math"
	"time"
)

var ErrInvalidRankingConfiguration = errors.New("invalid ranking configuration")

const (
	MaximumPerDemographicWeight   = 0.10
	MaximumTotalDemographicWeight = 0.20
)

type RankingSignal string

const (
	SignalRecency        RankingSignal = "recency"
	SignalInterest       RankingSignal = "interest"
	SignalSource         RankingSignal = "source_preference"
	SignalBehavior       RankingSignal = "behavior"
	SignalLocation       RankingSignal = "location"
	SignalAge            RankingSignal = "age"
	SignalGender         RankingSignal = "gender"
	SignalTextSimilarity RankingSignal = "text_similarity"
)

type SignalWeight struct {
	Enabled bool
	Weight  float64
}

type RankingConfiguration struct {
	Recency              SignalWeight
	Interest             SignalWeight
	SourcePreference     SignalWeight
	Behavior             SignalWeight
	Location             SignalWeight
	Age                  SignalWeight
	Gender               SignalWeight
	TextSimilarity       SignalWeight
	PerDemographicCap    float64
	TotalDemographicCap  float64
	NormalizationVersion string
}

func (configuration RankingConfiguration) Validate() error {
	weights := []SignalWeight{
		configuration.Recency,
		configuration.Interest,
		configuration.SourcePreference,
		configuration.Behavior,
		configuration.Location,
		configuration.Age,
		configuration.Gender,
		configuration.TextSimilarity,
	}
	for _, weight := range weights {
		if !validUnitWeight(weight.Weight) {
			return ErrInvalidRankingConfiguration
		}
	}
	primaryWeight := 0.0
	for _, weight := range []SignalWeight{configuration.Recency, configuration.Interest, configuration.SourcePreference, configuration.Behavior, configuration.TextSimilarity} {
		if weight.Enabled {
			primaryWeight += weight.Weight
		}
	}
	if primaryWeight == 0 || !validUnitWeight(configuration.PerDemographicCap) || !validUnitWeight(configuration.TotalDemographicCap) || configuration.TotalDemographicCap < configuration.PerDemographicCap {
		return ErrInvalidRankingConfiguration
	}
	if configuration.PerDemographicCap > MaximumPerDemographicWeight || configuration.TotalDemographicCap > MaximumTotalDemographicWeight {
		return ErrInvalidRankingConfiguration
	}
	for _, weight := range []SignalWeight{configuration.Location, configuration.Age, configuration.Gender} {
		if weight.Weight > configuration.PerDemographicCap {
			return ErrInvalidRankingConfiguration
		}
	}
	demographicTotal := 0.0
	for _, weight := range []SignalWeight{configuration.Location, configuration.Age, configuration.Gender} {
		if weight.Enabled {
			demographicTotal += weight.Weight
		}
	}
	if demographicTotal > configuration.TotalDemographicCap {
		return ErrInvalidRankingConfiguration
	}
	return nil
}

func validUnitWeight(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

type ScoreContribution struct {
	Signal        RankingSignal
	RawScore      float64
	Weight        float64
	WeightedScore float64
	ReasonCode    string
	ReasonValues  map[string]string
}

type RankingResult struct {
	ArticleID        ArticleID
	Score            float64
	Contributions    []ScoreContribution
	AlgorithmVersion string
	CalculatedAt     time.Time
}
