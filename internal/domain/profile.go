package domain

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidProfile = errors.New("invalid user profile")

// OptionalSignal keeps absence separate from whether a supplied value is
// enabled as a ranking input. A disabled value may remain stored for later use.
type OptionalSignal[T any] struct {
	Value   T
	Present bool
	Enabled bool
}

func (signal OptionalSignal[T]) Valid() bool {
	return signal.Present || !signal.Enabled
}

type Location struct {
	Country string
	Region  string
	City    OptionalSignal[string]
}

type WeightedInterest struct {
	Name   string
	Weight float64
}

type UserProfile struct {
	ID               ProfileID
	Interests        []WeightedInterest
	PreferredSources []SourceID
	Location         Location
	Age              OptionalSignal[int]
	Gender           OptionalSignal[string]
	UpdatedAt        time.Time
}

func (profile UserProfile) Validate() error {
	if profile.ID == "" || strings.TrimSpace(profile.Location.Country) == "" || strings.TrimSpace(profile.Location.Region) == "" {
		return ErrInvalidProfile
	}
	if !profile.Age.Valid() || !profile.Gender.Valid() || !profile.Location.City.Valid() {
		return ErrInvalidProfile
	}
	if profile.Age.Present && (profile.Age.Value < 0 || profile.Age.Value > 130) {
		return ErrInvalidProfile
	}
	for _, interest := range profile.Interests {
		if strings.TrimSpace(interest.Name) == "" || !validUnitWeight(interest.Weight) {
			return ErrInvalidProfile
		}
	}
	return nil
}
