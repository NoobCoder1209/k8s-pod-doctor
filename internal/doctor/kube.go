package doctor

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// BuildClient builds a kubernetes.Interface honouring the canonical kubeconfig
// precedence: --kubeconfig flag > $KUBECONFIG > ~/.kube/config. Returning an
// interface (not a concrete *Clientset) makes tests substitutable with
// fake.NewSimpleClientset.
func BuildClient(kubeconfig, kubeContext string) (kubernetes.Interface, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	cfg.UserAgent = "k8s-pod-doctor"

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	return cs, nil
}
