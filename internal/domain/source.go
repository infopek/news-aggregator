package domain

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

var ErrInvalidSource = errors.New("invalid source")

type SourceKind string

const (
	SourceKindFeed    SourceKind = "feed"
	SourceKindAPI     SourceKind = "api"
	SourceKindScraper SourceKind = "scraper"
)

type ContentPermission string

const (
	ContentMetadataOnly ContentPermission = "metadata_only"
	ContentFullAllowed  ContentPermission = "full_content_allowed"
)

type ScraperPolicyStatus string

const (
	ScraperPolicyNotApplicable ScraperPolicyStatus = "not_applicable"
	ScraperPolicyPending       ScraperPolicyStatus = "pending"
	ScraperPolicyApproved      ScraperPolicyStatus = "approved"
	ScraperPolicyRejected      ScraperPolicyStatus = "rejected"
)

type ScraperPolicy struct {
	Status      ScraperPolicyStatus
	TermsURL    string
	RobotsURL   string
	ReviewedAt  *time.Time
	ReviewNotes string
}

type Source struct {
	ID                SourceID
	Name              string
	URL               string
	Kind              SourceKind
	Enabled           bool
	AdapterConfig     map[string]string
	ContentPermission ContentPermission
	ScraperPolicy     ScraperPolicy
	CredentialRef     *CredentialID
	RefreshCursor     string
	LastSuccessAt     *time.Time
	LastError         string
	RetryAfter        *time.Time
}

func (source Source) Validate() error {
	if source.ID == "" || strings.TrimSpace(source.Name) == "" || !validHTTPURL(source.URL) {
		return ErrInvalidSource
	}
	if source.Kind != SourceKindFeed && source.Kind != SourceKindAPI && source.Kind != SourceKindScraper {
		return ErrInvalidSource
	}
	if source.ContentPermission != ContentMetadataOnly && source.ContentPermission != ContentFullAllowed {
		return ErrInvalidSource
	}
	if source.Kind == SourceKindScraper {
		if source.Enabled && source.ScraperPolicy.Status != ScraperPolicyApproved {
			return ErrInvalidSource
		}
		if source.ScraperPolicy.Status == ScraperPolicyApproved && source.ScraperPolicy.ReviewedAt == nil {
			return ErrInvalidSource
		}
	} else if source.ScraperPolicy.Status != "" && source.ScraperPolicy.Status != ScraperPolicyNotApplicable {
		return ErrInvalidSource
	}
	return nil
}

func validHTTPURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
