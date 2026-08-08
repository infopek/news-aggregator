package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type RefreshAPI struct {
	Service application.RefreshService
	Logger  *slog.Logger
	Timeout time.Duration
}

type refreshOutcomeResponse struct {
	SourceID     domain.SourceID `json:"sourceId"`
	Fetched      int             `json:"fetched"`
	Inserted     int             `json:"inserted"`
	Updated      int             `json:"updated"`
	Skipped      int             `json:"skipped"`
	Failed       int             `json:"failed"`
	ErrorCode    *string         `json:"errorCode"`
	ErrorSummary *string         `json:"errorSummary"`
}
type refreshResponse struct {
	ID         domain.RefreshRunID      `json:"id"`
	Status     domain.RefreshStatus     `json:"status"`
	StartedAt  time.Time                `json:"startedAt"`
	FinishedAt *time.Time               `json:"finishedAt"`
	Outcomes   []refreshOutcomeResponse `json:"outcomes"`
}

func NewRefreshHandler(api RefreshAPI) http.Handler {
	if api.Timeout <= 0 {
		api.Timeout = 5 * time.Second
	}
	if api.Logger == nil {
		api.Logger = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/refresh", api.start)
	mux.HandleFunc("GET /api/v1/refresh/{refreshId}", api.get)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), api.Timeout)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (a RefreshAPI) start(w http.ResponseWriter, r *http.Request) {
	if a.Service == nil {
		a.fail(w, r, application.ErrUnavailable)
		return
	}
	run, err := a.Service.StartRefresh(r.Context(), application.StartRefreshCommand{})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, toRefreshResponse(run))
}
func (a RefreshAPI) get(w http.ResponseWriter, r *http.Request) {
	if a.Service == nil {
		a.fail(w, r, application.ErrUnavailable)
		return
	}
	id := domain.RefreshRunID(r.PathValue("refreshId"))
	if id == "" {
		a.fail(w, r, application.ErrInvalidInput)
		return
	}
	run, err := a.Service.GetRefresh(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toRefreshResponse(run))
}

func (a RefreshAPI) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, application.ErrConflict) {
		id := randomID()
		a.Logger.Error("api request failed", "correlation_id", id, "code", "refresh_active", "method", r.Method, "path", r.URL.Path)
		writeAPIError(w, http.StatusConflict, "refresh_active", "A refresh is already active.", id)
		return
	}
	ConfigurationAPI{Logger: a.Logger}.fail(w, r, err)
}
func toRefreshResponse(run domain.RefreshRun) refreshResponse {
	outcomes := make([]refreshOutcomeResponse, 0, len(run.Outcomes))
	for _, value := range run.Outcomes {
		item := refreshOutcomeResponse{SourceID: value.SourceID, Fetched: value.Fetched, Inserted: value.Inserted, Updated: value.Updated, Skipped: value.Skipped, Failed: value.Failed}
		if value.ErrorCode != "" {
			x := value.ErrorCode
			item.ErrorCode = &x
		}
		if value.ErrorSummary != "" {
			x := value.ErrorSummary
			item.ErrorSummary = &x
		}
		outcomes = append(outcomes, item)
	}
	return refreshResponse{ID: run.ID, Status: run.Status, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, Outcomes: outcomes}
}
