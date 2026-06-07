// Package doctor exposes the pod-doctor library entry point.
package doctor

// Options is the wire between the CLI parser and the library Run.
// All flag/env state lives here — the CLI marshals into it, then calls Run.
type Options struct {
	Namespace  string `json:"namespace,omitempty"`
	PodName    string `json:"podName,omitempty"`
	AllFailing bool   `json:"allFailing,omitempty"`

	Kubeconfig string `json:"kubeconfig,omitempty"`
	Context    string `json:"context,omitempty"`

	Output  string `json:"output,omitempty"` // "text" | "json"
	Tail    int64  `json:"tail,omitempty"`
	NoColor bool   `json:"noColor,omitempty"`
}
