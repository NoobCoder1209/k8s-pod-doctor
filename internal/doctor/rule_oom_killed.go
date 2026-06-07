package doctor

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// oomKilledRule fires when a container was OOMKilled either currently or last
// run. Wins over CrashLoopBackOff for the same container at verdict time.
// Iterates regular containers AND native sidecar init containers.
func oomKilledRule(s *Snapshot) []Finding {
	p := s.Pod
	var out []Finding
	for _, cs := range allRunnableContainerStatuses(p) {
		f := oomFinding(p, cs)
		if f != nil {
			out = append(out, *f)
		}
	}
	return out
}

func oomFinding(p *corev1.Pod, cs corev1.ContainerStatus) *Finding {
	var (
		reasonSrc string
		exitCode  int32
		signal    int32
	)
	if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
		reasonSrc = "state.terminated"
		exitCode = cs.State.Terminated.ExitCode
		signal = cs.State.Terminated.Signal
	} else if last := cs.LastTerminationState.Terminated; last != nil && last.Reason == "OOMKilled" {
		reasonSrc = "lastState.terminated"
		exitCode = last.ExitCode
		signal = last.Signal
	} else {
		return nil
	}

	return &Finding{
		Code:      "OOMKilled",
		Severity:  SeverityCritical,
		Priority:  5,
		Title:     fmt.Sprintf("Container %s was OOMKilled", cs.Name),
		Message:   fmt.Sprintf("Container %s exceeded its memory limit and was killed (exit %d). Restart count: %d.", cs.Name, exitCode, cs.RestartCount),
		Container: cs.Name,
		Suggestions: []string{
			fmt.Sprintf("Raise resources.limits.memory for container %s, or profile actual usage", cs.Name),
			fmt.Sprintf("kubectl top pod %s -n %s --containers (requires metrics-server)", p.Name, p.Namespace),
		},
		Evidence: []string{
			fmt.Sprintf("%s.reason=OOMKilled", reasonSrc),
			fmt.Sprintf("%s.exitCode=%d signal=%d", reasonSrc, exitCode, signal),
			fmt.Sprintf("restartCount=%d", cs.RestartCount),
		},
	}
}
