package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/adapters/sqlite"
	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
	_ "modernc.org/sqlite"
)

func TestMigrationCompatibilityMatrix(t *testing.T) {
	t.Run("empty and current", func(t *testing.T) {
		store, path := openStore(t)
		store.Close()
		before := migrationCount(t, path)
		store = reopenStore(t, path)
		defer store.Close()
		if after := migrationCount(t, path); before != 3 || after != before {
			t.Fatalf("migration counts before=%d after=%d", before, after)
		}
	})
	t.Run("interrupted migration is atomic and retryable", func(t *testing.T) {
		dir := t.TempDir()
		migrationDir := filepath.Join(dir, "migrations")
		must(t, os.Mkdir(migrationDir, 0o755))
		must(t, os.WriteFile(filepath.Join(migrationDir, "0001_bad.sql"), []byte("CREATE TABLE partial(id INTEGER); INVALID SQL;"), 0o600))
		path := filepath.Join(dir, "db.sqlite")
		_, err := sqlite.Open(context.Background(), sqlite.Config{Path: path, MigrationDir: migrationDir})
		if err == nil {
			t.Fatal("bad migration succeeded")
		}
		db := rawDB(t, path)
		defer db.Close()
		var count int
		must(t, db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='partial'`).Scan(&count))
		if count != 0 {
			t.Fatal("partial migration was not rolled back")
		}
		must(t, os.WriteFile(filepath.Join(migrationDir, "0001_good.sql"), []byte("CREATE TABLE complete(id INTEGER);"), 0o600))
		must(t, os.Remove(filepath.Join(migrationDir, "0001_bad.sql")))
		store, err := sqlite.Open(context.Background(), sqlite.Config{Path: path, MigrationDir: migrationDir})
		must(t, err)
		store.Close()
	})
	t.Run("newer schema rejected", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "db.sqlite")
		db := rawDB(t, path)
		_, err := db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,name TEXT NOT NULL,applied_at_ms INTEGER NOT NULL); INSERT INTO schema_migrations VALUES(99,'0099_future.sql',0)`)
		must(t, err)
		db.Close()
		_, err = sqlite.Open(context.Background(), sqlite.Config{Path: path, MigrationDir: migrations()})
		if !errors.Is(err, sqlite.ErrNewerSchema) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("history gap rejected against two migrations", func(t *testing.T) {
		dir := twoMigrationDir(t)
		path := filepath.Join(t.TempDir(), "gap.sqlite")
		db := rawDB(t, path)
		_, err := db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,name TEXT NOT NULL,applied_at_ms INTEGER NOT NULL); INSERT INTO schema_migrations VALUES(2,'0002_second.sql',0)`)
		must(t, err)
		db.Close()
		_, err = sqlite.Open(context.Background(), sqlite.Config{Path: path, MigrationDir: dir})
		if !errors.Is(err, sqlite.ErrMigrationState) {
			t.Fatalf("gap error=%v", err)
		}
	})
	t.Run("history name mismatch rejected against two migrations", func(t *testing.T) {
		dir := twoMigrationDir(t)
		path := filepath.Join(t.TempDir(), "name.sqlite")
		db := rawDB(t, path)
		_, err := db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,name TEXT NOT NULL,applied_at_ms INTEGER NOT NULL); INSERT INTO schema_migrations VALUES(1,'0001_wrong.sql',0)`)
		must(t, err)
		db.Close()
		_, err = sqlite.Open(context.Background(), sqlite.Config{Path: path, MigrationDir: dir})
		if !errors.Is(err, sqlite.ErrMigrationState) {
			t.Fatalf("name error=%v", err)
		}
	})
}

func TestRepositoryRoundTripsAndRestart(t *testing.T) {
	store, path := openStore(t)
	ctx := context.Background()
	source := feedSource("source-1", "https://example.com/feed")
	source.RefreshCursor = "cursor-1"
	source.RefreshETag = `"etag-1"`
	source.RefreshLastModified = "Sun, 02 Aug 2026 12:00:00 GMT"
	must(t, store.Sources().Save(ctx, source))
	gotSource, err := store.Sources().Get(ctx, source.ID)
	must(t, err)
	if gotSource.RefreshCursor != source.RefreshCursor || gotSource.RefreshETag != source.RefreshETag || gotSource.RefreshLastModified != source.RefreshLastModified {
		t.Fatalf("source cursor mismatch: %+v", gotSource)
	}
	now := time.Date(2026, 8, 2, 12, 34, 56, 789000000, time.FixedZone("offset", 3600))
	profile := domain.UserProfile{ID: domain.LocalProfileID, Interests: []domain.WeightedInterest{{Name: "science", Weight: .75}}, PreferredSources: []domain.SourceID{source.ID}, Location: domain.OptionalSignal[domain.Location]{Present: true, Enabled: true, Value: domain.Location{Country: "HU", Region: "Budapest", City: domain.OptionalSignal[string]{Present: true, Value: "Budapest"}}}, Age: domain.OptionalSignal[int]{Present: true, Value: 37}, Gender: domain.OptionalSignal[string]{Present: true, Enabled: false, Value: "nonbinary"}, UpdatedAt: now}
	must(t, store.Profiles().Save(ctx, profile))
	got, err := store.Profiles().Get(ctx, domain.LocalProfileID)
	must(t, err)
	if !got.UpdatedAt.Equal(now) || !got.Location.Enabled || got.Location.Value.City.Value != "Budapest" || got.Gender.Enabled {
		t.Fatalf("profile mismatch: %+v", got)
	}
	filter := domain.FeedFilterState{ProfileID: domain.LocalProfileID, SourceID: source.ID, Read: domain.ReadFilterUnread, SavedOnly: true, SearchQuery: "science", UpdatedAt: now}
	must(t, store.FeedFilters().Save(ctx, filter))
	gotFilter, err := store.FeedFilters().Get(ctx, domain.LocalProfileID)
	must(t, err)
	if gotFilter.SourceID != source.ID || gotFilter.Read != domain.ReadFilterUnread || !gotFilter.SavedOnly || gotFilter.SearchQuery != "science" || !gotFilter.UpdatedAt.Equal(now) {
		t.Fatalf("filter mismatch: %+v", gotFilter)
	}
	article := domain.Article{ID: "article-1", Fingerprint: "fp-1", SourceID: source.ID, CanonicalURL: "https://example.com/a", Title: "A", PublishedAt: &now, FetchedAt: now.Add(time.Minute), FullContent: "body", ContentPermission: domain.ContentFullAllowed, Topics: []string{"science"}, TokenCount: 1}
	result, err := store.Articles().Upsert(ctx, article)
	must(t, err)
	if !result.Inserted {
		t.Fatal("first upsert not inserted")
	}
	gotArticle, err := store.Articles().Get(ctx, article.ID)
	must(t, err)
	if gotArticle.FullContent != "body" || len(gotArticle.Topics) != 1 || !gotArticle.PublishedAt.Equal(now) {
		t.Fatalf("article mismatch %+v", gotArticle)
	}
	trueValue := true
	library, err := store.Libraries().Apply(ctx, article.ID, domain.LibraryPatch{Read: &trueValue, Saved: &trueValue}, now)
	must(t, err)
	if library.ReadAt == nil || library.SavedAt == nil {
		t.Fatal("library flags lost")
	}
	config := domain.RankingConfiguration{Recency: domain.SignalWeight{Enabled: true, Weight: .4}, Interest: domain.SignalWeight{Enabled: true, Weight: .6}, PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1"}
	must(t, store.Rankings().SaveConfiguration(ctx, config))
	ranking := domain.RankingResult{ArticleID: article.ID, Score: .7, AlgorithmVersion: "v1", CalculatedAt: now, Contributions: []domain.ScoreContribution{{Signal: domain.SignalInterest, RawScore: .8, Weight: .6, WeightedScore: .48, ReasonCode: "topic", ReasonValues: map[string]string{"topic": "science"}}}}
	must(t, store.Rankings().SaveResults(ctx, []domain.RankingResult{ranking}))
	gotRanking, err := store.Rankings().GetResult(ctx, article.ID)
	must(t, err)
	if gotRanking.Score != .7 || gotRanking.Contributions[0].ReasonValues["topic"] != "science" {
		t.Fatalf("ranking mismatch %+v", gotRanking)
	}
	finished := now.Add(time.Minute)
	run := domain.RefreshRun{ID: "run-1", StartedAt: now, FinishedAt: &finished, Status: domain.RefreshPartialSuccess, Outcomes: []domain.SourceRefreshOutcome{{SourceID: source.ID, Fetched: 2, Inserted: 1, Failed: 1, ErrorCode: "fetch_failed", ErrorSummary: "safe summary"}}}
	must(t, store.Refreshes().Create(ctx, run))
	store.Close()
	store = reopenStore(t, path)
	defer store.Close()
	gotRun, err := store.Refreshes().Get(ctx, run.ID)
	must(t, err)
	if gotRun.Status != domain.RefreshPartialSuccess || len(gotRun.Outcomes) != 1 {
		t.Fatalf("refresh mismatch %+v", gotRun)
	}
	gotLibrary, err := store.Libraries().Get(ctx, article.ID)
	must(t, err)
	if gotLibrary.SavedAt == nil {
		t.Fatal("library did not persist")
	}
}

func TestConstraintsTransactionsCancellationAndHistory(t *testing.T) {
	store, path := openStore(t)
	defer store.Close()
	ctx := context.Background()
	source := feedSource("s1", "https://example.com/one")
	must(t, store.Sources().Save(ctx, source))
	duplicate := feedSource("s2", source.URL)
	if err := store.Sources().Save(ctx, duplicate); !errors.Is(err, application.ErrConflict) {
		t.Fatalf("unique error=%v", err)
	}
	article := domain.Article{ID: "a1", Fingerprint: "same", SourceID: source.ID, SourceExternalID: "external-1", CanonicalURL: "https://example.com/a", Title: "A", FetchedAt: time.Now(), FullContent: "accepted body", ContentPermission: domain.ContentFullAllowed, Topics: []string{"original"}}
	_, err := store.Articles().Upsert(ctx, article)
	must(t, err)
	byCanonical := article
	byCanonical.ID = "canonical-id"
	byCanonical.SourceExternalID = "external-2"
	byCanonical.Title = "canonical refresh"
	byCanonical.ContentPermission = domain.ContentMetadataOnly
	byCanonical.FullContent = ""
	byCanonical.FetchedAt = article.FetchedAt.Add(time.Second)
	result, err := store.Articles().Upsert(ctx, byCanonical)
	must(t, err)
	if result.ArticleID != "a1" || !result.Updated {
		t.Fatalf("canonical merge=%+v", result)
	}
	byFingerprint := byCanonical
	byFingerprint.ID = "fingerprint-id"
	byFingerprint.CanonicalURL = "https://example.com/new-canonical"
	byFingerprint.SourceExternalID = "external-3"
	byFingerprint.Topics = []string{"new"}
	byFingerprint.FetchedAt = byCanonical.FetchedAt.Add(time.Second)
	result, err = store.Articles().Upsert(ctx, byFingerprint)
	if !errors.Is(err, application.ErrConflict) {
		t.Fatalf("fingerprint/canonical disagreement error=%v result=%+v", err, result)
	}
	byExternal := byCanonical
	byExternal.ID = "external-id"
	byExternal.Fingerprint = "external-fp"
	byExternal.CanonicalURL = "https://example.com/external"
	byExternal.SourceExternalID = "external-1"
	byExternal.FetchedAt = byCanonical.FetchedAt.Add(time.Second)
	result, err = store.Articles().Upsert(ctx, byExternal)
	must(t, err)
	if result.ArticleID != "external-id" || !result.Inserted {
		t.Fatalf("reused external ID collapsed distinct canonical URL: %+v", result)
	}
	var articleCount int
	db := rawDB(t, path)
	must(t, db.QueryRow(`SELECT count(*) FROM articles`).Scan(&articleCount))
	var canonicalExternal, aliasesJSON string
	var firstSeen, lastSeen int64
	must(t, db.QueryRow(`SELECT external_id,external_ids_json,first_seen_at_ms,last_seen_at_ms FROM article_sources WHERE article_id='a1' AND source_id='s1'`).Scan(&canonicalExternal, &aliasesJSON, &firstSeen, &lastSeen))
	db.Close()
	if articleCount != 2 {
		t.Fatalf("dedup left %d articles", articleCount)
	}
	if canonicalExternal != "external-1" || aliasesJSON != `["external-1","external-2"]` || firstSeen != article.FetchedAt.UnixMilli() || lastSeen != byCanonical.FetchedAt.UnixMilli() {
		t.Fatalf("provenance external=%q aliases=%s first=%d last=%d", canonicalExternal, aliasesJSON, firstSeen, lastSeen)
	}
	merged, err := store.Articles().Get(ctx, "a1")
	must(t, err)
	if len(merged.Topics) != 1 || merged.FullContent != "" || merged.ContentPermission != domain.ContentMetadataOnly {
		t.Fatalf("authoritative metadata-only update retained disallowed data: %+v", merged)
	}
	missing := article
	missing.ID = "missing"
	missing.Fingerprint = "different"
	missing.CanonicalURL = "https://example.com/missing"
	missing.SourceID = "none"
	if _, err = store.Articles().Upsert(ctx, missing); !errors.Is(err, application.ErrConflict) {
		t.Fatalf("FK error=%v", err)
	}
	err = store.WithinTransaction(ctx, func(txctx context.Context) error {
		must(t, store.Sources().Save(txctx, feedSource("rollback", "https://example.com/rollback")))
		return errors.New("stop")
	})
	if err == nil {
		t.Fatal("rollback callback succeeded")
	}
	if _, err = store.Sources().Get(ctx, "rollback"); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("rollback source exists: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	err = store.WithinTransaction(cancelled, func(context.Context) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	trueValue := true
	_, err = store.Libraries().Apply(ctx, "a1", domain.LibraryPatch{Saved: &trueValue}, time.Now())
	must(t, err)
	must(t, store.Sources().Delete(ctx, source.ID))
	if _, err = store.Sources().Get(ctx, source.ID); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("deleted source visible: %v", err)
	}
	listed, err := store.Sources().List(ctx)
	must(t, err)
	if len(listed) != 0 {
		t.Fatalf("deleted source listed: %+v", listed)
	}
	db = rawDB(t, path)
	var retainedName string
	var deletedAt sql.NullInt64
	must(t, db.QueryRow(`SELECT name,deleted_at_ms FROM sources WHERE id=?`, source.ID).Scan(&retainedName, &deletedAt))
	db.Close()
	if retainedName != source.Name || !deletedAt.Valid {
		t.Fatalf("source tombstone missing: name=%q deleted=%v", retainedName, deletedAt.Valid)
	}
	if _, err = store.Articles().Get(ctx, "a1"); err != nil {
		t.Fatalf("history lost: %v", err)
	}
	if library, err := store.Libraries().Get(ctx, "a1"); err != nil || library.SavedAt == nil {
		t.Fatalf("library history lost: %+v %v", library, err)
	}
	must(t, store.Sources().Save(ctx, source))
	if _, err = store.Sources().Get(ctx, source.ID); err != nil {
		t.Fatalf("source restore failed: %v", err)
	}
	store.Close()
	store = reopenStore(t, path)
	defer store.Close()
	if _, err = store.Articles().Get(ctx, "a1"); err != nil {
		t.Fatalf("restart history lost: %v", err)
	}
}

func TestQueryFeedRejectsInvalidCursorAndPropagatesRepositoryErrors(t *testing.T) {
	store, path := openStore(t)
	defer store.Close()
	ctx := context.Background()
	source := feedSource("feed", "https://example.com/feed-errors")
	must(t, store.Sources().Save(ctx, source))
	article := domain.Article{ID: "feed-a", Fingerprint: "feed-fp", SourceID: source.ID, CanonicalURL: "https://example.com/feed-a", Title: "feed", FetchedAt: time.Now(), ContentPermission: domain.ContentMetadataOnly}
	_, err := store.Articles().Upsert(ctx, article)
	must(t, err)
	if _, err = store.Articles().QueryFeed(ctx, application.FeedQuery{Cursor: "not valid base64"}); !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("cursor error=%v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = store.Articles().QueryFeed(cancelled, application.FeedQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled feed error=%v", err)
	}
	page, err := store.Articles().QueryFeed(ctx, application.FeedQuery{})
	must(t, err)
	if len(page.Articles) != 1 || page.Articles[0].Library.ArticleID != article.ID || page.Articles[0].Ranking.ArticleID != article.ID {
		t.Fatalf("empty optional state feed=%+v", page)
	}
	value := true
	_, err = store.Libraries().Apply(ctx, article.ID, domain.LibraryPatch{Read: &value}, time.Now())
	must(t, err)
	page, err = store.Articles().QueryFeed(ctx, application.FeedQuery{})
	must(t, err)
	if page.Articles[0].Library.ReadAt == nil || page.Articles[0].Ranking.ArticleID != article.ID {
		t.Fatalf("missing ranking feed=%+v", page)
	}
	ranking := domain.RankingResult{ArticleID: article.ID, Score: .5, AlgorithmVersion: "test", CalculatedAt: time.Now()}
	must(t, store.Rankings().SaveResults(ctx, []domain.RankingResult{ranking}))
	db := rawDB(t, path)
	_, err = db.Exec(`DELETE FROM library_states WHERE article_id='feed-a'`)
	must(t, err)
	db.Close()
	page, err = store.Articles().QueryFeed(ctx, application.FeedQuery{})
	must(t, err)
	if page.Articles[0].Library.ArticleID != article.ID || page.Articles[0].Ranking.Score != .5 {
		t.Fatalf("missing library feed=%+v", page)
	}
	db = rawDB(t, path)
	_, err = db.Exec(`DROP TABLE ranking_contributions; DROP TABLE ranking_results`)
	must(t, err)
	db.Close()
	if _, err = store.Articles().QueryFeed(ctx, application.FeedQuery{}); !errors.Is(err, application.ErrUnavailable) {
		t.Fatalf("storage error=%v", err)
	}
}

func TestLockedDatabaseMapsToUnavailable(t *testing.T) {
	store, path := openStore(t)
	defer store.Close()
	locker := rawDB(t, path)
	defer locker.Close()
	_, err := locker.Exec(`PRAGMA busy_timeout=0; BEGIN EXCLUSIVE`)
	must(t, err)
	defer locker.Exec(`ROLLBACK`)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = store.Sources().Save(ctx, feedSource("locked", "https://example.com/locked"))
	if !errors.Is(err, application.ErrUnavailable) {
		t.Fatalf("locked error=%v", err)
	}
}

func TestPerSourceRefreshTransactions(t *testing.T) {
	store, _ := openStore(t)
	defer store.Close()
	ctx := context.Background()
	for _, s := range []domain.Source{feedSource("ok", "https://example.com/ok"), feedSource("failed", "https://example.com/failed")} {
		must(t, store.Sources().Save(ctx, s))
	}
	must(t, store.WithinTransaction(ctx, func(tx context.Context) error {
		_, err := store.Articles().Upsert(tx, domain.Article{ID: "ok-a", Fingerprint: "ok", SourceID: "ok", CanonicalURL: "https://example.com/ok/a", Title: "ok", FetchedAt: time.Now(), ContentPermission: domain.ContentMetadataOnly})
		return err
	}))
	_ = store.WithinTransaction(ctx, func(tx context.Context) error {
		_, err := store.Articles().Upsert(tx, domain.Article{ID: "bad-a", Fingerprint: "bad", SourceID: "failed", CanonicalURL: "https://example.com/fail/a", Title: "bad", FetchedAt: time.Now(), ContentPermission: domain.ContentMetadataOnly})
		if err != nil {
			return err
		}
		return errors.New("source fetch failed")
	})
	if _, err := store.Articles().Get(ctx, "ok-a"); err != nil {
		t.Fatalf("successful source not committed: %v", err)
	}
	if _, err := store.Articles().Get(ctx, "bad-a"); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("failed source committed: %v", err)
	}
}

func TestSchemaHasReferencesButNoCredentialValues(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(migrations(), "0001_initial.sql"))
	must(t, err)
	lower := strings.ToLower(string(body))
	for _, forbidden := range []string{"api_key", "access_token", "password", "secret_value", "credential_value"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("schema contains secret-bearing column %q", forbidden)
		}
	}
	if !strings.Contains(lower, "credential_ref") {
		t.Fatal("opaque credential reference missing")
	}
}

func openStore(t *testing.T) (*sqlite.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "news.sqlite")
	return openAt(t, path), path
}
func reopenStore(t *testing.T, path string) *sqlite.Store { t.Helper(); return openAt(t, path) }
func openAt(t *testing.T, path string) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(context.Background(), sqlite.Config{Path: path, MigrationDir: migrations(), BusyTimeout: 100 * time.Millisecond})
	must(t, err)
	return store
}
func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)")
	must(t, err)
	return db
}
func migrationCount(t *testing.T, path string) int {
	t.Helper()
	db := rawDB(t, path)
	defer db.Close()
	var count int
	must(t, db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&count))
	return count
}
func migrations() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "db", "migrations"))
}
func twoMigrationDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "0001_first.sql"), []byte(`CREATE TABLE first(id INTEGER);`), 0o600))
	must(t, os.WriteFile(filepath.Join(dir, "0002_second.sql"), []byte(`CREATE TABLE second(id INTEGER);`), 0o600))
	return dir
}
func feedSource(id domain.SourceID, url string) domain.Source {
	return domain.Source{ID: id, Name: string(id), URL: url, Kind: domain.SourceKindFeed, Enabled: true, ContentPermission: domain.ContentMetadataOnly, AdapterConfig: domain.AdapterConfiguration{Feed: &domain.FeedConfiguration{Format: domain.FeedFormatRSS}}, ScraperPolicy: domain.ScraperPolicy{Status: domain.ScraperPolicyNotApplicable}}
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
