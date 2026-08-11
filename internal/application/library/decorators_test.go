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

type profileServiceStub struct {
	profile       domain.UserProfile
	configuration domain.RankingConfiguration
}

func (s *profileServiceStub) GetProfile(context.Context) (domain.UserProfile, error) {
	return s.profile, nil
}

func (s *profileServiceStub) UpdateProfile(context.Context, application.UpdateProfileCommand) (domain.UserProfile, error) {
	s.profile.UpdatedAt = time.Now().UTC()
	return s.profile, nil
}

func (s *profileServiceStub) GetRankingConfiguration(context.Context) (domain.RankingConfiguration, error) {
	return s.configuration, nil
}

func (s *profileServiceStub) UpdateRankingConfiguration(context.Context, application.UpdateRankingConfigurationCommand) (domain.RankingConfiguration, error) {
	s.configuration.NormalizationVersion = "updated"
	return s.configuration, nil
}

type failingFull struct{ failures int }

func (f *failingFull) Full(context.Context) error {
	if f.failures > 0 {
		f.failures--
		return application.ErrUnavailable
	}
	return nil
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

func TestConfigurationDoesNotTurnRecomputeFailureIntoMutationFailure(t *testing.T) {
	base := &profileServiceStub{profile: domain.UserProfile{ID: domain.LocalProfileID}}
	recompute := &failingFull{failures: 2}
	status := &RecomputeStatus{}
	service := Configuration{Base: base, Recompute: recompute, Gate: &ranking.VersionGate{}, Status: status}

	profile, err := service.UpdateProfile(context.Background(), application.UpdateProfileCommand{})
	if err != nil || profile.UpdatedAt.IsZero() || !status.Failed() {
		t.Fatalf("profile=%+v error=%v failed=%v", profile, err, status.Failed())
	}
	configuration, err := service.UpdateRankingConfiguration(context.Background(), application.UpdateRankingConfigurationCommand{})
	if err != nil || configuration.NormalizationVersion != "updated" || !status.Failed() {
		t.Fatalf("configuration=%+v error=%v failed=%v", configuration, err, status.Failed())
	}
	if err := status.Retry(context.Background(), recompute); err != nil || status.Failed() {
		t.Fatalf("retry error=%v failed=%v", err, status.Failed())
	}
}
