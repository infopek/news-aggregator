package httpapi

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

// NewLocalHandler combines the versioned API and compiled frontend without
// allowing SPA navigation fallback to turn an unknown API route into HTML.
func NewLocalHandler(api http.Handler, assets fs.FS) http.Handler {
	if api == nil {
		api = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api" || strings.HasPrefix(request.URL.Path, "/api/") {
			api.ServeHTTP(response, request)
			return
		}
		serveSPA(response, request, assets)
	})
}

func serveSPA(response http.ResponseWriter, request *http.Request, assets fs.FS) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		response.Header().Set("Allow", "GET, HEAD")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+request.URL.Path), "/")
	if name == "." || name == "" {
		name = "index.html"
	}
	contents, err := fs.ReadFile(assets, name)
	if err != nil {
		name = "index.html"
		contents, err = fs.ReadFile(assets, name)
	}
	if err != nil {
		http.Error(response, "frontend assets unavailable", http.StatusServiceUnavailable)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		response.Header().Set("Content-Type", contentType)
	}
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(http.StatusOK)
	if request.Method == http.MethodGet {
		_, _ = response.Write(contents)
	}
}
