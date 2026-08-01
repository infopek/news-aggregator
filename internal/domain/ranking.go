package domain

import (
	"errors"
	"math"
	"time"
)

var ErrInvalidRankingConfiguration = errors.New("invalid ranking configuration")

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
	activeWeight := 0.0
	for _, weight := range weights {
		if !validUnitWeight(weight.Weight) {
			return ErrInvalidRankingConfiguration
		}
		if weight.Enabled {
			activeWeight += weight.Weight
		}
	}
	if activeWeight == 0 || !validUnitWeight(configuration.PerDemographicCap) || !validUnitWeight(configuration.TotalDemographicCap) || configuration.TotalDemographicCap < configuration.PerDemographicCap {
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
