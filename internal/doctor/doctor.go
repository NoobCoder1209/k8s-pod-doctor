package doctor

import (
	"context"
	"fmt"
	"io"
)

// Run is the library entry point. The CLI marshals flags into opts and calls
// this. Phase 1 returns a placeholder until the core agent loop lands.
func Run(ctx context.Context, opts Options, out io.Writer) error {
	_ = ctx
	if opts.Output != "" && opts.Output != "text" && opts.Output != "json" {
		return fmt.Errorf("invalid --output %q (want text|json)", opts.Output)
	}
	fmt.Fprintln(out, "pod-doctor: not yet implemented (phase 1 bootstrap)")
	return nil
}
