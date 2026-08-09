package application

import (
	"errors"
	"time"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrConflict            = errors.New("conflict")
	ErrInvalidInput        = errors.New("invalid input")
	ErrUnsupportedPlatform = errors.New("unsupported platform")
	ErrUnavailable         = errors.New("unavailable")
	ErrCredentialMissing   = errors.New("credential missing")
)

type RateLimitError struct {
	Retryable  bool
	RetryAfter time.Duration
}

func (*RateLimitError) Error() string { return "rate limited" }
