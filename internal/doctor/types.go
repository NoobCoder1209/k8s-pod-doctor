package doctor

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Severity classifies a Finding.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
	SeverityHealthy  Severity = "healthy"
)

func severityRank(s Severity) int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	}
	return 0
}

// Snapshot is everything pod-doctor collected about a single pod.
type Snapshot struct {
	Pod       *corev1.Pod       `json:"pod,omitempty"`
	Events    []corev1.Event    `json:"events,omitempty"`
	Logs      map[string]string `json:"logs,omitempty"`      // container -> tail body
	LogErrors map[string]string `json:"logErrors,omitempty"` // container -> reason we couldn't fetch
	FetchedAt time.Time         `json:"fetchedAt"`
}

// Finding is one diagnosis emitted by a Rule.
type Finding struct {
	Code        string   `json:"code"`
	Severity    Severity `json:"severity"`
	Title       string   `json:"title"`
	Message     string   `json:"message"`
	Container   string   `json:"container,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
	Priority    int      `json:"-"` // 1 = highest; for verdict picking only
}

// Rule applies one detection signature to a Snapshot.
type Rule func(s *Snapshot) []Finding

// PodRef is a stable, minimal pod reference for JSON output.
type PodRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
	Phase     string `json:"phase,omitempty"`
}

// ContainerBrief is the per-container summary surfaced in JSON.
type ContainerBrief struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restartCount"`
	State        string `json:"state,omitempty"`
	Reason       string `json:"reason,omitempty"`
	ExitCode     *int32 `json:"exitCode,omitempty"`
}

// SnapshotSummary is the JSON-friendly summary of a Snapshot.
type SnapshotSummary struct {
	Phase             string            `json:"phase,omitempty"`
	NodeName          string            `json:"nodeName,omitempty"`
	StartTime         *time.Time        `json:"startTime,omitempty"`
	Conditions        map[string]string `json:"conditions,omitempty"`
	ContainerStatuses []ContainerBrief  `json:"containerStatuses,omitempty"`
	InitStatuses      []ContainerBrief  `json:"initContainerStatuses,omitempty"`
	EventCount        int               `json:"eventCount"`
	LogErrors         map[string]string `json:"logErrors,omitempty"`
}

// Report is the top-level JSON output. SchemaVersion bumps only on breaking change.
type Report struct {
	SchemaVersion string          `json:"schemaVersion"`
	Tool          string          `json:"tool"`
	ToolVersion   string          `json:"toolVersion"`
	GeneratedAt   time.Time       `json:"generatedAt"`
	Pod           PodRef          `json:"pod"`
	Summary       SnapshotSummary `json:"summary"`
	Findings      []Finding       `json:"findings"`
	Verdict       *Finding        `json:"verdict,omitempty"`
	Healthy       bool            `json:"healthy"`
}
