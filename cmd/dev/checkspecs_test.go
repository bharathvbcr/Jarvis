package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// runDev runs the dev command line from dir with PATH set to path, as
// `go run ./cmd/dev <args>` would from that directory.
func runDev(t *testing.T, dir, path string, args ...string) error {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	saved := os.Args
	os.Args = append([]string{"dev"}, args...)
	t.Cleanup(func() { os.Args = saved })
	t.Setenv("PATH", path)
	return run()
}

// check-specs is the spec gate: it runs exactly `devcouncil spec validate
// --all --strict` from the repository root, passes when that passes, and fails
// when it fails or when devcouncil is not installed. A gate that could not run
// must not report the same result as one that ran and passed.
func TestCheckSpecsRunsTheStrictSpecGateFromTheRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake devcouncil is a shell script")
	}
	bin := t.TempDir()
	record := filepath.Join(t.TempDir(), "devcouncil.args")
	fake := "#!/bin/sh\nprintf '%s\\n' \"$(pwd -P)\" \"$*\" > \"$FAKE_DEVCOUNCIL_RECORD\"\nexit \"${FAKE_DEVCOUNCIL_EXIT:-0}\"\n"
	if err := os.WriteFile(filepath.Join(bin, "devcouncil"), []byte(fake), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_DEVCOUNCIL_RECORD", record)
	root := t.TempDir()
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("FAKE_DEVCOUNCIL_EXIT", "0")
	if err := runDev(t, root, bin, "check-specs"); err != nil {
		t.Fatalf("passing gate reported failure: %v", err)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("devcouncil was not run: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
	if len(lines) != 2 || lines[0] != want || lines[1] != "spec validate --all --strict" {
		t.Fatalf("ran %q, want cwd %q and args %q", lines, want, "spec validate --all --strict")
	}

	t.Setenv("FAKE_DEVCOUNCIL_EXIT", "1")
	if err := runDev(t, root, bin, "check-specs"); err == nil {
		t.Fatal("failing gate reported success")
	}

	if err := runDev(t, root, t.TempDir(), "check-specs"); err == nil {
		t.Fatal("missing devcouncil binary reported success")
	}
}
