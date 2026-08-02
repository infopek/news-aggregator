package domain

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidSource                 = errors.New("invalid source")
	ErrSensitiveAdapterConfiguration = errors.New("sensitive adapter configuration is forbidden")
)

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

type FeedFormat string

const (
	FeedFormatAuto FeedFormat = "auto"
	FeedFormatRSS  FeedFormat = "rss"
	FeedFormatAtom FeedFormat = "atom"
)

type FeedConfiguration struct {
	Format FeedFormat
}

type APIConfiguration struct {
	Provider string
	PageSize int
}

type ScraperConfiguration struct {
	ArticleSelector string
	TitleSelector   string
	ExcerptSelector string
	ContentSelector string
}

// AdapterConfiguration is a typed, non-secret sum type. Credential material
// is represented only by Source.CredentialRef and lives in CredentialStore.
type AdapterConfiguration struct {
	Feed    *FeedConfiguration
	API     *APIConfiguration
	Scraper *ScraperConfiguration
}

func (configuration AdapterConfiguration) ValidFor(kind SourceKind) bool {
	switch kind {
	case SourceKindFeed:
		return configuration.Feed != nil && configuration.API == nil && configuration.Scraper == nil &&
			(configuration.Feed.Format == FeedFormatAuto || configuration.Feed.Format == FeedFormatRSS || configuration.Feed.Format == FeedFormatAtom)
	case SourceKindAPI:
		return configuration.Feed == nil && configuration.API != nil && configuration.Scraper == nil &&
			strings.TrimSpace(configuration.API.Provider) != "" && configuration.API.PageSize >= 0
	case SourceKindScraper:
		return configuration.Feed == nil && configuration.API == nil && configuration.Scraper != nil &&
			strings.TrimSpace(configuration.Scraper.ArticleSelector) != "" && strings.TrimSpace(configuration.Scraper.TitleSelector) != ""
	default:
		return false
	}
}

// ParseAdapterConfiguration is the validation boundary for transport or file
// input. Only explicitly non-secret keys are accepted.
func ParseAdapterConfiguration(kind SourceKind, fields map[string]string) (AdapterConfiguration, error) {
	for key := range fields {
		normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
		compact := strings.ReplaceAll(normalized, "_", "")
		if strings.Contains(compact, "secret") || strings.Contains(compact, "credential") || strings.Contains(compact, "password") || strings.Contains(compact, "token") || compact == "apikey" || compact == "authorization" {
			return AdapterConfiguration{}, ErrSensitiveAdapterConfiguration
		}
	}

	switch kind {
	case SourceKindFeed:
		if !onlyKeys(fields, "format") {
			return AdapterConfiguration{}, ErrInvalidSource
		}
		format := FeedFormat(fields["format"])
		if format == "" {
			format = FeedFormatAuto
		}
		configuration := AdapterConfiguration{Feed: &FeedConfiguration{Format: format}}
		if !configuration.ValidFor(kind) {
			return AdapterConfiguration{}, ErrInvalidSource
		}
		return configuration, nil
	case SourceKindAPI:
		if !onlyKeys(fields, "provider", "page_size") {
			return AdapterConfiguration{}, ErrInvalidSource
		}
		pageSize := 0
		if fields["page_size"] != "" {
			parsed, err := strconv.Atoi(fields["page_size"])
			if err != nil {
				return AdapterConfiguration{}, ErrInvalidSource
			}
			pageSize = parsed
		}
		configuration := AdapterConfiguration{API: &APIConfiguration{Provider: fields["provider"], PageSize: pageSize}}
		if !configuration.ValidFor(kind) {
			return AdapterConfiguration{}, ErrInvalidSource
		}
		return configuration, nil
	case SourceKindScraper:
		if !onlyKeys(fields, "article_selector", "title_selector", "excerpt_selector", "content_selector") {
			return AdapterConfiguration{}, ErrInvalidSource
		}
		configuration := AdapterConfiguration{Scraper: &ScraperConfiguration{
			ArticleSelector: fields["article_selector"],
			TitleSelector:   fields["title_selector"],
			ExcerptSelector: fields["excerpt_selector"],
			ContentSelector: fields["content_selector"],
		}}
		if !configuration.ValidFor(kind) {
			return AdapterConfiguration{}, ErrInvalidSource
		}
		return configuration, nil
	default:
		return AdapterConfiguration{}, ErrInvalidSource
	}
}

func onlyKeys(fields map[string]string, allowed ...string) bool {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	for key := range fields {
		if _, ok := allowedSet[key]; !ok {
			return false
		}
	}
	return true
}

type Source struct {
	ID                SourceID
	Name              string
	URL               string
	Kind              SourceKind
	Enabled           bool
	AdapterConfig     AdapterConfiguration
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
	if !source.AdapterConfig.ValidFor(source.Kind) {
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
