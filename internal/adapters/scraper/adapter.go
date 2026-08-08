// Package scraper implements a fail-closed, selector-based public-page adapter.
package scraper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
	"golang.org/x/net/html"
)

const maxBody = int64(4 << 20)
const policyMaxAge = 180 * 24 * time.Hour

var ErrPolicyRefused = errors.New("scraper adapter: policy refused execution")
var ErrInvalidPage = errors.New("scraper adapter: invalid page")

type Adapter struct {
	Fetcher application.HTTPFetcher
	Now     func() time.Time
}

func (Adapter) Kind() domain.SourceKind { return domain.SourceKindScraper }

func (a Adapter) Fetch(ctx context.Context, source domain.Source, _ application.FetchCursor) (application.AdapterResult, error) {
	if a.Fetcher == nil || source.Kind != domain.SourceKindScraper || source.AdapterConfig.Scraper == nil || source.CredentialRef != nil {
		return application.AdapterResult{}, application.ErrInvalidInput
	}
	if !approved(source, a.now()) {
		return application.AdapterResult{}, ErrPolicyRefused
	}
	response, err := a.Fetcher.Fetch(ctx, application.FetchRequest{URL: source.URL, SourceID: source.ID, MaxBytes: maxBody, AllowedContentTypes: []string{"text/html", "application/xhtml+xml"}})
	if err != nil {
		return application.AdapterResult{}, err
	}
	if response.Body == nil {
		return application.AdapterResult{}, ErrInvalidPage
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return application.AdapterResult{}, fmt.Errorf("scraper adapter: HTTP status %d", response.StatusCode)
	}
	base, finalErr := url.Parse(response.FinalURL)
	original, originalErr := url.Parse(source.URL)
	if finalErr != nil || originalErr != nil || !strings.EqualFold(base.Hostname(), original.Hostname()) {
		return application.AdapterResult{}, ErrPolicyRefused
	}
	document, err := html.Parse(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return application.AdapterResult{}, ErrInvalidPage
	}
	config := source.AdapterConfig.Scraper
	articleSelector, err := parseSelector(config.ArticleSelector)
	if err != nil {
		return application.AdapterResult{}, application.ErrInvalidInput
	}
	titleSelector, err := parseSelector(config.TitleSelector)
	if err != nil {
		return application.AdapterResult{}, application.ErrInvalidInput
	}
	var excerptSelector, contentSelector selector
	if config.ExcerptSelector != "" {
		excerptSelector, err = parseSelector(config.ExcerptSelector)
		if err != nil {
			return application.AdapterResult{}, application.ErrInvalidInput
		}
	}
	if config.ContentSelector != "" {
		contentSelector, err = parseSelector(config.ContentSelector)
		if err != nil {
			return application.AdapterResult{}, application.ErrInvalidInput
		}
	}
	articles := findAll(document, articleSelector)
	if len(articles) == 0 {
		return application.AdapterResult{}, ErrInvalidPage
	}
	items := make([]application.AdapterItem, 0, len(articles))
	for _, article := range articles {
		titleNode := findFirst(article, titleSelector)
		if titleNode == nil {
			return application.AdapterResult{}, ErrInvalidPage
		}
		link := findFirst(article, selector{tag: "a", class: "canonical"})
		if link == nil {
			return application.AdapterResult{}, ErrInvalidPage
		}
		href := attribute(link, "href")
		resolved, err := base.Parse(href)
		if err != nil || resolved.Scheme != "https" && resolved.Scheme != "http" || !strings.EqualFold(resolved.Hostname(), original.Hostname()) {
			return application.AdapterResult{}, ErrPolicyRefused
		}
		item := application.AdapterItem{ExternalID: attribute(article, "data-fixture-id"), CanonicalURL: resolved.String(), Title: text(titleNode)}
		if config.ExcerptSelector != "" {
			if node := findFirst(article, excerptSelector); node != nil {
				item.Excerpt = text(node)
			}
		}
		if config.ContentSelector != "" && source.ContentPermission == domain.ContentFullAllowed {
			if node := findFirst(article, contentSelector); node != nil {
				item.FullContent = text(node)
			}
		}
		if timeNode := findFirst(article, selector{tag: "time"}); timeNode != nil {
			if parsed, parseErr := time.Parse(time.RFC3339, attribute(timeNode, "datetime")); parseErr == nil {
				item.PublishedAt = &parsed
			}
		}
		if item.ExternalID == "" {
			item.ExternalID = item.CanonicalURL
		}
		items = append(items, item)
	}
	return application.AdapterResult{Items: items}, nil
}

func (a Adapter) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}
func approved(source domain.Source, now time.Time) bool {
	p := source.ScraperPolicy
	if source.Validate() != nil || !source.Enabled || p.Status != domain.ScraperPolicyApproved || p.ReviewedAt == nil || p.ReviewedAt.After(now) || now.Sub(*p.ReviewedAt) > policyMaxAge {
		return false
	}
	sourceURL, e1 := url.Parse(source.URL)
	terms, e2 := url.Parse(p.TermsURL)
	robots, e3 := url.Parse(p.RobotsURL)
	return e1 == nil && e2 == nil && e3 == nil && strings.EqualFold(sourceURL.Hostname(), terms.Hostname()) && strings.EqualFold(sourceURL.Hostname(), robots.Hostname())
}

type selector struct{ tag, class string }

func parseSelector(raw string) (selector, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " #[]>+~,:*") {
		return selector{}, application.ErrInvalidInput
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 {
		return selector{}, application.ErrInvalidInput
	}
	s := selector{}
	if strings.HasPrefix(raw, ".") {
		s.class = parts[1]
	} else {
		s.tag = strings.ToLower(parts[0])
		if len(parts) == 2 {
			s.class = parts[1]
		}
	}
	return s, nil
}
func matches(n *html.Node, s selector) bool {
	if n.Type != html.ElementNode || s.tag != "" && n.Data != s.tag {
		return false
	}
	if s.class == "" {
		return true
	}
	for _, c := range strings.Fields(attribute(n, "class")) {
		if c == s.class {
			return true
		}
	}
	return false
}
func findFirst(root *html.Node, s selector) *html.Node {
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if matches(child, s) {
			return child
		}
		if found := findFirst(child, s); found != nil {
			return found
		}
	}
	return nil
}
func findAll(root *html.Node, s selector) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if matches(n, s) {
			out = append(out, n)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return out
}
func attribute(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}
func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(v *html.Node) {
		if v.Type == html.TextNode {
			b.WriteString(v.Data)
			b.WriteByte(' ')
		}
		for child := v.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}
