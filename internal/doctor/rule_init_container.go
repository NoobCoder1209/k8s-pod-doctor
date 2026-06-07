package doctor

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// initContainerFailureRule fires when an init container is blocking the pod.
// It surfaces the most-specific underlying cause (image pull, OOM, crash) when
// it can identify one.
func initContainerFailureRule(s *Snapshot) []Finding {
	p := s.Pod
	initCond := podCondition(p, corev1.PodInitialized)
	if initCond == nil || initCond.Status != corev1.ConditionFalse {
		return nil
	}

	for _, cs := range p.Status.InitContainerStatuses {
		// Native sidecars (restartPolicy: Always) don't block init; the
		// crashLoop / OOM / probe rules handle their failures directly.
		if isSidecarInit(p, cs.Name) {
			continue
		}
		if !isInitContainerFailing(cs) {
			continue
		}
		// Identify the underlying cause from the container status itself.
		underlyingCode, underlyingMessage := classifyContainerFailure(cs)
		title := fmt.Sprintf("Init container %s failed", cs.Name)
		message := fmt.Sprintf("Init container %s did not complete: %s — %s",
			cs.Name, underlyingCode, oneline(underlyingMessage))
		return []Finding{{
			Code:      "InitContainerFailure",
			Severity:  SeverityCritical,
			Priority:  3,
			Title:     title,
			Message:   message,
			Container: cs.Name,
			Suggestions: []string{
				fmt.Sprintf("kubectl logs %s -n %s -c %s --previous", p.Name, p.Namespace, cs.Name),
				"Check init container image and command in spec",
			},
			Evidence: collectInitEvidence(cs),
		}}
	}
	return nil
}

// isInitContainerFailing returns true if the init container is in a failure state.
// Sidecar (restartPolicy: Always) init containers are excluded — they look like
// regular containers and other rules handle them.
func isInitContainerFailing(cs corev1.ContainerStatus) bool {
	if cs.State.Waiting != nil {
		switch cs.State.Waiting.Reason {
		case "CrashLoopBackOff", "RunContainerError", "CreateContainerError",
			"CreateContainerConfigError", "ErrImagePull", "ImagePullBackOff",
			"ImageInspectError", "InvalidImageName":
			return true
		}
	}
	if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
		return true
	}
	if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.ExitCode != 0 && cs.RestartCount > 0 {
		return true
	}
	return false
}

// classifyContainerFailure returns a short code and message naming the
// underlying cause for a failing container's status.
func classifyContainerFailure(cs corev1.ContainerStatus) (code, message string) {
	if cs.State.Waiting != nil {
		switch cs.State.Waiting.Reason {
		case "ErrImagePull", "ImagePullBackOff", "ImageInspectError",
			"InvalidImageName", "RegistryUnavailable":
			return "ImagePullBackOff", cs.State.Waiting.Message
		case "CrashLoopBackOff":
			if last := cs.LastTerminationState.Terminated; last != nil && last.Reason == "OOMKilled" {
				return "OOMKilled", fmt.Sprintf("exit %d (OOMKilled)", last.ExitCode)
			}
			return "CrashLoopBackOff", cs.State.Waiting.Message
		}
		return cs.State.Waiting.Reason, cs.State.Waiting.Message
	}
	if cs.State.Terminated != nil {
		return cs.State.Terminated.Reason, cs.State.Terminated.Message
	}
	if last := cs.LastTerminationState.Terminated; last != nil {
		return last.Reason, last.Message
	}
	return "Unknown", ""
}

func collectInitEvidence(cs corev1.ContainerStatus) []string {
	var out []string
	if cs.State.Waiting != nil {
		out = append(out, fmt.Sprintf("state.waiting.reason=%s", cs.State.Waiting.Reason))
	}
	if cs.State.Terminated != nil {
		out = append(out, fmt.Sprintf("state.terminated.exitCode=%d reason=%s",
			cs.State.Terminated.ExitCode, cs.State.Terminated.Reason))
	}
	if last := cs.LastTerminationState.Terminated; last != nil {
		out = append(out, fmt.Sprintf("lastState.terminated.exitCode=%d reason=%s",
			last.ExitCode, last.Reason))
	}
	out = append(out, fmt.Sprintf("restartCount=%d", cs.RestartCount))
	return out
}
