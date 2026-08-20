package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/application/ingestion"
	"github.com/infopek/news-aggregator/internal/domain"
)

// These tests pin the two scheduling windows exercised by the native Windows
// shutdown smoke: cancellation before source enumeration and cancellation
// after source work has definitely entered a runner.
func TestRefreshShutdownSchedulingWindows(t *testing.T) {
	t.Run("before source enumeration", func(t *testing.T) {
		process, cancel := context.WithCancel(context.Background())
		repo := newShutdownRefreshRepo()
		coordinator := shutdownCoordinator(repo, shutdownSources{list: func(ctx context.Context) ([]domain.Source, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}, process, func(ctx context.Context, _ domain.SourceID) (ingestion.RunResult, error) {
			return ingestion.RunResult{}, ctx.Err()
		})
		run, err := coordinator.StartRefresh(context.Background(), application.StartRefreshCommand{})
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		coordinator.Wait()
		got, err := repo.Get(context.Background(), run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != domain.RefreshFailed || got.FinishedAt == nil || len(got.Outcomes) != 0 {
			t.Fatalf("pre-worker shutdown=%+v, want terminal failed run without invented outcomes", got)
		}
	})

	t.Run("with source work active", func(t *testing.T) {
		process, cancel := context.WithCancel(context.Background())
		repo := newShutdownRefreshRepo()
		started := make(chan struct{})
		coordinator := shutdownCoordinator(repo, shutdownSources{list: func(context.Context) ([]domain.Source, error) {
			return []domain.Source{{ID: "active", Kind: domain.SourceKindFeed, Enabled: true}}, nil
		}}, process, func(ctx context.Context, _ domain.SourceID) (ingestion.RunResult, error) {
			close(started)
			<-ctx.Done()
			return ingestion.RunResult{}, ctx.Err()
		})
		run, err := coordinator.StartRefresh(context.Background(), application.StartRefreshCommand{})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("source runner did not start")
		}
		cancel()
		if err := coordinator.Finalize(context.Background()); err != nil {
			t.Fatal(err)
		}
		got, err := repo.Get(context.Background(), run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != domain.RefreshCancelled || got.FinishedAt == nil || len(got.Outcomes) != 1 || got.Outcomes[0].ErrorCode != "cancelled" {
			t.Fatalf("active shutdown=%+v, want durable cancelled outcome", got)
		}
	})
}

type shutdownSources struct {
	list func(context.Context) ([]domain.Source, error)
}

func (s shutdownSources) List(ctx context.Context) ([]domain.Source, error) { return s.list(ctx) }
func (shutdownSources) Get(context.Context, domain.SourceID) (domain.Source, error) {
	return domain.Source{}, application.ErrNotFound
}
func (shutdownSources) Save(context.Context, domain.Source) error     { return nil }
func (shutdownSources) Delete(context.Context, domain.SourceID) error { return nil }

type shutdownRunner func(context.Context, domain.SourceID) (ingestion.RunResult, error)

func (r shutdownRunner) Run(ctx context.Context, id domain.SourceID) (ingestion.RunResult, error) {
	return r(ctx, id)
}

type shutdownClock struct{ now time.Time }

func (c shutdownClock) Now() time.Time { return c.now }

func shutdownCoordinator(repo *shutdownRefreshRepo, sources shutdownSources, process context.Context, runner shutdownRunner) *ingestion.Coordinator {
	return &ingestion.Coordinator{
		Refreshes: repo, Sources: sources, ProcessContext: process,
		Runners: map[domain.SourceKind]ingestion.SourceRunner{domain.SourceKindFeed: runner},
		Clock:   shutdownClock{now: time.Unix(100, 0)}, NewID: func() domain.RefreshRunID { return "shutdown-window" },
	}
}

type shutdownRefreshRepo struct {
	mu   sync.Mutex
	runs map[domain.RefreshRunID]domain.RefreshRun
}

func newShutdownRefreshRepo() *shutdownRefreshRepo {
	return &shutdownRefreshRepo{runs: make(map[domain.RefreshRunID]domain.RefreshRun)}
}
func (r *shutdownRefreshRepo) Create(_ context.Context, run domain.RefreshRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = run
	return nil
}
func (r *shutdownRefreshRepo) Get(_ context.Context, id domain.RefreshRunID) (domain.RefreshRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	if !ok {
		return domain.RefreshRun{}, application.ErrNotFound
	}
	return run, nil
}
func (r *shutdownRefreshRepo) Save(_ context.Context, run domain.RefreshRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = run
	return nil
}
func (r *shutdownRefreshRepo) Active(context.Context) (*domain.RefreshRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range r.runs {
		if run.Status == domain.RefreshRunning {
			copy := run
			return &copy, nil
		}
	}
	return nil, nil
}
