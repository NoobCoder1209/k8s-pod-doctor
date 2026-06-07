package doctor

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

var imagePullReasons = map[string]bool{
	"ErrImagePull":        true,
	"ImagePullBackOff":    true,
	"ImageInspectError":   true,
	"InvalidImageName":    true,
	"RegistryUnavailable": true,
}

// imagePullBackOffRule fires when a container cannot pull its image.
func imagePullBackOffRule(s *Snapshot) []Finding {
	p := s.Pod
	var out []Finding

	for _, cs := range p.Status.ContainerStatuses {
		f := pullFailureForContainer(p, cs, false)
		if f != nil {
			out = append(out, *f)
		}
	}
	// Init containers handled by initContainerFailureRule (which delegates),
	// but if the pod is otherwise scheduled fine and only init has the issue,
	// the init rule already wins. Don't double-report.
	return out
}

func pullFailureForContainer(p *corev1.Pod, cs corev1.ContainerStatus, isInit bool) *Finding {
	if cs.State.Waiting == nil {
		return nil
	}
	reason := cs.State.Waiting.Reason
	if !imagePullReasons[reason] {
		return nil
	}
	image := cs.Image
	if image == "" {
		image = lookupImageInSpec(p, cs.Name, isInit)
	}
	return &Finding{
		Code:      "ImagePullBackOff",
		Severity:  SeverityCritical,
		Priority:  4,
		Title:     fmt.Sprintf("Container %s cannot pull image", cs.Name),
		Message:   fmt.Sprintf("Image %q cannot be pulled: %s — %s", image, reason, oneline(cs.State.Waiting.Message)),
		Container: cs.Name,
		Suggestions: []string{
			fmt.Sprintf("Verify image exists and tag is correct: docker pull %s", image),
			fmt.Sprintf("Check imagePullSecrets and registry credentials in namespace %s", p.Namespace),
		},
		Evidence: []string{
			fmt.Sprintf("state.waiting.reason=%s", reason),
			fmt.Sprintf("state.waiting.message=%s", oneline(cs.State.Waiting.Message)),
		},
	}
}

func lookupImageInSpec(p *corev1.Pod, name string, isInit bool) string {
	src := p.Spec.Containers
	if isInit {
		src = p.Spec.InitContainers
	}
	for _, c := range src {
		if c.Name == name {
			return c.Image
		}
	}
	return ""
}
