package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func TestCompileForTenantAppliesOverlayAndDigest(t *testing.T) {
	root := t.TempDir()
	if err := seedOverlayFixture(root); err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	base, err := a.Compile("examples/balance.json")
	if err != nil {
		t.Fatal(err)
	}
	withOverlay, binding, err := a.CompileForTenant("examples/balance.json", "south")
	if err != nil {
		t.Fatal(err)
	}
	if binding.Overlay == nil || binding.Profile.Overlay == "" {
		t.Fatal("south profile did not load overlay")
	}
	if withOverlay.Capability().Targets["search"].Primary().Name != "Find member" {
		t.Fatalf("unexpected search ladder: %+v", withOverlay.Capability().Targets["search"])
	}
	if len(withOverlay.Capability().Targets["search"].Ladder()) != 2 {
		t.Fatalf("overlay did not extend ladder: %+v", withOverlay.Capability().Targets["search"])
	}
	if withOverlay.Capability().Targets["search"].Ladder()[1].Name != "Search" {
		t.Fatalf("overlay fallback rung missing: %+v", withOverlay.Capability().Targets["search"])
	}
	if withOverlay.Digest() == base.Digest() {
		t.Fatal("overlay digest matched base-only digest")
	}
	if withOverlay.Digest() != workflow.OverlayDigest(mustRead(t, filepath.Join(root, "examples", "balance.json")), binding.OverlayBytes) {
		t.Fatal("CompileForTenant digest mismatch vs OverlayDigest")
	}
}

func TestOverlayCannotWidenPolicy(t *testing.T) {
	root := t.TempDir()
	if err := seedOverlayFixture(root); err != nil {
		t.Fatal(err)
	}
	wide := workflow.Overlay{
		SchemaVersion: 1,
		CapabilityID:  "bank.balance",
		Revision:      "1",
		Tenant:        "south",
		Targets: map[string]workflow.Selector{
			"search": {
				Strategies: []workflow.Selector{{Role: "button", Name: "Confirm creation"}},
				Rationale:  "unauthorized control",
				Stability:  "semantic",
			},
		},
	}
	raw, err := json.Marshal(wide)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "overlays", "south-balance.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err = a.profile("south"); err == nil {
		t.Fatal("overlay widened policy with Confirm creation")
	}
}

func TestBuildDriftReportRungIndexAndAmbiguity(t *testing.T) {
	raw, err := os.ReadFile("../../examples/balance.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := workflow.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	obs := computer.Observation{Complete: true, Nodes: []computer.Node{
		{ID: "1", Role: "text_field", Name: "Member ID", Enabled: true, Editable: true},
		{ID: "2", Role: "button", Name: "Search", Enabled: true},
		{ID: "3", Role: "button", Name: "Search", Enabled: true},
		{ID: "4", Role: "text_field", Name: "Status"},
		{ID: "5", Role: "text_field", Name: "Balance"},
	}}
	report := BuildDriftReport(p, "north", "", "Jarvis Bank — North Cooperative", obs)
	byTarget := map[string]computer.TargetDrift{}
	for _, hit := range report.Targets {
		byTarget[hit.Target] = hit
	}
	if !byTarget["member"].Found || byTarget["member"].StrategyIndex != 0 {
		t.Fatalf("member: %+v", byTarget["member"])
	}
	if byTarget["search"].Found || !byTarget["search"].Ambiguous || byTarget["search"].StrategyIndex != 0 {
		t.Fatalf("search ambiguity: %+v", byTarget["search"])
	}
	if !byTarget["balance"].Found {
		t.Fatalf("balance: %+v", byTarget["balance"])
	}
}

func seedOverlayFixture(root string) error {
	for _, dir := range []string{"profiles", "policies", "overlays", "examples"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			return err
		}
	}
	if err := copyPolicyFixture(root); err != nil {
		return err
	}
	capRaw, err := os.ReadFile("../../examples/balance.json")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "examples", "balance.json"), capRaw, 0600); err != nil {
		return err
	}
	north, err := os.ReadFile("../../profiles/north.json")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "profiles", "north.json"), north, 0600); err != nil {
		return err
	}
	overlay := workflow.Overlay{
		SchemaVersion: 1,
		CapabilityID:  "bank.balance",
		Revision:      "1",
		Tenant:        "south",
		Targets: map[string]workflow.Selector{
			"search": {
				Strategies: []workflow.Selector{
					{Role: "button", Name: "Find member"},
					{Role: "button", Name: "Search"},
				},
				Rationale: "Find member primary for renamed-controls; Search as drift fallback",
				Stability: "semantic",
			},
		},
		Limits: &workflow.Limits{MaxActions: 30, ActiveSeconds: 300, ObservationAttempts: 3, InterventionSeconds: 600},
	}
	overlayRaw, err := json.Marshal(overlay)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "overlays", "south-balance.json"), overlayRaw, 0600); err != nil {
		return err
	}
	south, err := os.ReadFile("../../profiles/south.json")
	if err != nil {
		return err
	}
	var profile Profile
	if err = json.Unmarshal(south, &profile); err != nil {
		return err
	}
	profile.Overlay = "overlays/south-balance.json"
	southRaw, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "profiles", "south.json"), southRaw, 0600)
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
