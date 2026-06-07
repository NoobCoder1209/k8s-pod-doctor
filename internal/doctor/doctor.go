package doctor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/NoobCoder1209/k8s-pod-doctor/internal/version"
)

// Renderer is the contract between the doctor package and the render package.
// Defining it here avoids an import cycle.
type Renderer interface {
	RenderText(w io.Writer, snap *Snapshot, verdict *Finding, findings []Finding, healthy, noColor bool)
	RenderJSON(w io.Writer, snap *Snapshot, verdict *Finding, findings []Finding, healthy bool, toolVersion string, generatedAt time.Time) error
}

// activeRenderer is set in main via SetRenderer at startup; keeping the
// indirection out of the package boundary avoids the doctor->render import
// cycle. Tests can swap it freely.
var activeRenderer Renderer

// SetRenderer wires the renderer used by Run. Called from cmd/pod-doctor.
func SetRenderer(r Renderer) { activeRenderer = r }

// Run is the library entry point. It builds a clientset, collects snapshots,
// applies rules, and renders results to out.
func Run(ctx context.Context, opts Options, out io.Writer) error {
	if activeRenderer == nil {
		return errors.New("internal: renderer not configured (SetRenderer not called)")
	}

	client, err := BuildClient(opts.Kubeconfig, opts.Context)
	if err != nil {
		return err
	}
	tail := opts.Tail
	if tail <= 0 {
		tail = 100
	}

	if opts.AllFailing {
		return runAllFailing(ctx, client, opts, out, tail)
	}
	return runOne(ctx, client, opts, out, tail, opts.Namespace, opts.PodName)
}

func runOne(ctx context.Context, client kubernetes.Interface, opts Options, out io.Writer, tail int64, ns, name string) error {
	snap, err := CollectSnapshot(ctx, client, ns, name, tail)
	if err != nil {
		if errors.Is(err, ErrPodNotFound) {
			return fmt.Errorf("pod %s/%s not found in current cluster", ns, name)
		}
		return err
	}

	verdict, findings, healthy := Diagnose(snap)

	switch opts.Output {
	case "json":
		return activeRenderer.RenderJSON(out, snap, verdict, findings, healthy, version.Version, time.Now())
	default: // "text" or ""
		activeRenderer.RenderText(out, snap, verdict, findings, healthy, opts.NoColor)
		return nil
	}
}

func runAllFailing(ctx context.Context, client kubernetes.Interface, opts Options, out io.Writer, tail int64) error {
	pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}

	var failing []corev1.Pod
	for _, p := range pods.Items {
		if isPodUnhealthy(p) {
			failing = append(failing, p)
		}
	}

	if len(failing) == 0 {
		fmt.Fprintln(out, "No failing pods found.")
		return nil
	}

	if opts.Output == "json" {
		// One report per pod, encoded as a JSON array.
		fmt.Fprint(out, "[")
		for i, p := range failing {
			if i > 0 {
				fmt.Fprint(out, ",")
			}
			snap, err := CollectSnapshot(ctx, client, p.Namespace, p.Name, tail)
			if err != nil {
				continue
			}
			verdict, findings, healthy := Diagnose(snap)
			if err := activeRenderer.RenderJSON(out, snap, verdict, findings, healthy, version.Version, time.Now()); err != nil {
				return err
			}
		}
		fmt.Fprintln(out, "]")
		return nil
	}

	fmt.Fprintf(out, "Diagnosing %d failing pod(s)...\n\n", len(failing))
	for i, p := range failing {
		if i > 0 {
			fmt.Fprintln(out, "------------------------------------------------------------")
		}
		snap, err := CollectSnapshot(ctx, client, p.Namespace, p.Name, tail)
		if err != nil {
			fmt.Fprintf(out, "  (skip %s/%s: %v)\n", p.Namespace, p.Name, err)
			continue
		}
		verdict, findings, healthy := Diagnose(snap)
		activeRenderer.RenderText(out, snap, verdict, findings, healthy, opts.NoColor)
	}
	return nil
}

func isPodUnhealthy(p corev1.Pod) bool {
	switch p.Status.Phase {
	case corev1.PodPending, corev1.PodFailed, corev1.PodUnknown:
		return true
	case corev1.PodSucceeded:
		return false
	}
	// Running but not Ready, or any container Waiting/Terminated abnormally.
	if !podReady(p) {
		return true
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" && cs.State.Waiting.Reason != "ContainerCreating" {
			return true
		}
		if cs.RestartCount > 5 {
			return true
		}
	}
	return false
}

func podReady(p corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
