package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/computer"
)

func TestPolicyHashStable(t *testing.T) {
	raw, err := os.ReadFile("../../policies/jarvis-bank.json")
	if err != nil {
		t.Fatal(err)
	}
	first := PolicySHA256(raw)
	second := PolicySHA256(append([]byte(nil), raw...))
	if first == "" || first != second {
		t.Fatalf("PolicySHA256 unstable: %q vs %q", first, second)
	}
	if PolicySHA256(append(append([]byte(nil), raw...), '\n')) == first {
		t.Fatal("PolicySHA256 ignored byte changes")
	}
}

func TestAssertNarrowedRefusesWiden(t *testing.T) {
	raw, err := os.ReadFile("../../policies/jarvis-bank.json")
	if err != nil {
		t.Fatal(err)
	}
	var base PolicyDocument
	if err = json.Unmarshal(raw, &base); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		mut  func(*PolicyDocument)
	}{
		{"extra window", func(p *PolicyDocument) { p.Windows = append(p.Windows, "Unauthorized Window") }},
		{"extra read target", func(p *PolicyDocument) {
			p.ReadOnlyTargets = append(p.ReadOnlyTargets, PolicySelector{Role: "button", Name: "Confirm creation"})
		}},
		{"extra step kind", func(p *PolicyDocument) { p.PermittedStepKinds = append(p.PermittedStepKinds, "key_combo") }},
		{"extra effect", func(p *PolicyDocument) { p.PermittedEffects = append(p.PermittedEffects, "admin") }},
		{"unattended_change true", func(p *PolicyDocument) { p.UnattendedChange = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base
			candidate.Windows = append([]string{}, base.Windows...)
			candidate.EditableFields = append([]PolicySelector{}, base.EditableFields...)
			candidate.ReadOnlyTargets = append([]PolicySelector{}, base.ReadOnlyTargets...)
			candidate.PermittedStepKinds = append([]string{}, base.PermittedStepKinds...)
			candidate.PermittedEffects = append([]string{}, base.PermittedEffects...)
			candidate.SensitiveTargets = append([]PolicySelector{}, base.SensitiveTargets...)
			tc.mut(&candidate)
			if err := base.AssertNarrowed(candidate); err == nil {
				t.Fatal("expected widen refusal")
			}
		})
	}
}

func TestProfileCannotWidenReadOnlyTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "profiles"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := copyPolicyFixture(root); err != nil {
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
	if _, err = a.profile("north"); err == nil {
		t.Fatal("profile widened read_only_targets beyond reviewed policy")
	}
}
