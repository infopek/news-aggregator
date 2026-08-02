package credentials

import (
	"context"
	"sync"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

// Fake is a portable, non-secret test double. It records only configured
// references. Tests inject secret material just-in-time through Resolve.
type Fake struct {
	mu         sync.RWMutex
	configured map[domain.CredentialID]struct{}
	Resolve    func(context.Context, domain.CredentialID, func([]byte) error) error
}

var _ application.CredentialStore = (*Fake)(nil)

func NewFake(resolve func(context.Context, domain.CredentialID, func([]byte) error) error) *Fake {
	return &Fake{configured: make(map[domain.CredentialID]struct{}), Resolve: resolve}
}

func (store *Fake) Configured(_ context.Context, id domain.CredentialID) (bool, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	_, ok := store.configured[id]
	return ok, nil
}

func (store *Fake) Store(ctx context.Context, id domain.CredentialID, secret []byte) error {
	if err := ctx.Err(); err != nil {
		return safeError("write", ErrInterrupted)
	}
	if id == "" || len(secret) == 0 || len(secret) > MaxSecretBytes {
		return safeError("write", application.ErrInvalidInput)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.configured[id] = struct{}{}
	return nil
}

func (store *Fake) Delete(ctx context.Context, id domain.CredentialID) error {
	if err := ctx.Err(); err != nil {
		return safeError("delete", ErrInterrupted)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.configured[id]; !ok {
		return safeError("delete", ErrMissing)
	}
	delete(store.configured, id)
	return nil
}

func (store *Fake) WithSecret(ctx context.Context, id domain.CredentialID, use func([]byte) error) error {
	configured, _ := store.Configured(ctx, id)
	if !configured {
		return safeError("resolve", ErrMissing)
	}
	if store.Resolve == nil {
		return unsupported("resolve")
	}
	if err := store.Resolve(ctx, id, use); err != nil {
		return safeError("use", application.ErrUnavailable)
	}
	return nil
}
