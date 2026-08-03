package feeds

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type fakeFetcher struct {
	response application.FetchResponse
	err      error
	request  application.FetchRequest
}

func (f *fakeFetcher) Fetch(_ context.Context, request application.FetchRequest) (application.FetchResponse, error) {
	f.request = request
	return f.response, f.err
}

func source(format domain.FeedFormat) domain.Source {
	return domain.Source{ID: "feed", Name: "Feed", URL: "https://origin.example/rss", Kind: domain.SourceKindFeed, Enabled: true, AdapterConfig: domain.AdapterConfiguration{Feed: &domain.FeedConfiguration{Format: format}}, ContentPermission: domain.ContentMetadataOnly, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyNotApplicable}}
}
func response(body string) application.FetchResponse {
	return application.FetchResponse{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), FinalURL: "https://final.example/path/feed.xml", Headers: http.Header{"Etag": {`"new"`}, "Last-Modified": {"Sun, 02 Aug 2026 12:00:00 GMT"}}}
}

func TestRSSMappingsRelativeLinksNamespacesDatesAndPartialItems(t *testing.T) {
	body := `<?xml version="1.0"?><rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:dc="http://purl.org/dc/elements/1.1/"><channel>
<item><guid>shared</guid><title>First</title><link>../first?utm_source=x</link><dc:creator>Ada</dc:creator><description><![CDATA[<b>Summary</b>]]></description><content:encoded><![CDATA[<p>Body</p>]]></content:encoded><pubDate>Sun, 02 Aug 2026 12:00:00 +0200</pubDate><category>Science</category></item>
<item><guid>shared</guid><title>Second</title><link>/second</link><dc:date>2026-08-02T10:00:00Z</dc:date></item>
<item><guid>bad</guid><title></title></item></channel></rss>`
	f := &fakeFetcher{response: response(body)}
	got, err := (Adapter{Fetcher: f}).Fetch(context.Background(), source(domain.FeedFormatAuto), application.FetchCursor{ETag: `"old"`, LastModified: "old-date"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 || len(got.Warnings) != 1 {
		t.Fatalf("items=%d warnings=%v", len(got.Items), got.Warnings)
	}
	if got.Items[0].CanonicalURL != "https://final.example/first?utm_source=x" || got.Items[0].Author != "Ada" || got.Items[0].FullContent != "<p>Body</p>" || got.Items[0].PublishedAt == nil {
		t.Fatalf("first=%+v", got.Items[0])
	}
	if got.Items[0].ExternalID != got.Items[1].ExternalID || got.Items[0].CanonicalURL == got.Items[1].CanonicalURL {
		t.Fatalf("duplicate GUID mapping=%+v", got.Items)
	}
	if f.request.ETag != `"old"` || f.request.LastModified != "old-date" || f.request.MaxBytes != maxFeedBytes || len(f.request.AllowedContentTypes) == 0 {
		t.Fatalf("fetch boundary=%+v", f.request)
	}
	if got.NextCursor.ETag != `"new"` || got.NextCursor.LastModified == "" {
		t.Fatalf("cursor=%+v", got.NextCursor)
	}
}

func TestAtomMappingsAndEncoding(t *testing.T) {
	body := "<?xml version=\"1.0\" encoding=\"windows-1252\"?><feed xmlns=\"http://www.w3.org/2005/Atom\"><entry xml:lang=\"HU\"><id>a1</id><title>Caf\xe9</title><link rel=\"self\" href=\"/api/a1\"/><link rel=\"alternate\" href=\"article\"/><author><name>B\xe9la</name></author><published>2026-08-02T10:00:00.123Z</published><summary type=\"html\">Summary</summary><content type=\"html\">Body</content><category term=\"Local\"/></entry></feed>"
	f := &fakeFetcher{response: response(body)}
	got, err := (Adapter{Fetcher: f}).Fetch(context.Background(), source(domain.FeedFormatAtom), application.FetchCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items=%+v", got.Items)
	}
	i := got.Items[0]
	if i.Title != "Café" || i.Author != "Béla" || i.CanonicalURL != "https://final.example/path/article" || i.Language != "HU" || i.Excerpt != "Summary" || i.FullContent != "Body" || len(i.Topics) != 1 {
		t.Fatalf("item=%+v", i)
	}
}

func TestNotModifiedDoesNotParseAndPreservesAbsentValidators(t *testing.T) {
	f := &fakeFetcher{response: application.FetchResponse{StatusCode: http.StatusNotModified, Body: io.NopCloser(errorReader{}), FinalURL: "https://final.example/feed", Headers: http.Header{}}}
	got, err := (Adapter{Fetcher: f}).Fetch(context.Background(), source(domain.FeedFormatRSS), application.FetchCursor{ETag: `"same"`, LastModified: "date"})
	if err != nil || !got.Unchanged || len(got.Items) != 0 || got.NextCursor.ETag != `"same"` || got.NextCursor.LastModified != "date" {
		t.Fatalf("result=%+v err=%v", got, err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("must not read") }

func TestMalformedUnsupportedWrongFormatAndBounds(t *testing.T) {
	tests := []struct {
		name, body string
		format     domain.FeedFormat
	}{
		{"malformed", `<rss><channel><item></channel>`, domain.FeedFormatAuto},
		{"unsupported", `<html/>`, domain.FeedFormatAuto},
		{"trailing root", `<rss version="2.0"><channel/></rss><rss/>`, domain.FeedFormatAuto},
		{"wrong format", `<feed xmlns="http://www.w3.org/2005/Atom"></feed>`, domain.FeedFormatRSS},
		{"encoding", `<?xml version="1.0" encoding="x-danger"?><rss/>`, domain.FeedFormatAuto},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := &fakeFetcher{response: response(test.body)}
			_, err := (Adapter{Fetcher: f}).Fetch(context.Background(), source(test.format), application.FetchCursor{})
			if !errors.Is(err, ErrMalformedFeed) {
				t.Fatalf("err=%v", err)
			}
		})
	}
	t.Run("byte bound", func(t *testing.T) {
		f := &fakeFetcher{response: response(`<rss><channel>` + strings.Repeat("x", int(maxFeedBytes)) + `</channel></rss>`)}
		_, err := (Adapter{Fetcher: f}).Fetch(context.Background(), source(domain.FeedFormatAuto), application.FetchCursor{})
		if !errors.Is(err, ErrMalformedFeed) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("item bound", func(t *testing.T) {
		f := &fakeFetcher{response: response(`<rss><channel>` + strings.Repeat(`<item><title>x</title><link>/x</link></item>`, maxItems+1) + `</channel></rss>`)}
		_, err := (Adapter{Fetcher: f}).Fetch(context.Background(), source(domain.FeedFormatAuto), application.FetchCursor{})
		if !errors.Is(err, ErrMalformedFeed) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestRejectsInvalidSourceAndPropagatesFetcher(t *testing.T) {
	f := &fakeFetcher{err: context.DeadlineExceeded}
	_, err := (Adapter{Fetcher: f}).Fetch(context.Background(), source(domain.FeedFormatAuto), application.FetchCursor{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	_, err = (Adapter{}).Fetch(context.Background(), source(domain.FeedFormatAuto), application.FetchCursor{})
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}
