package ranking

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/domain"
)

type combinedGolden struct {
	AlgorithmVersion string `json:"algorithm_version"`
	CalculatedAt     string `json:"calculated_at"`
	Configuration    struct {
		InterestWeight      float64 `json:"interest_weight"`
		AgeWeight           float64 `json:"age_weight"`
		GenderWeight        float64 `json:"gender_weight"`
		PerDemographicCap   float64 `json:"per_demographic_cap"`
		TotalDemographicCap float64 `json:"total_demographic_cap"`
	} `json:"configuration"`
	Candidates []struct {
		ID          string  `json:"id"`
		PublishedAt string  `json:"published_at"`
		Interest    float64 `json:"interest"`
		Age         float64 `json:"age"`
		Gender      float64 `json:"gender"`
	} `json:"candidates"`
	Expected []struct {
		ID      string                 `json:"id"`
		Score   float64                `json:"score"`
		Signals []domain.RankingSignal `json:"signals"`
	} `json:"expected"`
}

func TestAggregateGoldenFixture(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(current), "..", "..", "..", "test", "fixtures", "ranking", "combined-golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture combinedGolden
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("fixture must contain exactly one JSON value: %v", err)
	}
	calculated, err := time.Parse(time.RFC3339, fixture.CalculatedAt)
	if err != nil {
		t.Fatal(err)
	}
	configuration := domain.RankingConfiguration{Interest: domain.SignalWeight{Enabled: true, Weight: fixture.Configuration.InterestWeight}, Age: domain.SignalWeight{Enabled: true, Weight: fixture.Configuration.AgeWeight}, Gender: domain.SignalWeight{Enabled: true, Weight: fixture.Configuration.GenderWeight}, PerDemographicCap: fixture.Configuration.PerDemographicCap, TotalDemographicCap: fixture.Configuration.TotalDemographicCap, NormalizationVersion: "v1"}
	input := AggregateInput{Configuration: configuration, CalculatedAt: calculated}
	for _, item := range fixture.Candidates {
		published, err := time.Parse(time.RFC3339, item.PublishedAt)
		if err != nil {
			t.Fatal(err)
		}
		input.Candidates = append(input.Candidates, Candidate{ArticleID: domain.ArticleID(item.ID), PublishedAt: &published, Signals: []SignalResult{{Signal: domain.SignalInterest, Score: item.Interest, ReasonCode: ReasonInterestMatch}}, Age: demographic(domain.SignalAge, item.Age), Gender: demographic(domain.SignalGender, item.Gender)})
	}
	results, err := Aggregate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(fixture.Expected) {
		t.Fatalf("results=%d", len(results))
	}
	for i, expected := range fixture.Expected {
		if string(results[i].ArticleID) != expected.ID || math.Abs(results[i].Score-expected.Score) > 1e-12 || results[i].AlgorithmVersion != fixture.AlgorithmVersion {
			t.Fatalf("result %d=%+v want %+v", i, results[i], expected)
		}
		signals := make([]domain.RankingSignal, len(results[i].Contributions))
		for j := range results[i].Contributions {
			signals[j] = results[i].Contributions[j].Signal
		}
		if !reflect.DeepEqual(signals, expected.Signals) {
			t.Fatalf("signals=%v want %v", signals, expected.Signals)
		}
	}
}

func TestAggregateReconcilesContributionsAndCapsDemographics(t *testing.T) {
	now := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	older, newer := now.Add(-2*time.Hour), now.Add(-time.Hour)
	configuration := aggregateConfiguration()
	input := AggregateInput{Configuration: configuration, CalculatedAt: now, Candidates: []Candidate{
		{ArticleID: "lower", PublishedAt: &newer, Signals: []SignalResult{{Signal: domain.SignalInterest, Score: .5, ReasonCode: ReasonInterestMatch}}, Age: demographic(domain.SignalAge, 1), Gender: demographic(domain.SignalGender, 1)},
		{ArticleID: "higher", PublishedAt: &older, Signals: []SignalResult{{Signal: domain.SignalInterest, Score: 1, ReasonCode: ReasonInterestMatch}}, Age: demographic(domain.SignalAge, 1), Gender: demographic(domain.SignalGender, 1)},
	}}
	results, err := Aggregate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if got := []domain.ArticleID{results[0].ArticleID, results[1].ArticleID}; !reflect.DeepEqual(got, []domain.ArticleID{"higher", "lower"}) {
		t.Fatalf("order=%v", got)
	}
	for _, result := range results {
		sum, demographicSum := 0.0, 0.0
		for _, contribution := range result.Contributions {
			sum += contribution.WeightedScore
			if math.Abs(contribution.RawScore*contribution.Weight-contribution.WeightedScore) > 1e-12 {
				t.Fatalf("contribution does not reconcile: %+v", contribution)
			}
			if contribution.Signal == domain.SignalAge || contribution.Signal == domain.SignalGender {
				demographicSum += contribution.WeightedScore
				if contribution.WeightedScore > configuration.PerDemographicCap+1e-12 {
					t.Fatalf("per-demographic cap exceeded: %+v", contribution)
				}
			}
		}
		if math.Abs(sum-result.Score) > 1e-12 || demographicSum > configuration.TotalDemographicCap+1e-12 || result.Score < 0 || result.Score > 1 {
			t.Fatalf("non-reconciling result: %+v sum=%v demographic=%v", result, sum, demographicSum)
		}
		if result.AlgorithmVersion != CombinedAlgorithmVersion+"+v1" || !result.CalculatedAt.Equal(now) {
			t.Fatalf("version/time mismatch: %+v", result)
		}
	}
}

func TestAggregateDisabledDemographicsAreMetamorphicallyInvisible(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	configuration := aggregateConfiguration()
	configuration.Age.Enabled, configuration.Gender.Enabled = false, false
	base := AggregateInput{Configuration: configuration, CalculatedAt: now, Candidates: []Candidate{{ArticleID: "a", Signals: []SignalResult{{Signal: domain.SignalInterest, Score: .5, ReasonCode: ReasonInterestMatch}}}}}
	withValues := base
	withValues.Candidates = append([]Candidate(nil), base.Candidates...)
	withValues.Candidates[0].Age, withValues.Candidates[0].Gender = demographic(domain.SignalAge, 1), demographic(domain.SignalGender, 1)
	want, err := Aggregate(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Aggregate(context.Background(), withValues)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("disabled demographics changed result:\n%+v\n%+v", want, got)
	}
}

func TestAggregateStableTiesNeutralAndHidden(t *testing.T) {
	now := time.Unix(20, 0).UTC()
	old, recent := now.Add(-time.Hour), now
	input := AggregateInput{Configuration: aggregateConfiguration(), CalculatedAt: now, Candidates: []Candidate{
		{ArticleID: "z", PublishedAt: &recent}, {ArticleID: "a", PublishedAt: &recent}, {ArticleID: "old", PublishedAt: &old},
		{ArticleID: "hidden", Signals: []SignalResult{{Signal: domain.SignalBehavior, Excluded: true, ReasonCode: ReasonArticleHidden}}},
	}}
	want, err := Aggregate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if got := []domain.ArticleID{want[0].ArticleID, want[1].ArticleID, want[2].ArticleID}; !reflect.DeepEqual(got, []domain.ArticleID{"a", "z", "old"}) {
		t.Fatalf("tie order=%v", got)
	}
	for _, result := range want {
		if len(result.Contributions) != 1 || result.Contributions[0].ReasonCode != ReasonNeutralDefault || result.Score != 0 {
			t.Fatalf("missing neutral reason: %+v", result)
		}
	}
	for i := 0; i < 50; i++ {
		got, err := Aggregate(context.Background(), input)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d changed: %v %+v", i, err, got)
		}
	}
}

func TestAggregateRejectsInvalidConfigurationAndSignals(t *testing.T) {
	now := time.Now().UTC()
	configuration := aggregateConfiguration()
	tests := []AggregateInput{
		{Configuration: domain.RankingConfiguration{}, CalculatedAt: now},
		{Configuration: configuration, CalculatedAt: time.Time{}},
		{Configuration: configuration, CalculatedAt: now, Candidates: []Candidate{{ArticleID: "a", Signals: []SignalResult{{Signal: domain.SignalInterest, Score: math.NaN()}}}}},
		{Configuration: configuration, CalculatedAt: now, Candidates: []Candidate{{ArticleID: "a"}, {ArticleID: "a"}}},
	}
	for i, input := range tests {
		if _, err := Aggregate(context.Background(), input); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}

func FuzzAggregateFiniteDeterministic(f *testing.F) {
	f.Add(.5, .5, .5)
	f.Fuzz(func(t *testing.T, primary, age, gender float64) {
		if math.IsNaN(primary) || math.IsInf(primary, 0) || math.IsNaN(age) || math.IsInf(age, 0) || math.IsNaN(gender) || math.IsInf(gender, 0) {
			return
		}
		primary, age, gender = clamp(primary), clamp(age), clamp(gender)
		input := AggregateInput{Configuration: aggregateConfiguration(), CalculatedAt: time.Unix(1, 0).UTC(), Candidates: []Candidate{{ArticleID: "a", Signals: []SignalResult{{Signal: domain.SignalInterest, Score: primary, ReasonCode: ReasonInterestMatch}}, Age: demographic(domain.SignalAge, age), Gender: demographic(domain.SignalGender, gender)}}}
		first, err := Aggregate(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Aggregate(context.Background(), input)
		if err != nil || !reflect.DeepEqual(first, second) || len(first) != 1 || math.IsNaN(first[0].Score) || math.IsInf(first[0].Score, 0) || first[0].Score < 0 || first[0].Score > 1 {
			t.Fatalf("invalid aggregate: %v %+v", err, first)
		}
	})
}

func aggregateConfiguration() domain.RankingConfiguration {
	return domain.RankingConfiguration{
		Interest: domain.SignalWeight{Enabled: true, Weight: .8},
		Age:      domain.SignalWeight{Enabled: true, Weight: .1}, Gender: domain.SignalWeight{Enabled: true, Weight: .1},
		PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1",
	}
}

func demographic(signal domain.RankingSignal, score float64) SignalResult {
	reason := ReasonAgeAdjustment
	if signal == domain.SignalGender {
		reason = ReasonGenderAdjustment
	}
	return SignalResult{Signal: signal, Score: score, ReasonCode: reason, ReasonValues: map[string]string{"source": "explicit_profile"}}
}
