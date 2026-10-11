//go:build !windows

package evidence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/childenv/childenvtest"
)

// The verifier is a child of a process that may hold a model key — `jarvis
// discover` reads GEMINI_API_KEY from its own environment. A stand-in
// verifier records its environment, then answers like a real one.
func TestTheVerifierDoesNotSeeProviderCredentials(t *testing.T) {
	childenvtest.SetParent(t)
	const gemini = "gemini-jarvis-childenvtest-parent"
	t.Setenv("GEMINI_API_KEY", gemini)
	dir := t.TempDir()
	dump := filepath.Join(dir, "env.txt")
	bin := filepath.Join(dir, "dcverify")
	body := "#!/bin/sh\nenv > '" + dump + "'\ncat <<'EOF'\n" + passed + "\nEOF\n"
	if err := os.WriteFile(bin, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(context.Background(), request(bin)); err != nil {
		t.Fatal(err)
	}
	seen, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("the stand-in verifier did not record its environment: %v", err)
	}
	childenvtest.Check(t, string(seen))
	if strings.Contains(string(seen), gemini) {
		t.Errorf("the verifier saw GEMINI_API_KEY:\n%s", seen)
	}
}
