package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type refreshServiceStub struct {
	start    domain.RefreshRun
	startErr error
	get      domain.RefreshRun
	getErr   error
}

func (s refreshServiceStub) StartRefresh(context.Context, application.StartRefreshCommand) (domain.RefreshRun, error) {
	return s.start, s.startErr
}
func (s refreshServiceStub) GetRefresh(context.Context, domain.RefreshRunID) (domain.RefreshRun, error) {
	return s.get, s.getErr
}

func TestRefreshHTTPStartAndGet(t *testing.T) {
	started := time.Date(2026, 8, 8, 1, 2, 3, 0, time.UTC)
	finished := started.Add(time.Second)
	running := domain.RefreshRun{ID: "run", Status: domain.RefreshRunning, StartedAt: started}
	terminal := domain.RefreshRun{ID: "run", Status: domain.RefreshPartialSuccess, StartedAt: started, FinishedAt: &finished, Outcomes: []domain.SourceRefreshOutcome{{SourceID: "a", Fetched: 2, Inserted: 1, Failed: 1, ErrorCode: "refresh_failed", ErrorSummary: "Source refresh failed."}}}
	handler := NewRefreshHandler(RefreshAPI{Service: refreshServiceStub{start: running, get: terminal}})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("start status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/refresh/run", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("get status=%d", response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "partial_success" {
		t.Fatalf("body=%v", body)
	}
}
func TestRefreshHTTPConflictAndNotFound(t *testing.T) {
	for _, test := range []struct {
		method, path string
		service      refreshServiceStub
		status       int
		code         string
	}{{http.MethodPost, "/api/v1/refresh", refreshServiceStub{startErr: application.ErrConflict}, 409, "refresh_active"}, {http.MethodGet, "/api/v1/refresh/missing", refreshServiceStub{getErr: application.ErrNotFound}, 404, "not_found"}} {
		h := NewRefreshHandler(RefreshAPI{Service: test.service})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
		if w.Code != test.status {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body["code"] != test.code {
			t.Fatalf("body=%v", body)
		}
	}
}
