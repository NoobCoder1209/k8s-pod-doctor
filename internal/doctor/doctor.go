package doctor

import (
	"context"
	"fmt"
	"io"
)

// Run is the library entry point. The CLI marshals flags into opts and calls
// this. Phase 1 returns a placeholder until the core agent loop lands.
func Run(_ context.Context, opts Options, out io.Writer) error {
	_ = opts // phase 2 wires opts to the diagnose package.
	fmt.Fprintln(out, "pod-doctor: not yet implemented (phase 1 bootstrap)")
	return nil
}
