package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// gussetEnv returns the cgo environment for a Jarvis that links the gusset
// engine, from DevCouncil's rust/gusset-engine/cgo-env.sh. That script builds
// the umbrella archive (the one Rust staticlib a Go binary may link, R14) and
// keys Go's caches on its hash: Go hashes cgo flags, not the archives they
// name, so without the key a rebuilt archive is not relinked and a cached
// "ok" is replayed against the old one.
//
// Manvi's go.mod replaces DevCouncil and gusset with the siblings of its own
// checkout. A --gusset or --devcouncil pointing anywhere else would build
// against one tree and report on another, so that layout is refused here
// rather than discovered as a confusing module error.
func gussetEnv(ctx context.Context, manvi, dc, gusset string) ([]string, error) {
	parent := filepath.Dir(manvi)
	for _, want := range []struct{ name, path string }{{"DevCouncil", dc}, {"gusset", gusset}} {
		if filepath.Clean(want.path) != filepath.Join(parent, want.name) {
			return nil, fmt.Errorf("%s must be checked out at %s, where Manvi's go.mod replace looks; got %s", want.name, filepath.Join(parent, want.name), want.path)
		}
	}
	if _, err := os.Stat(filepath.Join(gusset, "go.mod")); err != nil {
		return nil, fmt.Errorf("gusset checkout missing (run `go run ./cmd/dev bootstrap`): %w", err)
	}
	script := filepath.Join(dc, "rust", "gusset-engine", "cgo-env.sh")
	if _, err := os.Stat(script); err != nil {
		return nil, fmt.Errorf("DevCouncil checkout predates rust/gusset-engine/cgo-env.sh: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", script)
	cmd.Stderr = os.Stderr
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cgo-env.sh: %w", err)
	}
	return parseCgoEnv(out)
}

// parseCgoEnv reads cgo-env.sh's KEY=VALUE lines and insists on the three
// the linked build depends on.
func parseCgoEnv(out []byte) ([]string, error) {
	var env []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(bytes.TrimSpace(out)), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			continue
		}
		switch key {
		case "CGO_ENABLED", "CGO_LDFLAGS", "CGO_CFLAGS":
			seen[key] = true
			env = append(env, key+"="+value)
		}
	}
	for _, key := range []string{"CGO_ENABLED", "CGO_LDFLAGS", "CGO_CFLAGS"} {
		if !seen[key] {
			return nil, errors.New("cgo-env.sh did not print " + key)
		}
	}
	return env, nil
}
