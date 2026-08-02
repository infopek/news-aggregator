package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/infopek/news-aggregator/internal/adapters/sqlite"
)

func TestConfiguredPort(t *testing.T) {
	for _, test := range []struct {
		value string
		want  int
		valid bool
	}{
		{value: "", want: 0, valid: true},
		{value: "8090", want: 8090, valid: true},
		{value: "0"},
		{value: "70000"},
		{value: "news"},
	} {
		got, err := configuredPort(test.value)
		if (err == nil) != test.valid {
			t.Errorf("configuredPort(%q) error = %v", test.value, err)
		} else if err == nil && got != test.want {
			t.Errorf("configuredPort(%q) = %d, want %d", test.value, got, test.want)
		}
	}
}

func TestEmbeddedMigrationsStartAndRestartOutsideRepository(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir("../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		canonical, err := os.ReadFile(filepath.Join("../../db/migrations", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		embedded, err := embeddedMigrations.ReadFile(filepath.ToSlash(filepath.Join("migrations", entry.Name())))
		if err != nil {
			t.Fatal(err)
		}
		if string(canonical) != string(embedded) {
			t.Fatalf("embedded migration %s drifted", entry.Name())
		}
	}
	arbitrary := t.TempDir()
	if err := os.Chdir(arbitrary); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(original)
	data := filepath.Join(t.TempDir(), "data")
	for attempt := 0; attempt < 2; attempt++ {
		migrationDir, err := materializeMigrations(data)
		if err != nil {
			t.Fatal(err)
		}
		store, err := sqlite.Open(context.Background(), sqlite.Config{Path: filepath.Join(data, "news.db"), MigrationDir: migrationDir})
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
