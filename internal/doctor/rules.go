package doctor

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// allRules is the registry of failure-mode detectors. Order is documentation
// only — verdict picking is by Finding.Priority, set inside each rule.
var allRules = []Rule{
	pendingSchedulingRule,
	pendingVolumeRule,
	initContainerFailureRule,
	imagePullBackOffRule,
	oomKilledRule,
	probeFailureRule,
	crashLoopBackOffRule,
}

// Diagnose applies every rule to the snapshot and returns all findings,
// sorted and de-duplicated by resolveVerdict.
func Diagnose(s *Snapshot) (verdict *Finding, findings []Finding, healthy bool) {
	if s == nil || s.Pod == nil {
		return nil, nil, false
	}
	for _, r := range allRules {
		findings = append(findings, r(s)...)
	}
	return resolveVerdict(findings)
}

// --- helpers shared across rules -------------------------------------------

func podCondition(p *corev1.Pod, t corev1.PodConditionType) *corev1.PodCondition {
	for i := range p.Status.Conditions {
		if p.Status.Conditions[i].Type == t {
			return &p.Status.Conditions[i]
		}
	}
	return nil
}

// isSidecarInit returns true when the named init container has the native
// sidecar shape — restartPolicy: Always (Kubernetes 1.29+). Sidecars don't
// block pod initialisation and behave like regular containers, so they are
// classified by the same rules (CrashLoop, OOM, Probe) rather than by
// initContainerFailureRule.
func isSidecarInit(p *corev1.Pod, name string) bool {
	for _, ic := range p.Spec.InitContainers {
		if ic.Name == name && ic.RestartPolicy != nil && *ic.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			return true
		}
	}
	return false
}

// allRunnableContainerStatuses returns ContainerStatuses for both regular
// containers and sidecar init containers. Used by rules that classify
// runtime-state failures.
func allRunnableContainerStatuses(p *corev1.Pod) []corev1.ContainerStatus {
	out := make([]corev1.ContainerStatus, 0, len(p.Status.ContainerStatuses)+len(p.Status.InitContainerStatuses))
	out = append(out, p.Status.ContainerStatuses...)
	for _, cs := range p.Status.InitContainerStatuses {
		if isSidecarInit(p, cs.Name) {
			out = append(out, cs)
		}
	}
	return out
}

// eventsForContainer returns events whose involvedObject FieldPath names the
// container. Events without a FieldPath are NOT attributed to any specific
// container — that prevents pod-scoped events from being counted N times
// across N containers.
func eventsForContainer(s *Snapshot, container string) []corev1.Event {
	if s == nil {
		return nil
	}
	needle := "{" + container + "}"
	var out []corev1.Event
	for _, e := range s.Events {
		if strings.Contains(e.InvolvedObject.FieldPath, needle) {
			out = append(out, e)
		}
	}
	return out
}

// firstEventByReason returns the first event with one of the given reasons in
// the snapshot's events (which are pre-sorted newest-first).
func firstEventByReason(s *Snapshot, reasons ...string) *corev1.Event {
	for i := range s.Events {
		for _, r := range reasons {
			if s.Events[i].Reason == r {
				return &s.Events[i]
			}
		}
	}
	return nil
}
