package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type RankingRepository struct{ store *Store }

func (s *Store) Rankings() *RankingRepository { return &RankingRepository{store: s} }
func (r *RankingRepository) GetConfiguration(ctx context.Context) (domain.RankingConfiguration, error) {
	var c domain.RankingConfiguration
	err := r.store.q(ctx).QueryRowContext(ctx, `SELECT recency_enabled,recency_weight,interest_enabled,interest_weight,source_enabled,source_weight,behavior_enabled,behavior_weight,location_enabled,location_weight,age_enabled,age_weight,gender_enabled,gender_weight,text_similarity_enabled,text_similarity_weight,per_demographic_cap,total_demographic_cap,normalization_version FROM ranking_configurations WHERE profile_id=?`, domain.LocalProfileID).Scan(&c.Recency.Enabled, &c.Recency.Weight, &c.Interest.Enabled, &c.Interest.Weight, &c.SourcePreference.Enabled, &c.SourcePreference.Weight, &c.Behavior.Enabled, &c.Behavior.Weight, &c.Location.Enabled, &c.Location.Weight, &c.Age.Enabled, &c.Age.Weight, &c.Gender.Enabled, &c.Gender.Weight, &c.TextSimilarity.Enabled, &c.TextSimilarity.Weight, &c.PerDemographicCap, &c.TotalDemographicCap, &c.NormalizationVersion)
	if err == sql.ErrNoRows {
		return c, application.ErrNotFound
	}
	return c, mapError(err)
}
func (r *RankingRepository) SaveConfiguration(ctx context.Context, c domain.RankingConfiguration) error {
	if c.Validate() != nil {
		return application.ErrInvalidInput
	}
	_, err := r.store.q(ctx).ExecContext(ctx, `INSERT INTO ranking_configurations VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(profile_id)DO UPDATE SET recency_enabled=excluded.recency_enabled,recency_weight=excluded.recency_weight,interest_enabled=excluded.interest_enabled,interest_weight=excluded.interest_weight,source_enabled=excluded.source_enabled,source_weight=excluded.source_weight,behavior_enabled=excluded.behavior_enabled,behavior_weight=excluded.behavior_weight,location_enabled=excluded.location_enabled,location_weight=excluded.location_weight,age_enabled=excluded.age_enabled,age_weight=excluded.age_weight,gender_enabled=excluded.gender_enabled,gender_weight=excluded.gender_weight,text_similarity_enabled=excluded.text_similarity_enabled,text_similarity_weight=excluded.text_similarity_weight,per_demographic_cap=excluded.per_demographic_cap,total_demographic_cap=excluded.total_demographic_cap,normalization_version=excluded.normalization_version`, domain.LocalProfileID, c.Recency.Enabled, c.Recency.Weight, c.Interest.Enabled, c.Interest.Weight, c.SourcePreference.Enabled, c.SourcePreference.Weight, c.Behavior.Enabled, c.Behavior.Weight, c.Location.Enabled, c.Location.Weight, c.Age.Enabled, c.Age.Weight, c.Gender.Enabled, c.Gender.Weight, c.TextSimilarity.Enabled, c.TextSimilarity.Weight, c.PerDemographicCap, c.TotalDemographicCap, c.NormalizationVersion)
	return mapError(err)
}
func (r *RankingRepository) SaveResults(ctx context.Context, results []domain.RankingResult) error {
	return r.store.WithinTransaction(ctx, func(ctx context.Context) error {
		for _, v := range results {
			if v.ArticleID == "" || v.Score < 0 || v.Score > 1 || math.IsNaN(v.Score) || math.IsInf(v.Score, 0) || v.AlgorithmVersion == "" || len(v.Contributions) == 0 {
				return application.ErrInvalidInput
			}
			sum := 0.0
			for _, c := range v.Contributions {
				if c.RawScore < 0 || c.RawScore > 1 || math.IsNaN(c.RawScore) || math.IsInf(c.RawScore, 0) || c.Weight < 0 || c.Weight > 1 || math.IsNaN(c.Weight) || math.IsInf(c.Weight, 0) || c.WeightedScore < 0 || c.WeightedScore > 1 || math.IsNaN(c.WeightedScore) || math.IsInf(c.WeightedScore, 0) || math.Abs(c.RawScore*c.Weight-c.WeightedScore) > 0.000000001 || c.ReasonCode == "" {
					return application.ErrInvalidInput
				}
				sum += c.WeightedScore
			}
			if sum > 1.000000001 || math.Abs(sum-v.Score) > 0.000000001 {
				return application.ErrInvalidInput
			}
			if _, err := r.store.q(ctx).ExecContext(ctx, `INSERT INTO ranking_results(article_id,score,algorithm_version,calculated_at_ms)VALUES(?,?,?,?) ON CONFLICT(article_id)DO UPDATE SET score=excluded.score,algorithm_version=excluded.algorithm_version,calculated_at_ms=excluded.calculated_at_ms`, v.ArticleID, v.Score, v.AlgorithmVersion, millis(v.CalculatedAt)); err != nil {
				return mapError(err)
			}
			if _, err := r.store.q(ctx).ExecContext(ctx, `DELETE FROM ranking_contributions WHERE article_id=?`, v.ArticleID); err != nil {
				return mapError(err)
			}
			for i, c := range v.Contributions {
				values, err := json.Marshal(c.ReasonValues)
				if err != nil {
					return application.ErrInvalidInput
				}
				if _, err = r.store.q(ctx).ExecContext(ctx, `INSERT INTO ranking_contributions(article_id,ordinal,signal,raw_score,weight,weighted_score,reason_code,reason_values_json)VALUES(?,?,?,?,?,?,?,?)`, v.ArticleID, i, c.Signal, c.RawScore, c.Weight, c.WeightedScore, c.ReasonCode, string(values)); err != nil {
					return mapError(err)
				}
			}
		}
		return nil
	})
}
func (r *RankingRepository) GetResult(ctx context.Context, id domain.ArticleID) (domain.RankingResult, error) {
	var v domain.RankingResult
	v.ArticleID = id
	var at int64
	err := r.store.q(ctx).QueryRowContext(ctx, `SELECT score,algorithm_version,calculated_at_ms FROM ranking_results WHERE article_id=?`, id).Scan(&v.Score, &v.AlgorithmVersion, &at)
	if err == sql.ErrNoRows {
		return v, application.ErrNotFound
	}
	if err != nil {
		return v, mapError(err)
	}
	v.CalculatedAt = timeFromMillis(at)
	rows, err := r.store.q(ctx).QueryContext(ctx, `SELECT signal,raw_score,weight,weighted_score,reason_code,reason_values_json FROM ranking_contributions WHERE article_id=? ORDER BY ordinal`, id)
	if err != nil {
		return v, mapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.ScoreContribution
		var values string
		if err := rows.Scan(&c.Signal, &c.RawScore, &c.Weight, &c.WeightedScore, &c.ReasonCode, &values); err != nil {
			return v, mapError(err)
		}
		if json.Unmarshal([]byte(values), &c.ReasonValues) != nil {
			return v, application.ErrUnavailable
		}
		v.Contributions = append(v.Contributions, c)
	}
	return v, mapError(rows.Err())
}

func (r *RankingRepository) DeleteResults(ctx context.Context, ids []domain.ArticleID) error {
	for _, id := range ids {
		if id == "" {
			return application.ErrInvalidInput
		}
		if _, err := r.store.q(ctx).ExecContext(ctx, `DELETE FROM ranking_results WHERE article_id=?`, id); err != nil {
			return mapError(err)
		}
	}
	return nil
}
