package doctor

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// crashLoopBackOffRule is the fallback for containers stuck in
// CrashLoopBackOff that aren't already explained by OOM or probe failures.
// resolveVerdict() drops this for a container if a higher-priority finding
// already named it. Iterates regular containers AND native sidecar init
// containers.
func crashLoopBackOffRule(s *Snapshot) []Finding {
	p := s.Pod
	var out []Finding
	for _, cs := range allRunnableContainerStatuses(p) {
		if cs.State.Waiting == nil || cs.State.Waiting.Reason != "CrashLoopBackOff" {
			continue
		}
		out = append(out, *crashLoopFinding(p, cs))
	}
	return out
}

func crashLoopFinding(p *corev1.Pod, cs corev1.ContainerStatus) *Finding {
	exitCode := int32(-1)
	reason := "Unknown"
	message := ""
	if last := cs.LastTerminationState.Terminated; last != nil {
		exitCode = last.ExitCode
		reason = last.Reason
		message = last.Message
	}
	return &Finding{
		Code:      "CrashLoopBackOff",
		Severity:  SeverityCritical,
		Priority:  7,
		Title:     fmt.Sprintf("Container %s crash-looping", cs.Name),
		Message:   fmt.Sprintf("Container %s crashed %d times. Last exit: %d (%s).", cs.Name, cs.RestartCount, exitCode, reason),
		Container: cs.Name,
		Suggestions: []string{
			fmt.Sprintf("kubectl logs %s -n %s -c %s --previous", p.Name, p.Namespace, cs.Name),
			fmt.Sprintf("Check command/args/env for container %s in the pod spec", cs.Name),
		},
		Evidence: []string{
			"state.waiting.reason=CrashLoopBackOff",
			fmt.Sprintf("lastState.terminated.exitCode=%d reason=%s", exitCode, reason),
			fmt.Sprintf("lastState.terminated.message=%s", oneline(message)),
			fmt.Sprintf("restartCount=%d", cs.RestartCount),
		},
	}
}
