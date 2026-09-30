package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
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
		{"extra editable field", func(p *PolicyDocument) {
			p.EditableFields = append(p.EditableFields, PolicySelector{Role: "input", Name: "Admin Passcode"})
		}},
		{"removed sensitive target", func(p *PolicyDocument) {
			p.SensitiveTargets = p.SensitiveTargets[:len(p.SensitiveTargets)-1]
		}},
		{"schema version mismatch", func(p *PolicyDocument) { p.SchemaVersion = 99 }},
		{"application mismatch", func(p *PolicyDocument) { p.Application = "other-bank" }},
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

func TestPolicySelectorEqualityCoversVisualGeometry(t *testing.T) {
	base := workflow.VisualAnchor{
		SHA256:       "abc123",
		PNGBase64:    "cG5n",
		Width:        8,
		Height:       8,
		FrameWidth:   32,
		FrameHeight:  32,
		WindowWidth:  32,
		WindowHeight: 32,
		Scale:        1,
		Search:       workflow.PixelRect{Width: 8, Height: 8},
		Click:        workflow.PixelPoint{X: 1, Y: 1},
	}
	left := PolicySelector{Visual: &base}
	same := base
	if !left.equal(PolicySelector{Visual: &same}) {
		t.Fatal("identical visual anchors did not compare equal")
	}
	mutations := []struct {
		name string
		edit func(*workflow.VisualAnchor)
	}{
		{"search width", func(v *workflow.VisualAnchor) { v.Search.Width++ }},
		{"search height", func(v *workflow.VisualAnchor) { v.Search.Height++ }},
		{"click x", func(v *workflow.VisualAnchor) { v.Click.X++ }},
		{"click y", func(v *workflow.VisualAnchor) { v.Click.Y++ }},
		{"scale", func(v *workflow.VisualAnchor) { v.Scale++ }},
		{"frame width", func(v *workflow.VisualAnchor) { v.FrameWidth++ }},
		{"window width", func(v *workflow.VisualAnchor) { v.WindowWidth++ }},
		{"png bytes", func(v *workflow.VisualAnchor) { v.PNGBase64 = "eA==" }},
	}
	doc := PolicyDocument{
		SchemaVersion:      1,
		Application:        "jarvis-bank",
		Windows:            []string{"Jarvis Bank"},
		PermittedStepKinds: []string{"click"},
		PermittedEffects:   []string{"change"},
		EditableFields:     []PolicySelector{left},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			edited := base
			tc.edit(&edited)
			if left.equal(PolicySelector{Visual: &edited}) {
				t.Fatal("visual anchor compared equal after a geometry or template change")
			}
			candidate := doc
			candidate.EditableFields = []PolicySelector{{Visual: &edited}}
			if err := doc.AssertNarrowed(candidate); err == nil {
				t.Fatal("narrowed policy accepted a different visual anchor with the same digest")
			}
		})
	}
}

func TestOverlayWidenErrorNamesVisualAnchor(t *testing.T) {
	policy := PolicyDocument{
		SchemaVersion:      1,
		Application:        "jarvis-bank",
		Windows:            []string{"Jarvis Bank"},
		PermittedStepKinds: []string{"click"},
		PermittedEffects:   []string{"read"},
		ReadOnlyTargets:    []PolicySelector{{Role: "button", Name: "Search"}},
	}
	err := assertOverlayWithinPolicy(workflow.Overlay{
		Targets: map[string]workflow.Selector{
			"icon": {Visual: &workflow.VisualAnchor{SHA256: "abc123"}},
		},
	}, policy)
	if err == nil || !strings.Contains(err.Error(), "visual:abc123") {
		t.Fatalf("overlay widen error = %v", err)
	}
}

func TestPolicySelectorLabel(t *testing.T) {
	cases := []struct {
		selector PolicySelector
		want     string
	}{
		{PolicySelector{Name: "Submit"}, "Submit"},
		{PolicySelector{Identifier: "btn-submit"}, "btn-submit"},
		{PolicySelector{Role: "button"}, "button"},
		{PolicySelector{Visual: &workflow.VisualAnchor{SHA256: "abc123"}}, "visual:abc123"},
	}
	for _, tc := range cases {
		if got := tc.selector.label(); got != tc.want {
			t.Errorf("label() = %q; want %q", got, tc.want)
		}
	}
}
