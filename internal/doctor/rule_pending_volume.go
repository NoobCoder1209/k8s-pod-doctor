package doctor

import (
	"fmt"
	"regexp"

	corev1 "k8s.io/api/core/v1"
)

// volumeRefRegex extracts a volume name from kubelet mount-error messages
// like `MountVolume.SetUp failed for volume "config" : ...`.
var volumeRefRegex = regexp.MustCompile(`volume "([^"]+)"`)

// pendingVolumeRule fires when the pod is blocked on a volume failing to
// attach, mount, or provision. Covers PVCs (most common) but also surfaces
// non-PVC failures (missing Secret, ConfigMap, projected, CSI driver issues).
func pendingVolumeRule(s *Snapshot) []Finding {
	p := s.Pod

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

	// Identify the failing volume — prefer the one named in the event message
	// (correct when the pod has multiple volumes), fall back to the first PVC.
	volName := ""
	if mountEv != nil {
		if m := volumeRefRegex.FindStringSubmatch(mountEv.Message); len(m) == 2 {
			volName = m[1]
		}
	}
	claimName := claimNameForVolume(p, volName)
	if volName == "" {
		volName, claimName = firstPVCRef(p)
	}

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

	var message string
	switch {
	case claimName != "":
		message = fmt.Sprintf("Volume %q (PVC %q) is not ready: %s", volName, claimName, reason)
	case volName != "":
		message = fmt.Sprintf("Volume %q is not ready: %s — %s", volName, reason, oneline(mountEv.Message))
	case mountEv != nil:
		message = fmt.Sprintf("Volume mount failed: %s — %s", reason, oneline(mountEv.Message))
	default:
		message = fmt.Sprintf("Pod cannot finish initialisation: %s", reason)
	}

	evidence := []string{}
	if mountEv != nil {
		evidence = append(evidence, "event "+briefEvent(mountEv))
	} else if initCond != nil {
		evidence = append(evidence,
			fmt.Sprintf("condition PodInitialized=False, reason=%s", initCond.Reason))
	}

	return []Finding{{
		Code:        "PendingVolume",
		Severity:    SeverityCritical,
		Priority:    2,
		Title:       "Pod blocked on volume",
		Message:     message,
		Suggestions: suggestions,
		Evidence:    evidence,
	}}
}

// claimNameForVolume returns the PVC claim name for a given volume name on the
// pod, or "" if the volume is not a PVC.
func claimNameForVolume(p *corev1.Pod, volName string) string {
	if volName == "" {
		return ""
	}
	for _, v := range p.Spec.Volumes {
		if v.Name == volName && v.PersistentVolumeClaim != nil {
			return v.PersistentVolumeClaim.ClaimName
		}
	}
	return ""
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
