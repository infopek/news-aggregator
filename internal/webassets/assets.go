// Package webassets declares the build boundary between the Vue application
// and the Go executable. The frontend build writes to this package's dist
// directory so release binaries can embed the result without path traversal.
package webassets

import "embed"

// Files contains the compiled frontend. A checked-in placeholder keeps clean
// checkout Go builds valid before the first frontend build.
//
//go:embed all:dist
var Files embed.FS
