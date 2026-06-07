// Package version exposes build-time identification.
//
// The Version, Commit, and Date package vars are intentionally exported and
// mutable so the linker can inject values via -ldflags at build time, e.g.:
//
//	go build -ldflags "-X github.com/NoobCoder1209/k8s-pod-doctor/internal/version.Version=v1.0.0" ./cmd/pod-doctor
//
// They are not meant to be set at runtime — use String() to read the
// composed identifier.
package version

import "fmt"

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns "Version (Commit, Date)".
func String() string {
	return fmt.Sprintf("%s (%s, %s)", Version, Commit, Date)
}
