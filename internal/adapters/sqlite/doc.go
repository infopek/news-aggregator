// Package sqlite implements the application's authoritative local persistence.
//
// A Store uses one database connection. This deliberately serializes writes,
// avoids surprising in-memory pragma differences, and is sufficient for the
// single-user local process. WAL permits readers in other connections while a
// writer is active; a bounded busy timeout turns short contention into waiting
// and longer contention into application.ErrUnavailable. Foreign keys are
// enabled and verified when the connection is opened.
//
// WithinTransaction is the application transaction boundary. Repository calls
// made with its callback context share one transaction; an error or cancelled
// context rolls back every write in that callback. Refresh ingestion can use
// one callback per source, allowing successful sources to commit independently
// before the RefreshRun records partial success.
package sqlite
