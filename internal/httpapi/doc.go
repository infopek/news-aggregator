// Package httpapi defines the boundary between the loopback HTTP API and the
// application services. Transport handlers are implemented by later tasks.
package httpapi

import "github.com/infopek/news-aggregator/internal/application"

// Services is the complete application surface required by the versioned API.
// Keeping this composition here makes handler implementations depend on the
// use-case layer rather than persistence or ingestion details.
type Services interface {
	application.ProfileService
	application.SourceService
	application.RefreshService
	application.FeedService
}
