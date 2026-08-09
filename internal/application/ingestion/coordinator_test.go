package ingestion

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type refreshMemory struct {
	mu           sync.Mutex
	runs         map[domain.RefreshRunID]domain.RefreshRun
	saveFailures int
}

func (m *refreshMemory) Create(_ context.Context, v domain.RefreshRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		if r.Status == domain.RefreshRunning {
			return application.ErrConflict
		}
	}
	m.runs[v.ID] = v
	return nil
}
func (m *refreshMemory) Get(_ context.Context, id domain.RefreshRunID) (domain.RefreshRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.runs[id]
	if !ok {
		return v, application.ErrNotFound
	}
	return v, nil
}
func (m *refreshMemory) Save(_ context.Context, v domain.RefreshRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveFailures > 0 {
		m.saveFailures--
		return application.ErrUnavailable
	}
	m.runs[v.ID] = v
	return nil
}

func TestCoordinatorReconcilesFailedTerminalSave(t *testing.T) {
	repo := &refreshMemory{runs: map[domain.RefreshRunID]domain.RefreshRun{}, saveFailures: 4}
	c := &Coordinator{Refreshes: repo, Sources: sourceMemory{values: []domain.Source{{ID: "feed", Kind: domain.SourceKindFeed, Enabled: true}}}, Runners: map[domain.SourceKind]SourceRunner{domain.SourceKindFeed: runnerFunc(func(context.Context, domain.SourceID) (RunResult, error) {
		return RunResult{Fetched: 2, Writes: []application.ArticleWriteResult{{Inserted: true}}}, nil
	})}, Clock: fixedClock{time.Unix(10, 0)}, NewID: func() domain.RefreshRunID { return "first" }}
	first, err := c.StartRefresh(context.Background(), application.StartRefreshCommand{})
	if err != nil {
		t.Fatal(err)
	}
	c.Wait()
	stale, _ := repo.Get(context.Background(), first.ID)
	if stale.Status != domain.RefreshRunning {
		t.Fatalf("expected injected stale run, got %+v", stale)
	}
	if _, err := c.GetRefresh(context.Background(), first.ID); !errors.Is(err, application.ErrUnavailable) {
		t.Fatalf("first poll error=%v, want unavailable", err)
	}
	recovered, err := c.GetRefresh(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status == domain.RefreshRunning || recovered.FinishedAt == nil {
		t.Fatalf("unreconciled=%+v", recovered)
	}
	if recovered.Status != domain.RefreshSucceeded || len(recovered.Outcomes) != 1 || recovered.Outcomes[0].Fetched != 2 || recovered.Outcomes[0].Inserted != 1 || recovered.Outcomes[0].Skipped != 1 {
		t.Fatalf("recovered outcomes=%+v", recovered)
	}
	restarted := &Coordinator{Refreshes: repo, Sources: sourceMemory{}, Runners: c.Runners, Clock: fixedClock{time.Unix(20, 0)}, NewID: func() domain.RefreshRunID { return "unused" }}
	persisted, err := restarted.GetRefresh(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != recovered.Status || len(persisted.Outcomes) != 1 || persisted.Outcomes[0] != recovered.Outcomes[0] {
		t.Fatalf("persisted=%+v recovered=%+v", persisted, recovered)
	}
}

func TestCoordinatorReconcilesOrphanedRunDuringPolling(t *testing.T) {
	repo := &refreshMemory{runs: map[domain.RefreshRunID]domain.RefreshRun{}, saveFailures: 3}
	first := &Coordinator{Refreshes: repo, Sources: sourceMemory{}, Runners: map[domain.SourceKind]SourceRunner{domain.SourceKindFeed: runnerFunc(func(context.Context, domain.SourceID) (RunResult, error) { return RunResult{}, nil })}, Clock: fixedClock{time.Unix(10, 0)}, NewID: func() domain.RefreshRunID { return "orphan" }}
	run, err := first.StartRefresh(context.Background(), application.StartRefreshCommand{})
	if err != nil {
		t.Fatal(err)
	}
	first.Wait()
	restarted := &Coordinator{Refreshes: repo, Sources: sourceMemory{}, Runners: first.Runners, Clock: fixedClock{time.Unix(20, 0)}, NewID: func() domain.RefreshRunID { return "unused" }}
	got, err := restarted.GetRefresh(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.RefreshCancelled || got.FinishedAt == nil {
		t.Fatalf("orphan=%+v", got)
	}
}
func (m *refreshMemory) Active(context.Context) (*domain.RefreshRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.runs {
		if v.Status == domain.RefreshRunning {
			x := v
			return &x, nil
		}
	}
	return nil, nil
}

type sourceMemory struct{ values []domain.Source }

func (s sourceMemory) List(context.Context) ([]domain.Source, error) { return s.values, nil }
func (sourceMemory) Get(context.Context, domain.SourceID) (domain.Source, error) {
	return domain.Source{}, application.ErrNotFound
}
func (sourceMemory) Save(context.Context, domain.Source) error     { return nil }
func (sourceMemory) Delete(context.Context, domain.SourceID) error { return nil }

type runnerFunc func(context.Context, domain.SourceID) (RunResult, error)

func (f runnerFunc) Run(ctx context.Context, id domain.SourceID) (RunResult, error) {
	return f(ctx, id)
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestCoordinatorMixedOutcomeAndConflict(t *testing.T) {
	repo := &refreshMemory{runs: map[domain.RefreshRunID]domain.RefreshRun{}}
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	runner := runnerFunc(func(ctx context.Context, id domain.SourceID) (RunResult, error) {
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return RunResult{}, ctx.Err()
		}
		if id == "bad" {
			return RunResult{}, errors.New("secret provider detail")
		}
		return RunResult{Fetched: 2, Writes: []application.ArticleWriteResult{{Inserted: true}, {Updated: true}}}, nil
	})
	c := &Coordinator{Refreshes: repo, Sources: sourceMemory{[]domain.Source{{ID: "good", Kind: domain.SourceKindFeed, Enabled: true}, {ID: "bad", Kind: domain.SourceKindFeed, Enabled: true}}}, Runners: map[domain.SourceKind]SourceRunner{domain.SourceKindFeed: runner}, Clock: fixedClock{time.Unix(10, 0)}, NewID: func() domain.RefreshRunID { return "run" }, MaxConcurrency: 2}
	run, err := c.StartRefresh(context.Background(), application.StartRefreshCommand{})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err = c.StartRefresh(context.Background(), application.StartRefreshCommand{}); !errors.Is(err, application.ErrConflict) {
		t.Fatalf("conflict=%v", err)
	}
	close(release)
	waitTerminal(t, repo, run.ID)
	got, _ := repo.Get(context.Background(), run.ID)
	if got.Status != domain.RefreshPartialSuccess || len(got.Outcomes) != 2 {
		t.Fatalf("run=%+v", got)
	}
	for _, outcome := range got.Outcomes {
		if outcome.SourceID == "bad" && (outcome.ErrorSummary != "Source refresh failed." || outcome.ErrorCode != "refresh_failed") {
			t.Fatalf("unsafe outcome=%+v", outcome)
		}
	}
}

func TestCoordinatorCancellationIsTerminal(t *testing.T) {
	repo := &refreshMemory{runs: map[domain.RefreshRunID]domain.RefreshRun{}}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Coordinator{Refreshes: repo, Sources: sourceMemory{[]domain.Source{{ID: "slow", Kind: domain.SourceKindFeed, Enabled: true}}}, Runners: map[domain.SourceKind]SourceRunner{domain.SourceKindFeed: runnerFunc(func(ctx context.Context, _ domain.SourceID) (RunResult, error) {
		<-ctx.Done()
		return RunResult{}, ctx.Err()
	})}, Clock: fixedClock{time.Unix(10, 0)}, NewID: func() domain.RefreshRunID { return "cancelled" }, ProcessContext: ctx}
	run, err := c.StartRefresh(context.Background(), application.StartRefreshCommand{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	waitTerminal(t, repo, run.ID)
	got, _ := repo.Get(context.Background(), run.ID)
	if got.Status != domain.RefreshCancelled || got.FinishedAt == nil {
		t.Fatalf("run=%+v", got)
	}
}

func waitTerminal(t *testing.T, repo *refreshMemory, id domain.RefreshRunID) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		run, _ := repo.Get(context.Background(), id)
		if run.Status != domain.RefreshRunning {
			return
		}
		select {
		case <-deadline:
			t.Fatal("refresh remained active")
		case <-time.After(time.Millisecond):
		}
	}
}
