package render

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/NoobCoder1209/k8s-pod-doctor/internal/doctor"
)

// SchemaVersion of the JSON output. Bump only on breaking change.
const SchemaVersion = "1"

// BuildReport composes a stable, JSON-serialisable Report from a Snapshot.
func BuildReport(snap *doctor.Snapshot, verdict *doctor.Finding, findings []doctor.Finding, healthy bool, toolVersion string, generatedAt time.Time) doctor.Report {
	pod := snap.Pod
	r := doctor.Report{
		SchemaVersion: SchemaVersion,
		Tool:          "k8s-pod-doctor",
		ToolVersion:   toolVersion,
		GeneratedAt:   generatedAt.UTC(),
		Pod: doctor.PodRef{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			UID:       string(pod.UID),
			Phase:     string(pod.Status.Phase),
		},
		Summary:  summarise(snap),
		Findings: findings,
		Verdict:  verdict,
		Healthy:  healthy,
	}
	return r
}

// RenderJSON writes the report as indented JSON.
func RenderJSON(w io.Writer, r doctor.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func summarise(snap *doctor.Snapshot) doctor.SnapshotSummary {
	pod := snap.Pod
	conds := map[string]string{}
	for _, c := range pod.Status.Conditions {
		conds[string(c.Type)] = string(c.Status)
	}
	var startTime *time.Time
	if pod.Status.StartTime != nil {
		t := pod.Status.StartTime.Time
		startTime = &t
	}
	return doctor.SnapshotSummary{
		Phase:             string(pod.Status.Phase),
		NodeName:          pod.Spec.NodeName,
		StartTime:         startTime,
		Conditions:        conds,
		ContainerStatuses: briefStatuses(pod.Status.ContainerStatuses),
		InitStatuses:      briefStatuses(pod.Status.InitContainerStatuses),
		EventCount:        len(snap.Events),
	}
}

func briefStatuses(ss []corev1.ContainerStatus) []doctor.ContainerBrief {
	out := make([]doctor.ContainerBrief, 0, len(ss))
	for _, cs := range ss {
		b := doctor.ContainerBrief{
			Name:         cs.Name,
			Ready:        cs.Ready,
			RestartCount: cs.RestartCount,
		}
		switch {
		case cs.State.Running != nil:
			b.State = "running"
		case cs.State.Waiting != nil:
			b.State = "waiting"
			b.Reason = cs.State.Waiting.Reason
		case cs.State.Terminated != nil:
			b.State = "terminated"
			b.Reason = cs.State.Terminated.Reason
			ec := cs.State.Terminated.ExitCode
			b.ExitCode = &ec
		}
		out = append(out, b)
	}
	return out
}
