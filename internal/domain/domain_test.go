package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestOptionalSignalKeepsPresenceAndEnablementDistinct(t *testing.T) {
	tests := []struct {
		name   string
		signal OptionalSignal[int]
		valid  bool
	}{
		{name: "absent and disabled", signal: OptionalSignal[int]{}, valid: true},
		{name: "present and disabled", signal: OptionalSignal[int]{Value: 42, Present: true}, valid: true},
		{name: "present and enabled", signal: OptionalSignal[int]{Value: 42, Present: true, Enabled: true}, valid: true},
		{name: "absent cannot be enabled", signal: OptionalSignal[int]{Enabled: true}, valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.signal.Valid(); got != test.valid {
				t.Fatalf("Valid() = %v, want %v", got, test.valid)
			}
		})
	}
}

func TestArticleRejectsFullContentWithoutPermission(t *testing.T) {
	article := validArticle()
	article.ContentPermission = ContentMetadataOnly
	article.FullContent = "not permitted"

	if err := article.Validate(); !errors.Is(err, ErrInvalidArticle) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidArticle)
	}
}

func TestEnabledScraperRequiresApprovedReviewedPolicy(t *testing.T) {
	source := Source{
		ID:                "source-1",
		Name:              "Example",
		URL:               "https://example.com/news",
		Kind:              SourceKindScraper,
		Enabled:           true,
		ContentPermission: ContentMetadataOnly,
		ScraperPolicy:     ScraperPolicy{Status: ScraperPolicyPending},
	}
	if err := source.Validate(); !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("pending scraper Validate() error = %v, want %v", err, ErrInvalidSource)
	}

	now := time.Now()
	source.ScraperPolicy = ScraperPolicy{Status: ScraperPolicyApproved, ReviewedAt: &now}
	if err := source.Validate(); err != nil {
		t.Fatalf("approved scraper Validate() error = %v", err)
	}
}

func TestRankingConfigurationRejectsNonFiniteAndEmptyWeights(t *testing.T) {
	tests := []RankingConfiguration{
		{},
		{Interest: SignalWeight{Enabled: true, Weight: math.NaN()}},
		{Interest: SignalWeight{Enabled: true, Weight: 0.5}, PerDemographicCap: 0.2, TotalDemographicCap: 0.1},
	}
	for index, configuration := range tests {
		if err := configuration.Validate(); !errors.Is(err, ErrInvalidRankingConfiguration) {
			t.Fatalf("case %d Validate() error = %v, want %v", index, err, ErrInvalidRankingConfiguration)
		}
	}

	valid := RankingConfiguration{
		Interest:             SignalWeight{Enabled: true, Weight: 0.5},
		TextSimilarity:       SignalWeight{Enabled: true, Weight: 0.5},
		PerDemographicCap:    0.05,
		TotalDemographicCap:  0.1,
		NormalizationVersion: "v1",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid configuration Validate() error = %v", err)
	}
}

func validArticle() Article {
	return Article{
		ID:                "article-1",
		Fingerprint:       "fingerprint-1",
		SourceID:          "source-1",
		CanonicalURL:      "https://example.com/article",
		Title:             "Example article",
		FetchedAt:         time.Now(),
		ContentPermission: ContentFullAllowed,
	}
}
