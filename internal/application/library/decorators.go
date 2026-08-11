package library

import (
	"context"
	"sync"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/application/ingestion"
	"github.com/infopek/news-aggregator/internal/domain"
)

type FullRecomputer interface{ Full(context.Context) error }

type Configuration struct {
	Base      application.ProfileService
	Recompute FullRecomputer
	Gate      interface{ BeginMutation() func() }
}

func (c Configuration) GetProfile(ctx context.Context) (domain.UserProfile, error) {
	return c.Base.GetProfile(ctx)
}
func (c Configuration) GetRankingConfiguration(ctx context.Context) (domain.RankingConfiguration, error) {
	return c.Base.GetRankingConfiguration(ctx)
}
func (c Configuration) UpdateProfile(ctx context.Context, cmd application.UpdateProfileCommand) (domain.UserProfile, error) {
	done := c.Gate.BeginMutation()
	v, e := c.Base.UpdateProfile(ctx, cmd)
	done()
	if e == nil {
		e = c.Recompute.Full(ctx)
	}
	return v, e
}
func (c Configuration) UpdateRankingConfiguration(ctx context.Context, cmd application.UpdateRankingConfigurationCommand) (domain.RankingConfiguration, error) {
	done := c.Gate.BeginMutation()
	v, e := c.Base.UpdateRankingConfiguration(ctx, cmd)
	done()
	if e == nil {
		e = c.Recompute.Full(ctx)
	}
	return v, e
}

type Runner struct {
	Base      ingestion.SourceRunner
	Recompute FullRecomputer
	Gate      interface{ BeginMutation() func() }
	Status    *RecomputeStatus
}

func (r Runner) Run(ctx context.Context, id domain.SourceID) (ingestion.RunResult, error) {
	done := r.Gate.BeginMutation()
	v, e := r.Base.Run(ctx, id)
	done()
	if e == nil {
		go func() {
			err := r.Recompute.Full(context.WithoutCancel(ctx))
			if r.Status != nil {
				r.Status.record(err)
			}
		}()
	}
	return v, e
}

type RecomputeStatus struct {
	mu     sync.Mutex
	failed bool
}

func (s *RecomputeStatus) record(err error) { s.mu.Lock(); s.failed = err != nil; s.mu.Unlock() }
func (s *RecomputeStatus) Failed() bool     { s.mu.Lock(); defer s.mu.Unlock(); return s.failed }
