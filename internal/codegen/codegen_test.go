package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// The committed example is the contract: regenerating it must be byte-identical.
func TestGoReproducesTheCommittedExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "balance.json"))
	if err != nil {
		t.Fatal(err)
	}
	program, err := workflow.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Go(program)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("..", "..", "examples", "generated", "balance", "capability.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generated source drifted from examples/generated/balance/capability.go")
	}
}

func TestGoRefusesANilProgram(t *testing.T) {
	if _, err := Go(nil); err == nil {
		t.Fatal("nil program accepted")
	}
}
