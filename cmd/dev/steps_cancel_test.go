//go:build unix

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A step that exits 0 but leaves a child holding its output pipe is not a
// failed step: its tests passed. Output now goes through a pipe, where it used
// to go to an inherited file, and exec fails a command whose pipe stays open
// past WaitDelay even when the command exited 0. The step must pass, say what
// it left behind, and stop the leftover rather than orphan it.
func TestAStepThatLeavesAChildHoldingItsOutputStillPassesAndStopsTheChild(t *testing.T) {
	pidFile := t.TempDir() + "/leftover.pid"
	var log strings.Builder
	steps := []testStep{{id: "leaves-child", dir: t.TempDir(), name: "sh", args: []string{"-c", "sleep 60 & echo $! > " + pidFile + "; exit 0"}}}
	start := time.Now()
	if err := runSteps(context.Background(), steps, false, &log, execTestStep); err != nil {
		t.Fatalf("a step that exited 0 failed: %v\n%s", err, log.String())
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("the step held the plan for %s", time.Since(start))
	}
	if !strings.Contains(log.String(), "left a process holding its output") {
		t.Fatalf("the leftover was not reported:\n%s", log.String())
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("leftover child %d was not stopped", pid)
		}
	}
}

// A failure cancels the steps still running. exec's cancel kills only the
// direct child, so `go test` and `cargo` would die while the test binaries and
// rustc under them ran on, holding CPU and a host's build slot. The step is a
// shell that starts a grandchild and waits for it, which is the same shape.
func TestCancellingAStepStopsItsGrandchildren(t *testing.T) {
	pidFile := t.TempDir() + "/grandchild.pid"
	steps := []testStep{
		{id: "tree", dir: t.TempDir(), name: "sh", args: []string{"-c", "sleep 60 & echo $! > " + pidFile + "; wait"}},
		{id: "boom", name: "go"},
	}
	// boom is a stand-in that waits for the grandchild to exist and then fails,
	// so the failure lands while tree is still running.
	run := func(ctx context.Context, s testStep, w io.Writer) error {
		if s.id == "boom" {
			deadline := time.Now().Add(10 * time.Second)
			for {
				if raw, err := os.ReadFile(pidFile); err == nil && strings.HasSuffix(string(raw), "\n") {
					return errors.New("exit status 1")
				}
				if time.Now().After(deadline) {
					return errors.New("the grandchild never started")
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
		return execTestStep(ctx, s, w)
	}
	if err := runSteps(context.Background(), steps, false, io.Discard, run); err == nil || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("plan error = %v, want boom's failure", err)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d outlived its cancelled step", pid)
		}
	}
}
