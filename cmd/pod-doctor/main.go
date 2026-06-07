// Command pod-doctor is a read-only Kubernetes pod diagnostic CLI.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/NoobCoder1209/k8s-pod-doctor/internal/doctor"
	"github.com/NoobCoder1209/k8s-pod-doctor/internal/render"
)

func main() {
	doctor.SetRenderer(render.Adapter{})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "pod-doctor: %v\n", err)
		os.Exit(1)
	}
}
