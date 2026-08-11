package library

import (
	"context"
	"testing"
	"time"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/application/ingestion"
	"github.com/infopek/news-aggregator/internal/application/ranking"
	"github.com/infopek/news-aggregator/internal/domain"
)

type runnerStub struct{}

func (runnerStub) Run(context.Context, domain.SourceID) (ingestion.RunResult, error) {
	return ingestion.RunResult{Fetched: 3, Writes: []application.ArticleWriteResult{{Inserted: true}, {Updated: true}}}, nil
}

type blockingFull struct{ started, release chan struct{} }

func (b blockingFull) Full(context.Context) error {
	close(b.started)
	<-b.release
	return application.ErrUnavailable
}

func TestRunnerDoesNotTurnRecomputeFailureIntoIngestionFailure(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	status := &RecomputeStatus{}
	runner := Runner{Base: runnerStub{}, Recompute: blockingFull{started, release}, Gate: &ranking.VersionGate{}, Status: status}
	result, err := runner.Run(context.Background(), "source")
	if err != nil || result.Fetched != 3 || len(result.Writes) != 2 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	<-started
	close(release)
	deadline := time.Now().Add(time.Second)
	for !status.Failed() {
		if time.Now().After(deadline) {
			t.Fatal("ranking failure was not recorded")
		}
		time.Sleep(time.Millisecond)
	}
}
