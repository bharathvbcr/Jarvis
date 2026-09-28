package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/gussetcheck"
)

// errGussetNotLinked is what a cgo-off build answers. `dev build` ships that
// build, so this is the expected answer from the product binary; `dev gusset`
// builds one with the engine linked.
var errGussetNotLinked = errors.New("gusset-check: engine is not linked into this build (CGO_ENABLED=0); run `go run ./cmd/dev gusset` for a linked build")

// gussetCheck proves the in-process Rust engine behind Manvi's serve-plane
// health gate (policy decisions themselves are Go fnmatch): CPython fnmatch parity, the batched match-any path, and a real
// panic inside the linked archive caught at the boundary (I2). It reuses
// Manvi's gussetcheck rather than a Jarvis copy: one engine, one archive per
// binary (R14).
func gussetCheck(ctx context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := gussetcheck.SelfTest(ctx)
	// Bounded, like serve's exit: Close joins without a limit, so a stuck
	// engine would hang the check it is supposed to report on.
	err = errors.Join(err, gussetcheck.Shutdown(2*time.Second))
	_, _ = gussetcheck.DrainLogs(os.Stderr)
	if errors.Is(err, gussetcheck.ErrNotLinked) {
		return errGussetNotLinked
	}
	if err != nil {
		return fmt.Errorf("gusset-check: %w", err)
	}
	_, err = fmt.Fprintln(out, "gusset-check: ok (parity, match-any, panic firewall)")
	return err
}
