// Package newsapi implements a provider-neutral official JSON API adapter.
package newsapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

const maxBody = int64(4 << 20)

var ErrInvalidResponse = errors.New("official API adapter: invalid response")

type Adapter struct {
	Fetcher     application.HTTPFetcher
	Credentials application.CredentialResolver
}

func (Adapter) Kind() domain.SourceKind { return domain.SourceKindAPI }

type payload struct {
	Provider   string `json:"provider"`
	NextCursor string `json:"nextCursor"`
	Items      []item `json:"items"`
}

type item struct {
	ID, URL, Title, Author, PublishedAt, Excerpt, Content, Language string
	Topics                                                          []string `json:"topics"`
}

func (a Adapter) Fetch(ctx context.Context, source domain.Source, cursor application.FetchCursor) (application.AdapterResult, error) {
	if a.Fetcher == nil || source.Validate() != nil || source.Kind != domain.SourceKindAPI || source.AdapterConfig.API == nil {
		return application.AdapterResult{}, application.ErrInvalidInput
	}
	target, err := url.Parse(source.URL)
	if err != nil {
		return application.AdapterResult{}, application.ErrInvalidInput
	}
	query := target.Query()
	if cursor.Value != "" {
		query.Set("cursor", cursor.Value)
	}
	if source.AdapterConfig.API.PageSize > 0 {
		query.Set("page_size", strconv.Itoa(source.AdapterConfig.API.PageSize))
	}
	target.RawQuery = query.Encode()
	headers := map[string][]string{"Accept": {"application/json"}}
	fetch := func() (application.FetchResponse, error) {
		return a.Fetcher.Fetch(ctx, application.FetchRequest{URL: target.String(), SourceID: source.ID, Headers: headers, MaxBytes: maxBody, AllowedContentTypes: []string{"application/json"}})
	}
	var response application.FetchResponse
	if source.CredentialRef != nil {
		if a.Credentials == nil {
			return application.AdapterResult{}, application.ErrCredentialMissing
		}
		err = a.Credentials.WithSecret(ctx, *source.CredentialRef, func(secret []byte) error {
			headers["Authorization"] = []string{"Bearer " + string(secret)}
			defer delete(headers, "Authorization")
			response, err = fetch()
			return err
		})
	} else {
		response, err = fetch()
	}
	if err != nil {
		if ctx.Err() != nil {
			return application.AdapterResult{}, ctx.Err()
		}
		return application.AdapterResult{}, errors.New("official API adapter: request failed")
	}
	if response.Body == nil {
		return application.AdapterResult{}, ErrInvalidResponse
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return application.AdapterResult{}, application.ErrCredentialMissing
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return application.AdapterResult{}, fmt.Errorf("official API adapter: rate limited (retryable=%t retry_after=%s)", response.Retryable, response.RetryAfter)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return application.AdapterResult{}, fmt.Errorf("official API adapter: HTTP status %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxBody+1))
	decoder.DisallowUnknownFields()
	var body payload
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || body.Provider != source.AdapterConfig.API.Provider {
		return application.AdapterResult{}, ErrInvalidResponse
	}
	items := make([]application.AdapterItem, 0, len(body.Items))
	for _, value := range body.Items {
		if strings.TrimSpace(value.ID) == "" || strings.TrimSpace(value.URL) == "" || strings.TrimSpace(value.Title) == "" {
			return application.AdapterResult{}, ErrInvalidResponse
		}
		var published *time.Time
		if value.PublishedAt != "" {
			parsed, parseErr := time.Parse(time.RFC3339, value.PublishedAt)
			if parseErr != nil {
				return application.AdapterResult{}, ErrInvalidResponse
			}
			published = &parsed
		}
		items = append(items, application.AdapterItem{ExternalID: value.ID, CanonicalURL: value.URL, Title: value.Title, Author: value.Author, PublishedAt: published, Excerpt: value.Excerpt, FullContent: value.Content, Language: value.Language, Topics: value.Topics})
	}
	if body.NextCursor != "" && body.NextCursor == cursor.Value {
		return application.AdapterResult{}, ErrInvalidResponse
	}
	return application.AdapterResult{Items: items, NextCursor: application.FetchCursor{Value: body.NextCursor}}, nil
}
