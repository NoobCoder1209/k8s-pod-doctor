package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/NoobCoder1209/k8s-pod-doctor/internal/doctor"
	"github.com/NoobCoder1209/k8s-pod-doctor/internal/version"
)

func newRootCmd() *cobra.Command {
	opts := doctor.Options{}

	cmd := &cobra.Command{
		Use:   "pod-doctor [flags] <namespace> <pod-name>",
		Short: "Diagnose unhealthy Kubernetes pods (read-only)",
		Long: `pod-doctor inspects a Kubernetes pod and explains why it is unhealthy:
status, recent events, container log tails, and a verdict naming the most
common failure modes (CrashLoopBackOff, OOMKilled, ImagePullBackOff,
scheduling failures, PVC issues, probe failures, init container failures).

It is strictly read-only — never exec, delete, or patch.

Examples:
  pod-doctor default web-7d9f-abc
  pod-doctor --kubeconfig ~/.kube/dev kube-system coredns-xyz
  pod-doctor --all-failing -o json | jq '.[] | .verdict'
`,
		Args: cobra.MatchAll(validateArgsOrAllFailing(&opts)),

		// Both silenced: cobra won't print errors or usage on RunE failure.
		// main.go formats the single-line stderr message itself.
		SilenceUsage:  true,
		SilenceErrors: true,

		RunE: func(cmd *cobra.Command, args []string) error {
			if !opts.AllFailing {
				opts.Namespace, opts.PodName = args[0], args[1]
			}
			if _, ok := os.LookupEnv("NO_COLOR"); ok {
				opts.NoColor = true
			}
			return doctor.Run(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.Kubeconfig, "kubeconfig", "", "path to kubeconfig (overrides $KUBECONFIG)")
	f.StringVar(&opts.Context, "context", "", "kubeconfig context to use")
	f.StringVarP(&opts.Output, "output", "o", "text", "output format: text|json")
	f.Int64Var(&opts.Tail, "tail", 100, "lines of recent logs to include per container")
	f.BoolVar(&opts.AllFailing, "all-failing", false, "diagnose every non-Running pod in the cluster")
	f.BoolVarP(&opts.Verbose, "verbose", "v", false, "include extra diagnostic detail")
	f.BoolVar(&opts.NoColor, "no-color", false, "disable ANSI colour output (also via NO_COLOR env)")

	cmd.AddCommand(newVersionCmd())
	return cmd
}

// validateArgsOrAllFailing enforces: exactly 2 positional args XOR --all-failing.
func validateArgsOrAllFailing(opts *doctor.Options) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		switch {
		case opts.AllFailing && len(args) > 0:
			return errors.New("--all-failing cannot be combined with positional arguments")
		case opts.AllFailing:
			return nil
		case len(args) == 2:
			return nil
		case len(args) == 0:
			return errors.New("provide <namespace> <pod-name>, or pass --all-failing")
		default:
			return fmt.Errorf("expected <namespace> <pod-name> (2 args), got %d", len(args))
		}
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "pod-doctor %s (%s, %s)\n",
				version.Version, version.Commit, version.Date)
			return nil
		},
	}
}
