package application

import (
	"context"
	"testing"

	"github.com/infopek/news-aggregator/internal/domain"
)

type credentialManagerStore struct {
	configured map[domain.CredentialID]bool
	deleted    []domain.CredentialID
}

func (s *credentialManagerStore) Configured(_ context.Context, id domain.CredentialID) (bool, error) {
	return s.configured[id], nil
}
func (s *credentialManagerStore) Store(_ context.Context, id domain.CredentialID, _ []byte) error {
	s.configured[id] = true
	return nil
}
func (s *credentialManagerStore) Delete(_ context.Context, id domain.CredentialID) error {
	s.deleted = append(s.deleted, id)
	delete(s.configured, id)
	return nil
}

func TestCredentialManagerExposesStatusAndSourceScopedCleanup(t *testing.T) {
	store := &credentialManagerStore{configured: map[domain.CredentialID]bool{}}
	reference := func(id domain.SourceID) domain.CredentialID { return domain.CredentialID("ref-" + id) }
	manager := CredentialManager{Status: store, Writer: store, Reference: reference}
	if err := manager.Configure(context.Background(), ConfigureCredentialCommand{SourceID: "source-a", Secret: []byte("ephemeral")}); err != nil {
		t.Fatal(err)
	}
	configured, err := manager.Configured(context.Background(), "source-a")
	if err != nil || !configured {
		t.Fatalf("Configured() = %v, %v", configured, err)
	}
	// A display-name change does not enter reference derivation; immutable ID remains.
	if err := manager.Delete(context.Background(), DeleteCredentialCommand{SourceID: "source-a"}); err != nil {
		t.Fatal(err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "ref-source-a" {
		t.Fatalf("deleted = %v", store.deleted)
	}
}
