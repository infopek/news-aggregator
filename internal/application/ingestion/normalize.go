// Package ingestion converts adapter-neutral records into safe, stable domain
// articles. Adapters must pass their output through this package before storage.
package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"html"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

var ErrInvalidItem = errors.New("invalid ingestion item")

// IDGenerator is injected so tests and imports are repeatable. It receives the
// stable identity fingerprint, allowing callers to derive or allocate an ID.
type IDGenerator func(fingerprint string) domain.ArticleID

type Service struct {
	Articles application.ArticleRepository
	Clock    application.Clock
	NewID    IDGenerator
}

// Ingest normalizes and upserts every item in adapter order. A failure stops the
// batch so refresh orchestration can report the source outcome explicitly.
func (s Service) Ingest(ctx context.Context, source domain.Source, items []application.AdapterItem) ([]application.ArticleWriteResult, error) {
	if s.Articles == nil || s.Clock == nil || s.NewID == nil || source.Validate() != nil {
		return nil, application.ErrInvalidInput
	}
	results := make([]application.ArticleWriteResult, 0, len(items))
	for _, item := range items {
		article, err := Normalize(source, item, s.Clock.Now(), s.NewID)
		if err != nil {
			return nil, err
		}
		result, err := s.Articles.Upsert(ctx, article)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

// Normalize applies the canonical identity, metadata, date and permission
// policy. Missing optional values remain empty; no author/date/language is
// inferred.
func Normalize(source domain.Source, item application.AdapterItem, fetchedAt time.Time, newID IDGenerator) (domain.Article, error) {
	var article domain.Article
	if source.Validate() != nil || newID == nil || fetchedAt.IsZero() {
		return article, application.ErrInvalidInput
	}
	canonical, err := CanonicalURL(item.CanonicalURL, source.URL)
	if err != nil {
		return article, ErrInvalidItem
	}
	title := PlainText(item.Title)
	if title == "" {
		return article, ErrInvalidItem
	}
	fetchedAt = fetchedAt.UTC()
	fingerprint := Fingerprint(canonical)
	article = domain.Article{
		ID:                newID(fingerprint),
		Fingerprint:       fingerprint,
		SourceID:          source.ID,
		SourceExternalID:  strings.TrimSpace(item.ExternalID),
		CanonicalURL:      canonical,
		Title:             title,
		Author:            PlainText(item.Author),
		FetchedAt:         fetchedAt,
		Excerpt:           PlainText(item.Excerpt),
		ContentPermission: source.ContentPermission,
		Language:          normalizeLanguage(item.Language),
		Topics:            normalizeTopics(item.Topics),
	}
	if item.PublishedAt != nil && !item.PublishedAt.After(fetchedAt) {
		published := item.PublishedAt.UTC()
		article.PublishedAt = &published
	}
	if source.ContentPermission == domain.ContentFullAllowed {
		article.FullContent = PlainText(item.FullContent)
	}
	article.TokenCount = len(strings.Fields(article.FullContent))
	if article.ID == "" || article.Validate() != nil {
		return domain.Article{}, ErrInvalidItem
	}
	return article, nil
}

// CanonicalURL resolves relative publisher URLs, rejects malformed/non-HTTP
// links, removes fragments and well-known tracking parameters, then emits a
// deterministic query order. It deliberately does not rewrite content paths.
func CanonicalURL(raw, base string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\x00\r\n") {
		return "", ErrInvalidItem
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", ErrInvalidItem
	}
	if !parsed.IsAbs() {
		baseURL, baseErr := url.Parse(strings.TrimSpace(base))
		if baseErr != nil || !validWebURL(baseURL) {
			return "", ErrInvalidItem
		}
		parsed = baseURL.ResolveReference(parsed)
	}
	if !validWebURL(parsed) || parsed.User != nil {
		return "", ErrInvalidItem
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	parsed.Host = host
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	query := parsed.Query()
	for key := range query {
		if trackingParameter(key) {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func validWebURL(value *url.URL) bool {
	return value != nil && (strings.EqualFold(value.Scheme, "http") || strings.EqualFold(value.Scheme, "https")) && value.Hostname() != ""
}

func trackingParameter(key string) bool {
	key = strings.ToLower(key)
	switch key {
	case "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id", "utm_source_platform", "utm_creative_format", "utm_marketing_tactic",
		"mc_cid", "mc_eid", "fbclid", "gclid", "dclid", "msclkid", "igshid":
		return true
	default:
		return false
	}
}

// Fingerprint intentionally uses only the canonical publisher URL. External
// IDs remain provenance aliases: publishers sometimes reuse GUIDs for distinct
// stable URLs, so a GUID alone must never collapse those articles.
func Fingerprint(canonicalURL string) string {
	sum := sha256.Sum256([]byte("canonical-url\x00" + canonicalURL))
	return "url-sha256:" + hex.EncodeToString(sum[:])
}

// PlainText is a conservative safe representation for untrusted HTML. It drops
// tags, comments, script/style/template bodies and control characters, decodes
// entities, and normalizes whitespace. Stored full content therefore contains
// no executable markup or attacker-controlled link targets.
func PlainText(raw string) string {
	var out strings.Builder
	lower := strings.ToLower(raw)
	for i := 0; i < len(raw); {
		if strings.HasPrefix(lower[i:], "<!--") {
			end := strings.Index(lower[i+4:], "-->")
			if end < 0 {
				break
			}
			i += 4 + end + 3
			out.WriteByte(' ')
			continue
		}
		if raw[i] == '<' {
			end := tagEnd(raw, i+1)
			if end < 0 {
				out.WriteString(" &lt;")
				i++
				continue
			}
			tag := strings.TrimSpace(lower[i+1 : end])
			name := strings.Fields(strings.TrimLeft(tag, "/!"))
			if len(name) > 0 && (name[0] == "script" || name[0] == "style" || name[0] == "template") && !strings.HasPrefix(tag, "/") {
				closeTag := "</" + name[0]
				close := strings.Index(lower[end+1:], closeTag)
				if close < 0 {
					break
				}
				close += end + 1
				closeEnd := tagEnd(raw, close+1)
				if closeEnd < 0 {
					break
				}
				i = closeEnd + 1
				out.WriteByte(' ')
				continue
			}
			out.WriteByte(' ')
			i = end + 1
			continue
		}
		r, size := utf8.DecodeRuneInString(raw[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if unicode.IsControl(r) {
			out.WriteByte(' ')
		} else {
			out.WriteRune(r)
		}
		i += size
	}
	decoded := html.UnescapeString(out.String())
	decoded = strings.NewReplacer("<", " ", ">", " ").Replace(decoded)
	return strings.Join(strings.Fields(decoded), " ")
}

// tagEnd finds a tag terminator while respecting quoted attribute values. This
// avoids treating attacker-controlled '>' characters inside attributes as the
// end of a tag. An unterminated quote/tag is treated as malformed markup.
func tagEnd(raw string, start int) int {
	var quote byte
	for i := start; i < len(raw); i++ {
		switch {
		case quote != 0 && raw[i] == quote:
			quote = 0
		case quote == 0 && (raw[i] == '\'' || raw[i] == '"'):
			quote = raw[i]
		case quote == 0 && raw[i] == '>':
			return i
		}
	}
	return -1
}

func normalizeLanguage(raw string) string { return strings.ToLower(strings.TrimSpace(raw)) }

func normalizeTopics(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = PlainText(value)
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
