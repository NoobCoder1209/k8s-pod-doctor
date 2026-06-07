package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NoobCoder1209/k8s-pod-doctor/internal/doctor"
)

func TestBanner_NoColor_ContainsTitleAndMessage(t *testing.T) {
	var buf bytes.Buffer
	writeBanner(&buf, doctor.SeverityCritical,
		"Container web crash-looping",
		"Container web crashed 7 times. Last exit: 1 (Error).",
		true, // noColor
	)
	out := buf.String()
	if !strings.Contains(out, "CRITICAL") {
		t.Fatalf("missing severity label: %s", out)
	}
	if !strings.Contains(out, "Container web crash-looping") {
		t.Fatalf("missing title: %s", out)
	}
	if !strings.Contains(out, "Container web crashed 7 times.") {
		t.Fatalf("missing message: %s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("ANSI escape leaked despite noColor: %q", out)
	}
}

func TestWrap_BreaksLongInputAtWidth(t *testing.T) {
	lines := wrap("the quick brown fox jumps over the lazy dog and keeps running", 20)
	for _, l := range lines {
		if runeLen(l) > 20 {
			t.Fatalf("line over width: %q (len=%d)", l, runeLen(l))
		}
	}
	if len(lines) < 3 {
		t.Fatalf("want >=3 wrapped lines, got %d: %v", len(lines), lines)
	}
}

func TestPadRight_TruncatesOverWidth(t *testing.T) {
	got := padRight("this is a very long sentence indeed", 10)
	if runeLen(got) != 10 {
		t.Fatalf("want width 10, got %d (%q)", runeLen(got), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("want trailing ellipsis on truncation, got %q", got)
	}
}
