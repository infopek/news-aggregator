package credentials

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/infopek/news-aggregator/internal/domain"
)

const sentinel = "BACKEND003_SENTINEL_DO_NOT_LEAK_9d31"

func TestReferenceIsOpaqueStableAndSourceScoped(t *testing.T) {
	first := ReferenceForSource("source-1")
	if first != ReferenceForSource("source-1") {
		t.Fatal("reference changed for same source identity")
	}
	if first == ReferenceForSource("source-2") {
		t.Fatal("references collide across sources")
	}
	if strings.Contains(string(first), "source-1") || strings.Contains(targetName(first), "source-1") {
		t.Fatal("reference discloses source identity")
	}
}

func TestFakeStoresOnlyConfiguredStateAndResolvesJustInTime(t *testing.T) {
	ctx := context.Background()
	id := ReferenceForSource("stable-id")
	fake := NewFake(func(_ context.Context, got domain.CredentialID, use func([]byte) error) error {
		if got != id {
			t.Fatal("wrong credential reference")
		}
		secret := []byte(sentinel)
		defer wipePortable(secret)
		return use(secret)
	})
	provided := []byte(sentinel)
	if err := fake.Store(ctx, id, provided); err != nil {
		t.Fatal(err)
	}
	provided[0] = 'x'
	configured, err := fake.Configured(ctx, id)
	if err != nil || !configured {
		t.Fatalf("Configured() = %v, %v", configured, err)
	}
	if strings.Contains(fmt.Sprintf("%#v", fake), sentinel) {
		t.Fatal("fake retained supplied secret")
	}
	if err := fake.WithSecret(ctx, id, func(secret []byte) error {
		if string(secret) != sentinel {
			t.Fatal("resolver supplied wrong secret")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := fake.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	configured, err = fake.Configured(ctx, id)
	if err != nil || configured {
		t.Fatalf("Configured() after delete = %v, %v", configured, err)
	}
}

func TestErrorsNeverContainIdentifiersOrSecrets(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	fake := NewFake(nil)
	cases := []error{
		fake.Store(cancelled, domain.CredentialID(sentinel), []byte(sentinel)),
		fake.Delete(context.Background(), domain.CredentialID(sentinel)),
		fake.WithSecret(context.Background(), domain.CredentialID(sentinel), func([]byte) error { return nil }),
	}
	for _, err := range cases {
		if err == nil {
			t.Fatal("expected error")
		}
		if strings.Contains(err.Error(), sentinel) {
			t.Fatalf("error leaked sentinel: %v", err)
		}
	}
	resolving := NewFake(func(_ context.Context, _ domain.CredentialID, use func([]byte) error) error {
		return use([]byte(sentinel))
	})
	if err := resolving.Store(context.Background(), "configured", []byte("ignored")); err != nil {
		t.Fatal(err)
	}
	err := resolving.WithSecret(context.Background(), "configured", func(secret []byte) error {
		return fmt.Errorf("provider error included %s", secret)
	})
	if err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("callback error was not safely mapped: %v", err)
	}
}

func wipePortable(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
