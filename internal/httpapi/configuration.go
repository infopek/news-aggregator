package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

const maxRequestBytes = 16 << 10

type ConfigurationAPI struct {
	Profiles application.ProfileService
	Sources  application.SourceService
	Starters []domain.Source
	Logger   *slog.Logger
	Timeout  time.Duration
	NewID    func() string
}

func NewConfigurationHandler(api ConfigurationAPI) http.Handler {
	if api.Timeout <= 0 {
		api.Timeout = 5 * time.Second
	}
	if api.Logger == nil {
		api.Logger = slog.Default()
	}
	if api.NewID == nil {
		api.NewID = randomID
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/profile", api.getProfile)
	mux.HandleFunc("PUT /api/v1/profile", api.putProfile)
	mux.HandleFunc("GET /api/v1/ranking-config", api.getRanking)
	mux.HandleFunc("PUT /api/v1/ranking-config", api.putRanking)
	mux.HandleFunc("GET /api/v1/starter-sources", api.listStarters)
	mux.HandleFunc("GET /api/v1/sources", api.listSources)
	mux.HandleFunc("POST /api/v1/sources", api.createSource)
	mux.HandleFunc("GET /api/v1/sources/{sourceId}", api.getSource)
	mux.HandleFunc("PATCH /api/v1/sources/{sourceId}", api.updateSource)
	mux.HandleFunc("DELETE /api/v1/sources/{sourceId}", api.deleteSource)
	mux.HandleFunc("PUT /api/v1/sources/{sourceId}/credential", api.putCredential)
	mux.HandleFunc("DELETE /api/v1/sources/{sourceId}/credential", api.deleteCredential)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), api.Timeout)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

type optionalString struct {
	Value   *string `json:"value,omitempty"`
	Present bool    `json:"present"`
	Enabled bool    `json:"enabled"`
}
type optionalInt struct {
	Value   *int `json:"value,omitempty"`
	Present bool `json:"present"`
	Enabled bool `json:"enabled"`
}
type locationValue struct {
	Country string         `json:"country"`
	Region  string         `json:"region"`
	City    optionalString `json:"city"`
}
type optionalLocation struct {
	Value   *locationValue `json:"value,omitempty"`
	Present bool           `json:"present"`
	Enabled bool           `json:"enabled"`
}
type profileWrite struct {
	Interests          []weightedInterest `json:"interests"`
	PreferredSourceIDs []domain.SourceID  `json:"preferredSourceIds"`
	Location           optionalLocation   `json:"location"`
	Age                optionalInt        `json:"age"`
	Gender             optionalString     `json:"gender"`
}
type weightedInterest struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
}
type profileResponse struct {
	ID domain.ProfileID `json:"id"`
	profileWrite
	UpdatedAt time.Time `json:"updatedAt"`
}

func profileFromWrite(v profileWrite) domain.UserProfile {
	interests := make([]domain.WeightedInterest, 0, len(v.Interests))
	for _, item := range v.Interests {
		interests = append(interests, domain.WeightedInterest{Name: item.Name, Weight: item.Weight})
	}
	p := domain.UserProfile{ID: domain.LocalProfileID, Interests: interests, PreferredSources: v.PreferredSourceIDs,
		Age: domain.OptionalSignal[int]{Present: v.Age.Present, Enabled: v.Age.Enabled}, Gender: domain.OptionalSignal[string]{Present: v.Gender.Present, Enabled: v.Gender.Enabled}, Location: domain.OptionalSignal[domain.Location]{Present: v.Location.Present, Enabled: v.Location.Enabled}}
	if v.Age.Value != nil {
		p.Age.Value = *v.Age.Value
	}
	if v.Gender.Value != nil {
		p.Gender.Value = *v.Gender.Value
	}
	if v.Location.Value != nil {
		l := v.Location.Value
		p.Location.Value = domain.Location{Country: l.Country, Region: l.Region, City: domain.OptionalSignal[string]{Present: l.City.Present, Enabled: l.City.Enabled}}
		if l.City.Value != nil {
			p.Location.Value.City.Value = *l.City.Value
		}
	}
	return p
}
func profileToResponse(p domain.UserProfile) profileResponse {
	interests := make([]weightedInterest, 0, len(p.Interests))
	for _, item := range p.Interests {
		interests = append(interests, weightedInterest{Name: item.Name, Weight: item.Weight})
	}
	v := profileWrite{Interests: interests, PreferredSourceIDs: nonNil(p.PreferredSources), Age: optionalInt{Present: p.Age.Present, Enabled: p.Age.Enabled}, Gender: optionalString{Present: p.Gender.Present, Enabled: p.Gender.Enabled}, Location: optionalLocation{Present: p.Location.Present, Enabled: p.Location.Enabled}}
	if p.Age.Present {
		x := p.Age.Value
		v.Age.Value = &x
	}
	if p.Gender.Present {
		x := p.Gender.Value
		v.Gender.Value = &x
	}
	if p.Location.Present {
		l := p.Location.Value
		lv := locationValue{Country: l.Country, Region: l.Region, City: optionalString{Present: l.City.Present, Enabled: l.City.Enabled}}
		if l.City.Present {
			x := l.City.Value
			lv.City.Value = &x
		}
		v.Location.Value = &lv
	}
	return profileResponse{ID: p.ID, profileWrite: v, UpdatedAt: p.UpdatedAt}
}

type rankingWrite struct {
	Recency          signalWeight `json:"recency"`
	Interest         signalWeight `json:"interest"`
	SourcePreference signalWeight `json:"sourcePreference"`
	Behavior         signalWeight `json:"behavior"`
	Location         signalWeight `json:"location"`
	Age              signalWeight `json:"age"`
	Gender           signalWeight `json:"gender"`
	TextSimilarity   signalWeight `json:"textSimilarity"`
}
type signalWeight struct {
	Enabled bool    `json:"enabled"`
	Weight  float64 `json:"weight"`
}

func fromSignal(v signalWeight) domain.SignalWeight {
	return domain.SignalWeight{Enabled: v.Enabled, Weight: v.Weight}
}
func toSignal(v domain.SignalWeight) signalWeight {
	return signalWeight{Enabled: v.Enabled, Weight: v.Weight}
}

type rankingResponse struct {
	rankingWrite
	PerDemographicCap    float64 `json:"perDemographicCap"`
	TotalDemographicCap  float64 `json:"totalDemographicCap"`
	NormalizationVersion string  `json:"normalizationVersion"`
}

func rankingToResponse(c domain.RankingConfiguration) rankingResponse {
	return rankingResponse{rankingWrite: rankingWrite{toSignal(c.Recency), toSignal(c.Interest), toSignal(c.SourcePreference), toSignal(c.Behavior), toSignal(c.Location), toSignal(c.Age), toSignal(c.Gender), toSignal(c.TextSimilarity)}, PerDemographicCap: c.PerDemographicCap, TotalDemographicCap: c.TotalDemographicCap, NormalizationVersion: c.NormalizationVersion}
}

type adapterWrite struct {
	Format          string `json:"format,omitempty"`
	Provider        string `json:"provider,omitempty"`
	PageSize        int    `json:"pageSize,omitempty"`
	ArticleSelector string `json:"articleSelector,omitempty"`
	TitleSelector   string `json:"titleSelector,omitempty"`
	ExcerptSelector string `json:"excerptSelector,omitempty"`
	ContentSelector string `json:"contentSelector,omitempty"`
}
type policyWrite struct {
	Status      domain.ScraperPolicyStatus `json:"status"`
	TermsURL    *string                    `json:"termsUrl"`
	RobotsURL   *string                    `json:"robotsUrl"`
	ReviewedAt  *time.Time                 `json:"reviewedAt"`
	ReviewNotes *string                    `json:"reviewNotes"`
}
type sourceWrite struct {
	Name              string                   `json:"name"`
	URL               string                   `json:"url"`
	Kind              domain.SourceKind        `json:"kind"`
	Enabled           bool                     `json:"enabled"`
	ContentPermission domain.ContentPermission `json:"contentPermission"`
	AdapterConfig     adapterWrite             `json:"adapterConfig"`
	ScraperPolicy     policyWrite              `json:"scraperPolicy"`
}
type sourceResponse struct {
	ID                   domain.SourceID          `json:"id"`
	Name                 string                   `json:"name"`
	URL                  string                   `json:"url"`
	Kind                 domain.SourceKind        `json:"kind"`
	Enabled              bool                     `json:"enabled"`
	ContentPermission    domain.ContentPermission `json:"contentPermission"`
	AdapterConfig        any                      `json:"adapterConfig"`
	ScraperPolicy        policyWrite              `json:"scraperPolicy"`
	CredentialConfigured bool                     `json:"credentialConfigured"`
	LastSuccessAt        *time.Time               `json:"lastSuccessAt"`
	LastError            *string                  `json:"lastError"`
	RetryAfter           *time.Time               `json:"retryAfter"`
}

func sourceFromWrite(id domain.SourceID, v sourceWrite) domain.Source {
	s := domain.Source{ID: id, Name: v.Name, URL: v.URL, Kind: v.Kind, Enabled: v.Enabled, ContentPermission: v.ContentPermission, ScraperPolicy: domain.ScraperPolicy{Status: v.ScraperPolicy.Status, ReviewedAt: v.ScraperPolicy.ReviewedAt}}
	if v.ScraperPolicy.TermsURL != nil {
		s.ScraperPolicy.TermsURL = *v.ScraperPolicy.TermsURL
	}
	if v.ScraperPolicy.RobotsURL != nil {
		s.ScraperPolicy.RobotsURL = *v.ScraperPolicy.RobotsURL
	}
	if v.ScraperPolicy.ReviewNotes != nil {
		s.ScraperPolicy.ReviewNotes = *v.ScraperPolicy.ReviewNotes
	}
	switch v.Kind {
	case domain.SourceKindFeed:
		s.AdapterConfig.Feed = &domain.FeedConfiguration{Format: domain.FeedFormat(v.AdapterConfig.Format)}
	case domain.SourceKindAPI:
		s.AdapterConfig.API = &domain.APIConfiguration{Provider: v.AdapterConfig.Provider, PageSize: v.AdapterConfig.PageSize}
	case domain.SourceKindScraper:
		s.AdapterConfig.Scraper = &domain.ScraperConfiguration{ArticleSelector: v.AdapterConfig.ArticleSelector, TitleSelector: v.AdapterConfig.TitleSelector, ExcerptSelector: v.AdapterConfig.ExcerptSelector, ContentSelector: v.AdapterConfig.ContentSelector}
	}
	return s
}
func sourceToResponse(s domain.Source) sourceResponse {
	policy := policyWrite{Status: s.ScraperPolicy.Status, ReviewedAt: s.ScraperPolicy.ReviewedAt}
	if policy.Status == "" {
		policy.Status = domain.ScraperPolicyNotApplicable
	}
	if s.ScraperPolicy.TermsURL != "" {
		x := s.ScraperPolicy.TermsURL
		policy.TermsURL = &x
	}
	if s.ScraperPolicy.RobotsURL != "" {
		x := s.ScraperPolicy.RobotsURL
		policy.RobotsURL = &x
	}
	if s.ScraperPolicy.ReviewNotes != "" {
		x := s.ScraperPolicy.ReviewNotes
		policy.ReviewNotes = &x
	}
	var adapter any
	switch s.Kind {
	case domain.SourceKindFeed:
		adapter = struct {
			Format string `json:"format"`
		}{string(s.AdapterConfig.Feed.Format)}
	case domain.SourceKindAPI:
		adapter = struct {
			Provider string `json:"provider"`
			PageSize int    `json:"pageSize"`
		}{s.AdapterConfig.API.Provider, s.AdapterConfig.API.PageSize}
	case domain.SourceKindScraper:
		adapter = struct {
			ArticleSelector string `json:"articleSelector"`
			TitleSelector   string `json:"titleSelector"`
			ExcerptSelector string `json:"excerptSelector"`
			ContentSelector string `json:"contentSelector"`
		}{s.AdapterConfig.Scraper.ArticleSelector, s.AdapterConfig.Scraper.TitleSelector, s.AdapterConfig.Scraper.ExcerptSelector, s.AdapterConfig.Scraper.ContentSelector}
	}
	var last *string
	if strings.TrimSpace(s.LastError) != "" {
		x := s.LastError
		last = &x
	}
	return sourceResponse{ID: s.ID, Name: s.Name, URL: s.URL, Kind: s.Kind, Enabled: s.Enabled, ContentPermission: s.ContentPermission, AdapterConfig: adapter, ScraperPolicy: policy, CredentialConfigured: s.CredentialRef != nil, LastSuccessAt: s.LastSuccessAt, LastError: last, RetryAfter: s.RetryAfter}
}

func (a ConfigurationAPI) getProfile(w http.ResponseWriter, r *http.Request) {
	v, e := a.Profiles.GetProfile(r.Context())
	if e != nil {
		a.fail(w, r, e)
		return
	}
	writeJSON(w, 200, profileToResponse(v))
}
func (a ConfigurationAPI) putProfile(w http.ResponseWriter, r *http.Request) {
	var v profileWrite
	if !decodeRequired(w, r, &v, "interests", "preferredSourceIds", "location", "age", "gender") {
		return
	}
	seen := make(map[domain.SourceID]struct{}, len(v.PreferredSourceIDs))
	for _, id := range v.PreferredSourceIDs {
		if id == "" {
			writeAPIError(w, http.StatusBadRequest, "validation_failed", "The request is invalid.", randomID())
			return
		}
		if _, exists := seen[id]; exists {
			writeAPIError(w, http.StatusBadRequest, "validation_failed", "The request is invalid.", randomID())
			return
		}
		seen[id] = struct{}{}
	}
	p, e := a.Profiles.UpdateProfile(r.Context(), application.UpdateProfileCommand{Profile: profileFromWrite(v)})
	if e != nil {
		a.fail(w, r, e)
		return
	}
	writeJSON(w, 200, profileToResponse(p))
}
func (a ConfigurationAPI) getRanking(w http.ResponseWriter, r *http.Request) {
	v, e := a.Profiles.GetRankingConfiguration(r.Context())
	if e != nil {
		a.fail(w, r, e)
		return
	}
	writeJSON(w, 200, rankingToResponse(v))
}
func (a ConfigurationAPI) putRanking(w http.ResponseWriter, r *http.Request) {
	var v rankingWrite
	if !decodeRequired(w, r, &v, "recency", "interest", "sourcePreference", "behavior", "location", "age", "gender", "textSimilarity") {
		return
	}
	current, e := a.Profiles.GetRankingConfiguration(r.Context())
	if e != nil {
		a.fail(w, r, e)
		return
	}
	c := domain.RankingConfiguration{Recency: fromSignal(v.Recency), Interest: fromSignal(v.Interest), SourcePreference: fromSignal(v.SourcePreference), Behavior: fromSignal(v.Behavior), Location: fromSignal(v.Location), Age: fromSignal(v.Age), Gender: fromSignal(v.Gender), TextSimilarity: fromSignal(v.TextSimilarity), PerDemographicCap: current.PerDemographicCap, TotalDemographicCap: current.TotalDemographicCap, NormalizationVersion: current.NormalizationVersion}
	c, e = a.Profiles.UpdateRankingConfiguration(r.Context(), application.UpdateRankingConfigurationCommand{Configuration: c})
	if e != nil {
		a.fail(w, r, e)
		return
	}
	writeJSON(w, 200, rankingToResponse(c))
}
func (a ConfigurationAPI) listStarters(w http.ResponseWriter, r *http.Request) {
	items := make([]sourceResponse, 0, len(a.Starters))
	for _, s := range a.Starters {
		items = append(items, sourceToResponse(s))
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (a ConfigurationAPI) listSources(w http.ResponseWriter, r *http.Request) {
	v, e := a.Sources.ListSources(r.Context())
	if e != nil {
		a.fail(w, r, e)
		return
	}
	items := make([]sourceResponse, 0, len(v))
	for _, s := range v {
		items = append(items, sourceToResponse(s))
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (a ConfigurationAPI) getSource(w http.ResponseWriter, r *http.Request) {
	v, e := a.Sources.GetSource(r.Context(), domain.SourceID(r.PathValue("sourceId")))
	if e != nil {
		a.fail(w, r, e)
		return
	}
	writeJSON(w, 200, sourceToResponse(v))
}
func (a ConfigurationAPI) createSource(w http.ResponseWriter, r *http.Request) {
	var v sourceWrite
	if !decodeRequired(w, r, &v, "name", "url", "kind", "enabled", "contentPermission", "adapterConfig", "scraperPolicy") {
		return
	}
	id := domain.SourceID(a.NewID())
	if id == "" {
		a.fail(w, r, application.ErrUnavailable)
		return
	}
	if _, e := a.Sources.GetSource(r.Context(), id); e == nil {
		a.fail(w, r, application.ErrConflict)
		return
	} else if !errors.Is(e, application.ErrNotFound) {
		a.fail(w, r, e)
		return
	}
	s, e := a.Sources.SaveSource(r.Context(), application.SaveSourceCommand{Source: sourceFromWrite(id, v)})
	if e != nil {
		a.fail(w, r, e)
		return
	}
	writeJSON(w, 201, sourceToResponse(s))
}
func (a ConfigurationAPI) updateSource(w http.ResponseWriter, r *http.Request) {
	id := domain.SourceID(r.PathValue("sourceId"))
	if _, e := a.Sources.GetSource(r.Context(), id); e != nil {
		a.fail(w, r, e)
		return
	}
	var v sourceWrite
	if !decodeRequired(w, r, &v, "name", "url", "kind", "enabled", "contentPermission", "adapterConfig", "scraperPolicy") {
		return
	}
	s, e := a.Sources.SaveSource(r.Context(), application.SaveSourceCommand{Source: sourceFromWrite(id, v)})
	if e != nil {
		a.fail(w, r, e)
		return
	}
	writeJSON(w, 200, sourceToResponse(s))
}
func (a ConfigurationAPI) deleteSource(w http.ResponseWriter, r *http.Request) {
	e := a.Sources.DeleteSource(r.Context(), application.DeleteSourceCommand{SourceID: domain.SourceID(r.PathValue("sourceId"))})
	if e != nil {
		a.fail(w, r, e)
		return
	}
	w.WriteHeader(204)
}
func (a ConfigurationAPI) putCredential(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Secret string `json:"secret"`
	}
	if !decode(w, r, &v) {
		return
	}
	e := a.Sources.ConfigureCredential(r.Context(), application.ConfigureCredentialCommand{SourceID: domain.SourceID(r.PathValue("sourceId")), Secret: []byte(v.Secret)})
	v.Secret = ""
	if e != nil {
		a.fail(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]bool{"configured": true})
}
func (a ConfigurationAPI) deleteCredential(w http.ResponseWriter, r *http.Request) {
	e := a.Sources.DeleteCredential(r.Context(), application.DeleteCredentialCommand{SourceID: domain.SourceID(r.PathValue("sourceId"))})
	if e != nil {
		a.fail(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]bool{"configured": false})
}

func decode(w http.ResponseWriter, r *http.Request, d any) bool { return decodeRequired(w, r, d) }
func decodeRequired(w http.ResponseWriter, r *http.Request, d any, required ...string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	contents, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", "The request is invalid.", randomID())
		return false
	}
	dec := json.NewDecoder(strings.NewReader(string(contents)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(d); err != nil {
		writeAPIError(w, 400, "validation_failed", "The request is invalid.", randomID())
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeAPIError(w, 400, "validation_failed", "The request is invalid.", randomID())
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(contents, &object) != nil {
		writeAPIError(w, 400, "validation_failed", "The request is invalid.", randomID())
		return false
	}
	for _, field := range required {
		if _, ok := object[field]; !ok {
			writeAPIError(w, 400, "validation_failed", "The request is invalid.", randomID())
			return false
		}
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeAPIError(w http.ResponseWriter, status int, code, message, id string) {
	writeJSON(w, status, map[string]any{"code": code, "message": message, "correlationId": id, "fields": []any{}})
}
func (a ConfigurationAPI) fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := 500, "internal_error", "An unexpected error occurred."
	switch {
	case errors.Is(err, application.ErrInvalidInput):
		status, code, msg = 400, "validation_failed", "The request is invalid."
	case errors.Is(err, application.ErrNotFound):
		status, code, msg = 404, "not_found", "The requested resource was not found."
	case errors.Is(err, application.ErrConflict):
		status, code, msg = 409, "conflict", "The request conflicts with current state."
	case errors.Is(err, application.ErrUnsupportedPlatform):
		status, code, msg = 503, "unsupported_platform", "Credential storage is unsupported on this platform."
	case errors.Is(err, application.ErrUnavailable), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, code, msg = 503, "unavailable", "Storage is temporarily unavailable."
	}
	id := randomID()
	a.Logger.Error("api request failed", "correlation_id", id, "code", code, "method", r.Method, "path", r.URL.Path)
	writeAPIError(w, status, code, msg, id)
}
func randomID() string {
	var b [12]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "local-request"
	}
	return hex.EncodeToString(b[:])
}
func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}
