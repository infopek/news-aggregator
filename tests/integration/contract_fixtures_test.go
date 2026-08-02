package integration_test

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/domain"
)

type apiManifest map[string]string

type envelope struct {
	ID       string           `json:"id"`
	Status   string           `json:"status"`
	Code     string           `json:"code"`
	Article  *articleFixture  `json:"article"`
	Items    []articleFixture `json:"items"`
	Outcomes []struct {
		Failed int `json:"failed"`
	} `json:"outcomes"`
}

type articleFixture struct {
	ID                string  `json:"id"`
	SourceID          string  `json:"sourceId"`
	CanonicalURL      string  `json:"canonicalUrl"`
	Title             string  `json:"title"`
	FetchedAt         string  `json:"fetchedAt"`
	FullContent       *string `json:"fullContent"`
	ContentPermission string  `json:"contentPermission"`
	Ranking           struct {
		Contributions []struct {
			Signal     string `json:"signal"`
			ReasonCode string `json:"reasonCode"`
		} `json:"contributions"`
	} `json:"ranking"`
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "test", "fixtures"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestEveryAPIResponseFixtureDecodes(t *testing.T) {
	root := filepath.Join(fixtureRoot(t), "api")
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest apiManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	wantFamilies := map[string]bool{"Health": false, "Profile": false, "RankingConfiguration": false, "Source": false, "SourceList": false, "CredentialStatus": false, "RefreshRun": false, "FeedPage": false, "ArticleDetail": false, "LibraryState": false, "APIError": false}
	for file, family := range manifest {
		value, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		var decoded envelope
		if err := json.Unmarshal(value, &decoded); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		wantFamilies[family] = true
	}
	for family, covered := range wantFamilies {
		if !covered {
			t.Errorf("API response family %s has no fixture", family)
		}
	}
}

func TestFixtureDomainSemantics(t *testing.T) {
	root := filepath.Join(fixtureRoot(t), "api")
	var page struct {
		Items []articleFixture `json:"items"`
	}
	decodeJSON(t, filepath.Join(root, "feed-page.json"), &page)
	if len(page.Items) != 1 {
		t.Fatal("feed fixture must contain one item")
	}
	item := page.Items[0]
	article := domain.Article{ID: domain.ArticleID(item.ID), Fingerprint: "fixture-fingerprint", SourceID: domain.SourceID(item.SourceID), CanonicalURL: item.CanonicalURL, Title: item.Title, FetchedAt: mustTime(t, item.FetchedAt), ContentPermission: domain.ContentPermission(item.ContentPermission)}
	if err := article.Validate(); err != nil {
		t.Fatalf("positive article rejected: %v", err)
	}

	var leaked struct {
		Article     articleFixture `json:"article"`
		FullContent string         `json:"fullContent"`
	}
	decodeJSON(t, filepath.Join(root, "invalid-article-metadata-leak.json"), &leaked)
	article.FullContent = leaked.FullContent
	if !errors.Is(article.Validate(), domain.ErrInvalidArticle) {
		t.Fatal("metadata-only full content was not rejected")
	}

	configuration := domain.RankingConfiguration{Recency: domain.SignalWeight{Enabled: true, Weight: 1.01}, PerDemographicCap: .1, TotalDemographicCap: .2}
	if !errors.Is(configuration.Validate(), domain.ErrInvalidRankingConfiguration) {
		t.Fatal("invalid ranking weight was not rejected")
	}

	var allowed struct {
		Article struct {
			Ranking struct {
				Contributions []struct {
					ReasonCode string `json:"reasonCode"`
				} `json:"contributions"`
			} `json:"ranking"`
		} `json:"article"`
	}
	decodeJSON(t, filepath.Join(root, "article-full-content.json"), &allowed)
	if allowed.Article.Ranking.Contributions[0].ReasonCode != "FUTURE_REASON_CODE" {
		t.Fatal("unknown reason code must remain forward compatible")
	}
}

func TestIngestionShapesAreOfflineSanitizedAndDeterministic(t *testing.T) {
	root := filepath.Join(fixtureRoot(t), "ingestion")
	var manifest struct {
		Network     string `json:"network"`
		Credentials string `json:"credentials"`
		Fixtures    []struct{ File, Adapter, Format, Policy, Permission, ItemID string }
	}
	decodeJSON(t, filepath.Join(root, "manifest.json"), &manifest)
	if manifest.Network != "forbidden" || manifest.Credentials != "absent" || len(manifest.Fixtures) != 4 {
		t.Fatal("ingestion isolation contract is incomplete")
	}
	for _, fixture := range manifest.Fixtures {
		data, err := os.ReadFile(filepath.Join(root, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(data))
		for _, forbidden := range []string{"api_key", "apikey", "authorization:", "bearer ", "password", "secret"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s contains credential-like material", fixture.File)
			}
		}
		if fixture.Permission != "metadata_only" || !strings.Contains(string(data), fixture.ItemID) {
			t.Fatalf("%s lacks deterministic permission/id", fixture.File)
		}
		switch fixture.Format {
		case "rss":
			var value struct {
				XMLName xml.Name `xml:"rss"`
				Channel struct {
					Items []struct {
						GUID string `xml:"guid"`
					} `xml:"item"`
				} `xml:"channel"`
			}
			if xml.Unmarshal(data, &value) != nil || value.Channel.Items[0].GUID != fixture.ItemID {
				t.Fatalf("invalid RSS fixture")
			}
		case "atom":
			var value struct {
				Entries []struct {
					ID string `xml:"id"`
				} `xml:"entry"`
			}
			if xml.Unmarshal(data, &value) != nil || value.Entries[0].ID != fixture.ItemID {
				t.Fatalf("invalid Atom fixture")
			}
		case "":
			if fixture.Adapter == "api" {
				var value struct {
					Items []struct {
						ID string `json:"id"`
					} `json:"items"`
				}
				if json.Unmarshal(data, &value) != nil || value.Items[0].ID != fixture.ItemID {
					t.Fatal("invalid API fixture")
				}
			} else if fixture.Policy != "approved" {
				t.Fatal("scraper fixture is not approved")
			}
		}
	}
}

func decodeJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func mustTime(t *testing.T, value string) (result time.Time) {
	t.Helper()
	result, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
