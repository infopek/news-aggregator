//go:build !windows

package credentials

import (
	"context"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

// Store explicitly reports unsupported-platform errors. It never falls back
// to files, SQLite, environment variables, or another plaintext mechanism.
type Store struct{}

var _ application.CredentialStore = (*Store)(nil)

func NewStore() *Store { return &Store{} }
func (*Store) Configured(context.Context, domain.CredentialID) (bool, error) {
	return false, unsupported("status")
}
func (*Store) Store(context.Context, domain.CredentialID, []byte) error { return unsupported("write") }
func (*Store) Delete(context.Context, domain.CredentialID) error        { return unsupported("delete") }
func (*Store) WithSecret(context.Context, domain.CredentialID, func([]byte) error) error {
	return unsupported("resolve")
}
