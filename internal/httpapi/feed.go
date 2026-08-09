package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type FeedReader interface {
	GetFeed(context.Context, application.FeedQuery) (application.FeedPage, error)
	GetArticle(context.Context, domain.ArticleID) (application.ArticleDetail, error)
}

type FeedAPI struct {
	Service FeedReader
	Logger  *slog.Logger
	Timeout time.Duration
}

func NewFeedHandler(api FeedAPI) http.Handler {
	if api.Timeout <= 0 {
		api.Timeout = 5 * time.Second
	}
	if api.Logger == nil {
		api.Logger = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/feed", api.getFeed)
	mux.HandleFunc("GET /api/v1/articles/{articleId}", api.getArticle)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), api.Timeout)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (api FeedAPI) getFeed(w http.ResponseWriter, r *http.Request) {
	if api.Service == nil {
		api.fail(w, r, application.ErrUnavailable)
		return
	}
	query, err := parseFeedQuery(r)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	page, err := api.Service.GetFeed(r.Context(), query)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	items := make([]articleSummary, 0, len(page.Articles))
	for _, item := range page.Articles {
		items = append(items, summary(item))
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	writeJSON(w, http.StatusOK, struct {
		Items      []articleSummary `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}{items, next})
}

func (api FeedAPI) getArticle(w http.ResponseWriter, r *http.Request) {
	if api.Service == nil {
		api.fail(w, r, application.ErrUnavailable)
		return
	}
	id := domain.ArticleID(r.PathValue("articleId"))
	if id == "" {
		api.fail(w, r, application.ErrInvalidInput)
		return
	}
	detail, err := api.Service.GetArticle(r.Context(), id)
	if err != nil {
		api.fail(w, r, err)
		return
	}
	var full *string
	if detail.Article.ContentPermission == domain.ContentFullAllowed {
		value := detail.Article.FullContent
		full = &value
	}
	writeJSON(w, http.StatusOK, struct {
		Article     articleSummary `json:"article"`
		FullContent *string        `json:"fullContent"`
	}{summary(application.RankedArticle{Article: detail.Article, Library: detail.Library, Ranking: detail.Ranking}), full})
}

func parseFeedQuery(r *http.Request) (application.FeedQuery, error) {
	v := r.URL.Query()
	q := application.FeedQuery{Cursor: v.Get("cursor"), Limit: 30, Filter: application.FeedFilter{Text: v.Get("text")}}
	if raw := v.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return q, application.ErrInvalidInput
		}
		q.Limit = n
	}
	q.Filter.SourceIDs = make([]domain.SourceID, 0, len(v["sourceId"]))
	for _, id := range v["sourceId"] {
		if strings.TrimSpace(id) == "" {
			return q, application.ErrInvalidInput
		}
		q.Filter.SourceIDs = append(q.Filter.SourceIDs, domain.SourceID(id))
	}
	for key, target := range map[string]**bool{"read": &q.Filter.Read, "saved": &q.Filter.Saved} {
		if raw := v.Get(key); raw != "" {
			parsed, err := strconv.ParseBool(raw)
			if err != nil {
				return q, application.ErrInvalidInput
			}
			*target = &parsed
		}
	}
	if raw := v.Get("includeHidden"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return q, application.ErrInvalidInput
		}
		q.Filter.IncludeHidden = parsed
	}
	for key, target := range map[string]**time.Time{"publishedAfter": &q.Filter.PublishedAfter, "publishedBefore": &q.Filter.PublishedBefore} {
		if raw := v.Get(key); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return q, application.ErrInvalidInput
			}
			parsed = parsed.UTC()
			*target = &parsed
		}
	}
	if q.Filter.PublishedAfter != nil && q.Filter.PublishedBefore != nil && !q.Filter.PublishedAfter.Before(*q.Filter.PublishedBefore) {
		return q, application.ErrInvalidInput
	}
	return q, nil
}

type contributionResponse struct {
	Signal        domain.RankingSignal `json:"signal"`
	RawScore      float64              `json:"rawScore"`
	Weight        float64              `json:"weight"`
	WeightedScore float64              `json:"weightedScore"`
	ReasonCode    string               `json:"reasonCode"`
	ReasonValues  map[string]string    `json:"reasonValues"`
}
type articleRankingResponse struct {
	Score            float64                `json:"score"`
	Contributions    []contributionResponse `json:"contributions"`
	AlgorithmVersion string                 `json:"algorithmVersion"`
	CalculatedAt     time.Time              `json:"calculatedAt"`
}
type libraryResponse struct {
	ArticleID domain.ArticleID `json:"articleId"`
	ReadAt    *time.Time       `json:"readAt"`
	SavedAt   *time.Time       `json:"savedAt"`
	HiddenAt  *time.Time       `json:"hiddenAt"`
}
type articleSummary struct {
	ID                domain.ArticleID         `json:"id"`
	SourceID          domain.SourceID          `json:"sourceId"`
	CanonicalURL      string                   `json:"canonicalUrl"`
	Title             string                   `json:"title"`
	Author            string                   `json:"author,omitempty"`
	PublishedAt       *time.Time               `json:"publishedAt"`
	FetchedAt         time.Time                `json:"fetchedAt"`
	Excerpt           string                   `json:"excerpt,omitempty"`
	ContentPermission domain.ContentPermission `json:"contentPermission"`
	Language          string                   `json:"language,omitempty"`
	Topics            []string                 `json:"topics"`
	Library           libraryResponse          `json:"library"`
	Ranking           articleRankingResponse   `json:"ranking"`
}

func summary(v application.RankedArticle) articleSummary {
	contributions := make([]contributionResponse, 0, len(v.Ranking.Contributions))
	for _, c := range v.Ranking.Contributions {
		contributions = append(contributions, contributionResponse{c.Signal, c.RawScore, c.Weight, c.WeightedScore, c.ReasonCode, c.ReasonValues})
	}
	return articleSummary{ID: v.Article.ID, SourceID: v.Article.SourceID, CanonicalURL: v.Article.CanonicalURL, Title: v.Article.Title, Author: v.Article.Author, PublishedAt: v.Article.PublishedAt, FetchedAt: v.Article.FetchedAt, Excerpt: v.Article.Excerpt, ContentPermission: v.Article.ContentPermission, Language: v.Article.Language, Topics: nonNil(v.Article.Topics), Library: libraryResponse{v.Library.ArticleID, v.Library.ReadAt, v.Library.SavedAt, v.Library.HiddenAt}, Ranking: articleRankingResponse{v.Ranking.Score, contributions, v.Ranking.AlgorithmVersion, v.Ranking.CalculatedAt}}
}

func (api FeedAPI) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, application.ErrInvalidInput) {
		writeAPIError(w, 400, "validation_failed", "The request is invalid.", randomID())
		return
	}
	ConfigurationAPI{Logger: api.Logger}.fail(w, r, err)
}
