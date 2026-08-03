// Package feeds implements bounded RSS 2.0 and Atom ingestion.
package feeds

import (
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

const (
	maxFeedBytes = int64(8 << 20)
	maxItems     = 5000
	maxDepth     = 64
	maxItemText  = 1 << 20
	atomNS       = "http://www.w3.org/2005/Atom"
	dcNS         = "http://purl.org/dc/elements/1.1/"
	contentNS    = "http://purl.org/rss/1.0/modules/content/"
	xmlNS        = "http://www.w3.org/XML/1998/namespace"
)

var ErrMalformedFeed = errors.New("feed adapter: malformed feed")

type Adapter struct{ Fetcher application.HTTPFetcher }

func (Adapter) Kind() domain.SourceKind { return domain.SourceKindFeed }

func (a Adapter) Fetch(ctx context.Context, source domain.Source, cursor application.FetchCursor) (application.AdapterResult, error) {
	if a.Fetcher == nil || source.Validate() != nil || source.Kind != domain.SourceKindFeed {
		return application.AdapterResult{}, application.ErrInvalidInput
	}
	response, err := a.Fetcher.Fetch(ctx, application.FetchRequest{
		URL: source.URL, SourceID: source.ID, MaxBytes: maxFeedBytes,
		AllowedContentTypes: []string{"application/rss+xml", "application/atom+xml", "application/xml", "text/xml"},
		ETag:                cursor.ETag, LastModified: cursor.LastModified,
	})
	if err != nil {
		return application.AdapterResult{}, err
	}
	if response.Body == nil {
		return application.AdapterResult{}, fmt.Errorf("%w: missing response body", ErrMalformedFeed)
	}
	defer response.Body.Close()
	next := application.FetchCursor{Value: cursor.Value, ETag: header(response.Headers, "ETag"), LastModified: header(response.Headers, "Last-Modified")}
	if next.ETag == "" {
		next.ETag = cursor.ETag
	}
	if next.LastModified == "" {
		next.LastModified = cursor.LastModified
	}
	if response.StatusCode == http.StatusNotModified {
		return application.AdapterResult{Unchanged: true, NextCursor: next}, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return application.AdapterResult{}, fmt.Errorf("feed adapter: HTTP status %d", response.StatusCode)
	}
	base, err := url.Parse(response.FinalURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return application.AdapterResult{}, fmt.Errorf("%w: invalid final URL", ErrMalformedFeed)
	}
	items, warnings, format, err := parse(io.LimitReader(response.Body, maxFeedBytes+1), base)
	if err != nil {
		return application.AdapterResult{}, err
	}
	configured := source.AdapterConfig.Feed.Format
	if configured != domain.FeedFormatAuto && string(configured) != format {
		return application.AdapterResult{}, fmt.Errorf("%w: expected %s", ErrMalformedFeed, configured)
	}
	return application.AdapterResult{Items: items, NextCursor: next, Warnings: warnings}, nil
}

func header(values map[string][]string, name string) string { return http.Header(values).Get(name) }

func parse(r io.Reader, base *url.URL) ([]application.AdapterItem, []string, string, error) {
	counting := &countReader{r: r}
	d := xml.NewDecoder(bufio.NewReader(counting))
	d.Strict = true
	d.CharsetReader = charsetReader
	var root xml.StartElement
	for {
		t, err := d.Token()
		if err != nil {
			return nil, nil, "", malformed(err)
		}
		if start, ok := t.(xml.StartElement); ok {
			root = start
			break
		}
	}
	format := ""
	defaultLanguage := attr(node{attrs: attributes(root)}, xmlNS, "lang")
	switch strings.ToLower(root.Name.Local) {
	case "rss":
		if root.Name.Space != "" {
			return nil, nil, "", fmt.Errorf("%w: invalid RSS namespace", ErrMalformedFeed)
		}
		format = "rss"
	case "feed":
		if root.Name.Space != atomNS {
			return nil, nil, "", fmt.Errorf("%w: invalid Atom namespace", ErrMalformedFeed)
		}
		format = "atom"
	default:
		return nil, nil, "", fmt.Errorf("%w: unsupported root %q", ErrMalformedFeed, root.Name.Local)
	}
	var items []application.AdapterItem
	var warnings []string
	depth := 1
	for {
		t, err := d.Token()
		if err == io.EOF {
			return nil, nil, "", fmt.Errorf("%w: unclosed root", ErrMalformedFeed)
		}
		if err != nil {
			return nil, nil, "", malformed(err)
		}
		if _, ok := t.(xml.EndElement); ok {
			depth--
			if depth == 0 {
				if err := documentEnd(d); err != nil {
					return nil, nil, "", err
				}
				break
			}
			continue
		}
		start, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		name := strings.ToLower(start.Name.Local)
		if format == "rss" && start.Name.Space == "" && name == "language" {
			n, readErr := readNode(d, start, 0, new(int))
			if readErr != nil {
				return nil, nil, "", malformed(readErr)
			}
			defaultLanguage = strings.TrimSpace(n.text)
			continue
		}
		isItem := (format == "rss" && start.Name.Space == "" && name == "item") || (format == "atom" && start.Name.Space == atomNS && name == "entry")
		if !isItem {
			depth++
			continue
		}
		if len(items)+len(warnings) >= maxItems {
			return nil, nil, "", fmt.Errorf("%w: item limit exceeded", ErrMalformedFeed)
		}
		n, err := readNode(d, start, 0, new(int))
		if err != nil {
			return nil, nil, "", malformed(err)
		}
		item, err := mapItem(n, format, base)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("item %d skipped: invalid required metadata", len(items)+len(warnings)+1))
			continue
		}
		if item.Language == "" {
			item.Language = defaultLanguage
		}
		items = append(items, item)
	}
	return items, warnings, format, nil
}

func documentEnd(d *xml.Decoder) error {
	for {
		token, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return malformed(err)
		}
		if data, ok := token.(xml.CharData); ok && strings.TrimSpace(string(data)) == "" {
			continue
		}
		if _, ok := token.(xml.Comment); ok {
			continue
		}
		return fmt.Errorf("%w: trailing document content", ErrMalformedFeed)
	}
}

type node struct {
	name, space, text string
	attrs             map[string]string
	children          []node
}

func readNode(d *xml.Decoder, start xml.StartElement, depth int, size *int) (node, error) {
	if depth > maxDepth {
		return node{}, errors.New("element depth exceeded")
	}
	n := node{name: strings.ToLower(start.Name.Local), space: start.Name.Space, attrs: map[string]string{}}
	for _, a := range start.Attr {
		n.attrs[a.Name.Space+"\x00"+strings.ToLower(a.Name.Local)] = a.Value
	}
	for {
		t, err := d.Token()
		if err != nil {
			return node{}, err
		}
		switch value := t.(type) {
		case xml.CharData:
			*size += len(value)
			if *size > maxItemText {
				return node{}, errors.New("item text limit exceeded")
			}
			n.text += string(value)
		case xml.StartElement:
			child, err := readNode(d, value, depth+1, size)
			if err != nil {
				return node{}, err
			}
			n.children = append(n.children, child)
		case xml.EndElement:
			return n, nil
		}
	}
}

func mapItem(n node, format string, base *url.URL) (application.AdapterItem, error) {
	var out application.AdapterItem
	if format == "rss" {
		out.ExternalID = value(n, "", "guid")
		out.Title = value(n, "", "title")
		out.CanonicalURL = value(n, "", "link")
		out.Author = first(value(n, "", "author"), value(n, dcNS, "creator"))
		out.Excerpt = value(n, "", "description")
		out.FullContent = value(n, contentNS, "encoded")
		out.Language = attr(n, "", "language")
		out.Topics = values(n, "", "category")
		out.PublishedAt = parseDate(first(value(n, "", "pubdate"), value(n, "", "published"), value(n, "", "updated"), value(n, dcNS, "date")))
	} else {
		out.ExternalID = value(n, atomNS, "id")
		out.Title = value(n, atomNS, "title")
		out.CanonicalURL = atomLink(n)
		out.Author = nestedValue(n, "author", "name")
		out.Excerpt = value(n, atomNS, "summary")
		out.FullContent = value(n, atomNS, "content")
		out.Language = attr(n, xmlNS, "lang")
		out.Topics = atomCategories(n)
		out.PublishedAt = parseDate(first(value(n, atomNS, "published"), value(n, atomNS, "updated")))
	}
	resolved, err := base.Parse(strings.TrimSpace(out.CanonicalURL))
	if err != nil || resolved.Scheme == "" || resolved.Host == "" {
		return out, ErrMalformedFeed
	}
	out.CanonicalURL = resolved.String()
	if strings.TrimSpace(out.Title) == "" {
		return out, ErrMalformedFeed
	}
	if out.ExternalID == "" {
		out.ExternalID = out.CanonicalURL
	}
	return out, nil
}

func value(n node, space, name string) string {
	for _, c := range n.children {
		if c.name == name && c.space == space {
			return strings.TrimSpace(allText(c))
		}
	}
	return ""
}

func allText(n node) string {
	var out strings.Builder
	out.WriteString(n.text)
	for _, child := range n.children {
		out.WriteByte(' ')
		out.WriteString(allText(child))
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func attributes(start xml.StartElement) map[string]string {
	result := make(map[string]string, len(start.Attr))
	for _, a := range start.Attr {
		result[a.Name.Space+"\x00"+strings.ToLower(a.Name.Local)] = a.Value
	}
	return result
}
func values(n node, space, name string) []string {
	var out []string
	for _, c := range n.children {
		if c.name == name && c.space == space && strings.TrimSpace(allText(c)) != "" {
			out = append(out, strings.TrimSpace(allText(c)))
		}
	}
	return out
}
func attr(n node, space, name string) string { return strings.TrimSpace(n.attrs[space+"\x00"+name]) }
func nestedValue(n node, parent, child string) string {
	for _, c := range n.children {
		if c.name == parent && c.space == atomNS {
			return value(c, atomNS, child)
		}
	}
	return ""
}
func first(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
func atomLink(n node) string {
	var fallback string
	for _, c := range n.children {
		if c.name != "link" || c.space != atomNS {
			continue
		}
		href := attr(c, "", "href")
		if href == "" {
			continue
		}
		rel := strings.ToLower(attr(c, "", "rel"))
		if rel == "alternate" || rel == "" {
			return href
		}
		if fallback == "" {
			fallback = href
		}
	}
	return fallback
}
func atomCategories(n node) []string {
	var out []string
	for _, c := range n.children {
		if c.name == "category" && c.space == atomNS {
			if v := first(attr(c, "", "term"), c.text); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

func parseDate(raw string) *time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC850, time.ANSIC, "Mon, 2 Jan 2006 15:04:05 MST", "2006-01-02 15:04:05Z07:00"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(raw)); err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}

func malformed(err error) error { return fmt.Errorf("%w: %v", ErrMalformedFeed, err) }

type countReader struct {
	r    io.Reader
	read int64
}

func (r *countReader) Read(p []byte) (int, error) {
	remainingProbe := maxFeedBytes + 1 - r.read
	if remainingProbe <= 0 {
		return 0, errors.New("feed byte limit exceeded")
	}
	if int64(len(p)) > remainingProbe {
		p = p[:remainingProbe]
	}
	n, err := r.r.Read(p)
	r.read += int64(n)
	if r.read > maxFeedBytes {
		return n, errors.New("feed byte limit exceeded")
	}
	return n, err
}

func charsetReader(label string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "utf-8", "utf8", "us-ascii", "ascii":
		return input, nil
	case "iso-8859-1", "latin1", "latin-1":
		return &singleByteReader{r: input}, nil
	case "windows-1252", "cp1252":
		return &singleByteReader{r: input, windows1252: true}, nil
	default:
		return nil, fmt.Errorf("unsupported XML encoding %q", label)
	}
}

type singleByteReader struct {
	r           io.Reader
	windows1252 bool
	pending     []byte
}

func (r *singleByteReader) Read(p []byte) (int, error) {
	if len(r.pending) > 0 {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		return n, nil
	}
	b := make([]byte, max(1, len(p)/2))
	n, err := r.r.Read(b)
	var out strings.Builder
	for _, c := range b[:n] {
		runeValue := rune(c)
		if r.windows1252 && c >= 0x80 && c <= 0x9f {
			runeValue = cp1252[c-0x80]
		}
		out.WriteRune(runeValue)
	}
	r.pending = []byte(out.String())
	copied := copy(p, r.pending)
	r.pending = r.pending[copied:]
	if copied > 0 {
		return copied, nil
	}
	return 0, err
}

var cp1252 = [...]rune{'€', 0x81, '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 0x8d, 'Ž', 0x8f, 0x90, '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 0x9d, 'ž', 'Ÿ'}

var _ application.IngestionAdapter = Adapter{}
