package doctor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/NoobCoder1209/k8s-pod-doctor/internal/version"
)

// Renderer is the contract between the doctor package and the render package.
// Defining it here keeps doctor unaware of the concrete render implementation
// (the render package is what imports doctor).
type Renderer interface {
	RenderText(w io.Writer, snap *Snapshot, verdict *Finding, findings []Finding, healthy, noColor bool)
	RenderJSON(w io.Writer, snap *Snapshot, verdict *Finding, findings []Finding, healthy bool, toolVersion string, generatedAt time.Time) error
}

// Run is the library entry point. It builds a clientset, collects snapshots,
// applies rules, and renders results to out.
func Run(ctx context.Context, opts Options, r Renderer, out io.Writer) error {
	if r == nil {
		return errors.New("internal: renderer must not be nil")
	}

	client, err := BuildClient(opts.Kubeconfig, opts.Context)
	if err != nil {
		return friendlyKubeconfigError(err)
	}
	tail := opts.Tail
	if tail <= 0 {
		tail = 100
	}

	if opts.AllFailing {
		return runAllFailing(ctx, client, opts, r, out, tail)
	}
	return runOne(ctx, client, opts, r, out, tail, opts.Namespace, opts.PodName)
}

// friendlyKubeconfigError converts the most common client-go / clientcmd
// errors into a one-line stderr message users can act on. This is best-effort
// substring matching; unknown errors fall through unchanged.
func friendlyKubeconfigError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no configuration has been provided"):
		return errors.New("no kubeconfig found (set $KUBECONFIG, run `kubectl config`, or pass --kubeconfig)")
	case strings.Contains(msg, "no such file or directory") && strings.Contains(strings.ToLower(msg), "kubeconfig"):
		return errors.New("kubeconfig path does not exist (check --kubeconfig or $KUBECONFIG)")
	case strings.Contains(msg, "could not be loaded") || strings.Contains(msg, "stat "):
		return errors.New("kubeconfig file is not readable (check the path and permissions)")
	case strings.Contains(msg, "context") && strings.Contains(msg, "does not exist"):
		return fmt.Errorf("kubeconfig context not found: %s", err)
	}
	return err
}

func runOne(ctx context.Context, client kubernetes.Interface, opts Options, r Renderer, out io.Writer, tail int64, ns, name string) error {
	snap, err := CollectSnapshot(ctx, client, ns, name, tail)
	if err != nil {
		if errors.Is(err, ErrPodNotFound) {
			return fmt.Errorf("pod %s/%s not found in current cluster", ns, name)
		}
		return err
	}

	verdict, findings, healthy := Diagnose(snap)
	return renderOne(r, out, opts.Output, opts.NoColor, snap, verdict, findings, healthy)
}

func renderOne(r Renderer, out io.Writer, format string, noColor bool, snap *Snapshot, verdict *Finding, findings []Finding, healthy bool) error {
	switch format {
	case "json":
		return r.RenderJSON(out, snap, verdict, ensureSlice(findings), healthy, version.Version, time.Now())
	default:
		r.RenderText(out, snap, verdict, findings, healthy, noColor)
		return nil
	}
}

// ensureSlice converts a nil findings slice into an empty slice so the JSON
// schema produces `"findings": []` instead of `"findings": null`.
func ensureSlice(in []Finding) []Finding {
	if in == nil {
		return []Finding{}
	}
	return in
}

func runAllFailing(ctx context.Context, client kubernetes.Interface, opts Options, r Renderer, out io.Writer, tail int64) error {
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
		return renderAllJSON(ctx, client, r, out, failing, tail)
	}
	return renderAllText(ctx, client, r, out, opts.NoColor, failing, tail)
}

// renderAllJSON emits a JSON array of one Report per pod. Pods whose snapshot
// fails are emitted as {"pod": {...}, "error": "..."} placeholders so the
// output stays valid JSON. The closing `]` is always emitted, even if the
// renderer errors mid-stream (deferred). Any pods AFTER a renderer error are
// not rendered, but the document remains valid JSON regardless.
func renderAllJSON(ctx context.Context, client kubernetes.Interface, r Renderer, out io.Writer, failing []corev1.Pod, tail int64) (retErr error) {
	fmt.Fprint(out, "[")
	defer fmt.Fprintln(out, "]")

	first := true
	emitPlaceholder := func(p corev1.Pod, msg string) {
		if !first {
			fmt.Fprint(out, ",")
		}
		first = false
		fmt.Fprintf(out, `{"pod":{"namespace":%q,"name":%q},"error":%q}`,
			p.Namespace, p.Name, msg)
	}

	for _, p := range failing {
		snap, err := CollectSnapshot(ctx, client, p.Namespace, p.Name, tail)
		if err != nil {
			emitPlaceholder(p, err.Error())
			continue
		}
		if !first {
			fmt.Fprint(out, ",")
		}
		first = false
		verdict, findings, healthy := Diagnose(snap)
		if err := r.RenderJSON(out, snap, verdict, ensureSlice(findings), healthy, version.Version, time.Now()); err != nil {
			retErr = err
			return retErr
		}
	}
	return nil
}

func renderAllText(ctx context.Context, client kubernetes.Interface, r Renderer, out io.Writer, noColor bool, failing []corev1.Pod, tail int64) error {
	fmt.Fprintf(out, "Diagnosing %d failing pod(s)...\n\n", len(failing))
	first := true
	for _, p := range failing {
		snap, err := CollectSnapshot(ctx, client, p.Namespace, p.Name, tail)
		if err != nil {
			fmt.Fprintf(out, "  (skip %s/%s: %v)\n\n", p.Namespace, p.Name, err)
			continue
		}
		if !first {
			fmt.Fprintln(out, "------------------------------------------------------------")
		}
		first = false
		verdict, findings, healthy := Diagnose(snap)
		r.RenderText(out, snap, verdict, findings, healthy, noColor)
	}
	return nil
}

// isPodUnhealthy returns true for pods worth surfacing in --all-failing.
// We deliberately stay conservative: pods in non-Running phases, pods whose
// Ready condition is False, or pods with any container in an abnormal Waiting
// state. We do NOT use restart count as a proxy for unhealthy — long-running
// pods that crashed once weeks ago are not interesting.
func isPodUnhealthy(p corev1.Pod) bool {
	switch p.Status.Phase {
	case corev1.PodPending, corev1.PodFailed, corev1.PodUnknown:
		return true
	case corev1.PodSucceeded:
		return false
	}
	if !podReady(p) {
		return true
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" && cs.State.Waiting.Reason != "ContainerCreating" {
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
