//go:build !windows

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/childenv/childenvtest"
)

// The owned bank is a child of a process that may hold a model key — `jarvis
// discover` reads GEMINI_API_KEY from its own environment. A stand-in bank
// records its environment and exits.
func TestTheOwnedBankDoesNotSeeProviderCredentials(t *testing.T) {
	childenvtest.SetParent(t)
	const gemini = "gemini-jarvis-childenvtest-parent"
	t.Setenv("GEMINI_API_KEY", gemini)
	dir := t.TempDir()
	dump := filepath.Join(dir, "env.txt")
	bin := filepath.Join(dir, "jarvis-bank")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nenv > '"+dump+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	b, err := startOwnedBank(context.Background(), bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.stop() })
	select {
	case <-b.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the stand-in bank did not exit")
	}
	if b.exitErr != nil {
		t.Fatalf("the stand-in bank failed: %v", b.exitErr)
	}
	seen, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("the stand-in bank did not record its environment: %v", err)
	}
	childenvtest.Check(t, string(seen))
	if strings.Contains(string(seen), gemini) {
		t.Errorf("the owned bank saw GEMINI_API_KEY:\n%s", seen)
	}
}
