package render

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/NoobCoder1209/k8s-pod-doctor/internal/doctor"
)

func TestRenderJSON_StableSchema(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web", UID: "uid-1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "web",
				Ready:        false,
				RestartCount: 7,
				State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
			}},
		},
	}
	snap := &doctor.Snapshot{Pod: pod, FetchedAt: time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)}
	verdict := &doctor.Finding{Code: "CrashLoopBackOff", Severity: doctor.SeverityCritical, Title: "x", Message: "y"}

	rep := BuildReport(snap, verdict, []doctor.Finding{*verdict}, false, "0.0.0",
		time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC))

	var buf bytes.Buffer
	if err := RenderJSON(&buf, rep); err != nil {
		t.Fatal(err)
	}

	// Decode and verify the schema fields are populated as documented.
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"schemaVersion", "tool", "toolVersion", "generatedAt", "pod", "summary", "findings", "verdict", "healthy"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing key %q in JSON output: %s", key, buf.String())
		}
	}
	if got["schemaVersion"].(string) != "1" {
		t.Fatalf("schemaVersion drift: %v", got["schemaVersion"])
	}
	if got["healthy"].(bool) {
		t.Fatalf("expected healthy=false")
	}
}
