// Package domain defines the dependency-free vocabulary and invariants shared
// by application services and infrastructure adapters.
//
// OptionalSignal represents presence and ranking enablement independently.
// Article content permission is explicit and validated independently from
// whether text is present. RankingResult retains machine-readable signal
// contributions so presentation layers never need to recompute relevance.
package domain
