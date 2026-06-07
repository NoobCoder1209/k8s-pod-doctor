package doctor

import (
	"testing"
)

func TestResolveVerdict_Empty(t *testing.T) {
	verdict, ordered, healthy := resolveVerdict(nil)
	if !healthy {
		t.Fatalf("want healthy=true on empty input")
	}
	if verdict != nil || ordered != nil {
		t.Fatalf("want nil verdict/findings; got verdict=%v findings=%v", verdict, ordered)
	}
}

func TestResolveVerdict_PriorityOrdering(t *testing.T) {
	in := []Finding{
		{Code: "CrashLoopBackOff", Priority: 7, Severity: SeverityCritical, Container: "web"},
		{Code: "OOMKilled", Priority: 5, Severity: SeverityCritical, Container: "web"},
	}
	verdict, ordered, healthy := resolveVerdict(in)
	if healthy {
		t.Fatalf("not healthy")
	}
	if verdict.Code != "OOMKilled" {
		t.Fatalf("want OOMKilled verdict, got %s", verdict.Code)
	}
	// CrashLoopBackOff for the same container should be suppressed.
	if len(ordered) != 1 {
		t.Fatalf("want 1 finding after suppression, got %d", len(ordered))
	}
}

func TestResolveVerdict_DifferentContainersBothKept(t *testing.T) {
	in := []Finding{
		{Code: "OOMKilled", Priority: 5, Severity: SeverityCritical, Container: "web"},
		{Code: "ImagePullBackOff", Priority: 4, Severity: SeverityCritical, Container: "sidecar"},
	}
	verdict, ordered, healthy := resolveVerdict(in)
	if healthy {
		t.Fatalf("not healthy")
	}
	if verdict.Code != "ImagePullBackOff" {
		t.Fatalf("want ImagePullBackOff verdict (lower priority number), got %s", verdict.Code)
	}
	if len(ordered) != 2 {
		t.Fatalf("want both findings kept (different containers), got %d", len(ordered))
	}
}

func TestResolveVerdict_NoContainerNeverSuppressed(t *testing.T) {
	in := []Finding{
		{Code: "PendingScheduling", Priority: 1, Severity: SeverityCritical},
		{Code: "PendingPVC", Priority: 2, Severity: SeverityCritical},
	}
	_, ordered, _ := resolveVerdict(in)
	if len(ordered) != 2 {
		t.Fatalf("want both pod-level findings kept, got %d", len(ordered))
	}
}
