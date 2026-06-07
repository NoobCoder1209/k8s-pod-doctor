package doctor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

// GetPod fetches a single pod or returns a wrapped ErrPodNotFound.
func GetPod(ctx context.Context, c kubernetes.Interface, ns, name string) (*corev1.Pod, error) {
	p, err := c.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s/%s", ErrPodNotFound, ns, name)
		}
		return nil, fmt.Errorf("get pod %s/%s: %w", ns, name, err)
	}
	return p, nil
}

// ListPodEvents returns up to 50 events involving the named pod, newest first.
// The events API does not guarantee order, so we sort client-side.
func ListPodEvents(ctx context.Context, c kubernetes.Interface, ns, name string) ([]corev1.Event, error) {
	sel := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", name),
		fields.OneTermEqualSelector("involvedObject.namespace", ns),
	).String()

	list, err := c.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: sel,
		Limit:         50,
	})
	if err != nil {
		return nil, fmt.Errorf("list events for %s/%s: %w", ns, name, err)
	}
	evs := list.Items
	sort.SliceStable(evs, func(i, j int) bool {
		return eventTime(evs[i]).After(eventTime(evs[j]))
	})
	return evs, nil
}

// eventTime prefers LastTimestamp, falls back to EventTime, then CreationTimestamp.
func eventTime(e corev1.Event) time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time
	}
	if !e.EventTime.IsZero() {
		return e.EventTime.Time
	}
	return e.CreationTimestamp.Time
}

// TailLogs returns the last `tail` lines of the named container.
// Returns ErrContainerNotReady if the apiserver says the container is still
// ContainerCreating / PodInitializing. A 30s sub-deadline keeps a slow
// apiserver from hanging the diagnosis.
func TailLogs(ctx context.Context, c kubernetes.Interface, ns, pod, container string, tail int64) ([]byte, error) {
	logCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req := c.CoreV1().Pods(ns).GetLogs(pod, &corev1.PodLogOptions{
		Container: container,
		TailLines: ptr.To(tail),
	})
	stream, err := req.Stream(logCtx)
	if err != nil {
		if apierrors.IsBadRequest(err) {
			return nil, fmt.Errorf("%w: %s/%s container %s: %v", ErrContainerNotReady, ns, pod, container, err)
		}
		return nil, fmt.Errorf("open log stream %s/%s container %s: %w", ns, pod, container, err)
	}
	defer stream.Close()

	body, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("read log stream %s/%s container %s: %w", ns, pod, container, err)
	}
	return body, nil
}

// CollectSnapshot fetches the pod, its events, and a tail of every container's
// logs. Logs that cannot be fetched (container not ready, no logs yet) are
// recorded in Snapshot.LogErrors rather than failing the whole snapshot.
func CollectSnapshot(ctx context.Context, c kubernetes.Interface, ns, name string, tail int64) (*Snapshot, error) {
	pod, err := GetPod(ctx, c, ns, name)
	if err != nil {
		return nil, err
	}

	evs, err := ListPodEvents(ctx, c, ns, name)
	if err != nil {
		// Events are nice-to-have; degrade gracefully but still attach the
		// fetch error to the snapshot so the user knows.
		evs = nil
	}

	snap := &Snapshot{
		Pod:       pod,
		Events:    evs,
		Logs:      map[string]string{},
		LogErrors: map[string]string{},
		FetchedAt: time.Now().UTC(),
	}

	for _, c2 := range allContainerNames(pod) {
		body, lerr := TailLogs(ctx, c, ns, name, c2, tail)
		if lerr != nil {
			snap.LogErrors[c2] = friendlyLogError(lerr)
			continue
		}
		snap.Logs[c2] = string(body)
	}
	return snap, nil
}

// friendlyLogError converts wrapped collect errors to short, user-readable
// strings suitable for the "no logs: <reason>" rendering.
func friendlyLogError(err error) string {
	switch {
	case errors.Is(err, ErrContainerNotReady):
		return "container not yet started"
	default:
		return oneline(err.Error())
	}
}

func oneline(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	// Collapse runs of spaces.
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "no message"
	}
	return s
}

func allContainerNames(p *corev1.Pod) []string {
	names := make([]string, 0, len(p.Spec.InitContainers)+len(p.Spec.Containers))
	for _, c := range p.Spec.InitContainers {
		names = append(names, c.Name)
	}
	for _, c := range p.Spec.Containers {
		names = append(names, c.Name)
	}
	return names
}
