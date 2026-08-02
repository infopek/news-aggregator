package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/domain"
)

type configClock struct{ now time.Time }

func (c configClock) Now() time.Time { return c.now }

type configTx struct{}

func (configTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type configProfiles struct {
	value domain.UserProfile
	err   error
}

func (r *configProfiles) Get(context.Context, domain.ProfileID) (domain.UserProfile, error) {
	return r.value, r.err
}
func (r *configProfiles) Save(_ context.Context, p domain.UserProfile) error {
	r.value, r.err = p, nil
	return nil
}

type configRankings struct{ value domain.RankingConfiguration }

func (r *configRankings) GetConfiguration(context.Context) (domain.RankingConfiguration, error) {
	return r.value, nil
}
func (r *configRankings) SaveConfiguration(_ context.Context, c domain.RankingConfiguration) error {
	r.value = c
	return nil
}
func (*configRankings) SaveResults(context.Context, []domain.RankingResult) error { return nil }
func (*configRankings) GetResult(context.Context, domain.ArticleID) (domain.RankingResult, error) {
	return domain.RankingResult{}, ErrNotFound
}

type configSources struct {
	values  map[domain.SourceID]domain.Source
	saveErr error
}

func (r *configSources) List(context.Context) ([]domain.Source, error) {
	out := make([]domain.Source, 0, len(r.values))
	for _, s := range r.values {
		out = append(out, s)
	}
	return out, nil
}
func (r *configSources) Get(_ context.Context, id domain.SourceID) (domain.Source, error) {
	s, ok := r.values[id]
	if !ok {
		return s, ErrNotFound
	}
	return s, nil
}
func (r *configSources) Save(_ context.Context, s domain.Source) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	for id, v := range r.values {
		if id != s.ID && v.URL == s.URL {
			return ErrConflict
		}
	}
	r.values[s.ID] = s
	return nil
}
func (r *configSources) Delete(_ context.Context, id domain.SourceID) error {
	if _, ok := r.values[id]; !ok {
		return ErrNotFound
	}
	delete(r.values, id)
	return nil
}

type configVault struct {
	configured      map[domain.CredentialID]bool
	writes, deletes []domain.CredentialID
}

func (v *configVault) Configured(_ context.Context, id domain.CredentialID) (bool, error) {
	return v.configured[id], nil
}
func (v *configVault) Store(_ context.Context, id domain.CredentialID, secret []byte) error {
	v.writes = append(v.writes, id)
	v.configured[id] = true
	return nil
}
func (v *configVault) Delete(_ context.Context, id domain.CredentialID) error {
	v.deletes = append(v.deletes, id)
	delete(v.configured, id)
	return nil
}
func (*configVault) WithSecret(context.Context, domain.CredentialID, func([]byte) error) error {
	return ErrUnavailable
}

func TestProfileValidationMatrixPreservesOptionalStates(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	repo := &configProfiles{}
	service := ConfigurationService{Profiles: repo, Clock: configClock{now: now}}
	tests := []struct {
		name   string
		signal domain.OptionalSignal[int]
		valid  bool
	}{
		{"absent", domain.OptionalSignal[int]{}, true},
		{"present disabled", domain.OptionalSignal[int]{Present: true, Value: 42}, true},
		{"present enabled", domain.OptionalSignal[int]{Present: true, Enabled: true, Value: 42}, true},
		{"absent enabled", domain.OptionalSignal[int]{Enabled: true}, false},
		{"out of range", domain.OptionalSignal[int]{Present: true, Value: 131}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := service.UpdateProfile(context.Background(), UpdateProfileCommand{Profile: domain.UserProfile{ID: domain.LocalProfileID, Age: tc.signal}})
			if (err == nil) != tc.valid {
				t.Fatalf("error=%v", err)
			}
			if tc.valid && (got.Age.Present != tc.signal.Present || got.Age.Enabled != tc.signal.Enabled) {
				t.Fatalf("state changed: %+v", got.Age)
			}
		})
	}
}

func TestRankingDemographicCeilingMatrix(t *testing.T) {
	base := domain.RankingConfiguration{Recency: domain.SignalWeight{Enabled: true, Weight: .5}, PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1"}
	service := ConfigurationService{Rankings: &configRankings{}}
	tests := []struct {
		name   string
		mutate func(*domain.RankingConfiguration)
		valid  bool
	}{
		{"all disabled", func(*domain.RankingConfiguration) {}, true},
		{"at caps", func(c *domain.RankingConfiguration) {
			c.Age = domain.SignalWeight{Enabled: true, Weight: .1}
			c.Gender = domain.SignalWeight{Enabled: true, Weight: .1}
		}, true},
		{"per signal too high", func(c *domain.RankingConfiguration) { c.Age = domain.SignalWeight{Enabled: true, Weight: .11} }, false},
		{"configured cap too high", func(c *domain.RankingConfiguration) { c.PerDemographicCap = .11 }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mutate(&c)
			_, err := service.UpdateRankingConfiguration(context.Background(), UpdateRankingConfigurationCommand{Configuration: c})
			if (err == nil) != tc.valid {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestStarterImportPreservesEditsAndDuplicateURL(t *testing.T) {
	repo := &configSources{values: map[domain.SourceID]domain.Source{}}
	service := ConfigurationService{Sources: repo, Transactions: configTx{}}
	starter := testFeed("starter", "HTTPS://EXAMPLE.COM/feed#news")
	if _, err := service.ImportStarterSources(context.Background(), ImportStarterSourcesCommand{Sources: []domain.Source{starter}}); err != nil {
		t.Fatal(err)
	}
	edited := repo.values["starter"]
	edited.Name = "My edited name"
	repo.values["starter"] = edited
	if _, err := service.ImportStarterSources(context.Background(), ImportStarterSourcesCommand{Sources: []domain.Source{starter}}); err != nil {
		t.Fatal(err)
	}
	if repo.values["starter"].Name != "My edited name" {
		t.Fatal("starter reload overwrote user edit")
	}
	duplicate := testFeed("other", "https://example.com/feed")
	if _, err := service.ImportStarterSources(context.Background(), ImportStarterSourcesCommand{Sources: []domain.Source{duplicate}}); err != nil {
		t.Fatal(err)
	}
	if len(repo.values) != 1 {
		t.Fatalf("duplicate URL created %d records", len(repo.values))
	}
}

func TestSourceValidationMatrix(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	validScraper := domain.Source{ID: "scraper", Name: "Scraper", URL: "https://example.com/news", Kind: domain.SourceKindScraper, Enabled: true,
		AdapterConfig: domain.AdapterConfiguration{Scraper: &domain.ScraperConfiguration{ArticleSelector: "article", TitleSelector: "h2"}}, ContentPermission: domain.ContentMetadataOnly,
		ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyApproved, TermsURL: "https://example.com/terms", RobotsURL: "https://example.com/robots.txt", ReviewedAt: &now, ReviewNotes: "Approved manually"}}
	tests := []struct {
		name   string
		source domain.Source
		valid  bool
	}{
		{"feed", testFeed("feed", "https://example.com/rss"), true},
		{"official API", domain.Source{ID: "api", Name: "API", URL: "https://api.example.com/v1", Kind: domain.SourceKindAPI, Enabled: true, AdapterConfig: domain.AdapterConfiguration{API: &domain.APIConfiguration{Provider: "fixture", PageSize: 20}}, ContentPermission: domain.ContentFullAllowed, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyNotApplicable}}, true},
		{"approved scraper", validScraper, true},
		{"file URL", testFeed("file", "file:///tmp/feed"), false},
		{"kind mismatch", domain.Source{ID: "bad", Name: "Bad", URL: "https://example.com", Kind: domain.SourceKindAPI, AdapterConfig: domain.AdapterConfiguration{Feed: &domain.FeedConfiguration{Format: domain.FeedFormatRSS}}, ContentPermission: domain.ContentMetadataOnly}, false},
		{"unknown permission", func() domain.Source {
			s := testFeed("permission", "https://example.com/p")
			s.ContentPermission = "unknown"
			return s
		}(), false},
		{"scraper pending", func() domain.Source {
			s := validScraper
			s.ScraperPolicy.Status = domain.ScraperPolicyPending
			return s
		}(), false},
		{"scraper review missing terms", func() domain.Source { s := validScraper; s.ScraperPolicy.TermsURL = ""; return s }(), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &configSources{values: map[domain.SourceID]domain.Source{}}
			_, err := (ConfigurationService{Sources: repo}).SaveSource(context.Background(), SaveSourceCommand{Source: tc.source})
			if (err == nil) != tc.valid {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestCredentialDatabaseFailureCompensatesWithoutSentinelLeak(t *testing.T) {
	const sentinel = "SENTINEL-SECRET-MUST-NOT-LEAK"
	source := testFeed("source", "https://example.com/feed")
	oldReference := domain.CredentialID("old-opaque-reference")
	source.CredentialRef = &oldReference
	repo := &configSources{values: map[domain.SourceID]domain.Source{"source": source}, saveErr: errors.New("database unavailable")}
	vault := &configVault{configured: map[domain.CredentialID]bool{oldReference: true}}
	service := ConfigurationService{Sources: repo, Transactions: configTx{}, Credentials: vault, Clock: configClock{now: time.Unix(1, 0)}, CredentialReference: func(domain.SourceID) domain.CredentialID { return "opaque" }}
	err := service.ConfigureCredential(context.Background(), ConfigureCredentialCommand{SourceID: "source", Secret: []byte(sentinel)})
	if err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("unsafe error: %v", err)
	}
	if len(vault.writes) != 1 || len(vault.deletes) != 1 || vault.writes[0] != vault.deletes[0] {
		t.Fatalf("compensation mismatch writes=%v deletes=%v", vault.writes, vault.deletes)
	}
	if repo.values["source"].CredentialRef == nil || *repo.values["source"].CredentialRef != oldReference || !vault.configured[oldReference] {
		t.Fatal("failed replacement did not preserve the old credential reference")
	}
}

func testFeed(id domain.SourceID, rawURL string) domain.Source {
	return domain.Source{ID: id, Name: "Fixture", URL: rawURL, Kind: domain.SourceKindFeed, Enabled: true, AdapterConfig: domain.AdapterConfiguration{Feed: &domain.FeedConfiguration{Format: domain.FeedFormatRSS}}, ContentPermission: domain.ContentMetadataOnly, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyNotApplicable}}
}
