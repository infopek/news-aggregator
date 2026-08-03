package ranking

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/domain"
)

type fixedSignalClock struct{ now time.Time }

func (clock fixedSignalClock) Now() time.Time { return clock.now }

func TestRecencySignalBoundaries(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	future, fresh, middle, boundary, old := now.Add(time.Hour), now, now.Add(-DefaultRecencyWindow/2), now.Add(-DefaultRecencyWindow), now.Add(-DefaultRecencyWindow-time.Second)
	tests := []struct {
		name string
		on   bool
		at   *time.Time
		span time.Duration
		want float64
	}{
		{"maximum", true, &fresh, DefaultRecencyWindow, 1},
		{"future clamps safely", true, &future, DefaultRecencyWindow, 1},
		{"midpoint", true, &middle, DefaultRecencyWindow, .5},
		{"minimum boundary", true, &boundary, DefaultRecencyWindow, 0},
		{"older minimum", true, &old, DefaultRecencyWindow, 0},
		{"missing", true, nil, DefaultRecencyWindow, 0},
		{"disabled", false, &fresh, DefaultRecencyWindow, 0},
		{"malformed window", true, &fresh, 0, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := RecencySignal(test.on, now, test.at, test.span)
			assertSignal(t, got, 0, 1)
			if got.Score != test.want {
				t.Fatalf("score=%v want %v", got.Score, test.want)
			}
			if test.want == 0 && len(got.ReasonValues) != 0 {
				t.Fatalf("zero fabricated evidence: %v", got.ReasonValues)
			}
		})
	}
}

func TestRecencyExtractorUsesInjectedClock(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	published := now.Add(-DefaultRecencyWindow / 2)
	got := (RecencyExtractor{Clock: fixedSignalClock{now: now}, Window: DefaultRecencyWindow}).Extract(true, &published)
	if got.Score != .5 {
		t.Fatalf("score=%v", got.Score)
	}
	missingClock := (RecencyExtractor{Window: DefaultRecencyWindow}).Extract(true, &published)
	if missingClock.Score != 0 || len(missingClock.ReasonValues) != 0 {
		t.Fatalf("missing clock=%+v", missingClock)
	}
}

func TestInterestSignalBoundariesAndExplicitEvidence(t *testing.T) {
	interests := []domain.WeightedInterest{{Name: " Climate ", Weight: .25}, {Name: "SCIENCE", Weight: .75}}
	tests := []struct {
		name      string
		on        bool
		interests []domain.WeightedInterest
		topics    []string
		want      float64
	}{
		{"maximum", true, interests, []string{"science", "climate"}, 1},
		{"partial", true, interests, []string{"climate"}, .25},
		{"absent topics", true, interests, nil, 0},
		{"absent interests", true, nil, []string{"science"}, 0},
		{"disabled", false, interests, []string{"science"}, 0},
		{"malformed ignored", true, []domain.WeightedInterest{{Name: "science", Weight: math.NaN()}, {Name: "", Weight: 1}}, []string{"science"}, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := InterestSignal(test.on, test.interests, test.topics)
			assertSignal(t, got, 0, 1)
			if got.Score != test.want {
				t.Fatalf("score=%v want=%v", got.Score, test.want)
			}
			if got.Score == 0 && len(got.ReasonValues) != 0 {
				t.Fatalf("fabricated evidence: %v", got.ReasonValues)
			}
		})
	}
}

func TestTextSimilarityIntegrationPermissionDisabledAndMalformed(t *testing.T) {
	articles := []domain.Article{
		{ID: "allowed", Title: "science", FullContent: "climate", ContentPermission: domain.ContentFullAllowed},
		{ID: "forbidden", Title: "sports", FullContent: "science climate", ContentPermission: domain.ContentMetadataOnly},
	}
	got := TextSimilaritySignals(true, articles, []domain.WeightedInterest{{Name: "science climate", Weight: 1}})
	if len(got) != 2 || got[0].ArticleID != "allowed" || got[0].Result.Score == 0 {
		t.Fatalf("allowed result=%+v", got)
	}
	if got[1].ArticleID != "forbidden" || got[1].Result.Score != 0 || len(got[1].Result.ReasonValues) != 0 {
		t.Fatalf("forbidden content supplied evidence: %+v", got[1])
	}
	disabled := TextSimilaritySignals(false, articles, []domain.WeightedInterest{{Name: "science", Weight: 1}})
	malformed := TextSimilaritySignals(true, articles, []domain.WeightedInterest{{Name: "science", Weight: math.Inf(1)}})
	for _, set := range [][]ArticleSignalResult{disabled, malformed} {
		for _, item := range set {
			assertSignal(t, item.Result, 0, 1)
			if item.Result.Score != 0 || len(item.Result.ReasonValues) != 0 {
				t.Fatalf("inactive similarity supplied evidence: %+v", item)
			}
		}
	}
}

func TestSourcePreferenceRequiresExplicitSelection(t *testing.T) {
	tests := []struct {
		name      string
		on        bool
		source    domain.SourceID
		preferred []domain.SourceID
		want      float64
	}{
		{"selected", true, "source-a", []domain.SourceID{"source-b", "source-a"}, 1},
		{"incidental availability", true, "source-a", []domain.SourceID{"source-b"}, 0},
		{"no preference", true, "source-a", nil, 0},
		{"malformed source", true, "", []domain.SourceID{"source-a"}, 0},
		{"disabled", false, "source-a", []domain.SourceID{"source-a"}, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := SourcePreferenceSignal(test.on, test.source, test.preferred)
			assertSignal(t, got, 0, 1)
			if got.Score != test.want {
				t.Fatalf("score=%v want=%v", got.Score, test.want)
			}
		})
	}
}

func TestBehaviorTransitions(t *testing.T) {
	at := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		on       bool
		state    domain.LibraryState
		score    float64
		excluded bool
	}{
		{"untouched", true, domain.LibraryState{}, 0, false},
		{"read", true, domain.LibraryState{ReadAt: &at}, ReadBehaviorScore, false},
		{"saved", true, domain.LibraryState{SavedAt: &at}, SavedBehaviorScore, false},
		{"saved dominates read", true, domain.LibraryState{ReadAt: &at, SavedAt: &at}, SavedBehaviorScore, false},
		{"hidden excludes", true, domain.LibraryState{SavedAt: &at, HiddenAt: &at}, 0, true},
		{"restored retains saved", true, domain.LibraryState{SavedAt: &at, HiddenAt: nil}, SavedBehaviorScore, false},
		{"restored untouched", true, domain.LibraryState{HiddenAt: nil}, 0, false},
		{"disabled hidden does not exclude", false, domain.LibraryState{HiddenAt: &at}, 0, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := BehaviorSignal(test.on, test.state)
			assertSignal(t, got, 0, 1)
			if got.Score != test.score || got.Excluded != test.excluded {
				t.Fatalf("got=%+v", got)
			}
		})
	}
}

func TestLocationUsesOnlyExplicitCoarseValues(t *testing.T) {
	profile := domain.OptionalSignal[domain.Location]{Present: true, Enabled: true, Value: domain.Location{Country: "HU", Region: "Budapest", City: domain.OptionalSignal[string]{Present: true, Enabled: true, Value: "Budapest"}}}
	tests := []struct {
		name     string
		on       bool
		profile  domain.OptionalSignal[domain.Location]
		metadata CoarseLocationMetadata
		want     float64
	}{
		{"city", true, profile, CoarseLocationMetadata{Country: "hu", Region: "BUDAPEST", City: "Budapest"}, 1},
		{"region", true, profile, CoarseLocationMetadata{Country: "HU", Region: "Budapest"}, 2.0 / 3.0},
		{"country", true, profile, CoarseLocationMetadata{Country: "HU", Region: "Pest"}, 1.0 / 3.0},
		{"different country", true, profile, CoarseLocationMetadata{Country: "AT", Region: "Vienna", City: "Budapest"}, 0},
		{"missing region metadata", true, profile, CoarseLocationMetadata{Country: "HU"}, 1.0 / 3.0},
		{"absent profile", true, domain.OptionalSignal[domain.Location]{}, CoarseLocationMetadata{Country: "HU", Region: "Budapest"}, 0},
		{"profile disabled", true, domain.OptionalSignal[domain.Location]{Present: true, Enabled: false, Value: profile.Value}, CoarseLocationMetadata{Country: "HU", Region: "Budapest"}, 0},
		{"signal disabled", false, profile, CoarseLocationMetadata{Country: "HU", Region: "Budapest", City: "Budapest"}, 0},
		{"malformed profile", true, domain.OptionalSignal[domain.Location]{Present: true, Enabled: true, Value: domain.Location{Country: "", Region: "Budapest"}}, CoarseLocationMetadata{Country: "HU", Region: "Budapest"}, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := LocationSignal(test.on, test.profile, test.metadata)
			assertSignal(t, got, 0, 1)
			if got.Score != test.want {
				t.Fatalf("score=%v want=%v", got.Score, test.want)
			}
		})
	}
}

func TestSignalsAreDeterministicAndFixtureDocumentsRanges(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	published := now.Add(-time.Hour)
	first := []SignalResult{RecencySignal(true, now, &published, DefaultRecencyWindow), InterestSignal(true, []domain.WeightedInterest{{Name: "Go", Weight: 1}}, []string{"go"}), SourcePreferenceSignal(true, "s", []domain.SourceID{"s"}), BehaviorSignal(true, domain.LibraryState{}), LocationSignal(true, domain.OptionalSignal[domain.Location]{Present: true, Enabled: true, Value: domain.Location{Country: "HU", Region: "Pest"}}, CoarseLocationMetadata{Country: "HU", Region: "Pest"})}
	for i := 0; i < 100; i++ {
		if got := []SignalResult{RecencySignal(true, now, &published, DefaultRecencyWindow), InterestSignal(true, []domain.WeightedInterest{{Name: "Go", Weight: 1}}, []string{"go"}), SourcePreferenceSignal(true, "s", []domain.SourceID{"s"}), BehaviorSignal(true, domain.LibraryState{}), LocationSignal(true, domain.OptionalSignal[domain.Location]{Present: true, Enabled: true, Value: domain.Location{Country: "HU", Region: "Pest"}}, CoarseLocationMetadata{Country: "HU", Region: "Pest"})}; !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d differs", i)
		}
	}
	_, current, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(current), "..", "..", "..", "test", "fixtures", "ranking", "primary-signal-ranges.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture map[string]any
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
}

func FuzzSignalsFiniteBounded(f *testing.F) {
	f.Add("interest", "topic", "HU", "Budapest", 0.5)
	f.Fuzz(func(t *testing.T, interest, topic, country, region string, weight float64) {
		results := []SignalResult{InterestSignal(true, []domain.WeightedInterest{{Name: interest, Weight: weight}}, []string{topic}), LocationSignal(true, domain.OptionalSignal[domain.Location]{Present: true, Enabled: true, Value: domain.Location{Country: country, Region: region}}, CoarseLocationMetadata{Country: country, Region: region})}
		for _, result := range results {
			assertSignal(t, result, 0, 1)
		}
	})
}

func assertSignal(t *testing.T, result SignalResult, minimum, maximum float64) {
	t.Helper()
	if math.IsNaN(result.Score) || math.IsInf(result.Score, 0) || result.Score < minimum || result.Score > maximum {
		t.Fatalf("unbounded signal: %+v", result)
	}
	if result.ReasonCode == "" || result.ReasonValues == nil {
		t.Fatalf("unstable explanation: %+v", result)
	}
}
