package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCgoEnvKeepsOnlyTheCgoKeys(t *testing.T) {
	env, err := parseCgoEnv([]byte("CGO_ENABLED=1\nCGO_LDFLAGS=-L/x/target/release\nCGO_CFLAGS=-O2 -g -DDEVCOUNCIL_GUSSET_ENGINE_SHA256=ab\nPATH=/evil\n"))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "PATH=") || !strings.Contains(joined, "CGO_CFLAGS=-O2 -g -DDEVCOUNCIL_GUSSET_ENGINE_SHA256=ab") {
		t.Fatalf("env = %q", env)
	}
}

func TestParseCgoEnvRefusesAPartialEnvironment(t *testing.T) {
	if _, err := parseCgoEnv([]byte("CGO_ENABLED=1\nCGO_LDFLAGS=-L/x\n")); err == nil {
		t.Fatal("an environment without the cache key was accepted")
	}
}

// Manvi's go.mod replaces gusset and DevCouncil with its own siblings. A
// gusset anywhere else would be built against one tree and reported on
// another.
func TestGussetEnvRefusesALayoutManviWouldNotResolve(t *testing.T) {
	root := t.TempDir()
	manvi := filepath.Join(root, "Manvi")
	_, err := gussetEnv(context.Background(), manvi, filepath.Join(root, "DevCouncil"), filepath.Join(root, "elsewhere", "gusset"))
	if err == nil || !strings.Contains(err.Error(), "where Manvi's go.mod replace looks") {
		t.Fatalf("got %v", err)
	}
	_, err = gussetEnv(context.Background(), manvi, filepath.Join(root, "DevCouncil"), filepath.Join(root, "gusset"))
	if err == nil || !strings.Contains(err.Error(), "gusset checkout missing") {
		t.Fatalf("got %v", err)
	}
}

// A symlinked sibling is the directory Manvi's replace resolves; a textual
// comparison refused it.
func TestGussetEnvAcceptsASymlinkedSibling(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(t.TempDir(), "gusset-real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(root, "gusset")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if !sameDir(filepath.Join(root, "gusset"), real) {
		t.Fatal("a symlink and its target are not the same directory")
	}
	if sameDir(filepath.Join(root, "gusset"), root) {
		t.Fatal("different directories compared equal")
	}
}

// A lock file from before the gusset pin names what is missing.
func TestReadPinsNamesAMissingGussetPin(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	old := `{"schema_version":1,"manvi":{"url":"https://github.com/bharathvbcr/Manvi.git","revision":"af705d89137bd83fd4e8e0b7aa79eefdc340dd25"},"devcouncil":{"url":"https://github.com/bharathvbcr/DevCouncil.git","revision":"3197897490bff4fe2607d885cfe369ecc53363fb"}}`
	if err := os.WriteFile("upstream.lock.json", []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = readPins()
	if err == nil || !strings.Contains(err.Error(), "no gusset pin") {
		t.Fatalf("readPins() = %v, want a missing-gusset-pin error", err)
	}
}

// Manvi's and DevCouncil's own pins must name the revisions the lock pins;
// a lock bumped without them builds against one gusset while their CI
// proves another.
func TestUpstreamPinsMustAgree(t *testing.T) {
	root := t.TempDir()
	manvi, dc := filepath.Join(root, "Manvi"), filepath.Join(root, "DevCouncil")
	g, d := "709c2b7b85be0de7c263471d933f395bb3b1b27f", "663b998d7265ca6a14ff72492ed17064df19fe05"
	pins := lock{DevCouncil: source{Revision: d}, Gusset: source{Revision: g}}
	write := func(dir, rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	modulePins := func(dcRev, gRev string) string {
		return "# pins\nDevCouncil\thttps://github.com/bharathvbcr/DevCouncil.git\t" + dcRev +
			"\ngusset\thttps://github.com/bharathvbcr/gusset.git\t" + gRev + "\n"
	}
	write(manvi, "scripts/module-pins.txt", modulePins(d, g))
	write(dc, ".github/actions/setup-gusset/action.yml", "      with:\n        ref: "+g+"\n")
	if err := checkUpstreamPinsAgree(manvi, dc, pins); err != nil {
		t.Fatalf("agreeing pins refused: %v", err)
	}
	stale := "b6f5200bfff4730e4e5268bead7e743d684cb67e"
	write(manvi, "scripts/module-pins.txt", modulePins(d, stale))
	if err := checkUpstreamPinsAgree(manvi, dc, pins); err == nil || !strings.Contains(err.Error(), "module-pins.txt pins gusset") {
		t.Fatalf("Manvi pinning another gusset = %v", err)
	}
	write(manvi, "scripts/module-pins.txt", modulePins(d, g))
	write(dc, ".github/actions/setup-gusset/action.yml", "        ref: "+stale+"\n")
	if err := checkUpstreamPinsAgree(manvi, dc, pins); err == nil || !strings.Contains(err.Error(), "setup-gusset") {
		t.Fatalf("DevCouncil pinning another gusset = %v", err)
	}
}
