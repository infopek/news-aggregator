package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type ArticleRepository struct{ store *Store }

func (s *Store) Articles() *ArticleRepository { return &ArticleRepository{store: s} }
func (r *ArticleRepository) Get(ctx context.Context, id domain.ArticleID) (domain.Article, error) {
	return r.store.getArticle(ctx, id)
}
func (r *ArticleRepository) Upsert(ctx context.Context, v domain.Article) (application.ArticleWriteResult, error) {
	return r.store.upsertArticle(ctx, v)
}
func (r *ArticleRepository) ListForRanking(ctx context.Context) ([]domain.Article, error) {
	return r.store.listArticles(ctx, application.FeedQuery{})
}
func (r *ArticleRepository) QueryFeed(ctx context.Context, q application.FeedQuery) (application.FeedPage, error) {
	return r.store.queryFeed(ctx, q)
}

const articleColumns = `a.id,a.fingerprint,a.primary_source_id,a.canonical_url,a.title,a.author,a.published_at_ms,a.fetched_at_ms,a.excerpt,a.full_content,a.content_permission,a.language,a.token_count`

func (s *Store) upsertArticle(ctx context.Context, a domain.Article) (application.ArticleWriteResult, error) {
	var out application.ArticleWriteResult
	if err := a.Validate(); err != nil {
		return out, application.ErrInvalidInput
	}
	return out, s.WithinTransaction(ctx, func(ctx context.Context) error {
		observationSource, observationExternalID, observedAt := a.SourceID, a.SourceExternalID, a.FetchedAt
		existing, existed, err := s.resolveArticleIdentity(ctx, a)
		if err != nil {
			return err
		}
		if existed {
			stored, err := s.getArticle(ctx, existing)
			if err != nil {
				return err
			}
			a = mergeArticle(stored, a)
			a.ID = existing
		}
		full := any(nil)
		if a.ContentPermission == domain.ContentFullAllowed && a.FullContent != "" {
			full = a.FullContent
		}
		result, err := s.q(ctx).ExecContext(ctx, `INSERT INTO articles(id,fingerprint,primary_source_id,canonical_url,title,author,published_at_ms,fetched_at_ms,excerpt,full_content,content_permission,language,token_count) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET fingerprint=excluded.fingerprint,primary_source_id=excluded.primary_source_id,canonical_url=excluded.canonical_url,title=excluded.title,author=excluded.author,published_at_ms=excluded.published_at_ms,fetched_at_ms=excluded.fetched_at_ms,excerpt=excluded.excerpt,full_content=excluded.full_content,content_permission=excluded.content_permission,language=excluded.language,token_count=excluded.token_count`, a.ID, a.Fingerprint, a.SourceID, a.CanonicalURL, a.Title, a.Author, nullableMillis(a.PublishedAt), millis(a.FetchedAt), a.Excerpt, full, a.ContentPermission, a.Language, a.TokenCount)
		if err != nil {
			return mapError(err)
		}
		_, _ = result.RowsAffected()
		out = application.ArticleWriteResult{ArticleID: a.ID, Inserted: !existed, Updated: existed}
		aliases, err := s.externalAliases(ctx, a.ID, observationSource, observationExternalID)
		if err != nil {
			return err
		}
		aliasesJSON, err := json.Marshal(aliases)
		if err != nil {
			return application.ErrInvalidInput
		}
		canonicalExternalID, err := s.availableExternalID(ctx, a.ID, observationSource, observationExternalID)
		if err != nil {
			return err
		}
		if _, err = s.q(ctx).ExecContext(ctx, `INSERT INTO article_sources(article_id,source_id,external_id,first_seen_at_ms,last_seen_at_ms,external_ids_json) VALUES(?,?,?,?,?,?) ON CONFLICT(article_id,source_id) DO UPDATE SET external_id=CASE WHEN article_sources.external_id='' THEN excluded.external_id ELSE article_sources.external_id END,first_seen_at_ms=MIN(article_sources.first_seen_at_ms,excluded.first_seen_at_ms),last_seen_at_ms=MAX(article_sources.last_seen_at_ms,excluded.last_seen_at_ms),external_ids_json=excluded.external_ids_json`, a.ID, observationSource, canonicalExternalID, millis(observedAt), millis(observedAt), string(aliasesJSON)); err != nil {
			return mapError(err)
		}
		if _, err = s.q(ctx).ExecContext(ctx, `DELETE FROM article_topics WHERE article_id=?`, a.ID); err != nil {
			return mapError(err)
		}
		for _, topic := range a.Topics {
			if _, err = s.q(ctx).ExecContext(ctx, `INSERT INTO article_topics(article_id,topic)VALUES(?,?)`, a.ID, topic); err != nil {
				return mapError(err)
			}
		}
		return nil
	})
}

// availableExternalID honors the legacy unique canonical-alias column without
// allowing a publisher's reused GUID to collapse or reject a distinct URL. The
// reused value remains in external_ids_json as provenance.
func (s *Store) availableExternalID(ctx context.Context, articleID domain.ArticleID, sourceID domain.SourceID, incoming string) (string, error) {
	if incoming == "" {
		return "", nil
	}
	var owner domain.ArticleID
	err := s.q(ctx).QueryRowContext(ctx, `SELECT article_id FROM article_sources WHERE source_id=? AND external_id=?`, sourceID, incoming).Scan(&owner)
	if err == sql.ErrNoRows || owner == articleID {
		return incoming, nil
	}
	if err != nil {
		return "", mapError(err)
	}
	return "", nil
}

func (s *Store) resolveArticleIdentity(ctx context.Context, incoming domain.Article) (domain.ArticleID, bool, error) {
	ids := map[domain.ArticleID]struct{}{}
	rows, err := s.q(ctx).QueryContext(ctx, `SELECT id,fingerprint,canonical_url FROM articles WHERE id=? OR fingerprint=? OR canonical_url=?`, incoming.ID, incoming.Fingerprint, incoming.CanonicalURL)
	if err != nil {
		return "", false, mapError(err)
	}
	for rows.Next() {
		var id domain.ArticleID
		var fingerprint, canonicalURL string
		if err := rows.Scan(&id, &fingerprint, &canonicalURL); err != nil {
			rows.Close()
			return "", false, mapError(err)
		}
		// All stable identity components must agree. In particular, an injected
		// generated-ID collision cannot overwrite an unrelated article, and a
		// fingerprint collision cannot silently rewrite a publisher URL.
		if fingerprint != incoming.Fingerprint || canonicalURL != incoming.CanonicalURL {
			rows.Close()
			return "", false, application.ErrConflict
		}
		ids[id] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return "", false, mapError(err)
	}
	if incoming.SourceExternalID != "" {
		var id domain.ArticleID
		// External IDs are provenance aliases, not sufficient identity. Some
		// publishers reuse GUIDs; only accept an alias when the stable canonical
		// URL also agrees.
		err = s.q(ctx).QueryRowContext(ctx, `SELECT article_sources.article_id FROM article_sources JOIN articles ON articles.id=article_sources.article_id WHERE article_sources.source_id=? AND articles.canonical_url=? AND (article_sources.external_id=? OR EXISTS(SELECT 1 FROM json_each(article_sources.external_ids_json) WHERE value=?))`, incoming.SourceID, incoming.CanonicalURL, incoming.SourceExternalID, incoming.SourceExternalID).Scan(&id)
		if err != nil && err != sql.ErrNoRows {
			return "", false, mapError(err)
		}
		if err == nil {
			ids[id] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return incoming.ID, false, nil
	}
	if len(ids) > 1 {
		return "", false, application.ErrConflict
	}
	for id := range ids {
		return id, true, nil
	}
	panic("unreachable")
}

func (s *Store) externalAliases(ctx context.Context, articleID domain.ArticleID, sourceID domain.SourceID, incoming string) ([]string, error) {
	seen := map[string]struct{}{}
	var raw string
	err := s.q(ctx).QueryRowContext(ctx, `SELECT external_ids_json FROM article_sources WHERE article_id=? AND source_id=?`, articleID, sourceID).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return nil, mapError(err)
	}
	if err == nil {
		var existing []string
		if json.Unmarshal([]byte(raw), &existing) != nil {
			return nil, application.ErrUnavailable
		}
		for _, alias := range existing {
			if alias != "" {
				seen[alias] = struct{}{}
			}
		}
	}
	if incoming != "" {
		seen[incoming] = struct{}{}
	}
	aliases := make([]string, 0, len(seen))
	for alias := range seen {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases, nil
}

func mergeArticle(stored, incoming domain.Article) domain.Article {
	// A newer observation authoritatively refreshes mutable metadata. Permission
	// is intentionally included: a metadata-only observation must remove content
	// that is no longer permitted to be retained.
	result := stored
	if !incoming.FetchedAt.Before(stored.FetchedAt) {
		result.Fingerprint = incoming.Fingerprint
		result.CanonicalURL = incoming.CanonicalURL
		result.Title = incoming.Title
		result.Author = incoming.Author
		result.PublishedAt = incoming.PublishedAt
		result.FetchedAt = incoming.FetchedAt
		result.Excerpt = incoming.Excerpt
		result.Language = incoming.Language
		result.TokenCount = incoming.TokenCount
		result.ContentPermission = incoming.ContentPermission
		result.FullContent = incoming.FullContent
	}
	result.SourceID = stored.SourceID
	result.SourceExternalID = incoming.SourceExternalID
	seen := map[string]struct{}{}
	result.Topics = nil
	for _, topics := range [][]string{stored.Topics, incoming.Topics} {
		for _, topic := range topics {
			if _, ok := seen[topic]; !ok {
				seen[topic] = struct{}{}
				result.Topics = append(result.Topics, topic)
			}
		}
	}
	return result
}

func (s *Store) getArticle(ctx context.Context, id domain.ArticleID) (domain.Article, error) {
	a, err := scanArticle(s.q(ctx).QueryRowContext(ctx, `SELECT `+articleColumns+` FROM articles a WHERE a.id=?`, id))
	if err == sql.ErrNoRows {
		return a, application.ErrNotFound
	}
	if err != nil {
		return a, mapError(err)
	}
	if err = s.loadTopics(ctx, &a); err != nil {
		return a, err
	}
	_ = s.q(ctx).QueryRowContext(ctx, `SELECT external_id FROM article_sources WHERE article_id=? AND source_id=?`, a.ID, a.SourceID).Scan(&a.SourceExternalID)
	return a, nil
}
func (s *Store) loadTopics(ctx context.Context, a *domain.Article) error {
	rows, err := s.q(ctx).QueryContext(ctx, `SELECT topic FROM article_topics WHERE article_id=? ORDER BY topic`, a.ID)
	if err != nil {
		return mapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var topic string
		if err := rows.Scan(&topic); err != nil {
			return mapError(err)
		}
		a.Topics = append(a.Topics, topic)
	}
	return mapError(rows.Err())
}
func scanArticle(row scanner) (domain.Article, error) {
	var a domain.Article
	var published sql.NullInt64
	var full sql.NullString
	var fetched int64
	err := row.Scan(&a.ID, &a.Fingerprint, &a.SourceID, &a.CanonicalURL, &a.Title, &a.Author, &published, &fetched, &a.Excerpt, &full, &a.ContentPermission, &a.Language, &a.TokenCount)
	if published.Valid {
		t := timeFromMillis(published.Int64)
		a.PublishedAt = &t
	}
	a.FetchedAt = timeFromMillis(fetched)
	a.FullContent = full.String
	return a, err
}

func (s *Store) listArticles(ctx context.Context, q application.FeedQuery) ([]domain.Article, error) {
	where, args, err := feedWhere(q)
	if err != nil {
		return nil, err
	}
	query := `SELECT ` + articleColumns + ` FROM articles a LEFT JOIN library_states l ON l.article_id=a.id` + where + ` ORDER BY COALESCE(a.published_at_ms,a.fetched_at_ms) DESC,a.id DESC`
	if q.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, q.Limit)
	}
	rows, err := s.q(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, mapError(err)
	}
	var list []domain.Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			rows.Close()
			return nil, mapError(err)
		}
		list = append(list, a)
	}
	if err := rows.Close(); err != nil {
		return nil, mapError(err)
	}
	for i := range list {
		if err = s.loadTopics(ctx, &list[i]); err != nil {
			return nil, err
		}
	}
	return list, nil
}

type feedCursor struct {
	Score float64          `json:"score"`
	Time  int64            `json:"time"`
	ID    domain.ArticleID `json:"id"`
}

func feedWhere(q application.FeedQuery) (string, []any, error) {
	var clauses []string
	var args []any
	if len(q.Filter.SourceIDs) > 0 {
		marks := make([]string, len(q.Filter.SourceIDs))
		for i, id := range q.Filter.SourceIDs {
			marks[i] = "?"
			args = append(args, id)
		}
		clauses = append(clauses, "a.primary_source_id IN ("+strings.Join(marks, ",")+")")
	}
	if q.Filter.Read != nil {
		if *q.Filter.Read {
			clauses = append(clauses, "l.read_at_ms IS NOT NULL")
		} else {
			clauses = append(clauses, "l.read_at_ms IS NULL")
		}
	}
	if q.Filter.Saved != nil {
		if *q.Filter.Saved {
			clauses = append(clauses, "l.saved_at_ms IS NOT NULL")
		} else {
			clauses = append(clauses, "l.saved_at_ms IS NULL")
		}
	}
	if !q.Filter.IncludeHidden {
		clauses = append(clauses, "l.hidden_at_ms IS NULL")
	}
	if q.Filter.Text != "" {
		clauses = append(clauses, "(a.title LIKE ? ESCAPE '\\' OR a.excerpt LIKE ? ESCAPE '\\')")
		term := "%" + escapeLike(q.Filter.Text) + "%"
		args = append(args, term, term)
	}
	if q.Filter.PublishedAfter != nil {
		clauses = append(clauses, "COALESCE(a.published_at_ms,a.fetched_at_ms)>?")
		args = append(args, millis(*q.Filter.PublishedAfter))
	}
	if q.Filter.PublishedBefore != nil {
		clauses = append(clauses, "COALESCE(a.published_at_ms,a.fetched_at_ms)<?")
		args = append(args, millis(*q.Filter.PublishedBefore))
	}
	if q.Cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(q.Cursor)
		if err != nil {
			return "", nil, application.ErrInvalidInput
		}
		var cursor feedCursor
		if json.Unmarshal(decoded, &cursor) != nil || cursor.ID == "" || cursor.Score < 0 || cursor.Score > 1 {
			return "", nil, application.ErrInvalidInput
		}
		clauses = append(clauses, "(COALESCE(r.score,0)<? OR (COALESCE(r.score,0)=? AND (COALESCE(a.published_at_ms,a.fetched_at_ms)<? OR (COALESCE(a.published_at_ms,a.fetched_at_ms)=? AND a.id>?))))")
		args = append(args, cursor.Score, cursor.Score, cursor.Time, cursor.Time, cursor.ID)
	}
	if len(clauses) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args, nil
}

func (s *Store) queryFeed(ctx context.Context, q application.FeedQuery) (application.FeedPage, error) {
	var page application.FeedPage
	limit := q.Limit
	if limit == 0 {
		limit = 30
	}
	if limit < 1 || limit > 100 {
		return page, application.ErrInvalidInput
	}
	q.Limit = limit + 1
	where, args, err := feedWhere(q)
	if err != nil {
		return page, err
	}
	query := `SELECT ` + articleColumns + ` FROM articles a LEFT JOIN ranking_results r ON r.article_id=a.id LEFT JOIN library_states l ON l.article_id=a.id` + where + ` ORDER BY COALESCE(r.score,0) DESC,COALESCE(a.published_at_ms,a.fetched_at_ms) DESC,a.id ASC LIMIT ?`
	args = append(args, q.Limit)
	rows, err := s.q(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return page, mapError(err)
	}
	var articles []domain.Article
	for rows.Next() {
		a, scanErr := scanArticle(rows)
		if scanErr != nil {
			rows.Close()
			return page, mapError(scanErr)
		}
		articles = append(articles, a)
	}
	if err := rows.Close(); err != nil {
		return page, mapError(err)
	}
	hasMore := len(articles) > limit
	if hasMore {
		articles = articles[:limit]
	}
	for i := range articles {
		if err := s.loadTopics(ctx, &articles[i]); err != nil {
			return page, err
		}
	}
	for _, a := range articles {
		library, err := s.Libraries().Get(ctx, a.ID)
		if errors.Is(err, application.ErrNotFound) {
			library = domain.LibraryState{ArticleID: a.ID}
		} else if err != nil {
			return page, err
		}
		ranking, err := s.Rankings().GetResult(ctx, a.ID)
		if errors.Is(err, application.ErrNotFound) {
			ranking = domain.RankingResult{ArticleID: a.ID}
		} else if err != nil {
			return page, err
		}
		page.Articles = append(page.Articles, application.RankedArticle{Article: a, Library: library, Ranking: ranking})
	}
	if hasMore {
		last := articles[len(articles)-1]
		t := last.FetchedAt
		if last.PublishedAt != nil {
			t = *last.PublishedAt
		}
		ranking := page.Articles[len(page.Articles)-1].Ranking
		encoded, _ := json.Marshal(feedCursor{Score: ranking.Score, Time: millis(t), ID: last.ID})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	return page, nil
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}
