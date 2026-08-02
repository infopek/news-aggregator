package application

import "errors"

var (
	ErrNotFound            = errors.New("not found")
	ErrConflict            = errors.New("conflict")
	ErrInvalidInput        = errors.New("invalid input")
	ErrUnsupportedPlatform = errors.New("unsupported platform")
	ErrUnavailable         = errors.New("unavailable")
	ErrCredentialMissing   = errors.New("credential missing")
)
