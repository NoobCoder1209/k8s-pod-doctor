// Package render renders a Snapshot + Findings into either a human-readable
// boxed banner + sectioned text, or a stable JSON document.
package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"

	"github.com/NoobCoder1209/k8s-pod-doctor/internal/doctor"
)

// bannerWidth is the total inside width of the verdict banner (between the
// vertical bars). Total visible width is bannerWidth + 2 borders.
const bannerWidth = 60

// glyph picks a one-char glyph for the severity.
func glyph(s doctor.Severity) string {
	switch s {
	case doctor.SeverityCritical:
		return "✖"
	case doctor.SeverityWarning:
		return "▲"
	case doctor.SeverityInfo:
		return "ℹ"
	case doctor.SeverityHealthy:
		return "✔"
	}
	return "•"
}

// colourFor returns a color.Attribute matching the severity. Honours noColor.
func colourFor(s doctor.Severity, noColor bool) *color.Color {
	c := color.New()
	if noColor {
		c.DisableColor()
		return c
	}
	switch s {
	case doctor.SeverityCritical:
		return color.New(color.FgRed, color.Bold)
	case doctor.SeverityWarning:
		return color.New(color.FgYellow, color.Bold)
	case doctor.SeverityInfo:
		return color.New(color.FgCyan, color.Bold)
	case doctor.SeverityHealthy:
		return color.New(color.FgGreen, color.Bold)
	}
	return c
}

// writeBanner writes the boxed verdict banner to w.
//
//	╔══════════════════════════════════════════════════════════╗
//	║  ✖  CRITICAL: Container web crash-looping                ║
//	║  Container web crashed 7 times. Last exit: 1 (Error).    ║
//	╚══════════════════════════════════════════════════════════╝
func writeBanner(w io.Writer, severity doctor.Severity, title, message string, noColor bool) {
	c := colourFor(severity, noColor)
	border := func(s string) string { return c.Sprint(s) }

	fmt.Fprintln(w, border("╔"+strings.Repeat("═", bannerWidth)+"╗"))

	header := fmt.Sprintf("  %s  %s: %s", glyph(severity), strings.ToUpper(string(severity)), title)
	for _, line := range wrap(header, bannerWidth) {
		fmt.Fprintf(w, "%s%s%s\n",
			border("║"),
			padRight(line, bannerWidth),
			border("║"),
		)
	}
	for _, line := range wrap("  "+message, bannerWidth) {
		fmt.Fprintf(w, "%s%s%s\n",
			border("║"),
			padRight(line, bannerWidth),
			border("║"),
		)
	}

	fmt.Fprintln(w, border("╚"+strings.Repeat("═", bannerWidth)+"╝"))
}

// padRight pads s with spaces up to width counted in runes.
func padRight(s string, width int) string {
	n := runeLen(s)
	if n >= width {
		// Truncate with ellipsis.
		return truncate(s, width)
	}
	return s + strings.Repeat(" ", width-n)
}

// wrap word-wraps s into lines of at most width runes. Single words longer
// than width are broken at width.
func wrap(s string, width int) []string {
	var out []string
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	// Preserve leading-space indent: if s starts with spaces, keep them on
	// every wrapped line.
	indent := ""
	for i := 0; i < len(s) && s[i] == ' '; i++ {
		indent += " "
	}

	current := indent
	first := true
	for _, w := range words {
		candidate := current
		if !first && !strings.HasSuffix(current, " ") {
			candidate += " "
		}
		candidate += w
		if runeLen(candidate) > width && !first {
			out = append(out, current)
			current = indent + w
		} else {
			current = candidate
		}
		first = false
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}

func runeLen(s string) int {
	return len([]rune(s))
}

func truncate(s string, width int) string {
	if width < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 1 {
		return string(r[:width])
	}
	return string(r[:width-1]) + "…"
}
