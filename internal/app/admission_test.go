package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func TestBankProfilesAdmitOnlyJarvisBank(t *testing.T) {
	a, err := New(context.Background(), Config{Root: "../.."})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for _, tenant := range []string{"north", "south"} {
		program, binding, err := a.CompileForTenant("examples/balance.json", tenant)
		if err != nil {
			t.Fatal(tenant, err)
		}
		if program.Capability().Application != BankApplication || binding.Profile.Application != BankApplication {
			t.Fatalf("%s admitted %q", tenant, program.Capability().Application)
		}
		foreign := writeCapability(t, t.TempDir(), "fixture-app")
		if _, _, err = a.CompileForTenant(foreign, tenant); err == nil {
			t.Fatalf("%s admitted a capability from another application", tenant)
		}
	}
}

func TestFixtureApplicationAdmittedOnlyUnderItsProfile(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"profiles", "policies", "examples"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyPolicyFixture(root); err != nil {
		t.Fatal(err)
	}
	balance, err := os.ReadFile("../../examples/balance.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "examples", "balance.json"), balance, 0600); err != nil {
		t.Fatal(err)
	}
	north, err := os.ReadFile("../../profiles/north.json")
	if err != nil {
		t.Fatal(err)
	}
	var south Profile
	if err = json.Unmarshal(north, &south); err != nil {
		t.Fatal(err)
	}
	south.Tenant = "south"
	southRaw, err := json.Marshal(south)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "profiles", "south.json"), southRaw, 0600); err != nil {
		t.Fatal(err)
	}
	fixturePolicy := PolicyDocument{
		SchemaVersion:      1,
		Application:        "fixture-app",
		Windows:            []string{"Fixture Window"},
		PermittedStepKinds: []string{"set_value", "extract"},
		PermittedEffects:   []string{"read"},
	}
	policyRaw, err := json.Marshal(fixturePolicy)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "policies", "fixture-app.json"), policyRaw, 0600); err != nil {
		t.Fatal(err)
	}
	fixtureProfile := Profile{
		SchemaVersion: 1,
		Tenant:        "north",
		Application:   "fixture-app",
		Policy:        "fixture-app",
		Platforms:     []string{runtime.GOOS},
		WindowTitle:   "Fixture Window",
		Parameters:    []workflow.Parameter{{Name: "note", Type: "string", Sensitive: true}},
	}
	profileRaw, err := json.Marshal(fixtureProfile)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "profiles", "north.json"), profileRaw, 0600); err != nil {
		t.Fatal(err)
	}
	capability := writeCapability(t, root, "fixture-app")

	a, err := New(context.Background(), Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	admitted, binding, err := a.CompileForTenant(capability, "north")
	if err != nil {
		t.Fatal(err)
	}
	if admitted.Capability().Application != "fixture-app" || binding.Profile.Application != "fixture-app" {
		t.Fatalf("fixture profile admitted %q", admitted.Capability().Application)
	}
	// The target is not on the profile's read-only list. The bank check would
	// refuse this program; it must not run for a non-bank profile.
	if _, _, err = a.CompileForTenant("examples/balance.json", "north"); err == nil {
		t.Fatal("jarvis-bank capability admitted under the fixture profile")
	}
	if _, _, err = a.CompileForTenant(capability, "south"); err == nil {
		t.Fatal("fixture capability admitted under the bank profile")
	}
	bank, bankBinding, err := a.CompileForTenant("examples/balance.json", "south")
	if err != nil {
		t.Fatal(err)
	}
	if bank.Capability().Application != BankApplication || bankBinding.Profile.Application != BankApplication {
		t.Fatalf("south admitted %q", bank.Capability().Application)
	}
	bankOnly := writeCapability(t, root, BankApplication)
	if _, _, err = a.CompileForTenant(bankOnly, "south"); err == nil {
		t.Fatal("bank profile skipped validateBankProgram")
	}
}

func TestDiscoveryPublishesTheProfileApplication(t *testing.T) {
	a, err := New(context.Background(), Config{Root: "../.."})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	binding, err := a.profile("north")
	if err != nil {
		t.Fatal(err)
	}
	if binding.Profile.Application != BankApplication {
		t.Fatalf("north application = %q", binding.Profile.Application)
	}
	params, err := sensitiveInputs(binding.Profile, map[string]workflow.Value{
		"member_id": {Type: "string", Text: "M-1001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(params) != 1 || params[0].Name != "member_id" || !params[0].Sensitive {
		t.Fatalf("bank parameters = %+v", params)
	}
	if _, err = sensitiveInputs(binding.Profile, map[string]workflow.Value{
		"given_name": {Type: "string", Text: "Ada"},
	}); err == nil {
		t.Fatal("profile admitted a parameter it does not declare")
	}
	cap, err := assemblePublishedCapability(binding.Profile.Application, discoveryPublishArgs{
		ID: "bank.balance", Revision: "1", Description: "bank",
		Outputs:  []workflow.OutputDecl{{Name: "balance", Type: "string", Description: "balance"}},
		Outcomes: []workflow.OutcomeDef{{ID: "success", Kind: "success", Description: "done", Outputs: []string{"balance"}}},
	}, params, map[string]workflow.Selector{
		"balance": {Strategies: []workflow.Selector{{Role: "text_field", Name: "Balance"}}, Rationale: "labeled balance", Stability: "semantic"},
	}, []workflow.Step{{ID: "balance", Kind: "extract", Target: "balance", Effect: "read", Output: "balance", OutputType: "string"}})
	if err != nil {
		t.Fatal(err)
	}
	if cap.Application != BankApplication {
		t.Fatalf("bank discovery published %q", cap.Application)
	}
	other, err := assemblePublishedCapability("fixture-app", discoveryPublishArgs{
		ID: "fixture.note", Revision: "1", Description: "fixture",
		Outputs:  []workflow.OutputDecl{{Name: "note", Type: "string", Description: "note"}},
		Outcomes: []workflow.OutcomeDef{{ID: "success", Kind: "success", Description: "done", Outputs: []string{"note"}}},
	}, []workflow.Parameter{{Name: "note", Type: "string", Sensitive: true}}, map[string]workflow.Selector{
		"note": {Strategies: []workflow.Selector{{Role: "text_field", Name: "Note"}}, Rationale: "labeled note", Stability: "semantic"},
	}, []workflow.Step{{ID: "note", Kind: "extract", Target: "note", Effect: "read", Output: "note", OutputType: "string"}})
	if err != nil {
		t.Fatal(err)
	}
	if other.Application != "fixture-app" {
		t.Fatalf("fixture discovery published %q", other.Application)
	}
}

func TestSensitiveParametersComeFromTheProfile(t *testing.T) {
	profile := Profile{Parameters: []workflow.Parameter{{Name: "note", Type: "string", Sensitive: true}}}
	if _, err := sensitiveInputs(profile, map[string]workflow.Value{
		"member_id": {Type: "string", Text: "M-1001"},
	}); err == nil {
		t.Fatal("member_id was admitted from the fixed pair")
	}
	params, err := sensitiveInputs(profile, map[string]workflow.Value{
		"note": {Type: "string", Text: "n"},
	})
	if err != nil || len(params) != 1 || params[0].Name != "note" || !params[0].Sensitive {
		t.Fatalf("params = %+v err=%v", params, err)
	}
}

func TestDesktopLoaderRefusesInputsProfile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "profiles"), 0700); err != nil {
		t.Fatal(err)
	}
	inputs := Profile{SchemaVersion: 1, Tenant: "north", Application: InputsApplication, Kind: ProfileKindInputs}
	raw, err := json.Marshal(inputs)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "profiles", "north.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), Config{Root: root, Bank: filepath.Join(root, "missing-bank")})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err = a.profile("north"); !errors.Is(err, ErrDesktopRefusesInputs) {
		t.Fatalf("desktop loader: %v", err)
	}
	if err = a.RequireDesktopProfile("north"); !errors.Is(err, ErrDesktopRefusesInputs) {
		t.Fatalf("qualify check: %v", err)
	}
	if _, _, err = a.CompileForTenant("examples/balance.json", "north"); !errors.Is(err, ErrDesktopRefusesInputs) {
		t.Fatalf("compile: %v", err)
	}
	if _, err = a.Drift(context.Background(), "examples/balance.json", "north", 0); !errors.Is(err, ErrDesktopRefusesInputs) {
		t.Fatalf("drift: %v", err)
	}

	emptyWindow := Profile{SchemaVersion: 1, Tenant: "north", Application: BankApplication}
	raw, err = json.Marshal(emptyWindow)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "profiles", "north.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = a.profile("north")
	if err == nil {
		t.Fatal("empty window was a successful desktop load")
	}
	if errors.Is(err, ErrDesktopRefusesInputs) {
		t.Fatal("empty window was treated as an inputs profile")
	}
}

func writeCapability(t *testing.T, root, application string) string {
	t.Helper()
	cap := workflow.Capability{
		SchemaVersion: 1,
		ID:            "fixture.note",
		Revision:      "1",
		Application:   application,
		Description:   "fixture",
		Parameters:    []workflow.Parameter{{Name: "note", Type: "string", Sensitive: true}},
		Targets: map[string]workflow.Selector{
			"note": {Role: "text_field", Name: "Not In Profile"},
		},
		Steps: []workflow.Step{
			{ID: "enter", Kind: "set_value", Target: "note", Effect: "read", Input: workflow.Ref{Source: "input", Key: "note"}, Next: "done"},
			{ID: "done", Kind: "extract", Target: "note", Effect: "read", Output: "note", OutputType: "string"},
		},
		Limits: workflow.DefaultLimits(),
	}
	raw, err := json.Marshal(cap)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, application+".capability.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = workflow.Compile(raw); err != nil {
		t.Fatal(err)
	}
	return path
}
