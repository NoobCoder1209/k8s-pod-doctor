package doctor

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// withInitContainerSpec adds an init container spec entry with optional
// sidecar restartPolicy.
func withInitContainerSpec(name string, sidecar bool) func(*corev1.Pod) {
	return func(p *corev1.Pod) {
		ic := corev1.Container{Name: name}
		if sidecar {
			policy := corev1.ContainerRestartPolicyAlways
			ic.RestartPolicy = &policy
		}
		p.Spec.InitContainers = append(p.Spec.InitContainers, ic)
	}
}

func withInitContainerStatus(cs corev1.ContainerStatus) func(*corev1.Pod) {
	return func(p *corev1.Pod) {
		p.Status.InitContainerStatuses = append(p.Status.InitContainerStatuses, cs)
	}
}

func TestSidecarInit_CrashLoopReportedAsRegularCrashLoop(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:                 "envoy",
		RestartCount:         3,
		State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"}},
	}
	pod := mkPod("default", "web",
		withPhase(corev1.PodRunning),
		withInitContainerSpec("envoy", true), // sidecar
		withInitContainerStatus(cs),
	)
	snap := mkSnap(pod)

	// initContainerFailureRule must NOT fire for the sidecar.
	if got := initContainerFailureRule(snap); len(got) != 0 {
		t.Fatalf("init rule fired on sidecar: %+v", got)
	}
	// crashLoopBackOffRule MUST fire for the sidecar.
	if got := crashLoopBackOffRule(snap); len(got) != 1 || got[0].Container != "envoy" {
		t.Fatalf("crashLoop rule didn't pick up sidecar: %+v", got)
	}
}

func TestRegularInit_CrashLoopReportedAsInitFailure(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:                 "migrate",
		RestartCount:         5,
		State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"}},
	}
	pod := mkPod("default", "web",
		withPhase(corev1.PodPending),
		withCondition(corev1.PodInitialized, corev1.ConditionFalse, "ContainersNotInitialized", ""),
		withInitContainerSpec("migrate", false), // not a sidecar
		withInitContainerStatus(cs),
	)
	snap := mkSnap(pod)

	got := initContainerFailureRule(snap)
	if len(got) != 1 || got[0].Code != "InitContainerFailure" {
		t.Fatalf("want InitContainerFailure, got %+v", got)
	}
	if got[0].Container != "migrate" {
		t.Fatalf("wrong container: %s", got[0].Container)
	}
}

func TestSidecarInit_OOMReportedAsOOM(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:         "envoy",
		RestartCount: 1,
		State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			Reason:   "OOMKilled",
			ExitCode: 137,
			Signal:   9,
		}},
	}
	pod := mkPod("default", "web",
		withPhase(corev1.PodRunning),
		withInitContainerSpec("envoy", true),
		withInitContainerStatus(cs),
	)
	snap := mkSnap(pod)

	got := oomKilledRule(snap)
	if len(got) != 1 || got[0].Code != "OOMKilled" || got[0].Container != "envoy" {
		t.Fatalf("want OOM finding for sidecar envoy, got %+v", got)
	}
}

func TestPendingVolume_NonPVCFailureStillFires(t *testing.T) {
	pod := mkPod("default", "web",
		withPhase(corev1.PodPending),
		withCondition(corev1.PodInitialized, corev1.ConditionFalse, "ContainersNotInitialized", ""),
	)
	pod.Spec.Volumes = []corev1.Volume{{
		Name:         "config",
		VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "missing"}},
	}}
	ev := mkEvent("FailedMount", "MountVolume.SetUp failed for volume \"config\" : secret \"missing\" not found")
	snap := mkSnap(pod, ev)

	got := pendingVolumeRule(snap)
	if len(got) != 1 || got[0].Code != "PendingVolume" {
		t.Fatalf("want PendingVolume finding, got %+v", got)
	}
	// Suggestion list should NOT include kubectl describe pvc when there's no PVC.
	for _, s := range got[0].Suggestions {
		if strings.Contains(s, "describe pvc") {
			t.Fatalf("PVC suggestion present for non-PVC volume: %s", s)
		}
	}
}

func TestImagePullBackOff_AllReasonVariants(t *testing.T) {
	reasons := []string{"ErrImagePull", "ImagePullBackOff", "ImageInspectError", "InvalidImageName", "RegistryUnavailable"}
	for _, r := range reasons {
		t.Run(r, func(t *testing.T) {
			cs := corev1.ContainerStatus{
				Name:  "web",
				Image: "registry.example/foo:bar",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: r, Message: "test"}},
			}
			snap := mkSnap(mkPod("default", "web", withPhase(corev1.PodPending), withContainerStatus(cs)))
			got := imagePullBackOffRule(snap)
			if len(got) != 1 || got[0].Code != "ImagePullBackOff" {
				t.Fatalf("variant %s: want 1 finding, got %+v", r, got)
			}
		})
	}
}
