package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/infopek/news-aggregator/internal/adapters/credentials"
)

func TestCredentialSentinelAbsentFromErrorsLogsAndArtifacts(t *testing.T) {
	const secret = "BACKEND003_INTEGRATION_SENTINEL_42f7"
	ctx := context.Background()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	store := credentials.NewFake(nil)
	id := credentials.ReferenceForSource("source-private")
	if err := store.Store(ctx, id, []byte(secret)); err != nil {
		t.Fatal(err)
	}
	err := store.WithSecret(ctx, id, func([]byte) error { return nil })
	logger.Error("credential operation failed", "error", err)

	dir := t.TempDir()
	artifacts := []string{
		filepath.Join(dir, "diagnostic.log"),
		filepath.Join(dir, "snapshot.txt"),
		filepath.Join(dir, "temporary.sqlite"),
	}
	for _, path := range artifacts {
		if err := os.WriteFile(path, []byte(fmt.Sprintf("configured=true reference=%s error=%v logs=%s", id, err, logs.String())), 0o600); err != nil {
			t.Fatal(err)
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(body), secret) {
			t.Fatalf("sentinel leaked into %s", filepath.Base(path))
		}
	}
	if strings.Contains(logs.String(), secret) || strings.Contains(err.Error(), secret) {
		t.Fatal("sentinel leaked through diagnostic surface")
	}
}
