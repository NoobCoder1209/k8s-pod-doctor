package doctor

import (
	corev1 "k8s.io/api/core/v1"
)

// allRules is the registry of failure-mode detectors. Order is documentation
// only — verdict picking is by Finding.Priority, set inside each rule.
var allRules = []Rule{
	pendingSchedulingRule,
	pendingPVCRule,
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

func eventsForContainer(s *Snapshot, container string) []corev1.Event {
	if s == nil {
		return nil
	}
	var out []corev1.Event
	for _, e := range s.Events {
		if container == "" || e.InvolvedObject.FieldPath == "" {
			out = append(out, e)
			continue
		}
		// FieldPath is "spec.containers{name}" or "spec.initContainers{name}".
		if containsContainerRef(e.InvolvedObject.FieldPath, container) {
			out = append(out, e)
		}
	}
	return out
}

func containsContainerRef(fieldPath, container string) bool {
	needle := "{" + container + "}"
	for i := 0; i+len(needle) <= len(fieldPath); i++ {
		if fieldPath[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// firstEventByReason returns the first event with the given reason in the
// snapshot's events (which are pre-sorted newest-first).
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

// hasEventReason reports whether any event matches one of the reasons.
func hasEventReason(s *Snapshot, reasons ...string) bool {
	return firstEventByReason(s, reasons...) != nil
}
