package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strconv"
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
		var existing domain.ArticleID
		err := s.q(ctx).QueryRowContext(ctx, `SELECT id FROM articles WHERE fingerprint=? OR canonical_url=? LIMIT 1`, a.Fingerprint, a.CanonicalURL).Scan(&existing)
		if err != nil && err != sql.ErrNoRows {
			return mapError(err)
		}
		existed := err == nil
		if existed && existing != a.ID {
			return application.ErrConflict
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
		if _, err = s.q(ctx).ExecContext(ctx, `INSERT INTO article_sources(article_id,source_id,first_seen_at_ms,last_seen_at_ms) VALUES(?,?,?,?) ON CONFLICT(article_id,source_id) DO UPDATE SET last_seen_at_ms=excluded.last_seen_at_ms`, a.ID, a.SourceID, millis(a.FetchedAt), millis(a.FetchedAt)); err != nil {
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
	where, args := feedWhere(q)
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
func feedWhere(q application.FeedQuery) (string, []any) {
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
		clauses = append(clauses, "(a.title LIKE ? OR a.excerpt LIKE ?)")
		term := "%" + q.Filter.Text + "%"
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
		if err == nil {
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				if ms, e := strconv.ParseInt(parts[0], 10, 64); e == nil {
					clauses = append(clauses, "(COALESCE(a.published_at_ms,a.fetched_at_ms)<? OR (COALESCE(a.published_at_ms,a.fetched_at_ms)=? AND a.id<?))")
					args = append(args, ms, ms, parts[1])
				}
			}
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (s *Store) queryFeed(ctx context.Context, q application.FeedQuery) (application.FeedPage, error) {
	var page application.FeedPage
	articles, err := s.listArticles(ctx, q)
	if err != nil {
		return page, err
	}
	for _, a := range articles {
		library, _ := s.Libraries().Get(ctx, a.ID)
		ranking, _ := s.Rankings().GetResult(ctx, a.ID)
		page.Articles = append(page.Articles, application.RankedArticle{Article: a, Library: library, Ranking: ranking})
	}
	if q.Limit > 0 && len(articles) == q.Limit {
		last := articles[len(articles)-1]
		t := last.FetchedAt
		if last.PublishedAt != nil {
			t = *last.PublishedAt
		}
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d:%s", millis(t), last.ID)))
	}
	return page, nil
}
