package ingestion

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

func TestCanonicalURL(t *testing.T) {
	tests := []struct {
		name, raw, base, want string
		invalid               bool
	}{
		{"tracking fragment and order", "HTTPS://Example.COM:443/story?z=2&utm_source=x&a=1#comments", "", "https://example.com/story?a=1&z=2", false},
		{"relative", "../article?id=7&fbclid=bad", "https://news.example/section/feed.xml", "https://news.example/article?id=7", false},
		{"root", "http://EXAMPLE.com:80", "", "http://example.com/", false},
		{"preserve meaningful query", "https://example.com/a?ref=edition", "", "https://example.com/a?ref=edition", false},
		{"preserve double slash path", "https://example.com/edition//story", "", "https://example.com/edition//story", false},
		{"preserve trailing slash before query", "https://example.com/story/?id=7", "", "https://example.com/story/?id=7", false},
		{"remove exact mailchimp trackers", "https://example.com/a?mc_cid=bad&mc_eid=bad&id=7", "", "https://example.com/a?id=7", false},
		{"preserve publisher mc parameter", "https://example.com/a?mc_section=local", "", "https://example.com/a?mc_section=local", false},
		{"preserve unknown utm parameter", "https://example.com/a?utm_section=local", "", "https://example.com/a?utm_section=local", false},
		{"javascript", "javascript:alert(1)", "https://example.com/feed", "", true},
		{"userinfo", "https://user@example.com/a", "", "", true},
		{"malformed", ":// broken", "https://example.com/feed", "", true},
		{"missing", "", "https://example.com/feed", "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CanonicalURL(test.raw, test.base)
			if test.invalid && err == nil {
				t.Fatalf("expected rejection, got %q", got)
			}
			if !test.invalid && (err != nil || got != test.want) {
				t.Fatalf("got %q, %v; want %q", got, err, test.want)
			}
		})
	}
}

func TestNormalizePermissionDatesAndDeterminism(t *testing.T) {
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	item := application.AdapterItem{ExternalID: " guid-1 ", CanonicalURL: "/story?utm_medium=rss", Title: " <b>Same</b> story ", Author: "", PublishedAt: &future, Excerpt: "Hello <img src=x onerror=alert(1)> world", FullContent: `<p>Allowed</p><script>alert(1)</script><a href="javascript:evil()">link</a>`, Topics: []string{"Go", "Go", "<b>Security</b>"}}
	metadata := testSource(domain.ContentMetadataOnly)
	first, err := Normalize(metadata, item, now, deterministicID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Normalize(metadata, item, now, deterministicID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("not deterministic:\n%+v\n%+v", first, second)
	}
	if first.FullContent != "" || first.PublishedAt != nil || first.Author != "" || first.Language != "" {
		t.Fatalf("permission or optional metadata fabricated: %+v", first)
	}
	if first.Excerpt != "Hello world" || first.Title != "Same story" {
		t.Fatalf("metadata not sanitized: %+v", first)
	}

	allowed := testSource(domain.ContentFullAllowed)
	full, err := Normalize(allowed, item, now, deterministicID)
	if err != nil {
		t.Fatal(err)
	}
	if full.FullContent != "Allowed link" || strings.ContainsAny(full.FullContent, "<>") || strings.Contains(strings.ToLower(full.FullContent), "javascript") {
		t.Fatalf("unsafe normalized body %q", full.FullContent)
	}
	if full.TokenCount != 2 {
		t.Fatalf("token count=%d", full.TokenCount)
	}

	past := now.Add(-time.Hour)
	item.PublishedAt = &past
	withDate, err := Normalize(allowed, item, now, deterministicID)
	if err != nil || withDate.PublishedAt == nil || !withDate.PublishedAt.Equal(past) {
		t.Fatalf("past date lost: %+v %v", withDate, err)
	}
	item.PublishedAt = nil
	withoutDate, err := Normalize(allowed, item, now, deterministicID)
	if err != nil || withoutDate.PublishedAt != nil {
		t.Fatalf("absent date fabricated: %+v %v", withoutDate, err)
	}
}

func TestPlainTextMaliciousInputs(t *testing.T) {
	tests := map[string]string{
		`<svg onload=alert(1)><text>safe</text></svg>`:      "safe",
		`before<style>body{display:none}</style>after`:      "before after",
		`<!-- <script>bad</script> --><p>A&nbsp;B</p>`:      "A B",
		`<template><img src=x onerror=x></template>visible`: "visible",
		`before<SCRIPT data-x=">">alert(1)</SCRIPT>after`:   "before after",
		`<p title='malicious > delimiter'>quoted-safe</p>`:  "quoted-safe",
		`before<script>unterminated raw text`:               "before",
		`encoded &lt;img src=x onerror=alert(1)&gt; text`:   "encoded img src=x onerror=alert(1) text",
		`Kx`:                            "Kx",
		`Kx<SCRIPT>alert(1)</SCRIPT>世界`: "Kx 世界",
		"bad\x00\x01 good":              "bad good",
		"invalid\xffutf8":               "invalidutf8",
	}
	for raw, want := range tests {
		if got := PlainText(raw); got != want {
			t.Errorf("PlainText(%q)=%q want %q", raw, got, want)
		}
	}
}

func FuzzPlainTextUnicodeAndMalformedMarkup(f *testing.F) {
	for _, seed := range []string{
		"Kx",
		"Kx<SCRIPT>alert(1)</SCRIPT>世界",
		"İ<script data-x='>'>bad()</script>é",
		"invalid\xffutf8<tag title='unterminated>",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		got := PlainText(raw)
		if !utf8.ValidString(got) {
			t.Fatalf("invalid UTF-8 output %q", got)
		}
		if strings.ContainsAny(got, "<>") {
			t.Fatalf("markup delimiters survived in %q", got)
		}
	})
}

func TestSimilarTitlesHaveDistinctStableIdentity(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	source := testSource(domain.ContentMetadataOnly)
	a, err := Normalize(source, application.AdapterItem{ExternalID: "same-bad-guid", CanonicalURL: "/one", Title: "Market update"}, now, deterministicID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Normalize(source, application.AdapterItem{ExternalID: "same-bad-guid", CanonicalURL: "/two", Title: "Market update"}, now, deterministicID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint == b.Fingerprint || a.ID == b.ID {
		t.Fatalf("distinct URLs collapsed: %+v %+v", a, b)
	}
}

func TestNormalizationFixtureSnapshot(t *testing.T) {
	type fixtureCase struct {
		Name         string `json:"name"`
		InputURL     string `json:"input_url"`
		BaseURL      string `json:"base_url"`
		CanonicalURL string `json:"canonical_url"`
	}
	var fixture struct {
		FixedClock string        `json:"fixed_clock"`
		Cases      []fixtureCase `json:"cases"`
	}
	_, filename, _, _ := runtime.Caller(0)
	body, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "../../../test/fixtures/ingestion/normalization-matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if _, err = time.Parse(time.RFC3339, fixture.FixedClock); err != nil {
		t.Fatal(err)
	}
	for _, test := range fixture.Cases {
		if test.InputURL == "" || test.CanonicalURL == "" {
			continue
		}
		t.Run(test.Name, func(t *testing.T) {
			base := test.BaseURL
			if base == "" {
				base = "https://news.example/feed.xml"
			}
			first, err := CanonicalURL(test.InputURL, base)
			if err != nil || first != test.CanonicalURL {
				t.Fatalf("got %q,%v want %q", first, err, test.CanonicalURL)
			}
			second, err := CanonicalURL(test.InputURL, base)
			if err != nil || second != first {
				t.Fatalf("snapshot not deterministic: %q,%q,%v", first, second, err)
			}
		})
	}
}

func deterministicID(fingerprint string) domain.ArticleID {
	return domain.ArticleID("article-" + fingerprint[len(fingerprint)-12:])
}

func testSource(permission domain.ContentPermission) domain.Source {
	return domain.Source{ID: "source-1", Name: "Publisher", URL: "https://news.example/feed.xml", Kind: domain.SourceKindFeed, Enabled: true, AdapterConfig: domain.AdapterConfiguration{Feed: &domain.FeedConfiguration{Format: domain.FeedFormatAuto}}, ContentPermission: permission, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyNotApplicable}}
}
