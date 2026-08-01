package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestHealthHandlerMatchesOpenAPIResponse(t *testing.T) {
	response := httptest.NewRecorder()
	NewHealthHandler("0.1.0").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q", response.Header().Get("Content-Type"))
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"status": "ready", "version": "0.1.0"}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("health response = %#v, want %#v", body, want)
	}
}
