//go:build !windows

package credentials

import (
	"context"
	"errors"
	"testing"

	"github.com/infopek/news-aggregator/internal/application"
)

func TestPortableStoreIsExplicitlyUnsupported(t *testing.T) {
	store := NewStore()
	configured, err := store.Configured(context.Background(), "id")
	if configured || !errors.Is(err, application.ErrUnsupportedPlatform) {
		t.Fatalf("Configured() = %v, %v", configured, err)
	}
	if err := store.Store(context.Background(), "id", []byte(sentinel)); !errors.Is(err, application.ErrUnsupportedPlatform) {
		t.Fatalf("Store() error = %v", err)
	}
	if err := store.Delete(context.Background(), "id"); !errors.Is(err, application.ErrUnsupportedPlatform) {
		t.Fatalf("Delete() error = %v", err)
	}
}
