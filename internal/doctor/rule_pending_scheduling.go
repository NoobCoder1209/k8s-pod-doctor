package doctor

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// pendingSchedulingRule fires when the scheduler cannot place the pod on any node.
func pendingSchedulingRule(s *Snapshot) []Finding {
	p := s.Pod
	if p.Status.Phase != corev1.PodPending {
		return nil
	}
	cond := podCondition(p, corev1.PodScheduled)
	if cond == nil || cond.Status != corev1.ConditionFalse || cond.Reason != "Unschedulable" {
		return nil
	}

	msg := cond.Message
	if msg == "" {
		if e := firstEventByReason(s, "FailedScheduling"); e != nil {
			msg = e.Message
		}
	}

	return []Finding{{
		Code:     "PendingScheduling",
		Severity: SeverityCritical,
		Priority: 1,
		Title:    "Pod cannot be scheduled",
		Message:  fmt.Sprintf("Scheduler cannot place pod on any node: %s", oneline(msg)),
		Suggestions: []string{
			fmt.Sprintf("kubectl describe pod %s/%s | grep -A5 Events", p.Namespace, p.Name),
			"Check node capacity and tolerations: kubectl get nodes -o wide; kubectl describe nodes",
		},
		Evidence: []string{
			fmt.Sprintf("condition PodScheduled=False, reason=%s", cond.Reason),
			"event " + briefEvent(firstEventByReason(s, "FailedScheduling")),
		},
	}}
}

func oneline(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return "no message"
	}
	return s
}

func briefEvent(e *corev1.Event) string {
	if e == nil {
		return "(none)"
	}
	return fmt.Sprintf("%s: %s", e.Reason, oneline(e.Message))
}
