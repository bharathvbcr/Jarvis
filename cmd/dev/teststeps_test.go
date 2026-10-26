package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// dc-verify's json_contract audits DevCouncil's Go host and fails, by design,
// when there is none. `dev test` ran it without one, so it failed on every
// clean bootstrap. The plan must build the host first and hand it over.
func TestTestStepsBuildTheDevCouncilHostBeforeDcVerify(t *testing.T) {
	root, manvi, dc := "/j", "/j/.local/upstream/Manvi", "/j/.local/upstream/DevCouncil"
	host := filepath.Join(t.TempDir(), "devcouncil")
	engine := []string{"CGO_ENABLED=1", "CGO_LDFLAGS=-L/x", "CGO_CFLAGS=-DKEY=1"}
	steps := testSteps(root, manvi, dc, host, engine)

	build := slices.IndexFunc(steps, func(s testStep) bool {
		return s.name == "go" && slices.Contains(s.args, "./cmd/devcouncil")
	})
	verify := slices.IndexFunc(steps, func(s testStep) bool {
		return s.name == "cargo" && slices.Contains(s.args, "dc-verify")
	})
	if build < 0 || verify < 0 {
		t.Fatalf("plan lacks the host build (%d) or the dc-verify leg (%d): %+v", build, verify, steps)
	}
	if build > verify {
		t.Fatalf("host is built at step %d, after dc-verify runs at step %d", build, verify)
	}
	b := steps[build]
	if b.dir != filepath.Join(dc, "backend", "go_orchestrator") || !slices.Equal(b.args, []string{"build", "-o", host, "./cmd/devcouncil"}) {
		t.Fatalf("host build = %+v", b)
	}
	// Jarvis's go.work sits above .local/upstream, so without GOWORK=off the
	// DevCouncil module is outside the workspace and does not build. gusset
	// refuses to compile without cgo, so the engine environment must reach it.
	for _, want := range append([]string{"GOWORK=off"}, engine...) {
		if !slices.Contains(b.env, want) {
			t.Fatalf("host build env %q lacks %q", b.env, want)
		}
	}
	if !slices.Contains(steps[verify].env, "DEVCOUNCIL_BIN="+host) {
		t.Fatalf("dc-verify env %q does not name the host it was built for", steps[verify].env)
	}
	if rel, err := filepath.Rel(dc, host); err == nil && !strings.HasPrefix(rel, "..") {
		t.Fatalf("host %s is inside the DevCouncil checkout, which check-pins requires clean", host)
	}
}

// The step must actually build when the checkout sits under a directory whose
// go.work does not list it, which is where bootstrap puts it.
func TestDevCouncilHostBuildsUnderAnEnclosingWorkspace(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", []byte("module example.com/host\n\ngo 1.26.6\n"))
	writeFixture(t, root, "go.work", []byte("go 1.26.6\n\nuse .\n"))
	manvi, dc := filepath.Join(root, ".local/upstream/Manvi"), filepath.Join(root, ".local/upstream/DevCouncil")
	orchestrator := filepath.Join(dc, "backend", "go_orchestrator")
	writeFixture(t, orchestrator, "go.mod", []byte("module github.com/bharathvbcr/DevCouncil/backend/go_orchestrator\n\ngo 1.26.6\n"))
	writeFixture(t, orchestrator, "cmd/devcouncil/main.go", []byte("package main\n\nfunc main() {}\n"))
	host := filepath.Join(t.TempDir(), "devcouncil")

	steps := testSteps(root, manvi, dc, host, nil)
	i := slices.IndexFunc(steps, func(s testStep) bool { return slices.Contains(s.args, "./cmd/devcouncil") })
	if i < 0 {
		t.Fatal("plan has no host build")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	s := steps[i]
	cmd := exec.CommandContext(ctx, s.name, s.args...)
	cmd.Dir = s.dir
	// GOWORK= resets any inherited value, so only the step's env decides.
	cmd.Env = append(append(os.Environ(), "GOWORK=", "CGO_ENABLED=0", "GOPROXY=off", "GOTOOLCHAIN=local"), s.env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("host build under an enclosing go.work: %v\n%s", err, out)
	}
	if info, err := os.Stat(host); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("host not built at %s: %v", host, err)
	}
}
