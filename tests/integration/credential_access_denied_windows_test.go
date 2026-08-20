//go:build windows

package integration_test

import (
	"errors"
	"strings"
	"syscall"
	"testing"
	_ "unsafe"

	"github.com/infopek/news-aggregator/internal/adapters/credentials"
)

// mapNativeCredentialError links to the Windows adapter's native error
// boundary so CI can deterministically inject ERROR_ACCESS_DENIED without
// changing machine-wide Credential Manager policy.
//
//go:linkname mapNativeCredentialError github.com/infopek/news-aggregator/internal/adapters/credentials.mapNativeError
func mapNativeCredentialError(string, error) error

func TestWindowsCredentialAccessDeniedIsSafe(t *testing.T) {
	const sentinel = "VERIFY003_ACCESS_DENIED_SECRET"
	err := mapNativeCredentialError("resolve", syscall.ERROR_ACCESS_DENIED)
	if !errors.Is(err, credentials.ErrAccessDenied) {
		t.Fatalf("access-denied category = %v", err)
	}
	message := err.Error()
	for _, forbidden := range []string{sentinel, "ERROR_ACCESS_DENIED", "Access is denied", "source-"} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("access-denied error disclosed native or credential detail: %q", message)
		}
	}
}
