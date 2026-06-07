package render

import (
	"io"
	"time"

	"github.com/NoobCoder1209/k8s-pod-doctor/internal/doctor"
)

// Adapter implements doctor.Renderer using the package-level RenderText and
// the JSON serialiser. This indirection avoids a doctor → render import cycle.
type Adapter struct{}

// RenderText satisfies doctor.Renderer.
func (Adapter) RenderText(w io.Writer, snap *doctor.Snapshot, verdict *doctor.Finding, findings []doctor.Finding, healthy, noColor bool) {
	RenderText(w, snap, verdict, findings, healthy, noColor)
}

// RenderJSON satisfies doctor.Renderer.
func (Adapter) RenderJSON(w io.Writer, snap *doctor.Snapshot, verdict *doctor.Finding, findings []doctor.Finding, healthy bool, toolVersion string, generatedAt time.Time) error {
	return RenderJSON(w, BuildReport(snap, verdict, findings, healthy, toolVersion, generatedAt))
}
