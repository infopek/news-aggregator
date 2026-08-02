//go:build windows

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/adapters/credentials"
	"github.com/infopek/news-aggregator/internal/domain"
)

func TestWindowsCredentialLifecycle(t *testing.T) {
	ctx := context.Background()
	store := credentials.NewStore()
	// Test-only identity stays inside the application's namespace. Cleanup is
	// registered before writes so interrupted assertions do not leave entries.
	id := credentials.ReferenceForSource(domain.SourceID("integration-" + fmt.Sprint(time.Now().UnixNano())))
	t.Cleanup(func() { _ = store.Delete(context.Background(), id) })
	first, replacement := []byte("windows-test-first"), []byte("windows-test-replacement")
	if err := store.Store(ctx, id, first); err != nil {
		t.Fatal(err)
	}
	assertResolved(t, store, id, first)
	if err := store.Store(ctx, id, replacement); err != nil {
		t.Fatal(err)
	}
	assertResolved(t, store, id, replacement)
	if err := store.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	configured, err := store.Configured(ctx, id)
	if err != nil || configured {
		t.Fatalf("Configured() after delete = %v, %v", configured, err)
	}
	if err := store.WithSecret(ctx, id, func([]byte) error { return nil }); !errors.Is(err, credentials.ErrMissing) {
		t.Fatalf("missing error = %v", err)
	}
}

func assertResolved(t *testing.T, store *credentials.Store, id domain.CredentialID, want []byte) {
	t.Helper()
	if err := store.WithSecret(context.Background(), id, func(got []byte) error {
		if string(got) != string(want) {
			return errors.New("resolved credential did not match")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
