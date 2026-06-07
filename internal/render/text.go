package render

import (
	"fmt"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/NoobCoder1209/k8s-pod-doctor/internal/doctor"
)

// RenderText writes a human-readable diagnosis of the snapshot.
//
// Order: boxed verdict banner, then sections — Status, Recent events,
// Logs (if any), Findings (if more than one).
func RenderText(w io.Writer, snap *doctor.Snapshot, verdict *doctor.Finding, findings []doctor.Finding, healthy, noColor bool) {
	if snap == nil || snap.Pod == nil {
		fmt.Fprintln(w, "(no pod data)")
		return
	}

	// 1) Banner
	switch {
	case healthy:
		writeBanner(w, doctor.SeverityHealthy,
			"Pod looks healthy",
			"All containers ready, no failure signatures detected.",
			noColor)
	case verdict != nil:
		writeBanner(w, verdict.Severity, verdict.Title, verdict.Message, noColor)
	default:
		writeBanner(w, doctor.SeverityInfo, "No verdict", "Could not classify pod state.", noColor)
	}

	fmt.Fprintln(w)

	// 2) Status
	writeStatus(w, snap.Pod)

	// 3) Recent events
	writeEvents(w, snap.Events)

	// 4) Logs (truncate each container's tail to keep output reasonable).
	writeLogs(w, snap)

	// 5) Findings + suggestions.
	if !healthy && len(findings) > 0 {
		writeFindings(w, findings, noColor)
	}
}

func writeStatus(w io.Writer, pod *corev1.Pod) {
	fmt.Fprintln(w, sectionHeader("Status"))
	fmt.Fprintf(w, "  Pod:         %s/%s\n", pod.Namespace, pod.Name)
	fmt.Fprintf(w, "  Phase:       %s\n", pod.Status.Phase)
	if pod.Spec.NodeName != "" {
		fmt.Fprintf(w, "  Node:        %s\n", pod.Spec.NodeName)
	}
	if pod.Status.StartTime != nil {
		fmt.Fprintf(w, "  Started:     %s\n", pod.Status.StartTime.Time.Format("2006-01-02T15:04:05Z07:00"))
	}
	if len(pod.Status.Conditions) > 0 {
		fmt.Fprintln(w, "  Conditions:")
		for _, cnd := range pod.Status.Conditions {
			r := ""
			if cnd.Reason != "" {
				r = fmt.Sprintf(" (%s)", cnd.Reason)
			}
			fmt.Fprintf(w, "    %-18s %s%s\n", cnd.Type, cnd.Status, r)
		}
	}
	if len(pod.Status.InitContainerStatuses) > 0 {
		fmt.Fprintln(w, "  Init containers:")
		for _, cs := range pod.Status.InitContainerStatuses {
			fmt.Fprintf(w, "    %s\n", briefContainerLine(cs))
		}
	}
	if len(pod.Status.ContainerStatuses) > 0 {
		fmt.Fprintln(w, "  Containers:")
		for _, cs := range pod.Status.ContainerStatuses {
			fmt.Fprintf(w, "    %s\n", briefContainerLine(cs))
		}
	}
	fmt.Fprintln(w)
}

func briefContainerLine(cs corev1.ContainerStatus) string {
	state := "unknown"
	switch {
	case cs.State.Running != nil:
		state = "Running"
	case cs.State.Waiting != nil:
		state = "Waiting:" + cs.State.Waiting.Reason
	case cs.State.Terminated != nil:
		state = fmt.Sprintf("Terminated:%s(exit %d)", cs.State.Terminated.Reason, cs.State.Terminated.ExitCode)
	}
	return fmt.Sprintf("%-20s ready=%v restarts=%d state=%s",
		cs.Name, cs.Ready, cs.RestartCount, state)
}

func writeEvents(w io.Writer, evs []corev1.Event) {
	fmt.Fprintln(w, sectionHeader("Recent events"))
	if len(evs) == 0 {
		fmt.Fprintln(w, "  (none)")
		fmt.Fprintln(w)
		return
	}
	max := 10
	if len(evs) < max {
		max = len(evs)
	}
	for i := 0; i < max; i++ {
		e := evs[i]
		ts := ""
		if !e.LastTimestamp.IsZero() {
			ts = e.LastTimestamp.Time.Format("15:04:05")
		} else if !e.EventTime.IsZero() {
			ts = e.EventTime.Time.Format("15:04:05")
		}
		fmt.Fprintf(w, "  %-8s %-9s %-20s %s\n", ts, e.Type, e.Reason, oneline(e.Message))
	}
	fmt.Fprintln(w)
}

func writeLogs(w io.Writer, snap *doctor.Snapshot) {
	if len(snap.Logs) == 0 && len(snap.LogErrors) == 0 {
		return
	}
	fmt.Fprintln(w, sectionHeader("Logs"))
	for name, body := range snap.Logs {
		body = strings.TrimRight(body, "\n")
		if body == "" {
			continue
		}
		fmt.Fprintf(w, "  -- %s --\n", name)
		// Indent each log line by two spaces, cap at last 20 lines.
		lines := strings.Split(body, "\n")
		max := 20
		start := 0
		if len(lines) > max {
			start = len(lines) - max
			fmt.Fprintf(w, "    ... (truncated, showing last %d lines)\n", max)
		}
		for _, line := range lines[start:] {
			fmt.Fprintf(w, "    %s\n", line)
		}
	}
	for name, errMsg := range snap.LogErrors {
		fmt.Fprintf(w, "  -- %s -- (no logs: %s)\n", name, oneline(errMsg))
	}
	fmt.Fprintln(w)
}

func writeFindings(w io.Writer, findings []doctor.Finding, noColor bool) {
	fmt.Fprintln(w, sectionHeader("Findings"))
	for i, f := range findings {
		c := colourFor(f.Severity, noColor)
		fmt.Fprintf(w, "  %d. [%s] %s\n", i+1, c.Sprint(strings.ToUpper(string(f.Severity))), f.Title)
		fmt.Fprintf(w, "     %s\n", f.Message)
		if f.Container != "" {
			fmt.Fprintf(w, "     container: %s\n", f.Container)
		}
		if len(f.Suggestions) > 0 {
			fmt.Fprintln(w, "     suggestions:")
			for _, s := range f.Suggestions {
				fmt.Fprintf(w, "       - %s\n", s)
			}
		}
	}
	fmt.Fprintln(w)
}

func sectionHeader(title string) string {
	return fmt.Sprintf("== %s ==", title)
}

func oneline(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return s
}
