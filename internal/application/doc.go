// Package application defines use-case contracts and infrastructure ports.
// It contains no concrete persistence, network, source-provider, or
// platform-specific implementation.
//
// Port inventory:
//   - ProfileRepository, SourceRepository, ArticleRepository,
//     LibraryRepository, RankingRepository, and RefreshRepository own durable
//     records behind TransactionManager boundaries.
//   - HTTPFetcher is the only outbound HTTP boundary used by ingestion.
//   - CredentialStore keeps secrets write-only except for callback-scoped use
//     by trusted adapters.
//   - IngestionAdapter normalizes feed, API, and scraper providers behind one
//     source-kind contract.
//   - Ranker accepts local domain input and returns explainable RankingResults.
//   - Clock makes time-dependent behavior deterministic in tests.
//
// Service interfaces define profile, source, refresh, feed, article, and
// library use cases for later HTTP handlers without coupling them to transport.
package application
