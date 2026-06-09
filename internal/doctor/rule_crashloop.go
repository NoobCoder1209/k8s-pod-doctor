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
//
// Detection: a container is "crash-looping" when EITHER
//   - State.Waiting.Reason == "CrashLoopBackOff" (kubelet has flipped to
//     backoff between restarts), OR
//   - restartCount >= 2 AND lastState.terminated.exitCode != 0 (kubelet has
//     restarted the container repeatedly because it keeps exiting non-zero;
//     between restarts the state oscillates through Terminated{Error},
//     Waiting{CrashLoopBackOff}, Running, and back).
//
// The OOM and Probe rules win over this for the same container via verdict
// suppression, so even when an OOMKilled container is technically also
// crash-looping the user sees "OOMKilled" as the cause and CrashLoopBackOff
// is dropped.
func crashLoopBackOffRule(s *Snapshot) []Finding {
	p := s.Pod
	var out []Finding
	for _, cs := range allRunnableContainerStatuses(p) {
		if !isCrashLooping(cs) {
			continue
		}
		out = append(out, *crashLoopFinding(p, cs))
	}
	return out
}

// isCrashLooping reports whether the container is stuck in a restart loop.
func isCrashLooping(cs corev1.ContainerStatus) bool {
	if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
		return true
	}
	// Repeated non-zero exits with the kubelet restarting the container.
	if cs.RestartCount >= 2 {
		if last := cs.LastTerminationState.Terminated; last != nil && last.ExitCode != 0 {
			return true
		}
		if cur := cs.State.Terminated; cur != nil && cur.ExitCode != 0 {
			return true
		}
	}
	return false
}

func crashLoopFinding(p *corev1.Pod, cs corev1.ContainerStatus) *Finding {
	exitCode := int32(-1)
	reason := "Unknown"
	message := ""
	// Prefer LastTerminationState; fall back to State.Terminated for the
	// rare case where we caught the container mid-restart.
	if last := cs.LastTerminationState.Terminated; last != nil {
		exitCode, reason, message = last.ExitCode, last.Reason, last.Message
	} else if cur := cs.State.Terminated; cur != nil {
		exitCode, reason, message = cur.ExitCode, cur.Reason, cur.Message
	}

	stateEvidence := "state."
	switch {
	case cs.State.Waiting != nil:
		stateEvidence += "waiting.reason=" + cs.State.Waiting.Reason
	case cs.State.Terminated != nil:
		stateEvidence = fmt.Sprintf("state.terminated.exitCode=%d reason=%s",
			cs.State.Terminated.ExitCode, cs.State.Terminated.Reason)
	case cs.State.Running != nil:
		stateEvidence += "running"
	default:
		stateEvidence = "state=unknown"
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
			stateEvidence,
			fmt.Sprintf("lastState.terminated.exitCode=%d reason=%s", exitCode, reason),
			fmt.Sprintf("lastState.terminated.message=%s", oneline(message)),
			fmt.Sprintf("restartCount=%d", cs.RestartCount),
		},
	}
}
