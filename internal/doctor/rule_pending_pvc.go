package doctor

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// pendingVolumeRule fires when the pod is blocked on a volume failing to
// attach, mount, or provision. Covers PVCs (most common) but also surfaces
// non-PVC failures (missing Secret, ConfigMap, projected, CSI driver issues).
func pendingVolumeRule(s *Snapshot) []Finding {
	p := s.Pod

	// Look for any volume-related event reason.
	mountEv := firstEventByReason(s,
		"FailedAttachVolume", "FailedMount",
		"ProvisioningFailed", "WaitForFirstConsumer",
	)

	initCond := podCondition(p, corev1.PodInitialized)
	stuckOnInit := initCond != nil && initCond.Status == corev1.ConditionFalse

	if mountEv == nil {
		// Without a mount event, only fire when init is genuinely blocked AND
		// the pod has at least one volume that is allowed to be the cause.
		if !stuckOnInit || len(p.Spec.Volumes) == 0 {
			return nil
		}
	}

	volName, claimName := firstPVCRef(p)
	reason := ""
	switch {
	case mountEv != nil:
		reason = mountEv.Reason
	case initCond != nil:
		reason = initCond.Reason
	}

	suggestions := []string{
		"kubectl describe pod " + p.Namespace + "/" + p.Name + " | grep -A5 Events",
	}
	if claimName != "" {
		suggestions = append(suggestions,
			fmt.Sprintf("kubectl describe pvc %s -n %s", claimName, p.Namespace))
	}
	suggestions = append(suggestions,
		"Check StorageClass / Secret / ConfigMap referenced by the pod's volumes")

	message := "A volume required by this pod is not ready"
	if claimName != "" {
		message = fmt.Sprintf("Volume %q (PVC %q) is not ready: %s", volName, claimName, reason)
	} else if mountEv != nil {
		message = fmt.Sprintf("Volume mount failed: %s — %s", reason, oneline(mountEv.Message))
	} else {
		message = fmt.Sprintf("Pod cannot finish initialisation: %s", reason)
	}

	return []Finding{{
		Code:        "PendingVolume",
		Severity:    SeverityCritical,
		Priority:    2,
		Title:       "Pod blocked on volume",
		Message:     message,
		Suggestions: suggestions,
		Evidence: []string{
			"event " + briefEvent(mountEv),
		},
	}}
}

// firstPVCRef returns the first PVC volume's (volume name, claim name) — or
// empty strings if the pod has no PVC volume.
func firstPVCRef(p *corev1.Pod) (string, string) {
	for _, v := range p.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			return v.Name, v.PersistentVolumeClaim.ClaimName
		}
	}
	return "", ""
}
