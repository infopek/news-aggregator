package credentials

import (
	"errors"

	"github.com/infopek/news-aggregator/internal/application"
)

var (
	ErrMissing      = errors.New("credential missing")
	ErrAccessDenied = errors.New("credential access denied")
	ErrInterrupted  = errors.New("credential operation interrupted")
)

func safeError(operation string, category error) error {
	return &storeError{operation: operation, category: category}
}

type storeError struct {
	operation string
	category  error
}

func (err *storeError) Error() string {
	return "credential " + err.operation + ": " + err.category.Error()
}
func (err *storeError) Unwrap() error { return err.category }

func unsupported(operation string) error {
	return safeError(operation, application.ErrUnsupportedPlatform)
}
