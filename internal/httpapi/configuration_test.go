package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type fakeConfiguration struct {
	profile    domain.UserProfile
	ranking    domain.RankingConfiguration
	sources    map[domain.SourceID]domain.Source
	err        error
	secretSeen string
}

func (f *fakeConfiguration) GetProfile(context.Context) (domain.UserProfile, error) {
	return f.profile, f.err
}
func (f *fakeConfiguration) UpdateProfile(_ context.Context, c application.UpdateProfileCommand) (domain.UserProfile, error) {
	if f.err != nil {
		return domain.UserProfile{}, f.err
	}
	f.profile = c.Profile
	f.profile.UpdatedAt = time.Unix(1, 0).UTC()
	return f.profile, nil
}
func (f *fakeConfiguration) GetRankingConfiguration(context.Context) (domain.RankingConfiguration, error) {
	return f.ranking, f.err
}
func (f *fakeConfiguration) UpdateRankingConfiguration(_ context.Context, c application.UpdateRankingConfigurationCommand) (domain.RankingConfiguration, error) {
	return c.Configuration, f.err
}
func (f *fakeConfiguration) ListSources(context.Context) ([]domain.Source, error) {
	v := make([]domain.Source, 0, len(f.sources))
	for _, s := range f.sources {
		v = append(v, s)
	}
	return v, f.err
}
func (f *fakeConfiguration) GetSource(_ context.Context, id domain.SourceID) (domain.Source, error) {
	if f.err != nil {
		return domain.Source{}, f.err
	}
	s, ok := f.sources[id]
	if !ok {
		return s, application.ErrNotFound
	}
	return s, nil
}
func (f *fakeConfiguration) SaveSource(_ context.Context, c application.SaveSourceCommand) (domain.Source, error) {
	if f.err != nil {
		return domain.Source{}, f.err
	}
	f.sources[c.Source.ID] = c.Source
	return c.Source, nil
}
func (f *fakeConfiguration) ImportStarterSources(context.Context, application.ImportStarterSourcesCommand) ([]domain.Source, error) {
	return nil, f.err
}
func (f *fakeConfiguration) DeleteSource(_ context.Context, c application.DeleteSourceCommand) error {
	if f.err != nil {
		return f.err
	}
	if _, ok := f.sources[c.SourceID]; !ok {
		return application.ErrNotFound
	}
	delete(f.sources, c.SourceID)
	return nil
}
func (f *fakeConfiguration) ConfigureCredential(_ context.Context, c application.ConfigureCredentialCommand) error {
	f.secretSeen = string(c.Secret)
	return f.err
}
func (f *fakeConfiguration) DeleteCredential(context.Context, application.DeleteCredentialCommand) error {
	return f.err
}

func validFake() *fakeConfiguration {
	return &fakeConfiguration{profile: domain.UserProfile{ID: domain.LocalProfileID, UpdatedAt: time.Unix(1, 0).UTC()}, ranking: domain.RankingConfiguration{Recency: domain.SignalWeight{Enabled: true, Weight: .5}, PerDemographicCap: .1, TotalDemographicCap: .2, NormalizationVersion: "v1"}, sources: map[domain.SourceID]domain.Source{}}
}
func request(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestConfigurationRoutesSuccessAndStrictBodies(t *testing.T) {
	f := validFake()
	h := NewConfigurationHandler(ConfigurationAPI{Profiles: f, Sources: f, NewID: func() string { return "generated" }})
	profile := `{"interests":[{"name":"technology","weight":0.5}],"preferredSourceIds":[],"location":{"present":false,"enabled":false},"age":{"present":false,"enabled":false},"gender":{"present":false,"enabled":false}}`
	if w := request(t, h, "PUT", "/api/v1/profile", profile); w.Code != 200 {
		t.Fatalf("profile: %d %s", w.Code, w.Body.String())
	} else if strings.Contains(w.Body.String(), `"Name"`) || !strings.Contains(w.Body.String(), `"name":"technology"`) {
		t.Fatalf("profile response uses non-contract field names: %s", w.Body.String())
	}
	ranking := `{"recency":{"enabled":true,"weight":1},"interest":{"enabled":false,"weight":0},"sourcePreference":{"enabled":false,"weight":0},"behavior":{"enabled":false,"weight":0},"location":{"enabled":false,"weight":0},"age":{"enabled":false,"weight":0},"gender":{"enabled":false,"weight":0},"textSimilarity":{"enabled":false,"weight":0}}`
	if w := request(t, h, "PUT", "/api/v1/ranking-config", ranking); w.Code != 200 || strings.Contains(w.Body.String(), `"Enabled"`) || !strings.Contains(w.Body.String(), `"enabled":true`) {
		t.Fatalf("ranking response: %d %s", w.Code, w.Body.String())
	}
	source := `{"name":"Feed","url":"https://example.com/feed","kind":"feed","enabled":true,"contentPermission":"metadata_only","adapterConfig":{"format":"auto"},"scraperPolicy":{"status":"not_applicable","termsUrl":null,"robotsUrl":null,"reviewedAt":null,"reviewNotes":null}}`
	if w := request(t, h, "POST", "/api/v1/sources", source); w.Code != 201 || !strings.Contains(w.Body.String(), `"id":"generated"`) {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	for name, body := range map[string]string{"malformed": "{", "unknown": strings.TrimSuffix(profile, "}") + `,"future":true}`, "trailing": profile + ` {}`, "oversized": `{"secret":"` + strings.Repeat("x", maxRequestBytes) + `"}`} {
		w := request(t, h, "PUT", "/api/v1/profile", body)
		if w.Code != 400 {
			t.Errorf("%s: %d", name, w.Code)
		}
		assertErrorShape(t, w)
	}
	if w := request(t, h, "GET", "/api/v1/missing", ""); w.Code != 404 {
		t.Errorf("unknown API = %d", w.Code)
	}
}

func TestSourceAdaptersRejectCrossKindProperties(t *testing.T) {
	feedBase := `{"name":"Feed","url":"https://example.com/feed","kind":"feed","enabled":true,"contentPermission":"metadata_only","adapterConfig":ADAPTER,"scraperPolicy":{"status":"not_applicable","termsUrl":null,"robotsUrl":null,"reviewedAt":null,"reviewNotes":null}}`
	cases := map[string]string{
		"feed with api field":    `{"format":"auto","provider":"wrong"}`,
		"api with feed field":    `{"provider":"official","pageSize":0,"format":"rss"}`,
		"scraper with api field": `{"articleSelector":"article","titleSelector":"h1","pageSize":10}`,
	}
	for name, adapter := range cases {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(feedBase, "ADAPTER", adapter, 1)
			if strings.HasPrefix(name, "api ") {
				body = strings.Replace(body, `"kind":"feed"`, `"kind":"api"`, 1)
			}
			if strings.HasPrefix(name, "scraper ") {
				body = strings.Replace(body, `"kind":"feed"`, `"kind":"scraper"`, 1)
			}
			f := validFake()
			w := request(t, NewConfigurationHandler(ConfigurationAPI{Profiles: f, Sources: f}), "POST", "/api/v1/sources", body)
			if w.Code != 400 || !strings.Contains(w.Body.String(), `"code":"validation_failed"`) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestNestedRequiredPropertiesRejectOmissionButAcceptFalseAndZero(t *testing.T) {
	profile := `{"interests":[{"name":"technology","weight":0}],"preferredSourceIds":[],"location":{"present":true,"enabled":false,"value":{"country":"HU","region":"Central","city":{"present":false,"enabled":false}}},"age":{"present":true,"enabled":false,"value":0},"gender":{"present":false,"enabled":false}}`
	ranking := `{"recency":{"enabled":true,"weight":1},"interest":{"enabled":false,"weight":0},"sourcePreference":{"enabled":false,"weight":0},"behavior":{"enabled":false,"weight":0},"location":{"enabled":false,"weight":0},"age":{"enabled":false,"weight":0},"gender":{"enabled":false,"weight":0},"textSimilarity":{"enabled":false,"weight":0}}`
	f := validFake()
	h := NewConfigurationHandler(ConfigurationAPI{Profiles: f, Sources: f})
	if w := request(t, h, "PUT", "/api/v1/profile", profile); w.Code != 200 {
		t.Fatalf("explicit false/zero profile rejected: %d %s", w.Code, w.Body.String())
	}
	if w := request(t, h, "PUT", "/api/v1/ranking-config", ranking); w.Code != 200 {
		t.Fatalf("explicit false/zero ranking rejected: %d %s", w.Code, w.Body.String())
	}
	cases := []struct {
		name, method, path, body string
		mutate                   func(map[string]any)
	}{
		{"optional string present", "PUT", "/api/v1/profile", profile, func(v map[string]any) { delete(v["gender"].(map[string]any), "present") }},
		{"optional string", "PUT", "/api/v1/profile", profile, func(v map[string]any) { delete(v["gender"].(map[string]any), "enabled") }},
		{"optional integer", "PUT", "/api/v1/profile", profile, func(v map[string]any) { delete(v["age"].(map[string]any), "present") }},
		{"optional integer enabled", "PUT", "/api/v1/profile", profile, func(v map[string]any) { delete(v["age"].(map[string]any), "enabled") }},
		{"optional location present", "PUT", "/api/v1/profile", profile, func(v map[string]any) { delete(v["location"].(map[string]any), "present") }},
		{"optional location", "PUT", "/api/v1/profile", profile, func(v map[string]any) { delete(v["location"].(map[string]any), "enabled") }},
		{"location country", "PUT", "/api/v1/profile", profile, func(v map[string]any) { delete(v["location"].(map[string]any)["value"].(map[string]any), "country") }},
		{"location value", "PUT", "/api/v1/profile", profile, func(v map[string]any) { delete(v["location"].(map[string]any)["value"].(map[string]any), "region") }},
		{"location city", "PUT", "/api/v1/profile", profile, func(v map[string]any) { delete(v["location"].(map[string]any)["value"].(map[string]any), "city") }},
		{"city optional string", "PUT", "/api/v1/profile", profile, func(v map[string]any) {
			delete(v["location"].(map[string]any)["value"].(map[string]any)["city"].(map[string]any), "present")
		}},
		{"city optional string enabled", "PUT", "/api/v1/profile", profile, func(v map[string]any) {
			delete(v["location"].(map[string]any)["value"].(map[string]any)["city"].(map[string]any), "enabled")
		}},
		{"weighted interest name", "PUT", "/api/v1/profile", profile, func(v map[string]any) { delete(v["interests"].([]any)[0].(map[string]any), "name") }},
		{"weighted interest", "PUT", "/api/v1/profile", profile, func(v map[string]any) { delete(v["interests"].([]any)[0].(map[string]any), "weight") }},
		{"signal weight", "PUT", "/api/v1/ranking-config", ranking, func(v map[string]any) { delete(v["interest"].(map[string]any), "enabled") }},
		{"signal weight value", "PUT", "/api/v1/ranking-config", ranking, func(v map[string]any) { delete(v["interest"].(map[string]any), "weight") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal([]byte(tc.body), &value); err != nil {
				t.Fatal(err)
			}
			tc.mutate(value)
			body, _ := json.Marshal(value)
			w := request(t, h, tc.method, tc.path, string(body))
			if w.Code != 400 {
				t.Fatalf("omission accepted: %d %s", w.Code, w.Body.String())
			}
		})
	}
	source := `{"name":"Feed","url":"https://example.com/feed","kind":"feed","enabled":true,"contentPermission":"metadata_only","adapterConfig":{"format":"auto"},"scraperPolicy":{"status":"not_applicable","termsUrl":null,"robotsUrl":null,"reviewedAt":null,"reviewNotes":null}}`
	for _, field := range []string{"status", "termsUrl", "robotsUrl", "reviewedAt", "reviewNotes"} {
		t.Run("policy "+field, func(t *testing.T) {
			var value map[string]any
			_ = json.Unmarshal([]byte(source), &value)
			delete(value["scraperPolicy"].(map[string]any), field)
			body, _ := json.Marshal(value)
			w := request(t, h, "POST", "/api/v1/sources", string(body))
			if w.Code != 400 {
				t.Fatalf("omitted %s accepted: %d %s", field, w.Code, w.Body.String())
			}
		})
	}
}

func TestCredentialDocumentedFailuresByOperation(t *testing.T) {
	for _, method := range []string{"PUT", "DELETE"} {
		for _, tc := range []struct {
			err  error
			code string
		}{{application.ErrUnsupportedPlatform, "unsupported_platform"}, {application.ErrUnavailable, "unavailable"}} {
			f := validFake()
			f.err = tc.err
			body := ""
			if method == "PUT" {
				body = `{"secret":"never-log-this"}`
			}
			w := request(t, NewConfigurationHandler(ConfigurationAPI{Profiles: f, Sources: f}), method, "/api/v1/sources/source-1/credential", body)
			if w.Code != 503 || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) || strings.Contains(w.Body.String(), "never-log-this") {
				t.Fatalf("%s %v: %d %s", method, tc.err, w.Code, w.Body.String())
			}
		}
	}
}

func TestErrorFamiliesAreStableAndSanitized(t *testing.T) {
	secret := "TOP-SECRET-SENTINEL"
	var logs bytes.Buffer
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{application.ErrInvalidInput, 400, "validation_failed"}, {application.ErrNotFound, 404, "not_found"}, {application.ErrConflict, 409, "conflict"}, {application.ErrUnsupportedPlatform, 503, "unsupported_platform"}, {application.ErrUnavailable, 503, "unavailable"}, {errors.New("database exploded " + secret), 500, "internal_error"}} {
		f := validFake()
		f.err = tc.err
		h := NewConfigurationHandler(ConfigurationAPI{Profiles: f, Sources: f, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
		w := request(t, h, "GET", "/api/v1/profile", "")
		if w.Code != tc.status || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) {
			t.Errorf("%v: %d %s", tc.err, w.Code, w.Body.String())
		}
		assertErrorShape(t, w)
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatal("internal error leaked to logs")
	}
}

func TestCredentialIsWriteOnlyAndNeverLogged(t *testing.T) {
	secret := "SENTINEL-never-return"
	var logs bytes.Buffer
	f := validFake()
	h := NewConfigurationHandler(ConfigurationAPI{Profiles: f, Sources: f, Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	w := request(t, h, "PUT", "/api/v1/sources/source-1/credential", `{"secret":"`+secret+`"}`)
	if w.Code != 200 || f.secretSeen != secret {
		t.Fatalf("credential write: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), secret) || strings.Contains(logs.String(), secret) {
		t.Fatal("credential leaked")
	}
	w = request(t, h, "DELETE", "/api/v1/sources/source-1/credential", "")
	if w.Code != 200 || w.Body.String() != "{\"configured\":false}\n" {
		t.Fatalf("credential delete: %d %s", w.Code, w.Body.String())
	}
	if w := request(t, h, "GET", "/api/v1/sources/source-1/credential", ""); w.Code != 405 {
		t.Errorf("credential read status = %d", w.Code)
	}
}

func TestRequestCancellationAndTimeout(t *testing.T) {
	f := validFake()
	f.err = context.Canceled
	h := NewConfigurationHandler(ConfigurationAPI{Profiles: f, Sources: f, Timeout: time.Millisecond})
	w := request(t, h, "GET", "/api/v1/profile", "")
	if w.Code != 503 || !strings.Contains(w.Body.String(), "unavailable") {
		t.Fatalf("cancelled: %d %s", w.Code, w.Body.String())
	}
}

func assertErrorShape(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	var v struct {
		Code, Message, CorrelationID string
		Fields                       []any `json:"fields"`
	}
	if json.Unmarshal(w.Body.Bytes(), &v) != nil || v.Code == "" || v.Message == "" || v.CorrelationID == "" || v.Fields == nil {
		t.Fatalf("invalid API error: %s", w.Body.String())
	}
}
