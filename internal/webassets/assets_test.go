package webassets

import (
	"io/fs"
	"testing"
)

func TestEmbeddedDistDirectoryExists(t *testing.T) {
	entries, err := fs.ReadDir(Files, "dist")
	if err != nil {
		t.Fatalf("read embedded dist directory: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embedded dist directory is empty")
	}
}
