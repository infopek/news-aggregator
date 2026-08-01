package domain

import "time"

type ReadFilter string

const (
	ReadFilterAll    ReadFilter = "all"
	ReadFilterRead   ReadFilter = "read"
	ReadFilterUnread ReadFilter = "unread"
)

// FeedFilterState is the single local profile's authoritative persisted feed
// view. SourceID is empty when articles from every source are shown.
type FeedFilterState struct {
	ProfileID     ProfileID
	SourceID      SourceID
	Read          ReadFilter
	SavedOnly     bool
	IncludeHidden bool
	SearchQuery   string
	UpdatedAt     time.Time
}

func (state FeedFilterState) Valid() bool {
	return state.ProfileID == LocalProfileID &&
		(state.Read == ReadFilterAll || state.Read == ReadFilterRead || state.Read == ReadFilterUnread)
}
