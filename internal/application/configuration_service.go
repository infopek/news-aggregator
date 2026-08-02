package application

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"

	"github.com/infopek/news-aggregator/internal/domain"
)

var configurationSourceLocks sync.Map

func lockConfigurationSource(id domain.SourceID) func() {
	value, _ := configurationSourceLocks.LoadOrStore(id, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

type ConfigurationService struct {
	Profiles            ProfileRepository
	Rankings            RankingRepository
	Sources             SourceRepository
	Transactions        TransactionManager
	Credentials         CredentialStore
	Clock               Clock
	CredentialReference CredentialReference
}

func (service ConfigurationService) Initialize(ctx context.Context, profile domain.UserProfile, ranking domain.RankingConfiguration) error {
	if err := service.readyProfile(); err != nil {
		return err
	}
	if profile.ID == "" {
		profile.ID = domain.LocalProfileID
	}
	if profile.ID != domain.LocalProfileID || profile.Validate() != nil || ranking.Validate() != nil {
		return ErrInvalidInput
	}
	_, err := service.Profiles.Get(ctx, domain.LocalProfileID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	profile.UpdatedAt = service.Clock.Now()
	return service.Transactions.WithinTransaction(ctx, func(txctx context.Context) error {
		if err := service.Profiles.Save(txctx, profile); err != nil {
			return err
		}
		return service.Rankings.SaveConfiguration(txctx, ranking)
	})
}

func (service ConfigurationService) GetProfile(ctx context.Context) (domain.UserProfile, error) {
	if service.Profiles == nil {
		return domain.UserProfile{}, ErrInvalidInput
	}
	return service.Profiles.Get(ctx, domain.LocalProfileID)
}

func (service ConfigurationService) UpdateProfile(ctx context.Context, command UpdateProfileCommand) (domain.UserProfile, error) {
	if service.Profiles == nil || service.Clock == nil {
		return domain.UserProfile{}, ErrInvalidInput
	}
	p := command.Profile
	if p.ID == "" {
		p.ID = domain.LocalProfileID
	}
	if p.ID != domain.LocalProfileID || p.Validate() != nil {
		return domain.UserProfile{}, ErrInvalidInput
	}
	p.UpdatedAt = service.Clock.Now()
	if err := service.Profiles.Save(ctx, p); err != nil {
		return domain.UserProfile{}, err
	}
	return p, nil
}

func (service ConfigurationService) GetRankingConfiguration(ctx context.Context) (domain.RankingConfiguration, error) {
	if service.Rankings == nil {
		return domain.RankingConfiguration{}, ErrInvalidInput
	}
	return service.Rankings.GetConfiguration(ctx)
}

func (service ConfigurationService) UpdateRankingConfiguration(ctx context.Context, command UpdateRankingConfigurationCommand) (domain.RankingConfiguration, error) {
	c := command.Configuration
	if service.Rankings == nil || c.Validate() != nil {
		return domain.RankingConfiguration{}, ErrInvalidInput
	}
	if err := service.Rankings.SaveConfiguration(ctx, c); err != nil {
		return domain.RankingConfiguration{}, err
	}
	return c, nil
}

func (service ConfigurationService) ListSources(ctx context.Context) ([]domain.Source, error) {
	if service.Sources == nil {
		return nil, ErrInvalidInput
	}
	return service.Sources.List(ctx)
}

func (service ConfigurationService) GetSource(ctx context.Context, id domain.SourceID) (domain.Source, error) {
	if service.Sources == nil || id == "" {
		return domain.Source{}, ErrInvalidInput
	}
	return service.Sources.Get(ctx, id)
}

func (service ConfigurationService) SaveSource(ctx context.Context, command SaveSourceCommand) (domain.Source, error) {
	s := command.Source
	if service.Sources == nil || s.ID == "" {
		return domain.Source{}, ErrInvalidInput
	}
	unlock := lockConfigurationSource(s.ID)
	defer unlock()
	existing, getErr := service.Sources.Get(ctx, s.ID)
	switch {
	case getErr == nil:
		// These fields are application-owned operational state. A transport
		// payload can neither detach nor fabricate a credential reference, nor
		// overwrite ingestion progress with a stale whole-source representation.
		s.CredentialRef = existing.CredentialRef
		s.RefreshCursor = existing.RefreshCursor
		s.RefreshETag = existing.RefreshETag
		s.RefreshLastModified = existing.RefreshLastModified
		s.LastSuccessAt = existing.LastSuccessAt
		s.LastError = existing.LastError
		s.RetryAfter = existing.RetryAfter
	case errors.Is(getErr, ErrNotFound):
		if s.CredentialRef != nil {
			return domain.Source{}, ErrInvalidInput
		}
	default:
		return domain.Source{}, getErr
	}
	normalized, err := normalizeSourceURL(s.URL)
	if err != nil {
		return domain.Source{}, ErrInvalidInput
	}
	s.URL = normalized
	if s.Validate() != nil {
		return domain.Source{}, ErrInvalidInput
	}
	if err := service.Sources.Save(ctx, s); err != nil {
		return domain.Source{}, err
	}
	return s, nil
}

func (service ConfigurationService) ImportStarterSources(ctx context.Context, command ImportStarterSourcesCommand) ([]domain.Source, error) {
	if service.Sources == nil || service.Transactions == nil {
		return nil, ErrInvalidInput
	}
	for _, starter := range command.Sources {
		if starter.ID == "" || starter.CredentialRef != nil {
			return nil, ErrInvalidInput
		}
	}
	if err := service.Transactions.WithinTransaction(ctx, func(txctx context.Context) error {
		for _, starter := range command.Sources {
			if starter.ID == "" {
				return ErrInvalidInput
			}
			if _, err := service.Sources.Get(txctx, starter.ID); err == nil {
				continue
			} else if !errors.Is(err, ErrNotFound) {
				return err
			}
			normalized, err := normalizeSourceURL(starter.URL)
			if err != nil {
				return ErrInvalidInput
			}
			starter.URL = normalized
			if starter.Validate() != nil {
				return ErrInvalidInput
			}
			if err := service.Sources.Save(txctx, starter); err != nil {
				// A user-created source with the same URL wins over the starter.
				if errors.Is(err, ErrConflict) {
					continue
				}
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return service.Sources.List(ctx)
}

func (service ConfigurationService) DeleteSource(ctx context.Context, command DeleteSourceCommand) error {
	if service.Sources == nil || service.Transactions == nil || command.SourceID == "" {
		return ErrInvalidInput
	}
	unlock := lockConfigurationSource(command.SourceID)
	defer unlock()
	source, err := service.Sources.Get(ctx, command.SourceID)
	if err != nil {
		return err
	}
	if source.CredentialRef != nil && service.Credentials == nil {
		return ErrInvalidInput
	}
	if source.CredentialRef == nil {
		return service.Transactions.WithinTransaction(ctx, func(txctx context.Context) error { return service.Sources.Delete(txctx, command.SourceID) })
	}
	ref := *source.CredentialRef
	err = service.Credentials.WithSecret(ctx, ref, func(prior []byte) error {
		if err := service.Credentials.Delete(ctx, ref); err != nil {
			return err
		}
		if err := service.Transactions.WithinTransaction(ctx, func(txctx context.Context) error { return service.Sources.Delete(txctx, command.SourceID) }); err != nil {
			if restoreErr := service.Credentials.Store(context.WithoutCancel(ctx), ref, prior); restoreErr != nil {
				return ErrUnavailable
			}
			return err
		}
		return nil
	})
	if errors.Is(err, ErrCredentialMissing) {
		// The reference is already stale; removing the source cannot orphan a secret.
		return service.Transactions.WithinTransaction(ctx, func(txctx context.Context) error { return service.Sources.Delete(txctx, command.SourceID) })
	}
	return err
}

func (service ConfigurationService) ConfigureCredential(ctx context.Context, command ConfigureCredentialCommand) error {
	if service.Sources == nil || service.Transactions == nil || service.Credentials == nil || service.CredentialReference == nil || command.SourceID == "" || len(command.Secret) == 0 {
		return ErrInvalidInput
	}
	unlock := lockConfigurationSource(command.SourceID)
	defer unlock()
	source, err := service.Sources.Get(ctx, command.SourceID)
	if err != nil {
		return err
	}
	reference := service.CredentialReference(command.SourceID)
	if reference == "" {
		return ErrInvalidInput
	}
	if source.CredentialRef != nil {
		if *source.CredentialRef != reference {
			return ErrUnavailable
		}
		// Credential-store replacement is atomic at the stable key; no database
		// write is needed and the persisted reference remains resolvable.
		return service.Credentials.Store(ctx, reference, command.Secret)
	}
	// Establish durable ownership before writing the secret. A vault failure
	// leaves no unowned secret and rolls the reference back to unconfigured.
	source.CredentialRef = &reference
	if err := service.Transactions.WithinTransaction(ctx, func(txctx context.Context) error { return service.Sources.Save(txctx, source) }); err != nil {
		return err
	}
	if err := service.Credentials.Store(ctx, reference, command.Secret); err != nil {
		source.CredentialRef = nil
		if rollbackErr := service.Transactions.WithinTransaction(context.WithoutCancel(ctx), func(txctx context.Context) error { return service.Sources.Save(txctx, source) }); rollbackErr != nil {
			return ErrUnavailable
		}
		return err
	}
	return nil
}

func (service ConfigurationService) DeleteCredential(ctx context.Context, command DeleteCredentialCommand) error {
	if service.Sources == nil || service.Transactions == nil || service.Credentials == nil || command.SourceID == "" {
		return ErrInvalidInput
	}
	unlock := lockConfigurationSource(command.SourceID)
	defer unlock()
	source, err := service.Sources.Get(ctx, command.SourceID)
	if err != nil {
		return err
	}
	if source.CredentialRef == nil {
		return nil
	}
	ref := *source.CredentialRef
	err = service.Credentials.WithSecret(ctx, ref, func(prior []byte) error {
		if err := service.Credentials.Delete(ctx, ref); err != nil {
			return err
		}
		source.CredentialRef = nil
		if err := service.Transactions.WithinTransaction(ctx, func(txctx context.Context) error { return service.Sources.Save(txctx, source) }); err != nil {
			source.CredentialRef = &ref
			if restoreErr := service.Credentials.Store(context.WithoutCancel(ctx), ref, prior); restoreErr != nil {
				return ErrUnavailable
			}
			return err
		}
		return nil
	})
	if errors.Is(err, ErrCredentialMissing) {
		source.CredentialRef = nil
		return service.Transactions.WithinTransaction(ctx, func(txctx context.Context) error { return service.Sources.Save(txctx, source) })
	}
	return err
}

func (service ConfigurationService) readyProfile() error {
	if service.Profiles == nil || service.Rankings == nil || service.Transactions == nil || service.Clock == nil {
		return ErrInvalidInput
	}
	return nil
}

func normalizeSourceURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", ErrInvalidInput
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), nil
}

var _ ProfileService = ConfigurationService{}
var _ SourceService = ConfigurationService{}
