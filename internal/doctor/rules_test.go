package doctor

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// helpers used across rule tests.

func mkPod(ns, name string, opts ...func(*corev1.Pod)) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID("uid-" + name)},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

func withPhase(phase corev1.PodPhase) func(*corev1.Pod) {
	return func(p *corev1.Pod) { p.Status.Phase = phase }
}

func withCondition(t corev1.PodConditionType, st corev1.ConditionStatus, reason, msg string) func(*corev1.Pod) {
	return func(p *corev1.Pod) {
		p.Status.Conditions = append(p.Status.Conditions, corev1.PodCondition{
			Type: t, Status: st, Reason: reason, Message: msg,
		})
	}
}

func withContainerStatus(cs corev1.ContainerStatus) func(*corev1.Pod) {
	return func(p *corev1.Pod) {
		p.Status.ContainerStatuses = append(p.Status.ContainerStatuses, cs)
	}
}

func mkSnap(p *corev1.Pod, evs ...corev1.Event) *Snapshot {
	return &Snapshot{Pod: p, Events: evs, FetchedAt: time.Now()}
}

func mkEvent(reason, message string) corev1.Event {
	return corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev-" + reason},
		Reason:     reason,
		Message:    message,
	}
}

func TestPendingSchedulingRule(t *testing.T) {
	tests := []struct {
		name string
		snap *Snapshot
		want bool
	}{
		{
			name: "scheduler said Unschedulable",
			snap: mkSnap(mkPod("default", "web",
				withPhase(corev1.PodPending),
				withCondition(corev1.PodScheduled, corev1.ConditionFalse, "Unschedulable", "0/3 nodes are available: 3 Insufficient memory."),
			)),
			want: true,
		},
		{
			name: "running pod — not pending",
			snap: mkSnap(mkPod("default", "web",
				withPhase(corev1.PodRunning),
				withCondition(corev1.PodScheduled, corev1.ConditionTrue, "", ""),
			)),
			want: false,
		},
		{
			name: "pending but PodScheduled true",
			snap: mkSnap(mkPod("default", "web",
				withPhase(corev1.PodPending),
				withCondition(corev1.PodScheduled, corev1.ConditionTrue, "", ""),
			)),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pendingSchedulingRule(tt.snap)
			if (len(got) > 0) != tt.want {
				t.Fatalf("want hit=%v, got %d findings", tt.want, len(got))
			}
			if tt.want && got[0].Code != "PendingScheduling" {
				t.Fatalf("want code PendingScheduling, got %s", got[0].Code)
			}
		})
	}
}

func TestOOMKilledRule_LastTermination(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:         "app",
		RestartCount: 3,
		State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			Reason:   "OOMKilled",
			ExitCode: 137,
			Signal:   9,
		}},
	}
	snap := mkSnap(mkPod("default", "web",
		withPhase(corev1.PodRunning),
		withContainerStatus(cs),
	))
	got := oomKilledRule(snap)
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d", len(got))
	}
	if got[0].Code != "OOMKilled" {
		t.Fatalf("want code OOMKilled, got %s", got[0].Code)
	}
	if got[0].Container != "app" {
		t.Fatalf("want container app, got %s", got[0].Container)
	}
}

func TestOOMKilledRule_Negative(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:  "app",
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}
	snap := mkSnap(mkPod("default", "web",
		withPhase(corev1.PodRunning),
		withContainerStatus(cs),
	))
	if got := oomKilledRule(snap); len(got) != 0 {
		t.Fatalf("want 0 findings, got %d", len(got))
	}
}

func TestImagePullBackOffRule(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:  "web",
		Image: "nginx:notreal",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
			Reason:  "ImagePullBackOff",
			Message: "Back-off pulling image \"nginx:notreal\"",
		}},
	}
	snap := mkSnap(mkPod("default", "web", withPhase(corev1.PodPending), withContainerStatus(cs)))
	got := imagePullBackOffRule(snap)
	if len(got) != 1 || got[0].Code != "ImagePullBackOff" {
		t.Fatalf("want 1 ImagePullBackOff, got %+v", got)
	}
}

func TestCrashLoopBackOffRule(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:         "app",
		RestartCount: 7,
		State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			ExitCode: 1, Reason: "Error", Message: "panic: something",
		}},
	}
	snap := mkSnap(mkPod("default", "web", withPhase(corev1.PodRunning), withContainerStatus(cs)))
	got := crashLoopBackOffRule(snap)
	if len(got) != 1 || got[0].Code != "CrashLoopBackOff" {
		t.Fatalf("want 1 CrashLoopBackOff, got %+v", got)
	}
}

func TestProbeFailureRule(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:         "web",
		RestartCount: 4,
		State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}
	ev := mkEvent("Unhealthy", "Liveness probe failed: HTTP probe failed with statuscode: 500")
	ev.InvolvedObject.FieldPath = "spec.containers{web}"
	snap := mkSnap(mkPod("default", "web", withPhase(corev1.PodRunning), withContainerStatus(cs)), ev)
	got := probeFailureRule(snap)
	if len(got) != 1 || got[0].Code != "ProbeFailure" {
		t.Fatalf("want 1 ProbeFailure, got %+v", got)
	}
	if got[0].Severity != SeverityCritical {
		t.Fatalf("liveness probe should be critical, got %s", got[0].Severity)
	}
}

func TestDiagnose_HealthyPod(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:  "web",
		Ready: true,
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}
	snap := mkSnap(mkPod("default", "web",
		withPhase(corev1.PodRunning),
		withCondition(corev1.PodReady, corev1.ConditionTrue, "", ""),
		withContainerStatus(cs),
	))
	verdict, findings, healthy := Diagnose(snap)
	if !healthy || verdict != nil || len(findings) != 0 {
		t.Fatalf("want healthy=true verdict=nil; got healthy=%v verdict=%v findings=%d", healthy, verdict, len(findings))
	}
}

// --- Negative-case tests (close coverage gaps for phase 3.3) ---------------

func TestImagePullBackOff_Negative(t *testing.T) {
	tests := []struct {
		name string
		cs   corev1.ContainerStatus
	}{
		{"running", corev1.ContainerStatus{
			Name:  "web",
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		}},
		{"container creating", corev1.ContainerStatus{
			Name: "web",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
				Reason: "ContainerCreating",
			}},
		}},
		{"crash loop is not pull failure", corev1.ContainerStatus{
			Name: "web",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
				Reason: "CrashLoopBackOff",
			}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := mkSnap(mkPod("default", "web", withPhase(corev1.PodRunning), withContainerStatus(tt.cs)))
			if got := imagePullBackOffRule(snap); len(got) != 0 {
				t.Fatalf("want 0 findings, got %d: %+v", len(got), got)
			}
		})
	}
}

func TestCrashLoopBackOff_Negative(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:         "app",
		Ready:        true,
		RestartCount: 0,
		State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}
	snap := mkSnap(mkPod("default", "web", withPhase(corev1.PodRunning), withContainerStatus(cs)))
	if got := crashLoopBackOffRule(snap); len(got) != 0 {
		t.Fatalf("want 0 findings, got %d: %+v", len(got), got)
	}
}

func TestProbeFailure_Negative(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:  "web",
		Ready: true,
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}
	// No probe events at all, healthy container.
	snap := mkSnap(mkPod("default", "web", withPhase(corev1.PodRunning), withContainerStatus(cs)))
	if got := probeFailureRule(snap); len(got) != 0 {
		t.Fatalf("want 0 findings without Unhealthy events, got %d: %+v", len(got), got)
	}
}

func TestProbeFailure_ReadinessIsWarning(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:         "web",
		Ready:        false,
		RestartCount: 0,
		State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}
	ev := mkEvent("Unhealthy", "Readiness probe failed: HTTP probe failed with statuscode: 503")
	ev.InvolvedObject.FieldPath = "spec.containers{web}"
	snap := mkSnap(mkPod("default", "web", withPhase(corev1.PodRunning), withContainerStatus(cs)), ev)

	got := probeFailureRule(snap)
	if len(got) != 1 || got[0].Code != "ProbeFailure" {
		t.Fatalf("want 1 ProbeFailure, got %+v", got)
	}
	if got[0].Severity != SeverityWarning {
		t.Fatalf("readiness probe should be warning, got %s", got[0].Severity)
	}
}

func TestProbeFailure_EventForOtherContainerIgnored(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:  "web",
		Ready: true,
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}
	// Unhealthy event names a DIFFERENT container.
	ev := mkEvent("Unhealthy", "Liveness probe failed")
	ev.InvolvedObject.FieldPath = "spec.containers{sidecar}"
	snap := mkSnap(mkPod("default", "web", withPhase(corev1.PodRunning), withContainerStatus(cs)), ev)
	if got := probeFailureRule(snap); len(got) != 0 {
		t.Fatalf("event for other container should not attribute to web: %+v", got)
	}
}

func TestPendingVolume_Negative_NoEventNoVolumeStuck(t *testing.T) {
	// Running pod, no volume issues anywhere.
	snap := mkSnap(mkPod("default", "web", withPhase(corev1.PodRunning),
		withCondition(corev1.PodInitialized, corev1.ConditionTrue, "", ""),
	))
	if got := pendingVolumeRule(snap); len(got) != 0 {
		t.Fatalf("want 0 findings, got %d: %+v", len(got), got)
	}
}

func TestPendingVolume_PVCPositive_AddsDescribePvcSuggestion(t *testing.T) {
	pod := mkPod("default", "web", withPhase(corev1.PodPending),
		withCondition(corev1.PodInitialized, corev1.ConditionFalse, "ContainersNotInitialized", ""),
	)
	pod.Spec.Volumes = []corev1.Volume{{
		Name: "data",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "cache"},
		},
	}}
	ev := mkEvent("FailedMount", `MountVolume.SetUp failed for volume "data" : timed out waiting for the condition`)
	snap := mkSnap(pod, ev)

	got := pendingVolumeRule(snap)
	if len(got) != 1 || got[0].Code != "PendingVolume" {
		t.Fatalf("want PendingVolume, got %+v", got)
	}
	hasDescribe := false
	for _, s := range got[0].Suggestions {
		if s == "kubectl describe pvc cache -n default" {
			hasDescribe = true
		}
	}
	if !hasDescribe {
		t.Fatalf("missing describe pvc suggestion: %v", got[0].Suggestions)
	}
}

func TestInitContainerFailure_Negative_CompletedInitDoesNotFire(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name: "migrate",
		State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			ExitCode: 0,
			Reason:   "Completed",
		}},
	}
	pod := mkPod("default", "web",
		withPhase(corev1.PodRunning),
		withCondition(corev1.PodInitialized, corev1.ConditionTrue, "", ""),
	)
	pod.Spec.InitContainers = []corev1.Container{{Name: "migrate"}}
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{cs}

	if got := initContainerFailureRule(mkSnap(pod)); len(got) != 0 {
		t.Fatalf("completed init container should not fire: %+v", got)
	}
}

func TestPendingScheduling_FromEventWhenConditionMessageEmpty(t *testing.T) {
	pod := mkPod("default", "web",
		withPhase(corev1.PodPending),
		withCondition(corev1.PodScheduled, corev1.ConditionFalse, "Unschedulable", ""),
	)
	ev := mkEvent("FailedScheduling", "0/3 nodes are available: 3 node(s) had untolerated taint")
	snap := mkSnap(pod, ev)
	got := pendingSchedulingRule(snap)
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d", len(got))
	}
	if !strings.Contains(got[0].Message, "untolerated taint") {
		t.Fatalf("want message to include taint info from event, got %q", got[0].Message)
	}
}
