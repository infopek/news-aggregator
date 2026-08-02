package sqlite

import (
	"context"
	"database/sql"

	"github.com/infopek/news-aggregator/internal/application"
	"github.com/infopek/news-aggregator/internal/domain"
)

type ProfileRepository struct{ store *Store }

func (s *Store) Profiles() *ProfileRepository { return &ProfileRepository{store: s} }
func (r *ProfileRepository) Save(ctx context.Context, profile domain.UserProfile) error {
	return r.store.saveProfile(ctx, profile)
}
func (r *ProfileRepository) Get(ctx context.Context, id domain.ProfileID) (domain.UserProfile, error) {
	return r.store.getProfile(ctx, id)
}

func (s *Store) saveProfile(ctx context.Context, profile domain.UserProfile) error {
	if err := profile.Validate(); err != nil {
		return application.ErrInvalidInput
	}
	return s.WithinTransaction(ctx, func(ctx context.Context) error {
		q := s.q(ctx)
		var country, region, city, gender any
		var age any
		if profile.Location.Present {
			country = profile.Location.Value.Country
			region = profile.Location.Value.Region
			if profile.Location.Value.City.Present {
				city = profile.Location.Value.City.Value
			}
		}
		if profile.Age.Present {
			age = profile.Age.Value
		}
		if profile.Gender.Present {
			gender = profile.Gender.Value
		}
		_, err := q.ExecContext(ctx, `INSERT INTO profiles(id,location_present,location_enabled,country,region,city_present,city_enabled,city,age_present,age_enabled,age,gender_present,gender_enabled,gender,updated_at_ms)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET location_present=excluded.location_present,location_enabled=excluded.location_enabled,country=excluded.country,region=excluded.region,city_present=excluded.city_present,city_enabled=excluded.city_enabled,city=excluded.city,age_present=excluded.age_present,age_enabled=excluded.age_enabled,age=excluded.age,gender_present=excluded.gender_present,gender_enabled=excluded.gender_enabled,gender=excluded.gender,updated_at_ms=excluded.updated_at_ms`, profile.ID, profile.Location.Present, profile.Location.Enabled, country, region, profile.Location.Present && profile.Location.Value.City.Present, profile.Location.Present && profile.Location.Value.City.Enabled, city, profile.Age.Present, profile.Age.Enabled, age, profile.Gender.Present, profile.Gender.Enabled, gender, millis(profile.UpdatedAt))
		if err != nil {
			return mapError(err)
		}
		if _, err = q.ExecContext(ctx, `DELETE FROM profile_interests WHERE profile_id=?`, profile.ID); err != nil {
			return mapError(err)
		}
		if _, err = q.ExecContext(ctx, `DELETE FROM profile_preferred_sources WHERE profile_id=?`, profile.ID); err != nil {
			return mapError(err)
		}
		for _, interest := range profile.Interests {
			if _, err = q.ExecContext(ctx, `INSERT INTO profile_interests(profile_id,name,weight) VALUES(?,?,?)`, profile.ID, interest.Name, interest.Weight); err != nil {
				return mapError(err)
			}
		}
		for _, sourceID := range profile.PreferredSources {
			if _, err = q.ExecContext(ctx, `INSERT INTO profile_preferred_sources(profile_id,source_id) VALUES(?,?)`, profile.ID, sourceID); err != nil {
				return mapError(err)
			}
		}
		return nil
	})
}

func (s *Store) getProfile(ctx context.Context, id domain.ProfileID) (domain.UserProfile, error) {
	var p domain.UserProfile
	var lp, le, cp, ce, ap, ae, gp, ge bool
	var country, region, city, gender sql.NullString
	var age sql.NullInt64
	var updated int64
	err := s.q(ctx).QueryRowContext(ctx, `SELECT id,location_present,location_enabled,country,region,city_present,city_enabled,city,age_present,age_enabled,age,gender_present,gender_enabled,gender,updated_at_ms FROM profiles WHERE id=?`, id).Scan(&p.ID, &lp, &le, &country, &region, &cp, &ce, &city, &ap, &ae, &age, &gp, &ge, &gender, &updated)
	if err == sql.ErrNoRows {
		return p, application.ErrNotFound
	}
	if err != nil {
		return p, mapError(err)
	}
	p.Location = domain.OptionalSignal[domain.Location]{Present: lp, Enabled: le}
	if lp {
		p.Location.Value = domain.Location{Country: country.String, Region: region.String, City: domain.OptionalSignal[string]{Value: city.String, Present: cp, Enabled: ce}}
	}
	p.Age = domain.OptionalSignal[int]{Value: int(age.Int64), Present: ap, Enabled: ae}
	p.Gender = domain.OptionalSignal[string]{Value: gender.String, Present: gp, Enabled: ge}
	p.UpdatedAt = timeFromMillis(updated)
	rows, err := s.q(ctx).QueryContext(ctx, `SELECT name,weight FROM profile_interests WHERE profile_id=? ORDER BY name`, id)
	if err != nil {
		return p, mapError(err)
	}
	for rows.Next() {
		var v domain.WeightedInterest
		if err := rows.Scan(&v.Name, &v.Weight); err != nil {
			rows.Close()
			return p, mapError(err)
		}
		p.Interests = append(p.Interests, v)
	}
	if err := rows.Close(); err != nil {
		return p, mapError(err)
	}
	rows, err = s.q(ctx).QueryContext(ctx, `SELECT source_id FROM profile_preferred_sources WHERE profile_id=? ORDER BY source_id`, id)
	if err != nil {
		return p, mapError(err)
	}
	for rows.Next() {
		var v domain.SourceID
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return p, mapError(err)
		}
		p.PreferredSources = append(p.PreferredSources, v)
	}
	if err := rows.Close(); err != nil {
		return p, mapError(err)
	}
	return p, nil
}
