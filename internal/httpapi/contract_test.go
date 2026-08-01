package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type openAPIDocument struct {
	OpenAPI    string                                `json:"openapi"`
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	} `json:"components"`
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate contract test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func loadContract(t *testing.T) openAPIDocument {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(repositoryRoot(t), "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var document openAPIDocument
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatalf("OpenAPI must remain JSON-compatible YAML: %v", err)
	}
	return document
}

func TestContractIsVersionedAndCoversLocalWorkflows(t *testing.T) {
	document := loadContract(t)
	if !strings.HasPrefix(document.OpenAPI, "3.1.") {
		t.Fatalf("OpenAPI 3.1 required, got %q", document.OpenAPI)
	}
	for path := range document.Paths {
		if !strings.HasPrefix(path, "/api/v1/") {
			t.Errorf("unversioned path %q", path)
		}
	}
	required := []string{
		"/api/v1/profile", "/api/v1/ranking-config", "/api/v1/sources",
		"/api/v1/sources/{sourceId}/credential", "/api/v1/refresh",
		"/api/v1/refresh/{refreshId}", "/api/v1/feed",
		"/api/v1/articles/{articleId}", "/api/v1/articles/{articleId}/library-state",
	}
	for _, path := range required {
		if document.Paths[path] == nil {
			t.Errorf("missing workflow path %q", path)
		}
	}
	if _, exists := document.Paths["/api/v1/sources/{sourceId}/credential"]["get"]; exists {
		t.Error("credentials must never have a read endpoint")
	}
}

func TestCredentialSecretIsWriteOnly(t *testing.T) {
	document := loadContract(t)
	var credential struct {
		Properties map[string]struct {
			WriteOnly bool `json:"writeOnly"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(document.Components.Schemas["CredentialWrite"], &credential); err != nil {
		t.Fatal(err)
	}
	if !credential.Properties["secret"].WriteOnly {
		t.Error("CredentialWrite.secret must be writeOnly")
	}
}

func TestFixturesDoNotExposeCredentialsAndRespectMetadataOnly(t *testing.T) {
	fixtureRoot := filepath.Join(repositoryRoot(t), "test", "fixtures", "api")
	entries, err := os.ReadDir(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(fixtureRoot, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(contents, &value); err != nil {
			t.Errorf("%s: %v", entry.Name(), err)
		}
		assertNoSecretKey(t, entry.Name(), value)
	}

	contents, err := os.ReadFile(filepath.Join(fixtureRoot, "article-metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var article struct {
		Article struct {
			ContentPermission string `json:"contentPermission"`
		} `json:"article"`
		FullContent *string `json:"fullContent"`
	}
	if err := json.Unmarshal(contents, &article); err != nil {
		t.Fatal(err)
	}
	if article.Article.ContentPermission != "metadata_only" || article.FullContent != nil {
		t.Error("metadata-only article fixture must not contain full content")
	}
}

func assertNoSecretKey(t *testing.T, path string, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lower := strings.ToLower(key)
			if lower == "secret" || strings.Contains(lower, "apikey") || strings.Contains(lower, "api_key") || lower == "token" || lower == "password" {
				t.Errorf("%s contains credential-like key %q", path, key)
			}
			assertNoSecretKey(t, path+"."+key, child)
		}
	case []any:
		for _, child := range typed {
			assertNoSecretKey(t, path, child)
		}
	}
}
