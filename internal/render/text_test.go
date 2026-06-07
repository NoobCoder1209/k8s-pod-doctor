package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/NoobCoder1209/k8s-pod-doctor/internal/doctor"
)

// makeCanonicalCrashLoopSnapshot is the fixture used by render tests. Time
// fields are deterministic so test output is stable.
func makeCanonicalCrashLoopSnapshot() *doctor.Snapshot {
	startTime := metav1.NewTime(time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web-7d9f-abc", UID: "uid-1"},
		Spec: corev1.PodSpec{
			NodeName:   "kind-worker",
			Containers: []corev1.Container{{Name: "web"}},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			StartTime: &startTime,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
				{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "web",
				Ready:        false,
				RestartCount: 7,
				State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
					ExitCode: 1,
					Reason:   "Error",
				}},
			}},
		},
	}
	return &doctor.Snapshot{
		Pod: pod,
		Events: []corev1.Event{{
			ObjectMeta:    metav1.ObjectMeta{Name: "ev-1"},
			Reason:        "BackOff",
			Type:          "Warning",
			Message:       "Back-off restarting failed container",
			LastTimestamp: metav1.NewTime(time.Date(2026, 6, 7, 9, 30, 0, 0, time.UTC)),
		}},
		Logs:      map[string]string{"web": "hello\nworld\n"},
		FetchedAt: time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC),
	}
}

func TestRenderText_AllSectionsPresent(t *testing.T) {
	snap := makeCanonicalCrashLoopSnapshot()
	verdict := &doctor.Finding{
		Code:        "CrashLoopBackOff",
		Severity:    doctor.SeverityCritical,
		Title:       "Container web crash-looping",
		Message:     "Container web crashed 7 times. Last exit: 1 (Error).",
		Container:   "web",
		Suggestions: []string{"kubectl logs web-7d9f-abc -n default -c web --previous"},
	}

	var buf bytes.Buffer
	RenderText(&buf, snap, verdict, []doctor.Finding{*verdict}, false, true /*noColor*/)
	out := buf.String()

	// Check the structural sections are all present.
	for _, want := range []string{
		"CRITICAL", "Container web crash-looping",
		"== Status ==",
		"Pod:", "default/web-7d9f-abc",
		"== Recent events ==",
		"BackOff",
		"Back-off restarting failed container",
		"== Logs ==",
		"hello", "world",
		"== Findings ==",
		"kubectl logs web-7d9f-abc",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in rendered text:\n%s", want, out)
		}
	}

	// Section order is part of the contract: Status before Events before
	// Logs before Findings. Catches accidental reordering.
	statusIdx := strings.Index(out, "== Status ==")
	eventsIdx := strings.Index(out, "== Recent events ==")
	logsIdx := strings.Index(out, "== Logs ==")
	findingsIdx := strings.Index(out, "== Findings ==")
	if !(statusIdx >= 0 && eventsIdx > statusIdx && logsIdx > eventsIdx && findingsIdx > logsIdx) {
		t.Fatalf("section order wrong: status=%d events=%d logs=%d findings=%d",
			statusIdx, eventsIdx, logsIdx, findingsIdx)
	}
}

func TestRenderText_HealthyPodShowsHealthyBanner(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "ok"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "web", Ready: true,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}
	snap := &doctor.Snapshot{Pod: pod, FetchedAt: time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)}

	var buf bytes.Buffer
	RenderText(&buf, snap, nil, nil, true /*healthy*/, true /*noColor*/)
	out := buf.String()

	if !strings.Contains(out, "HEALTHY") {
		t.Fatalf("missing HEALTHY banner: %s", out)
	}
	if strings.Contains(out, "== Findings ==") {
		t.Fatalf("healthy pod should have no Findings section: %s", out)
	}
}

func TestRenderText_NilSnapshotNoCrash(t *testing.T) {
	var buf bytes.Buffer
	RenderText(&buf, nil, nil, nil, false, true)
	if !strings.Contains(buf.String(), "no pod data") {
		t.Fatalf("want graceful nil handling, got %q", buf.String())
	}
}
