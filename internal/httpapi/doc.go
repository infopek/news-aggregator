// Package httpapi defines the boundary between the loopback HTTP API and the
// application services. Transport handlers are implemented by later tasks.
package httpapi

import (
	"context"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

// SourceQueries supplies the two API reads intentionally not present in the
// CONTRACT-002 command-oriented SourceService. CONTRACT-006 implements these
// application queries together with starter-catalog import behavior.
type SourceQueries interface {
	GetSource(context.Context, domain.SourceID) (domain.Source, error)
	ListStarterSources(context.Context) ([]domain.Source, error)
}

// Services is the complete application surface required by the versioned API.
// Keeping this composition here makes handler implementations depend on the
// use-case layer rather than persistence or ingestion details.
type Services interface {
	application.ProfileService
	application.SourceService
	SourceQueries
	application.RefreshService
	application.FeedService
}
