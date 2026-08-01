package domain

import "time"

type RefreshStatus string

const (
	RefreshRunning        RefreshStatus = "running"
	RefreshSucceeded      RefreshStatus = "succeeded"
	RefreshPartialSuccess RefreshStatus = "partial_success"
	RefreshFailed         RefreshStatus = "failed"
	RefreshCancelled      RefreshStatus = "cancelled"
)

type SourceRefreshOutcome struct {
	SourceID     SourceID
	Fetched      int
	Inserted     int
	Updated      int
	Skipped      int
	Failed       int
	ErrorCode    string
	ErrorSummary string
}

type RefreshRun struct {
	ID         RefreshRunID
	StartedAt  time.Time
	FinishedAt *time.Time
	Status     RefreshStatus
	Outcomes   []SourceRefreshOutcome
}
