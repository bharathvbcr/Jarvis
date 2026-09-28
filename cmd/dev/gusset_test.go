package main

import (
	"context"
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
