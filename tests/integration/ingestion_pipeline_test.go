package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/application/ingestion"
	"github.com/infopek/news-aggregator/internal/domain"
)

type ingestionClock struct{ value time.Time }

func (c ingestionClock) Now() time.Time { return c.value }

func TestIngestionInsertRepeatUpdateCollisionAndProvenance(t *testing.T) {
	store, path := openStore(t)
	defer store.Close()
	ctx := context.Background()
	source := feedSource("ingestion-source", "https://publisher.example/feed.xml")
	source.ContentPermission = domain.ContentFullAllowed
	must(t, store.Sources().Save(ctx, source))
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	service := ingestion.Service{Articles: store.Articles(), Clock: ingestionClock{now}, NewID: func(fp string) domain.ArticleID { return domain.ArticleID("id-" + fp[len(fp)-16:]) }}

	first := application.AdapterItem{ExternalID: "guid-1", CanonicalURL: "/story?utm_source=feed", Title: "Story", FullContent: `<p>safe</p><script>bad()</script>`, Topics: []string{"one"}}
	results, err := service.Ingest(ctx, source, []application.AdapterItem{first})
	must(t, err)
	if len(results) != 1 || !results[0].Inserted {
		t.Fatalf("insert=%+v", results)
	}
	firstID := results[0].ArticleID

	now = now.Add(time.Minute)
	service.Clock = ingestionClock{now}
	repeat := first
	repeat.CanonicalURL = "https://publisher.example/story"
	repeat.ExternalID = "guid-renamed"
	repeat.Title = "Story updated"
	repeat.Topics = []string{"two"}
	results, err = service.Ingest(ctx, source, []application.AdapterItem{repeat})
	must(t, err)
	if !results[0].Updated || results[0].ArticleID != firstID {
		t.Fatalf("repeat=%+v", results)
	}

	// A reused publisher GUID and fuzzy-equal title do not override a distinct
	// stable canonical URL.
	distinct := repeat
	distinct.CanonicalURL = "/different"
	distinct.ExternalID = "guid-1"
	results, err = service.Ingest(ctx, source, []application.AdapterItem{distinct})
	must(t, err)
	if !results[0].Inserted {
		t.Fatalf("reused GUID collapsed: %+v", results)
	}

	db := rawDB(t, path)
	defer db.Close()
	var count int
	must(t, db.QueryRow(`SELECT count(*) FROM articles`).Scan(&count))
	if count != 2 {
		t.Fatalf("article count=%d", count)
	}
	var aliases string
	must(t, db.QueryRow(`SELECT external_ids_json FROM article_sources WHERE article_id=?`, "id-"+ingestion.Fingerprint("https://publisher.example/story")[len(ingestion.Fingerprint("https://publisher.example/story"))-16:]).Scan(&aliases))
	if aliases != `["guid-1","guid-renamed"]` {
		t.Fatalf("aliases=%s", aliases)
	}
	var body sql.NullString
	must(t, db.QueryRow(`SELECT full_content FROM articles WHERE canonical_url='https://publisher.example/story'`).Scan(&body))
	if !body.Valid || body.String != "safe" || strings.ContainsAny(body.String, "<>") {
		t.Fatalf("stored unsafe body=%q", body.String)
	}
}

func TestIngestionMetadataOnlyNeverPersistsBody(t *testing.T) {
	store, path := openStore(t)
	defer store.Close()
	ctx := context.Background()
	source := feedSource("metadata-source", "https://metadata.example/feed")
	source.ContentPermission = domain.ContentMetadataOnly
	must(t, store.Sources().Save(ctx, source))
	service := ingestion.Service{Articles: store.Articles(), Clock: ingestionClock{time.Unix(100, 0).UTC()}, NewID: func(string) domain.ArticleID { return "metadata-id" }}
	_, err := service.Ingest(ctx, source, []application.AdapterItem{{CanonicalURL: "/article", Title: `<img src=x onerror=x>Allowed title`, Excerpt: `<b>Allowed excerpt</b>`, FullContent: `<p>forbidden body</p>`}})
	must(t, err)
	db := rawDB(t, path)
	defer db.Close()
	var title, excerpt string
	var body sql.NullString
	must(t, db.QueryRow(`SELECT title,excerpt,full_content FROM articles WHERE id='metadata-id'`).Scan(&title, &excerpt, &body))
	if title != "Allowed title" || excerpt != "Allowed excerpt" || body.Valid {
		t.Fatalf("metadata policy title=%q excerpt=%q body=%q", title, excerpt, body.String)
	}
}

func TestIngestionNewerMetadataOnlyObservationClearsStoredBody(t *testing.T) {
	store, _ := openStore(t)
	defer store.Close()
	ctx := context.Background()
	source := feedSource("permission-source", "https://permission.example/feed")
	source.ContentPermission = domain.ContentFullAllowed
	must(t, store.Sources().Save(ctx, source))
	now := time.Unix(100, 0).UTC()
	service := ingestion.Service{Articles: store.Articles(), Clock: ingestionClock{now}, NewID: func(string) domain.ArticleID { return "permission-id" }}
	item := application.AdapterItem{CanonicalURL: "/article", Title: "Title", FullContent: "<p>previously permitted</p>"}
	_, err := service.Ingest(ctx, source, []application.AdapterItem{item})
	must(t, err)

	source.ContentPermission = domain.ContentMetadataOnly
	service.Clock = ingestionClock{now.Add(time.Minute)}
	item.FullContent = "<p>must be discarded</p>"
	_, err = service.Ingest(ctx, source, []application.AdapterItem{item})
	must(t, err)
	stored, err := store.Articles().Get(ctx, "permission-id")
	must(t, err)
	if stored.ContentPermission != domain.ContentMetadataOnly || stored.FullContent != "" {
		t.Fatalf("permission downgrade retained body: %+v", stored)
	}
}

func TestIngestionGeneratedIDCollisionIsConflict(t *testing.T) {
	store, _ := openStore(t)
	defer store.Close()
	ctx := context.Background()
	source := feedSource("collision-source", "https://collision.example/feed")
	must(t, store.Sources().Save(ctx, source))
	service := ingestion.Service{Articles: store.Articles(), Clock: ingestionClock{time.Unix(100, 0).UTC()}, NewID: func(string) domain.ArticleID { return "always-the-same-id" }}
	_, err := service.Ingest(ctx, source, []application.AdapterItem{{CanonicalURL: "/one", Title: "Same title"}})
	must(t, err)
	_, err = service.Ingest(ctx, source, []application.AdapterItem{{CanonicalURL: "/two", Title: "Same title"}})
	if !errors.Is(err, application.ErrConflict) {
		t.Fatalf("generated ID collision error=%v", err)
	}
	stored, err := store.Articles().Get(ctx, "always-the-same-id")
	must(t, err)
	if stored.CanonicalURL != "https://collision.example/one" {
		t.Fatalf("collision overwrote existing identity: %+v", stored)
	}
}
