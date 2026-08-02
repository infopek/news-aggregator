package domain

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidArticle = errors.New("invalid article")

type Article struct {
	ID          ArticleID
	Fingerprint string
	SourceID    SourceID
	// SourceExternalID identifies this observation within SourceID. It is
	// non-secret adapter metadata and participates in deduplication.
	SourceExternalID  string
	CanonicalURL      string
	Title             string
	Author            string
	PublishedAt       *time.Time
	FetchedAt         time.Time
	Excerpt           string
	FullContent       string
	ContentPermission ContentPermission
	Language          string
	Topics            []string
	TokenCount        int
}

func (article Article) Validate() error {
	if article.ID == "" || article.SourceID == "" || strings.TrimSpace(article.Fingerprint) == "" || strings.TrimSpace(article.Title) == "" || !validHTTPURL(article.CanonicalURL) {
		return ErrInvalidArticle
	}
	if article.ContentPermission != ContentMetadataOnly && article.ContentPermission != ContentFullAllowed {
		return ErrInvalidArticle
	}
	if article.ContentPermission == ContentMetadataOnly && article.FullContent != "" {
		return ErrInvalidArticle
	}
	if article.TokenCount < 0 {
		return ErrInvalidArticle
	}
	return nil
}

type LibraryState struct {
	ArticleID ArticleID
	ReadAt    *time.Time
	SavedAt   *time.Time
	HiddenAt  *time.Time
}

type LibraryPatch struct {
	Read   *bool
	Saved  *bool
	Hidden *bool
}
