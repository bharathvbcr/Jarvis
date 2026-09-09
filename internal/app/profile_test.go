package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/computer"
)

func TestTenantBindingsCannotWidenBusinessPolicy(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "profiles"), 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../profiles/north.json")
	if err != nil {
		t.Fatal(err)
	}
	var p Profile
	if err = json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	p.ReadOnlyTargets = append(p.ReadOnlyTargets, computer.Selector{Role: "button", Name: "Confirm creation"})
	raw, err = json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "profiles", "north.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, _, err = a.profile("north"); err == nil {
		t.Fatal("binding widened account-changing authorization")
	}
}
func TestBothProfilesBindCanonicalCapability(t *testing.T) {
	a, err := New(context.Background(), Config{Root: "../.."})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	program, err := a.Compile("examples/balance.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []string{"north", "south"} {
		p, _, err := a.profile(tenant)
		if err != nil {
			t.Fatal(err)
		}
		if err = validateBankProgram(program, p); err != nil {
			t.Fatal(err)
		}
	}
}
