package application

import (
	"context"
	"errors"
	"strings"
	"sync"
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
	mu        sync.Mutex
	values    map[domain.SourceID]domain.Source
	saveErr   error
	deleteErr error
}

func (r *configSources) List(context.Context) ([]domain.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Source, 0, len(r.values))
	for _, s := range r.values {
		out = append(out, s)
	}
	return out, nil
}
func (r *configSources) Get(_ context.Context, id domain.SourceID) (domain.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.values[id]
	if !ok {
		return s, ErrNotFound
	}
	return s, nil
}
func (r *configSources) Save(_ context.Context, s domain.Source) error {
	r.mu.Lock()
	defer r.mu.Unlock()
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.values[id]; !ok {
		return ErrNotFound
	}
	delete(r.values, id)
	return nil
}

type configVault struct {
	mu              sync.Mutex
	secrets         map[domain.CredentialID][]byte
	writes, deletes []domain.CredentialID
	storeErr        error
	deleteErr       error
	storeEntered    chan struct{}
	storeRelease    chan struct{}
}

func (v *configVault) Configured(_ context.Context, id domain.CredentialID) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.secrets[id]
	return ok, nil
}
func (v *configVault) Store(_ context.Context, id domain.CredentialID, secret []byte) error {
	if v.storeEntered != nil {
		v.storeEntered <- struct{}{}
		<-v.storeRelease
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.storeErr != nil {
		return v.storeErr
	}
	v.writes = append(v.writes, id)
	v.secrets[id] = append([]byte(nil), secret...)
	return nil
}
func (v *configVault) Delete(_ context.Context, id domain.CredentialID) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.deleteErr != nil {
		return v.deleteErr
	}
	if _, ok := v.secrets[id]; !ok {
		return ErrCredentialMissing
	}
	v.deletes = append(v.deletes, id)
	delete(v.secrets, id)
	return nil
}
func (v *configVault) WithSecret(_ context.Context, id domain.CredentialID, use func([]byte) error) error {
	v.mu.Lock()
	secret, ok := v.secrets[id]
	copyOfSecret := append([]byte(nil), secret...)
	v.mu.Unlock()
	if !ok {
		return ErrCredentialMissing
	}
	defer func() {
		for i := range copyOfSecret {
			copyOfSecret[i] = 0
		}
	}()
	return use(copyOfSecret)
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
	repo.mu.Lock()
	edited := repo.values["starter"]
	edited.Name = "My edited name"
	repo.values["starter"] = edited
	repo.mu.Unlock()
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

func TestSourceCommandsCannotInjectOrDetachCredentialReference(t *testing.T) {
	stable := domain.CredentialID("stable-reference")
	configured := testFeed("source", "https://example.com/feed")
	configured.CredentialRef = &stable
	repo := &configSources{values: map[domain.SourceID]domain.Source{"source": configured}}
	service := ConfigurationService{Sources: repo, Transactions: configTx{}}
	fabricated := domain.CredentialID("fabricated")
	create := testFeed("new", "https://example.com/new")
	create.CredentialRef = &fabricated
	if _, err := service.SaveSource(context.Background(), SaveSourceCommand{Source: create}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("create injection error=%v", err)
	}
	update := configured
	update.Name = "Edited"
	update.CredentialRef = nil
	got, err := service.SaveSource(context.Background(), SaveSourceCommand{Source: update})
	if err != nil || got.CredentialRef == nil || *got.CredentialRef != stable {
		t.Fatalf("nil detached credential: %+v %v", got, err)
	}
	update.CredentialRef = &fabricated
	got, err = service.SaveSource(context.Background(), SaveSourceCommand{Source: update})
	if err != nil || got.CredentialRef == nil || *got.CredentialRef != stable {
		t.Fatalf("fabricated ref attached: %+v %v", got, err)
	}
	starter := testFeed("starter-injection", "https://example.com/starter")
	starter.CredentialRef = &fabricated
	if _, err := service.ImportStarterSources(context.Background(), ImportStarterSourcesCommand{Sources: []domain.Source{starter}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("starter injection error=%v", err)
	}
}

func TestStableCredentialLifecycleAndDatabaseFailureDoNotLeak(t *testing.T) {
	const sentinel = "SENTINEL-SECRET-MUST-NOT-LEAK"
	source := testFeed("source", "https://example.com/feed")
	repo := &configSources{values: map[domain.SourceID]domain.Source{"source": source}, saveErr: errors.New("database unavailable")}
	vault := &configVault{secrets: map[domain.CredentialID][]byte{}}
	service := ConfigurationService{Sources: repo, Transactions: configTx{}, Credentials: vault, Clock: configClock{now: time.Unix(1, 0)}, CredentialReference: func(domain.SourceID) domain.CredentialID { return "opaque" }}
	err := service.ConfigureCredential(context.Background(), ConfigureCredentialCommand{SourceID: "source", Secret: []byte(sentinel)})
	if err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("unsafe error: %v", err)
	}
	if len(vault.writes) != 0 || len(vault.secrets) != 0 {
		t.Fatalf("DB failure wrote an unowned secret: %v", vault.writes)
	}
}

func TestStableCredentialReferenceResolvesAcrossServiceRestart(t *testing.T) {
	repo := &configSources{values: map[domain.SourceID]domain.Source{"source": testFeed("source", "https://example.com/feed")}}
	vault := &configVault{secrets: map[domain.CredentialID][]byte{}}
	reference := func(domain.SourceID) domain.CredentialID { return "stable" }
	service := ConfigurationService{Sources: repo, Transactions: configTx{}, Credentials: vault, CredentialReference: reference}
	if err := service.ConfigureCredential(context.Background(), ConfigureCredentialCommand{SourceID: "source", Secret: []byte("first")}); err != nil {
		t.Fatal(err)
	}
	// Reconstructing the service models a process restart: the repository ref
	// and BACKEND-003 derivation still identify exactly the same vault entry.
	service = ConfigurationService{Sources: repo, Transactions: configTx{}, Credentials: vault, CredentialReference: reference}
	source, err := repo.Get(context.Background(), "source")
	if err != nil || source.CredentialRef == nil || *source.CredentialRef != reference("source") {
		t.Fatalf("persisted reference=%+v %v", source, err)
	}
	var resolved string
	if err := vault.WithSecret(context.Background(), *source.CredentialRef, func(secret []byte) error { resolved = string(secret); return nil }); err != nil || resolved != "first" {
		t.Fatalf("resolved=%q error=%v", resolved, err)
	}
	// A missing stable entry is repaired in place without an old-key cleanup.
	vault.mu.Lock()
	delete(vault.secrets, "stable")
	vault.mu.Unlock()
	if err := service.ConfigureCredential(context.Background(), ConfigureCredentialCommand{SourceID: "source", Secret: []byte("repaired")}); err != nil {
		t.Fatal(err)
	}
	if string(vault.secrets["stable"]) != "repaired" || len(vault.secrets) != 1 {
		t.Fatal("stable-key repair failed")
	}
}

func TestCredentialDeleteDatabaseFailureRestoresResolvablePair(t *testing.T) {
	ref := domain.CredentialID("stable")
	source := testFeed("source", "https://example.com/feed")
	source.CredentialRef = &ref
	repo := &configSources{values: map[domain.SourceID]domain.Source{"source": source}, saveErr: errors.New("database unavailable")}
	vault := &configVault{secrets: map[domain.CredentialID][]byte{ref: []byte("prior")}}
	service := ConfigurationService{Sources: repo, Transactions: configTx{}, Credentials: vault, CredentialReference: func(domain.SourceID) domain.CredentialID { return ref }}
	err := service.DeleteCredential(context.Background(), DeleteCredentialCommand{SourceID: "source"})
	if err == nil || strings.Contains(err.Error(), "prior") {
		t.Fatalf("unsafe error=%v", err)
	}
	got, _ := repo.Get(context.Background(), "source")
	if got.CredentialRef == nil || *got.CredentialRef != ref || string(vault.secrets[ref]) != "prior" {
		t.Fatalf("pair not restored: %+v %q", got, vault.secrets[ref])
	}
}

func TestSourceDeleteDatabaseFailureRestoresVaultEntry(t *testing.T) {
	ref := domain.CredentialID("stable")
	source := testFeed("source", "https://example.com/feed")
	source.CredentialRef = &ref
	repo := &configSources{values: map[domain.SourceID]domain.Source{"source": source}, deleteErr: errors.New("database unavailable")}
	vault := &configVault{secrets: map[domain.CredentialID][]byte{ref: []byte("prior")}}
	service := ConfigurationService{Sources: repo, Transactions: configTx{}, Credentials: vault}
	if err := service.DeleteSource(context.Background(), DeleteSourceCommand{SourceID: "source"}); err == nil {
		t.Fatal("delete unexpectedly succeeded")
	}
	got, _ := repo.Get(context.Background(), "source")
	if got.CredentialRef == nil || string(vault.secrets[ref]) != "prior" {
		t.Fatalf("pair not restored: %+v", got)
	}
}

func TestCredentialOperationsSerializePerSource(t *testing.T) {
	reference := func(domain.SourceID) domain.CredentialID { return "stable" }
	newService := func(vault *configVault) (ConfigurationService, *configSources) {
		repo := &configSources{values: map[domain.SourceID]domain.Source{"source": testFeed("source", "https://example.com/feed")}}
		return ConfigurationService{Sources: repo, Transactions: configTx{}, Credentials: vault, CredentialReference: reference}, repo
	}
	t.Run("latest configure wins", func(t *testing.T) {
		vault := &configVault{secrets: map[domain.CredentialID][]byte{"unrelated": []byte("keep")}, storeEntered: make(chan struct{}, 1), storeRelease: make(chan struct{})}
		service, repo := newService(vault)
		firstDone := make(chan error, 1)
		go func() {
			firstDone <- service.ConfigureCredential(context.Background(), ConfigureCredentialCommand{SourceID: "source", Secret: []byte("first")})
		}()
		<-vault.storeEntered
		secondDone := make(chan error, 1)
		go func() {
			secondDone <- service.ConfigureCredential(context.Background(), ConfigureCredentialCommand{SourceID: "source", Secret: []byte("second")})
		}()
		close(vault.storeRelease)
		if err := <-firstDone; err != nil {
			t.Fatal(err)
		}
		if err := <-secondDone; err != nil {
			t.Fatal(err)
		}
		got, _ := repo.Get(context.Background(), "source")
		if got.CredentialRef == nil || *got.CredentialRef != "stable" || string(vault.secrets["stable"]) != "second" || string(vault.secrets["unrelated"]) != "keep" || len(vault.secrets) != 2 {
			t.Fatalf("inconsistent latest state: %+v %q", got, vault.secrets["stable"])
		}
	})
	for _, operation := range []string{"delete credential", "delete source"} {
		t.Run(operation, func(t *testing.T) {
			vault := &configVault{secrets: map[domain.CredentialID][]byte{"unrelated": []byte("keep")}, storeEntered: make(chan struct{}, 1), storeRelease: make(chan struct{})}
			service, repo := newService(vault)
			configured := make(chan error, 1)
			go func() {
				configured <- service.ConfigureCredential(context.Background(), ConfigureCredentialCommand{SourceID: "source", Secret: []byte("value")})
			}()
			<-vault.storeEntered
			done := make(chan error, 1)
			go func() {
				if operation == "delete credential" {
					done <- service.DeleteCredential(context.Background(), DeleteCredentialCommand{SourceID: "source"})
				} else {
					done <- service.DeleteSource(context.Background(), DeleteSourceCommand{SourceID: "source"})
				}
			}()
			close(vault.storeRelease)
			if err := <-configured; err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if len(vault.secrets) != 1 || string(vault.secrets["unrelated"]) != "keep" {
				t.Fatalf("orphaned or unrelated credential changed: %v", vault.secrets)
			}
			got, err := repo.Get(context.Background(), "source")
			if operation == "delete credential" && (err != nil || got.CredentialRef != nil) {
				t.Fatalf("credential detach failed: %+v %v", got, err)
			}
			if operation == "delete source" && !errors.Is(err, ErrNotFound) {
				t.Fatalf("source delete failed: %v", err)
			}
		})
	}
}

func testFeed(id domain.SourceID, rawURL string) domain.Source {
	return domain.Source{ID: id, Name: "Fixture", URL: rawURL, Kind: domain.SourceKindFeed, Enabled: true, AdapterConfig: domain.AdapterConfiguration{Feed: &domain.FeedConfiguration{Format: domain.FeedFormatRSS}}, ContentPermission: domain.ContentMetadataOnly, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyNotApplicable}}
}
