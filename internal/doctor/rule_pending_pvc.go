package doctor

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// pendingPVCRule fires when the pod is blocked on persistent volume binding/mount.
func pendingPVCRule(s *Snapshot) []Finding {
	p := s.Pod
	pvcVols := pvcVolumes(p)
	if len(pvcVols) == 0 {
		return nil
	}

	// Look for any volume-related event reason.
	mountEv := firstEventByReason(s, "FailedAttachVolume", "FailedMount", "ProvisioningFailed", "WaitForFirstConsumer")

	// Or condition Initialized=False because containers can't init.
	initCond := podCondition(p, corev1.PodInitialized)
	stuck := initCond != nil && initCond.Status == corev1.ConditionFalse

	if mountEv == nil && !stuck {
		return nil
	}

	// Compose a one-line message naming the first PVC volume we know about.
	var (
		volName, claimName, reason string
	)
	if len(pvcVols) > 0 {
		volName = pvcVols[0].Name
		if pvcVols[0].PersistentVolumeClaim != nil {
			claimName = pvcVols[0].PersistentVolumeClaim.ClaimName
		}
	}
	if mountEv != nil {
		reason = mountEv.Reason
	} else if initCond != nil {
		reason = initCond.Reason
	}

	return []Finding{{
		Code:     "PendingPVC",
		Severity: SeverityCritical,
		Priority: 2,
		Title:    "Pod blocked on persistent volume",
		Message:  fmt.Sprintf("Volume %q (PVC %q) is not ready: %s", volName, claimName, reason),
		Suggestions: []string{
			fmt.Sprintf("kubectl describe pvc %s -n %s", claimName, p.Namespace),
			"Check StorageClass and PV availability: kubectl get sc; kubectl get pv",
		},
		Evidence: []string{
			"event " + briefEvent(mountEv),
		},
	}}
}

func pvcVolumes(p *corev1.Pod) []corev1.Volume {
	var out []corev1.Volume
	for _, v := range p.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			out = append(out, v)
		}
	}
	return out
}
