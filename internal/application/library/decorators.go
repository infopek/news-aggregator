package library

import (
	"context"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/application/ingestion"
	"github.com/infopek/news-aggregator/internal/domain"
)

type FullRecomputer interface{ Full(context.Context) error }

type Configuration struct {
	Base      application.ProfileService
	Recompute FullRecomputer
}

func (c Configuration) GetProfile(ctx context.Context) (domain.UserProfile, error) {
	return c.Base.GetProfile(ctx)
}
func (c Configuration) GetRankingConfiguration(ctx context.Context) (domain.RankingConfiguration, error) {
	return c.Base.GetRankingConfiguration(ctx)
}
func (c Configuration) UpdateProfile(ctx context.Context, cmd application.UpdateProfileCommand) (domain.UserProfile, error) {
	v, e := c.Base.UpdateProfile(ctx, cmd)
	if e == nil {
		e = c.Recompute.Full(ctx)
	}
	return v, e
}
func (c Configuration) UpdateRankingConfiguration(ctx context.Context, cmd application.UpdateRankingConfigurationCommand) (domain.RankingConfiguration, error) {
	v, e := c.Base.UpdateRankingConfiguration(ctx, cmd)
	if e == nil {
		e = c.Recompute.Full(ctx)
	}
	return v, e
}

type Runner struct {
	Base      ingestion.SourceRunner
	Recompute FullRecomputer
}

func (r Runner) Run(ctx context.Context, id domain.SourceID) (ingestion.RunResult, error) {
	v, e := r.Base.Run(ctx, id)
	if e == nil {
		e = r.Recompute.Full(ctx)
	}
	return v, e
}
