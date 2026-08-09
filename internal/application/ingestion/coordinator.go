package ingestion

import (
	"context"
	"errors"
	"sync"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type SourceRunner interface {
	Run(context.Context, domain.SourceID) (RunResult, error)
}

type Coordinator struct {
	Refreshes      application.RefreshRepository
	Sources        application.SourceRepository
	Runners        map[domain.SourceKind]SourceRunner
	Clock          application.Clock
	NewID          func() domain.RefreshRunID
	ProcessContext context.Context
	MaxConcurrency int

	mu       sync.Mutex
	active   bool
	activeID domain.RefreshRunID
	pending  *domain.RefreshRun
	wg       sync.WaitGroup
}

func (c *Coordinator) StartRefresh(ctx context.Context, _ application.StartRefreshCommand) (domain.RefreshRun, error) {
	if c.Refreshes == nil || c.Sources == nil || c.Clock == nil || c.NewID == nil || len(c.Runners) == 0 {
		return domain.RefreshRun{}, application.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domain.RefreshRun{}, err
	}
	c.mu.Lock()
	if c.active {
		c.mu.Unlock()
		return domain.RefreshRun{}, application.ErrConflict
	}
	pending := c.pending
	c.pending = nil
	c.active = true
	c.mu.Unlock()
	if pending != nil {
		if err := c.Refreshes.Save(ctx, *pending); err != nil {
			c.setPending(pending)
			c.release()
			return domain.RefreshRun{}, application.ErrUnavailable
		}
	}
	active, err := c.Refreshes.Active(ctx)
	if err != nil {
		c.release()
		return domain.RefreshRun{}, err
	}
	if active != nil {
		finished := c.Clock.Now().UTC()
		if finished.Before(active.StartedAt) {
			finished = active.StartedAt
		}
		active.Status, active.FinishedAt, active.Outcomes = domain.RefreshCancelled, &finished, nil
		if err := c.Refreshes.Save(ctx, *active); err != nil {
			c.release()
			return domain.RefreshRun{}, application.ErrUnavailable
		}
	}
	run := domain.RefreshRun{ID: c.NewID(), StartedAt: c.Clock.Now().UTC(), Status: domain.RefreshRunning}
	if run.ID == "" || run.StartedAt.IsZero() {
		c.release()
		return domain.RefreshRun{}, application.ErrUnavailable
	}
	if err := c.Refreshes.Create(ctx, run); err != nil {
		c.release()
		if errors.Is(err, application.ErrConflict) {
			return domain.RefreshRun{}, application.ErrConflict
		}
		return domain.RefreshRun{}, err
	}
	c.mu.Lock()
	c.activeID = run.ID
	c.mu.Unlock()
	process := c.ProcessContext
	if process == nil {
		process = context.Background()
	}
	c.wg.Add(1)
	go func() { defer c.wg.Done(); c.execute(process, run) }()
	return run, nil
}

func (c *Coordinator) Wait() { c.wg.Wait() }

func (c *Coordinator) GetRefresh(ctx context.Context, id domain.RefreshRunID) (domain.RefreshRun, error) {
	if c.Refreshes == nil || id == "" {
		return domain.RefreshRun{}, application.ErrInvalidInput
	}
	c.mu.Lock()
	if c.pending != nil && c.pending.ID == id {
		pending := *c.pending
		c.mu.Unlock()
		if c.Refreshes.Save(ctx, pending) == nil {
			c.clearPending(id)
		}
		return pending, nil
	}
	activeID := c.activeID
	c.mu.Unlock()
	run, err := c.Refreshes.Get(ctx, id)
	if err != nil {
		return domain.RefreshRun{}, err
	}
	if run.Status == domain.RefreshRunning && activeID != id {
		finished := c.Clock.Now().UTC()
		if finished.Before(run.StartedAt) {
			finished = run.StartedAt
		}
		run.Status, run.FinishedAt, run.Outcomes = domain.RefreshCancelled, &finished, nil
		if err := c.Refreshes.Save(ctx, run); err != nil {
			return domain.RefreshRun{}, application.ErrUnavailable
		}
	}
	return run, nil
}

func (c *Coordinator) execute(ctx context.Context, run domain.RefreshRun) {
	defer c.release()
	sources, err := c.Sources.List(ctx)
	if err != nil {
		c.finish(run, domain.RefreshFailed, nil)
		return
	}
	enabled := make([]domain.Source, 0, len(sources))
	for _, source := range sources {
		if source.Enabled {
			enabled = append(enabled, source)
		}
	}
	limit := c.MaxConcurrency
	if limit <= 0 {
		limit = 4
	}
	if limit > 16 {
		limit = 16
	}
	type indexed struct {
		index   int
		outcome domain.SourceRefreshOutcome
	}
	jobs := make(chan struct {
		index  int
		source domain.Source
	})
	results := make(chan indexed, len(enabled))
	var workers sync.WaitGroup
	for i := 0; i < limit && i < len(enabled); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				results <- indexed{job.index, c.runSource(ctx, job.source)}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index, source := range enabled {
			select {
			case jobs <- struct {
				index  int
				source domain.Source
			}{index, source}:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { workers.Wait(); close(results) }()
	outcomes := make([]domain.SourceRefreshOutcome, len(enabled))
	completed := 0
	for result := range results {
		outcomes[result.index] = result.outcome
		completed++
	}
	if completed < len(enabled) {
		for i := range outcomes {
			if outcomes[i].SourceID == "" {
				outcomes[i] = cancelledOutcome(enabled[i].ID)
			}
		}
	}
	status := summarize(outcomes, ctx.Err() != nil)
	c.finish(run, status, outcomes)
}

func (c *Coordinator) runSource(ctx context.Context, source domain.Source) domain.SourceRefreshOutcome {
	outcome := domain.SourceRefreshOutcome{SourceID: source.ID}
	runner := c.Runners[source.Kind]
	if runner == nil {
		outcome.Failed = 1
		outcome.ErrorCode = "adapter_unavailable"
		outcome.ErrorSummary = "Source adapter is unavailable."
		return outcome
	}
	result, err := runner.Run(ctx, source.ID)
	if err != nil {
		outcome.Failed = 1
		var rateLimit *application.RateLimitError
		if errors.As(err, &rateLimit) {
			outcome.ErrorCode = "rate_limited"
			outcome.ErrorSummary = "Source is rate limited."
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			outcome.ErrorCode = "cancelled"
			outcome.ErrorSummary = "Refresh was cancelled."
		} else {
			outcome.ErrorCode = "refresh_failed"
			outcome.ErrorSummary = "Source refresh failed."
		}
		return outcome
	}
	outcome.Fetched = result.Fetched
	for _, write := range result.Writes {
		if write.Inserted {
			outcome.Inserted++
		}
		if write.Updated {
			outcome.Updated++
		}
	}
	outcome.Skipped = outcome.Fetched - outcome.Inserted - outcome.Updated
	if outcome.Skipped < 0 {
		outcome.Skipped = 0
	}
	return outcome
}

func (c *Coordinator) finish(run domain.RefreshRun, status domain.RefreshStatus, outcomes []domain.SourceRefreshOutcome) {
	finished := c.Clock.Now().UTC()
	if finished.Before(run.StartedAt) {
		finished = run.StartedAt
	}
	run.Status, run.FinishedAt, run.Outcomes = status, &finished, outcomes
	ctx := context.WithoutCancel(c.processContext())
	for attempt := 0; attempt < 3; attempt++ {
		if c.Refreshes.Save(ctx, run) == nil {
			return
		}
	}
	c.setPending(&run)
}
func (c *Coordinator) processContext() context.Context {
	if c.ProcessContext != nil {
		return c.ProcessContext
	}
	return context.Background()
}
func (c *Coordinator) release()                          { c.mu.Lock(); c.active = false; c.activeID = ""; c.mu.Unlock() }
func (c *Coordinator) setPending(run *domain.RefreshRun) { c.mu.Lock(); c.pending = run; c.mu.Unlock() }
func (c *Coordinator) clearPending(id domain.RefreshRunID) {
	c.mu.Lock()
	if c.pending != nil && c.pending.ID == id {
		c.pending = nil
	}
	c.mu.Unlock()
}
func cancelledOutcome(id domain.SourceID) domain.SourceRefreshOutcome {
	return domain.SourceRefreshOutcome{SourceID: id, Failed: 1, ErrorCode: "cancelled", ErrorSummary: "Refresh was cancelled."}
}
func summarize(outcomes []domain.SourceRefreshOutcome, cancelled bool) domain.RefreshStatus {
	if cancelled {
		return domain.RefreshCancelled
	}
	failed, succeeded := 0, 0
	for _, outcome := range outcomes {
		if outcome.Failed > 0 {
			failed++
		} else {
			succeeded++
		}
	}
	if failed == 0 {
		return domain.RefreshSucceeded
	}
	if succeeded == 0 {
		return domain.RefreshFailed
	}
	return domain.RefreshPartialSuccess
}

var _ application.RefreshService = (*Coordinator)(nil)
