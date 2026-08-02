package application

import (
	"context"

	"github.com/infopek/news-aggregator/internal/domain"
)

// CredentialReference derives the stable opaque reference owned by a source.
// It is injected so application code does not know platform naming details.
type CredentialReference func(domain.SourceID) domain.CredentialID

type CredentialManager struct {
	Status    CredentialStatusReader
	Writer    CredentialWriter
	Reference CredentialReference
}

func (manager CredentialManager) Configured(ctx context.Context, sourceID domain.SourceID) (bool, error) {
	if manager.Status == nil || manager.Reference == nil || sourceID == "" {
		return false, ErrInvalidInput
	}
	return manager.Status.Configured(ctx, manager.Reference(sourceID))
}

func (manager CredentialManager) Configure(ctx context.Context, command ConfigureCredentialCommand) error {
	if manager.Writer == nil || manager.Reference == nil || command.SourceID == "" || len(command.Secret) == 0 {
		return ErrInvalidInput
	}
	return manager.Writer.Store(ctx, manager.Reference(command.SourceID), command.Secret)
}

// Delete is the documented source-cleanup path. Its derived reference scopes
// deletion to exactly one source and cannot affect other application entries.
func (manager CredentialManager) Delete(ctx context.Context, command DeleteCredentialCommand) error {
	if manager.Writer == nil || manager.Reference == nil || command.SourceID == "" {
		return ErrInvalidInput
	}
	return manager.Writer.Delete(ctx, manager.Reference(command.SourceID))
}
