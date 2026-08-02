package sqlite

import (
	"context"
	"database/sql"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type SourceRepository struct{ store *Store }

func (s *Store) Sources() *SourceRepository { return &SourceRepository{store: s} }
func (r *SourceRepository) Save(ctx context.Context, v domain.Source) error {
	return r.store.saveSource(ctx, v)
}
func (r *SourceRepository) List(ctx context.Context) ([]domain.Source, error) {
	return r.store.listSources(ctx)
}
func (r *SourceRepository) Get(ctx context.Context, id domain.SourceID) (domain.Source, error) {
	return r.store.getSource(ctx, id)
}
func (r *SourceRepository) Delete(ctx context.Context, id domain.SourceID) error {
	return r.store.deleteSource(ctx, id)
}
func (s *Store) saveSource(ctx context.Context, source domain.Source) error {
	if err := source.Validate(); err != nil {
		return application.ErrInvalidInput
	}
	var feed, provider, article, title, excerpt, content any
	var page any
	switch source.Kind {
	case domain.SourceKindFeed:
		feed = source.AdapterConfig.Feed.Format
	case domain.SourceKindAPI:
		provider = source.AdapterConfig.API.Provider
		page = source.AdapterConfig.API.PageSize
	case domain.SourceKindScraper:
		article = source.AdapterConfig.Scraper.ArticleSelector
		title = source.AdapterConfig.Scraper.TitleSelector
		excerpt = source.AdapterConfig.Scraper.ExcerptSelector
		content = source.AdapterConfig.Scraper.ContentSelector
	}
	status := source.ScraperPolicy.Status
	if status == "" {
		status = domain.ScraperPolicyNotApplicable
	}
	var credential any
	if source.CredentialRef != nil {
		credential = string(*source.CredentialRef)
	}
	_, err := s.q(ctx).ExecContext(ctx, `INSERT INTO sources(id,name,url,kind,enabled,content_permission,feed_format,api_provider,api_page_size,scraper_article_selector,scraper_title_selector,scraper_excerpt_selector,scraper_content_selector,scraper_policy_status,scraper_terms_url,scraper_robots_url,scraper_reviewed_at_ms,scraper_review_notes,credential_ref,refresh_cursor,refresh_etag,refresh_last_modified,last_success_at_ms,last_error,retry_after_ms)
	VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,url=excluded.url,kind=excluded.kind,enabled=excluded.enabled,content_permission=excluded.content_permission,feed_format=excluded.feed_format,api_provider=excluded.api_provider,api_page_size=excluded.api_page_size,scraper_article_selector=excluded.scraper_article_selector,scraper_title_selector=excluded.scraper_title_selector,scraper_excerpt_selector=excluded.scraper_excerpt_selector,scraper_content_selector=excluded.scraper_content_selector,scraper_policy_status=excluded.scraper_policy_status,scraper_terms_url=excluded.scraper_terms_url,scraper_robots_url=excluded.scraper_robots_url,scraper_reviewed_at_ms=excluded.scraper_reviewed_at_ms,scraper_review_notes=excluded.scraper_review_notes,credential_ref=excluded.credential_ref,refresh_cursor=excluded.refresh_cursor,refresh_etag=excluded.refresh_etag,refresh_last_modified=excluded.refresh_last_modified,last_success_at_ms=excluded.last_success_at_ms,last_error=excluded.last_error,retry_after_ms=excluded.retry_after_ms,deleted_at_ms=NULL`, source.ID, source.Name, source.URL, source.Kind, source.Enabled, source.ContentPermission, feed, provider, page, article, title, excerpt, content, status, nullIfEmpty(source.ScraperPolicy.TermsURL), nullIfEmpty(source.ScraperPolicy.RobotsURL), nullableMillis(source.ScraperPolicy.ReviewedAt), nullIfEmpty(source.ScraperPolicy.ReviewNotes), credential, source.RefreshCursor, source.RefreshETag, source.RefreshLastModified, nullableMillis(source.LastSuccessAt), source.LastError, nullableMillis(source.RetryAfter))
	return mapError(err)
}

func (s *Store) listSources(ctx context.Context) ([]domain.Source, error) {
	rows, err := s.q(ctx).QueryContext(ctx, sourceSelect+` WHERE deleted_at_ms IS NULL ORDER BY name,id`)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	var result []domain.Source
	for rows.Next() {
		v, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, mapError(rows.Err())
}
func (s *Store) getSource(ctx context.Context, id domain.SourceID) (domain.Source, error) {
	row := s.q(ctx).QueryRowContext(ctx, sourceSelect+` WHERE id=? AND deleted_at_ms IS NULL`, id)
	v, err := scanSource(row)
	if err == sql.ErrNoRows {
		return v, application.ErrNotFound
	}
	return v, mapError(err)
}
func (s *Store) deleteSource(ctx context.Context, id domain.SourceID) error {
	result, err := s.q(ctx).ExecContext(ctx, `UPDATE sources SET deleted_at_ms=?,enabled=0,credential_ref=NULL WHERE id=? AND deleted_at_ms IS NULL`, timeNowMillis(), id)
	if err != nil {
		return mapError(err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return application.ErrNotFound
	}
	return nil
}

const sourceSelect = `SELECT id,name,url,kind,enabled,content_permission,feed_format,api_provider,api_page_size,scraper_article_selector,scraper_title_selector,scraper_excerpt_selector,scraper_content_selector,scraper_policy_status,scraper_terms_url,scraper_robots_url,scraper_reviewed_at_ms,scraper_review_notes,credential_ref,refresh_cursor,refresh_etag,refresh_last_modified,last_success_at_ms,last_error,retry_after_ms FROM sources`

type scanner interface{ Scan(...any) error }

func scanSource(row scanner) (domain.Source, error) {
	var v domain.Source
	var feed, provider, article, title, excerpt, content, terms, robots, notes, credential sql.NullString
	var page, reviewed, last, retry sql.NullInt64
	if err := row.Scan(&v.ID, &v.Name, &v.URL, &v.Kind, &v.Enabled, &v.ContentPermission, &feed, &provider, &page, &article, &title, &excerpt, &content, &v.ScraperPolicy.Status, &terms, &robots, &reviewed, &notes, &credential, &v.RefreshCursor, &v.RefreshETag, &v.RefreshLastModified, &last, &v.LastError, &retry); err != nil {
		return v, err
	}
	switch v.Kind {
	case domain.SourceKindFeed:
		v.AdapterConfig.Feed = &domain.FeedConfiguration{Format: domain.FeedFormat(feed.String)}
	case domain.SourceKindAPI:
		v.AdapterConfig.API = &domain.APIConfiguration{Provider: provider.String, PageSize: int(page.Int64)}
	case domain.SourceKindScraper:
		v.AdapterConfig.Scraper = &domain.ScraperConfiguration{ArticleSelector: article.String, TitleSelector: title.String, ExcerptSelector: excerpt.String, ContentSelector: content.String}
	}
	v.ScraperPolicy.TermsURL = terms.String
	v.ScraperPolicy.RobotsURL = robots.String
	v.ScraperPolicy.ReviewNotes = notes.String
	if reviewed.Valid {
		t := timeFromMillis(reviewed.Int64)
		v.ScraperPolicy.ReviewedAt = &t
	}
	if credential.Valid {
		x := domain.CredentialID(credential.String)
		v.CredentialRef = &x
	}
	if last.Valid {
		t := timeFromMillis(last.Int64)
		v.LastSuccessAt = &t
	}
	if retry.Valid {
		t := timeFromMillis(retry.Int64)
		v.RetryAfter = &t
	}
	return v, nil
}
func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
