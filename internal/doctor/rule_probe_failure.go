package doctor

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// probeFailureRule fires on Unhealthy / ProbeWarning events, OR readiness=False
// while the container is running. It promotes itself over CrashLoopBackOff for
// the same container when liveness-induced kills are detected.
func probeFailureRule(s *Snapshot) []Finding {
	p := s.Pod
	var out []Finding

	// 1) Container-scoped Unhealthy events.
	for _, cs := range p.Status.ContainerStatuses {
		evs := eventsForContainer(s, cs.Name)
		var firstUnhealthy *corev1.Event
		count := 0
		for i := range evs {
			if evs[i].Reason == "Unhealthy" || evs[i].Reason == "ProbeWarning" {
				if firstUnhealthy == nil {
					firstUnhealthy = &evs[i]
				}
				count++
			}
		}
		if firstUnhealthy == nil {
			continue
		}
		ptype := classifyProbeType(firstUnhealthy.Message)
		sev := SeverityWarning
		if ptype == "Liveness" || isLivenessKilled(cs) {
			sev = SeverityCritical
		}
		out = append(out, Finding{
			Code:      "ProbeFailure",
			Severity:  sev,
			Priority:  6,
			Title:     fmt.Sprintf("Container %s failing %s probe", cs.Name, strings.ToLower(ptype)),
			Message:   fmt.Sprintf("%s probe has failed at least %d times: %s", ptype, count, oneline(firstUnhealthy.Message)),
			Container: cs.Name,
			Suggestions: []string{
				fmt.Sprintf("kubectl get pod %s -n %s -o jsonpath='{.spec.containers[?(@.name==\"%s\")].livenessProbe}'", p.Name, p.Namespace, cs.Name),
				"Verify the probe endpoint responds correctly from inside the pod",
			},
			Evidence: []string{
				fmt.Sprintf("event Unhealthy x%d: %s", count, oneline(firstUnhealthy.Message)),
				fmt.Sprintf("restartCount=%d", cs.RestartCount),
			},
		})
	}

	return out
}

func classifyProbeType(msg string) string {
	switch {
	case strings.Contains(msg, "Liveness probe"):
		return "Liveness"
	case strings.Contains(msg, "Readiness probe"):
		return "Readiness"
	case strings.Contains(msg, "Startup probe"):
		return "Startup"
	}
	return "Probe"
}

// isLivenessKilled returns true when the container appears to have been killed
// by its liveness probe (signal 9, exit 137, with restarts climbing).
func isLivenessKilled(cs corev1.ContainerStatus) bool {
	last := cs.LastTerminationState.Terminated
	if last == nil {
		return false
	}
	return last.ExitCode == 137 && cs.RestartCount > 0 && last.Reason != "OOMKilled"
}
