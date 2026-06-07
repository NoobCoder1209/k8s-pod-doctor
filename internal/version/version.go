// Package version exposes build-time identification.
//
// Values are injected via -ldflags at build time:
//
//	-X github.com/NoobCoder1209/k8s-pod-doctor/internal/version.Version=v1.0.0
package version

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
