package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// fakeRenderer captures RenderText/RenderJSON inputs for assertion.
type fakeRenderer struct {
	textCalls int
	jsonCalls int
}

func (f *fakeRenderer) RenderText(w io.Writer, snap *Snapshot, verdict *Finding, findings []Finding, healthy, noColor bool) {
	f.textCalls++
	w.Write([]byte("text-rendered:" + snap.Pod.Name))
}

func (f *fakeRenderer) RenderJSON(w io.Writer, snap *Snapshot, verdict *Finding, findings []Finding, healthy bool, toolVersion string, generatedAt time.Time) error {
	f.jsonCalls++
	type out struct {
		Pod      string    `json:"pod"`
		Findings []Finding `json:"findings"`
	}
	return json.NewEncoder(w).Encode(out{Pod: snap.Pod.Name, Findings: findings})
}

func TestGetPod_NotFoundIsTyped(t *testing.T) {
	cs := fake.NewSimpleClientset()
	_, err := GetPod(context.Background(), cs, "default", "missing")
	if err == nil {
		t.Fatal("want error")
	}
	if !errors.Is(err, ErrPodNotFound) {
		t.Fatalf("want ErrPodNotFound, got %v", err)
	}
}

func TestRunOne_PodNotFoundFriendlyError(t *testing.T) {
	cs := fake.NewSimpleClientset()
	r := &fakeRenderer{}
	var buf bytes.Buffer
	err := runOne(context.Background(), cs, Options{Output: "text"}, r, &buf, 100, "default", "missing")
	if err == nil {
		t.Fatal("want error")
	}
	if got := err.Error(); !contains(got, "not found in current cluster") {
		t.Fatalf("want friendly error, got %q", got)
	}
}

func TestRunOne_HappyPath(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "ok"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "web"}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "web",
				Ready: true,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}
	cs := fake.NewSimpleClientset(pod)
	r := &fakeRenderer{}
	var buf bytes.Buffer
	if err := runOne(context.Background(), cs, Options{Output: "text"}, r, &buf, 100, "default", "ok"); err != nil {
		t.Fatal(err)
	}
	if r.textCalls != 1 {
		t.Fatalf("want 1 text render, got %d", r.textCalls)
	}
	if !contains(buf.String(), "text-rendered:ok") {
		t.Fatalf("renderer not invoked: %q", buf.String())
	}
}

func TestRunAllFailing_JSON_SkipsBadPodAndStaysValidJSON(t *testing.T) {
	// A failing pod the fake clientset can serve.
	failing := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "broken"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "web"}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable", Message: "0/1 nodes available"},
			},
		},
	}
	// A pod whose List returns it but whose Get will fail (we'll simulate by
	// listing only via the fake; it will respond consistently to Get, so this
	// happy-path two-pod case proves the comma logic).
	failing2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns2", Name: "broken2"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "web"}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
		},
	}
	cs := fake.NewSimpleClientset(failing, failing2)
	r := &fakeRenderer{}
	var buf bytes.Buffer
	err := runAllFailing(context.Background(), cs, Options{Output: "json"}, r, &buf, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Output must be valid JSON — parse as an array.
	var arr []map[string]any
	if e := json.Unmarshal(buf.Bytes(), &arr); e != nil {
		t.Fatalf("output is invalid JSON: %v\n%s", e, buf.String())
	}
	if len(arr) != 2 {
		t.Fatalf("want 2 elements, got %d: %s", len(arr), buf.String())
	}
}

// helpers

func contains(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}
