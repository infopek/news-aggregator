package integration_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/domain"
)

type optionalFixture[T any] struct {
	Present bool `json:"present"`
	Enabled bool `json:"enabled"`
	Value   *T   `json:"value,omitempty"`
}
type locationFixture struct {
	Country string                  `json:"country"`
	Region  string                  `json:"region"`
	City    optionalFixture[string] `json:"city"`
}
type interestFixture struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
}
type profileFixture struct {
	ID                 string                           `json:"id"`
	Interests          []interestFixture                `json:"interests"`
	PreferredSourceIDs []string                         `json:"preferredSourceIds"`
	Location           optionalFixture[locationFixture] `json:"location"`
	Age                optionalFixture[int]             `json:"age"`
	Gender             optionalFixture[string]          `json:"gender"`
	UpdatedAt          string                           `json:"updatedAt"`
}
type signalWeightFixture struct {
	Enabled bool    `json:"enabled"`
	Weight  float64 `json:"weight"`
}
type rankingConfigurationFixture struct {
	Recency              signalWeightFixture `json:"recency"`
	Interest             signalWeightFixture `json:"interest"`
	SourcePreference     signalWeightFixture `json:"sourcePreference"`
	Behavior             signalWeightFixture `json:"behavior"`
	Location             signalWeightFixture `json:"location"`
	Age                  signalWeightFixture `json:"age"`
	Gender               signalWeightFixture `json:"gender"`
	TextSimilarity       signalWeightFixture `json:"textSimilarity"`
	PerDemographicCap    float64             `json:"perDemographicCap"`
	TotalDemographicCap  float64             `json:"totalDemographicCap"`
	NormalizationVersion string              `json:"normalizationVersion"`
}
type scraperPolicyFixture struct {
	Status      string  `json:"status"`
	TermsURL    *string `json:"termsUrl"`
	RobotsURL   *string `json:"robotsUrl"`
	ReviewedAt  *string `json:"reviewedAt"`
	ReviewNotes *string `json:"reviewNotes"`
}
type sourceFixture struct {
	ID                   string               `json:"id"`
	Name                 string               `json:"name"`
	URL                  string               `json:"url"`
	Kind                 string               `json:"kind"`
	Enabled              bool                 `json:"enabled"`
	ContentPermission    string               `json:"contentPermission"`
	AdapterConfig        json.RawMessage      `json:"adapterConfig"`
	ScraperPolicy        scraperPolicyFixture `json:"scraperPolicy"`
	CredentialConfigured bool                 `json:"credentialConfigured"`
	LastSuccessAt        *string              `json:"lastSuccessAt"`
	LastError            *string              `json:"lastError"`
	RetryAfter           *string              `json:"retryAfter"`
}
type libraryFixture struct {
	ArticleID string  `json:"articleId"`
	ReadAt    *string `json:"readAt"`
	SavedAt   *string `json:"savedAt"`
	HiddenAt  *string `json:"hiddenAt"`
}
type contributionFixture struct {
	Signal        string            `json:"signal"`
	RawScore      float64           `json:"rawScore"`
	Weight        float64           `json:"weight"`
	WeightedScore float64           `json:"weightedScore"`
	ReasonCode    string            `json:"reasonCode"`
	ReasonValues  map[string]string `json:"reasonValues"`
}
type rankingFixture struct {
	Score            float64               `json:"score"`
	Contributions    []contributionFixture `json:"contributions"`
	AlgorithmVersion string                `json:"algorithmVersion"`
	CalculatedAt     string                `json:"calculatedAt"`
}
type articleFixture struct {
	ID                string         `json:"id"`
	SourceID          string         `json:"sourceId"`
	CanonicalURL      string         `json:"canonicalUrl"`
	Title             string         `json:"title"`
	Author            *string        `json:"author,omitempty"`
	PublishedAt       *string        `json:"publishedAt"`
	FetchedAt         string         `json:"fetchedAt"`
	Excerpt           *string        `json:"excerpt,omitempty"`
	ContentPermission string         `json:"contentPermission"`
	Language          *string        `json:"language,omitempty"`
	Topics            []string       `json:"topics"`
	Library           libraryFixture `json:"library"`
	Ranking           rankingFixture `json:"ranking"`
}
type articleDetailFixture struct {
	Article     articleFixture `json:"article"`
	FullContent *string        `json:"fullContent"`
}
type refreshFixture struct {
	ID         string  `json:"id"`
	Status     string  `json:"status"`
	StartedAt  string  `json:"startedAt"`
	FinishedAt *string `json:"finishedAt"`
	Outcomes   []struct {
		SourceID     string  `json:"sourceId"`
		Fetched      int     `json:"fetched"`
		Inserted     int     `json:"inserted"`
		Updated      int     `json:"updated"`
		Skipped      int     `json:"skipped"`
		Failed       int     `json:"failed"`
		ErrorCode    *string `json:"errorCode"`
		ErrorSummary *string `json:"errorSummary"`
	} `json:"outcomes"`
}
type apiErrorFixture struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlationId"`
	Fields        []struct {
		Path    string `json:"path"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"fields"`
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "test", "fixtures"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestEveryAPIResponseFamilyStrictlyDecodesAndMatchesDomain(t *testing.T) {
	root := filepath.Join(fixtureRoot(t), "api")
	manifest := strictFile[map[string]string](t, filepath.Join(root, "manifest.json"))
	covered := map[string]bool{}
	for file, family := range manifest {
		path := filepath.Join(root, file)
		switch family {
		case "Health":
			value := strictFile[struct {
				Status  string `json:"status"`
				Version string `json:"version"`
			}](t, path)
			if value.Status != "ready" || value.Version == "" {
				t.Fatal("invalid health fixture")
			}
		case "Profile":
			assertProfile(t, strictFile[profileFixture](t, path))
		case "RankingConfiguration":
			assertRankingConfiguration(t, strictFile[rankingConfigurationFixture](t, path))
		case "Source":
			assertSource(t, strictFile[sourceFixture](t, path))
		case "SourceList":
			value := strictFile[struct {
				Items []sourceFixture `json:"items"`
			}](t, path)
			if len(value.Items) == 0 {
				t.Fatal("source list must be populated")
			}
			for _, source := range value.Items {
				assertSource(t, source)
			}
		case "CredentialStatus":
			value := strictFile[struct {
				Configured bool `json:"configured"`
			}](t, path)
			if !value.Configured {
				t.Fatal("credential status fixture lost configured state")
			}
		case "RefreshRun":
			assertRefresh(t, strictFile[refreshFixture](t, path))
		case "FeedPage":
			value := strictFile[struct {
				Items      []articleFixture `json:"items"`
				NextCursor *string          `json:"nextCursor"`
			}](t, path)
			for _, article := range value.Items {
				assertArticle(t, article, nil)
			}
		case "ArticleDetail":
			value := strictFile[articleDetailFixture](t, path)
			assertArticle(t, value.Article, value.FullContent)
		case "LibraryState":
			assertLibrary(t, strictFile[libraryFixture](t, path))
		case "APIError":
			value := strictFile[apiErrorFixture](t, path)
			if value.Code != "validation_failed" || value.CorrelationID == "" || len(value.Fields) == 0 || value.Fields[0].Path == "" {
				t.Fatal("API error details were not decoded")
			}
		default:
			t.Fatalf("unhandled response family %s", family)
		}
		covered[family] = true
	}
	for _, family := range []string{"Health", "Profile", "RankingConfiguration", "Source", "SourceList", "CredentialStatus", "RefreshRun", "FeedPage", "ArticleDetail", "LibraryState", "APIError"} {
		if !covered[family] {
			t.Errorf("API response family %s has no fixture", family)
		}
	}
}

func assertProfile(t *testing.T, value profileFixture) {
	t.Helper()
	profile := domain.UserProfile{ID: domain.ProfileID(value.ID), UpdatedAt: mustTime(t, value.UpdatedAt), Location: domain.OptionalSignal[domain.Location]{Present: value.Location.Present, Enabled: value.Location.Enabled}, Age: domain.OptionalSignal[int]{Present: value.Age.Present, Enabled: value.Age.Enabled}, Gender: domain.OptionalSignal[string]{Present: value.Gender.Present, Enabled: value.Gender.Enabled}}
	if value.Age.Value != nil {
		profile.Age.Value = *value.Age.Value
	}
	if value.Gender.Value != nil {
		profile.Gender.Value = *value.Gender.Value
	}
	if value.Location.Value != nil {
		city := value.Location.Value.City
		profile.Location.Value = domain.Location{Country: value.Location.Value.Country, Region: value.Location.Value.Region, City: domain.OptionalSignal[string]{Present: city.Present, Enabled: city.Enabled}}
		if city.Value != nil {
			profile.Location.Value.City.Value = *city.Value
		}
	}
	for _, item := range value.Interests {
		profile.Interests = append(profile.Interests, domain.WeightedInterest{Name: item.Name, Weight: item.Weight})
	}
	for _, id := range value.PreferredSourceIDs {
		profile.PreferredSources = append(profile.PreferredSources, domain.SourceID(id))
	}
	if err := profile.Validate(); err != nil {
		t.Fatalf("profile fixture rejected: %v", err)
	}
	if !value.Location.Present && (value.Location.Value != nil || value.Location.Enabled) {
		t.Fatal("absent location leaked a value")
	}
	if value.Age.Present && !value.Age.Enabled && value.Age.Value == nil {
		t.Fatal("disabled present age lost its stored value")
	}
}

func assertRankingConfiguration(t *testing.T, value rankingConfigurationFixture) {
	t.Helper()
	convert := func(v signalWeightFixture) domain.SignalWeight {
		return domain.SignalWeight{Enabled: v.Enabled, Weight: v.Weight}
	}
	configuration := domain.RankingConfiguration{Recency: convert(value.Recency), Interest: convert(value.Interest), SourcePreference: convert(value.SourcePreference), Behavior: convert(value.Behavior), Location: convert(value.Location), Age: convert(value.Age), Gender: convert(value.Gender), TextSimilarity: convert(value.TextSimilarity), PerDemographicCap: value.PerDemographicCap, TotalDemographicCap: value.TotalDemographicCap, NormalizationVersion: value.NormalizationVersion}
	if err := configuration.Validate(); err != nil {
		t.Fatalf("ranking fixture rejected: %v", err)
	}
}

func assertSource(t *testing.T, value sourceFixture) {
	t.Helper()
	fields := map[string]string{}
	switch value.Kind {
	case "feed":
		config := strictBytes[struct {
			Format string `json:"format"`
		}](t, value.AdapterConfig)
		fields["format"] = config.Format
	case "api":
		config := strictBytes[struct {
			Provider string `json:"provider"`
			PageSize int    `json:"pageSize"`
		}](t, value.AdapterConfig)
		fields["provider"] = config.Provider
		fields["page_size"] = fmt.Sprint(config.PageSize)
	case "scraper":
		config := strictBytes[struct {
			ArticleSelector string `json:"articleSelector"`
			TitleSelector   string `json:"titleSelector"`
			ExcerptSelector string `json:"excerptSelector,omitempty"`
			ContentSelector string `json:"contentSelector,omitempty"`
		}](t, value.AdapterConfig)
		fields["article_selector"] = config.ArticleSelector
		fields["title_selector"] = config.TitleSelector
		fields["excerpt_selector"] = config.ExcerptSelector
		fields["content_selector"] = config.ContentSelector
	}
	adapter, err := domain.ParseAdapterConfiguration(domain.SourceKind(value.Kind), fields)
	if err != nil {
		t.Fatalf("adapter fixture rejected: %v", err)
	}
	policy := domain.ScraperPolicy{Status: domain.ScraperPolicyStatus(value.ScraperPolicy.Status)}
	if value.ScraperPolicy.ReviewedAt != nil {
		parsed := mustTime(t, *value.ScraperPolicy.ReviewedAt)
		policy.ReviewedAt = &parsed
	}
	if value.ScraperPolicy.TermsURL != nil {
		policy.TermsURL = *value.ScraperPolicy.TermsURL
	}
	if value.ScraperPolicy.RobotsURL != nil {
		policy.RobotsURL = *value.ScraperPolicy.RobotsURL
	}
	if value.ScraperPolicy.ReviewNotes != nil {
		policy.ReviewNotes = *value.ScraperPolicy.ReviewNotes
	}
	source := domain.Source{ID: domain.SourceID(value.ID), Name: value.Name, URL: value.URL, Kind: domain.SourceKind(value.Kind), Enabled: value.Enabled, AdapterConfig: adapter, ContentPermission: domain.ContentPermission(value.ContentPermission), ScraperPolicy: policy}
	if err := source.Validate(); err != nil {
		t.Fatalf("source fixture rejected: %v", err)
	}
}

func assertRefresh(t *testing.T, value refreshFixture) {
	t.Helper()
	run := domain.RefreshRun{ID: domain.RefreshRunID(value.ID), StartedAt: mustTime(t, value.StartedAt), Status: domain.RefreshStatus(value.Status)}
	if value.FinishedAt != nil {
		parsed := mustTime(t, *value.FinishedAt)
		run.FinishedAt = &parsed
	}
	successes, failures := 0, 0
	for _, item := range value.Outcomes {
		outcome := domain.SourceRefreshOutcome{SourceID: domain.SourceID(item.SourceID), Fetched: item.Fetched, Inserted: item.Inserted, Updated: item.Updated, Skipped: item.Skipped, Failed: item.Failed}
		if item.ErrorCode != nil {
			outcome.ErrorCode = *item.ErrorCode
		}
		if item.ErrorSummary != nil {
			outcome.ErrorSummary = *item.ErrorSummary
		}
		run.Outcomes = append(run.Outcomes, outcome)
		if item.Failed > 0 {
			failures++
		} else {
			successes++
		}
	}
	if run.Status != domain.RefreshPartialSuccess || run.FinishedAt == nil || successes == 0 || failures == 0 {
		t.Fatal("partial refresh must contain successful and failed outcomes")
	}
}

func assertArticle(t *testing.T, value articleFixture, fullContent *string) {
	t.Helper()
	content := ""
	if fullContent != nil {
		content = *fullContent
	}
	article := domain.Article{ID: domain.ArticleID(value.ID), Fingerprint: "fixture:" + value.ID, SourceID: domain.SourceID(value.SourceID), CanonicalURL: value.CanonicalURL, Title: value.Title, FetchedAt: mustTime(t, value.FetchedAt), FullContent: content, ContentPermission: domain.ContentPermission(value.ContentPermission), Topics: value.Topics}
	if value.Author != nil {
		article.Author = *value.Author
	}
	if value.Excerpt != nil {
		article.Excerpt = *value.Excerpt
	}
	if value.Language != nil {
		article.Language = *value.Language
	}
	if value.PublishedAt != nil {
		parsed := mustTime(t, *value.PublishedAt)
		article.PublishedAt = &parsed
	}
	if err := article.Validate(); err != nil {
		t.Fatalf("article fixture rejected: %v", err)
	}
	assertLibrary(t, value.Library)
	if value.Ranking.Score < 0 || value.Ranking.Score > 1 || value.Ranking.AlgorithmVersion == "" {
		t.Fatal("invalid fixture ranking")
	}
	for _, item := range value.Ranking.Contributions {
		if item.Signal == "" || item.ReasonCode == "" || item.RawScore < 0 || item.RawScore > 1 || item.Weight < 0 || item.Weight > 1 || item.WeightedScore < 0 || item.WeightedScore > 1 {
			t.Fatal("invalid ranking contribution")
		}
	}
}

func assertLibrary(t *testing.T, value libraryFixture) {
	t.Helper()
	state := domain.LibraryState{ArticleID: domain.ArticleID(value.ArticleID)}
	if value.ReadAt != nil {
		parsed := mustTime(t, *value.ReadAt)
		state.ReadAt = &parsed
	}
	if value.SavedAt != nil {
		parsed := mustTime(t, *value.SavedAt)
		state.SavedAt = &parsed
	}
	if value.HiddenAt != nil {
		parsed := mustTime(t, *value.HiddenAt)
		state.HiddenAt = &parsed
	}
	if state.ArticleID == "" {
		t.Fatal("library state lost article id")
	}
}

func TestNegativeFixtureDomainSemantics(t *testing.T) {
	root := filepath.Join(fixtureRoot(t), "api")
	leaked := strictFile[articleDetailFixture](t, filepath.Join(root, "invalid-article-metadata-leak.json"))
	article := domain.Article{ID: domain.ArticleID(leaked.Article.ID), Fingerprint: "fixture:" + leaked.Article.ID, SourceID: domain.SourceID(leaked.Article.SourceID), CanonicalURL: leaked.Article.CanonicalURL, Title: leaked.Article.Title, FetchedAt: mustTime(t, leaked.Article.FetchedAt), FullContent: *leaked.FullContent, ContentPermission: domain.ContentPermission(leaked.Article.ContentPermission)}
	if !errors.Is(article.Validate(), domain.ErrInvalidArticle) {
		t.Fatal("metadata-only content leak was not rejected")
	}
	secret := strictFile[struct {
		AdapterConfig     map[string]any       `json:"adapterConfig"`
		Name              string               `json:"name"`
		URL               string               `json:"url"`
		Kind              string               `json:"kind"`
		Enabled           bool                 `json:"enabled"`
		ContentPermission string               `json:"contentPermission"`
		ScraperPolicy     scraperPolicyFixture `json:"scraperPolicy"`
	}](t, filepath.Join(root, "invalid-secret-source.fixture"))
	fields := map[string]string{}
	for key, value := range secret.AdapterConfig {
		fields[key] = fmt.Sprint(value)
	}
	if _, err := domain.ParseAdapterConfiguration(domain.SourceKind(secret.Kind), fields); !errors.Is(err, domain.ErrSensitiveAdapterConfiguration) {
		t.Fatal("secret adapter field was not rejected by domain")
	}
	invalid := strictFile[rankingConfigurationFixture](t, filepath.Join(root, "invalid-ranking-weight.json"))
	convert := func(v signalWeightFixture) domain.SignalWeight {
		return domain.SignalWeight{Enabled: v.Enabled, Weight: v.Weight}
	}
	configuration := domain.RankingConfiguration{Recency: convert(invalid.Recency), Interest: convert(invalid.Interest), SourcePreference: convert(invalid.SourcePreference), Behavior: convert(invalid.Behavior), Location: convert(invalid.Location), Age: convert(invalid.Age), Gender: convert(invalid.Gender), TextSimilarity: convert(invalid.TextSimilarity), PerDemographicCap: invalid.PerDemographicCap, TotalDemographicCap: invalid.TotalDemographicCap}
	if !errors.Is(configuration.Validate(), domain.ErrInvalidRankingConfiguration) {
		t.Fatal("invalid fixture weight was not rejected")
	}
}

func TestIngestionShapesAreOfflineSanitizedAndDeterministic(t *testing.T) {
	root := filepath.Join(fixtureRoot(t), "ingestion")
	manifest := strictFile[struct {
		Network     string `json:"network"`
		Credentials string `json:"credentials"`
		Fixtures    []struct {
			File       string `json:"file"`
			Adapter    string `json:"adapter"`
			Format     string `json:"format,omitempty"`
			Provider   string `json:"provider,omitempty"`
			Policy     string `json:"policy,omitempty"`
			Permission string `json:"permission"`
			ItemID     string `json:"itemId"`
			Selectors  *struct {
				Article     string `json:"article"`
				Title       string `json:"title"`
				Canonical   string `json:"canonical"`
				PublishedAt string `json:"publishedAt"`
				Excerpt     string `json:"excerpt"`
			} `json:"selectors,omitempty"`
			Expected *struct {
				Title        string `json:"title"`
				CanonicalURL string `json:"canonicalUrl"`
				PublishedAt  string `json:"publishedAt"`
				Excerpt      string `json:"excerpt"`
			} `json:"expected,omitempty"`
		} `json:"fixtures"`
		NegativeFixtures []struct {
			File   string `json:"file"`
			Reason string `json:"reason"`
		} `json:"negativeFixtures"`
	}](t, filepath.Join(root, "manifest.json"))
	if manifest.Network != "forbidden" || manifest.Credentials != "absent" || len(manifest.Fixtures) != 4 {
		t.Fatal("ingestion isolation contract incomplete")
	}
	for _, fixture := range manifest.Fixtures {
		data, err := os.ReadFile(filepath.Join(root, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		assertSanitized(t, fixture.File, data)
		if fixture.Permission != "metadata_only" {
			t.Fatal("fixture permission missing")
		}
		switch {
		case fixture.Format == "rss":
			var value struct {
				Channel struct {
					Items []struct {
						GUID        string `xml:"guid"`
						Title       string `xml:"title"`
						Link        string `xml:"link"`
						Description string `xml:"description"`
						Published   string `xml:"pubDate"`
					} `xml:"item"`
				} `xml:"channel"`
			}
			if xml.Unmarshal(data, &value) != nil || len(value.Channel.Items) != 1 || value.Channel.Items[0].GUID != fixture.ItemID || value.Channel.Items[0].Title == "" || value.Channel.Items[0].Link == "" {
				t.Fatal("invalid RSS shape")
			}
		case fixture.Format == "atom":
			var value struct {
				Entries []struct {
					ID    string `xml:"id"`
					Title string `xml:"title"`
					Link  struct {
						Href string `xml:"href,attr"`
					} `xml:"link"`
					Updated string `xml:"updated"`
					Summary string `xml:"summary"`
				} `xml:"entry"`
			}
			if xml.Unmarshal(data, &value) != nil || len(value.Entries) != 1 || value.Entries[0].ID != fixture.ItemID || value.Entries[0].Link.Href == "" {
				t.Fatal("invalid Atom shape")
			}
		case fixture.Adapter == "api":
			value := strictBytes[struct {
				Provider   string `json:"provider"`
				NextCursor string `json:"nextCursor"`
				Items      []struct {
					ID          string `json:"id"`
					URL         string `json:"url"`
					Title       string `json:"title"`
					PublishedAt string `json:"publishedAt"`
					Excerpt     string `json:"excerpt"`
				} `json:"items"`
			}](t, data)
			if value.Provider != fixture.Provider || len(value.Items) != 1 || value.Items[0].ID != fixture.ItemID {
				t.Fatal("invalid official API shape")
			}
		case fixture.Adapter == "scraper":
			if fixture.Selectors == nil || fixture.Selectors.Article != "article" || fixture.Selectors.Title != "h1" || fixture.Selectors.Canonical != "a.canonical" || fixture.Selectors.PublishedAt != "time[datetime]" || fixture.Selectors.Excerpt != "p.excerpt" {
				t.Fatal("scraper selector contract is incomplete")
			}
			assertScraper(t, data, fixture.ItemID, fixture.Policy, fixture.Permission, fixture.Expected)
		}
	}
	if len(manifest.NegativeFixtures) != 1 {
		t.Fatal("scraper negative proof missing")
	}
	invalid, err := os.ReadFile(filepath.Join(root, manifest.NegativeFixtures[0].File))
	if err != nil {
		t.Fatal(err)
	}
	if parseScraper(invalid) != nil {
		t.Fatal("arbitrary text containing id passed scraper structure validation")
	}
}

type scraperArticle struct {
	ID         string `xml:"data-fixture-id,attr"`
	Permission string `xml:"data-content-permission,attr"`
	Title      string `xml:"h1"`
	Link       struct {
		Class string `xml:"class,attr"`
		Href  string `xml:"href,attr"`
	} `xml:"a"`
	Published struct {
		Datetime string `xml:"datetime,attr"`
	} `xml:"time"`
	Excerpt struct {
		Class string `xml:"class,attr"`
		Text  string `xml:",chardata"`
	} `xml:"p"`
}

func parseScraper(data []byte) *scraperArticle {
	var page struct {
		Article *scraperArticle `xml:"body>article"`
	}
	if xml.Unmarshal(data, &page) != nil || page.Article == nil || page.Article.ID == "" || page.Article.Title == "" || page.Article.Link.Class != "canonical" || page.Article.Link.Href == "" || page.Article.Published.Datetime == "" || page.Article.Excerpt.Class != "excerpt" || strings.TrimSpace(page.Article.Excerpt.Text) == "" {
		return nil
	}
	return page.Article
}
func assertScraper(t *testing.T, data []byte, id, policy, permission string, expected *struct {
	Title        string `json:"title"`
	CanonicalURL string `json:"canonicalUrl"`
	PublishedAt  string `json:"publishedAt"`
	Excerpt      string `json:"excerpt"`
}) {
	t.Helper()
	value := parseScraper(data)
	if value == nil || policy != "approved" || value.ID != id || value.Permission != permission || expected == nil || value.Title != expected.Title || value.Link.Href != expected.CanonicalURL || value.Published.Datetime != expected.PublishedAt || strings.TrimSpace(value.Excerpt.Text) != expected.Excerpt {
		t.Fatal("approved scraper fields did not normalize as documented")
	}
	mustTime(t, value.Published.Datetime)
}
func assertSanitized(t *testing.T, file string, data []byte) {
	t.Helper()
	lower := strings.ToLower(string(data))
	for _, forbidden := range []string{"api_key", "apikey", "authorization:", "bearer ", "password", "secret"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("%s contains credential-like material", file)
		}
	}
}
func strictFile[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strictBytes[T](t, data)
}
func strictBytes[T any](t *testing.T, data []byte) T {
	t.Helper()
	var value T
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatal("fixture has trailing JSON")
	}
	return value
}
func mustTime(t *testing.T, value string) (result time.Time) {
	t.Helper()
	result, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
