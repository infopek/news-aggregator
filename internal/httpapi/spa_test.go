package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestLocalHandlerServesStaticAssetsAndSPAFallback(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":    {Data: []byte("<main>app</main>")},
		"assets/app.js": {Data: []byte("console.log('app')")},
	}
	handler := NewLocalHandler(nil, assets)

	for _, test := range []struct {
		path string
		body string
	}{
		{path: "/assets/app.js", body: "console.log('app')"},
		{path: "/saved/articles", body: "<main>app</main>"},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.String() != test.body {
			t.Errorf("GET %s: status %d body %q", test.path, response.Code, response.Body.String())
		}
	}
}

func TestLocalHandlerDoesNotFallbackForUnknownAPI(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("app")}}
	handler := NewLocalHandler(http.NotFoundHandler(), assets)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown API status = %d, want 404", response.Code)
	}
	if response.Body.String() == "app" {
		t.Fatal("unknown API route received SPA entrypoint")
	}
}

func TestLocalHandlerReportsMissingAssets(t *testing.T) {
	handler := NewLocalHandler(nil, fstest.MapFS{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing assets status = %d, want 503", response.Code)
	}
}
