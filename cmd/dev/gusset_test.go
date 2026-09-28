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
