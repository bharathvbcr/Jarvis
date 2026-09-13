package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/agent"
	"github.com/bharathvbcr/Manvi/manvi/core/bus"
	"github.com/bharathvbcr/Manvi/manvi/credentials"
	"github.com/bharathvbcr/Manvi/manvi/llm"
	"github.com/bharathvbcr/Manvi/manvi/llm/replay"
	"github.com/bharathvbcr/Manvi/manvi/session"
	"github.com/bharathvbcr/Manvi/manvi/tools"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func TestDiscoveryFixtureProviderLoadsWithoutGeminiKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	root := t.TempDir()
	ledger, err := openCampaignLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	provider, path, err := newDiscoveryLLMProvider(DiscoveryProviderFixture, "", func() (credentials.Secret, error) {
		t.Fatal("fixture provider must not resolve Gemini credentials")
		return credentials.Secret{}, nil
	}, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if provider == nil || path == "" {
		t.Fatalf("missing provider or fixture path: provider=%v path=%q", provider, path)
	}
	if discoveryEvidenceClass(DiscoveryProviderFixture) != DiscoveryEvidenceNotLive {
		t.Fatalf("fixture evidence class = %q", discoveryEvidenceClass(DiscoveryProviderFixture))
	}
	if discoveryLive(DiscoveryProviderFixture) {
		t.Fatal("fixture provider labelled live")
	}
	if _, _, err := newDiscoveryLLMProvider("unknown", "", nil, ledger); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestAssemblePublishedCapabilityDeclaresSchemaContract(t *testing.T) {
	member, err := discoveryTargetSelector(discoveryStepArgs{
		Kind: "set_value", Name: "Member ID", Parameter: "member_id",
		Rationale: "Labeled member identifier field on the lookup form", Stability: "semantic",
	})
	if err != nil {
		t.Fatal(err)
	}
	search, err := discoveryTargetSelector(discoveryStepArgs{
		Kind: "press", Name: "Search",
		Rationale: "Primary search action for member lookup", Stability: "semantic",
		Strategies: []workflow.Selector{
			{Role: "button", Name: "Search"},
			{Identifier: "search.button"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	balance, err := discoveryTargetSelector(discoveryStepArgs{
		Kind: "extract", Name: "Balance", Output: "balance", OutputType: "money",
		Rationale: "Balance field holds the verified USD amount", Stability: "semantic",
	})
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]workflow.Value{"member_id": {Type: "string", Text: "M-1001"}}
	stepMember, err := discoveryStepFromArgs("member", "member", discoveryStepArgs{Kind: "set_value", Name: "Member ID", Parameter: "member_id", Rationale: "x", Stability: "semantic"}, inputs)
	if err != nil {
		t.Fatal(err)
	}
	stepSearch, err := discoveryStepFromArgs("search", "search", discoveryStepArgs{Kind: "press", Name: "Search", Rationale: "x", Stability: "semantic"}, inputs)
	if err != nil {
		t.Fatal(err)
	}
	stepBalance, err := discoveryStepFromArgs("balance", "balance", discoveryStepArgs{Kind: "extract", Name: "Balance", Output: "balance", OutputType: "money", Rationale: "x", Stability: "semantic"}, inputs)
	if err != nil {
		t.Fatal(err)
	}
	cap, err := assemblePublishedCapability(discoveryPublishArgs{
		ID: "bank.balance", Revision: "1", Description: "Look up a member and return an independently verified USD balance.",
		Outputs: []workflow.OutputDecl{{Name: "balance", Type: "money", Currency: "USD", Description: "Verified member balance in USD minor units"}},
		Outcomes: []workflow.OutcomeDef{
			{ID: "success", Kind: "success", Description: "Member found and balance extracted", Outputs: []string{"balance"}},
			{ID: "not_found", Kind: "business", Description: "Member ID does not exist", Outputs: []string{}},
		},
		Recoveries: []workflow.Recovery{{
			ID: "dismiss_notice",
			When: workflow.RecoveryWhen{
				Target:    "search",
				Predicate: workflow.Predicate{Op: "contains", Expected: workflow.Ref{Source: "literal", Literal: workflow.Value{Type: "string", Text: "Session notice"}}},
			},
			Action: workflow.RecoveryAction{Kind: "press", Target: "search"},
			Max:    1,
		}},
	}, []workflow.Parameter{{Name: "member_id", Type: "string", Sensitive: true}}, map[string]workflow.Selector{
		"member": member, "search": search, "balance": balance,
	}, []workflow.Step{stepMember, stepSearch, stepBalance})
	if err != nil {
		t.Fatal(err)
	}
	if cap.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d", cap.SchemaVersion)
	}
	if len(cap.Outputs) != 1 || len(cap.Outcomes) != 2 || len(cap.Recoveries) != 1 {
		t.Fatalf("contract fields missing: %+v", cap)
	}
	if len(cap.Targets["search"].Strategies) != 2 || cap.Targets["search"].Rationale == "" || cap.Targets["search"].Stability != "semantic" {
		t.Fatalf("ladder missing: %+v", cap.Targets["search"])
	}
	last := cap.Steps[len(cap.Steps)-1]
	if last.Kind != "conclude" || last.Outcome != "success" {
		t.Fatalf("missing success conclude: %+v", last)
	}
	raw, err := json.Marshal(cap)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.Compile(raw); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryFixtureTranscriptDrivesPublishAssembly(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	root := t.TempDir()
	ledger, err := openCampaignLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	provider, _, err := newDiscoveryLLMProvider(DiscoveryProviderFixture, "", nil, ledger)
	if err != nil {
		t.Fatal(err)
	}
	inner, ok := provider.(*replay.Provider)
	if !ok {
		t.Fatalf("provider type %T", provider)
	}
	if inner.Name() != "fixture" {
		t.Fatalf("provider name = %q", inner.Name())
	}

	b := bus.New()
	log := session.NewLog()
	registry := tools.NewRegistry(b)
	inputs := map[string]workflow.Value{"member_id": {Type: "string", Text: "M-1001"}}
	params := []workflow.Parameter{{Name: "member_id", Type: "string", Sensitive: true}}
	targets := map[string]workflow.Selector{}
	steps := []workflow.Step{}
	var published workflow.Capability
	register := func(name, schema string, fn tools.Handler) {
		t.Helper()
		if err := registry.Register(tools.Tool{Schema: llm.ToolSchema{Name: name, Description: name, InputSchema: json.RawMessage(schema)}, Handler: fn, Group: tools.GroupCore}); err != nil {
			t.Fatal(err)
		}
	}
	register("desktop_observe", `{"type":"object","properties":{},"additionalProperties":false}`, func(context.Context, tools.Call) tools.Result {
		return tools.Result{Text: `{"fixture":"observe"}`}
	})
	register("desktop_step", desktopStepSchema, func(_ context.Context, call tools.Call) tools.Result {
		var args discoveryStepArgs
		if err := workflow.DecodeStrict(call.Arguments, &args); err != nil {
			return tools.Result{Text: err.Error(), IsError: true}
		}
		ladder, err := discoveryTargetSelector(args)
		if err != nil {
			return tools.Result{Text: err.Error(), IsError: true}
		}
		id := fmt.Sprintf("step_%02d", len(steps)+1)
		target := fmt.Sprintf("target_%02d", len(steps)+1)
		step, err := discoveryStepFromArgs(id, target, args, inputs)
		if err != nil {
			return tools.Result{Text: err.Error(), IsError: true}
		}
		targets[target] = ladder
		steps = append(steps, step)
		return tools.Result{Text: `{"fixture":"step"}`}
	})
	register("capability_publish", capabilityPublishSchema, func(_ context.Context, call tools.Call) tools.Result {
		var args discoveryPublishArgs
		if err := workflow.DecodeStrict(call.Arguments, &args); err != nil {
			return tools.Result{Text: err.Error(), IsError: true}
		}
		cap, err := assemblePublishedCapability(args, params, targets, steps)
		if err != nil {
			return tools.Result{Text: err.Error(), IsError: true}
		}
		published = cap
		return tools.Result{Text: `{"published":true}`}
	})

	loop, err := agent.NewLoop(agent.Config{
		Provider: provider, Model: DiscoveryModel, SystemPrompt: discoverySystemPrompt(DiscoveryPromptRevision),
		MaxSteps: 40, MaxTokens: 8192, AssertInvariant: true,
	}, b, log, registry)
	if err != nil {
		t.Fatal(err)
	}
	out, err := loop.Run(context.Background(), llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: "publish balance\nAvailable input references: member_id"}}})
	if err != nil {
		t.Fatal(err)
	}
	if published.ID != "bank.balance" || published.SchemaVersion != 1 {
		t.Fatalf("published = %+v outcome=%+v", published, out)
	}
	if discoveryEvidenceClass(DiscoveryProviderFixture) != DiscoveryEvidenceNotLive {
		t.Fatal("evidence class drifted")
	}
	metaPath := filepath.Join(root, "discovery.json")
	meta := struct {
		Live          bool   `json:"live"`
		EvidenceClass string `json:"evidence_class"`
		Provider      string `json:"provider"`
	}{Live: discoveryLive(DiscoveryProviderFixture), EvidenceClass: discoveryEvidenceClass(DiscoveryProviderFixture), Provider: DiscoveryProviderFixture}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"not live"`) || strings.Contains(string(body), `"live": true`) {
		t.Fatalf("metadata not labelled not live: %s", body)
	}
}

func TestDiscoverRejectsUnknownProviderWithoutCredential(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	a, err := New(context.Background(), Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.Discover(DiscoveryRequest{Tenant: "north", Task: "x", Provider: "openai", Inputs: map[string]workflow.Value{"member_id": {Type: "string", Text: "M-1001"}}}); err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("err = %v", err)
	}
	if _, err := a.Discover(DiscoveryRequest{Tenant: "north", Task: "x", Provider: DiscoveryProviderGemini, Inputs: map[string]workflow.Value{"member_id": {Type: "string", Text: "M-1001"}}}); err == nil || !strings.Contains(err.Error(), "Gemini credential") {
		t.Fatalf("gemini without key err = %v", err)
	}
	// Fixture admission must not require GEMINI_API_KEY; it may still fail later on broker/bank.
	_, err = a.Discover(DiscoveryRequest{Tenant: "north", Task: "Look up balance", Provider: DiscoveryProviderFixture, Inputs: map[string]workflow.Value{"member_id": {Type: "string", Text: "M-1001"}}})
	if err != nil && strings.Contains(err.Error(), "Gemini credential") {
		t.Fatalf("fixture path required Gemini: %v", err)
	}
}

func TestDiscoverySystemPromptMentionsOutcomesAndLadder(t *testing.T) {
	prompt := discoverySystemPrompt(DiscoveryPromptRevision)
	for _, needle := range []string{"outcomes", "ladder", "outputs", "stability", "schema_version stays 1", DiscoveryPromptRevision} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q", needle)
		}
	}
	if !strings.Contains(desktopStepSchema, "rationale") || !strings.Contains(capabilityPublishSchema, "outcomes") {
		t.Fatal("tool schemas missing contract fields")
	}
}

// The shipped binary is built with -trimpath, so deriving the default fixture
// path from runtime.Caller named a module-relative path that does not exist on
// disk and `--provider fixture` failed outside `go test`. The transcript is
// embedded instead; this pins that the default needs no source tree and no
// particular working directory.
func TestDefaultDiscoveryFixtureNeedsNoSourceTree(t *testing.T) {
	if len(defaultDiscoveryFixture) == 0 {
		t.Fatal("default discovery fixture was not embedded")
	}
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	ledger, err := openCampaignLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	provider, source, err := newDiscoveryLLMProvider(DiscoveryProviderFixture, "", nil, ledger)
	if err != nil {
		t.Fatalf("embedded fixture unavailable from an unrelated directory: %v", err)
	}
	if _, ok := provider.(*replay.Provider); !ok {
		t.Fatalf("provider type %T", provider)
	}
	if source != defaultDiscoveryFixtureName {
		t.Fatalf("source = %q, want the embedded name", source)
	}

	// An explicitly supplied path must still be read from disk, and a missing
	// one must fail loudly rather than silently falling back to the embedded copy.
	if _, _, err := newDiscoveryLLMProvider(DiscoveryProviderFixture, filepath.Join(dir, "absent.json"), nil, ledger); err == nil {
		t.Fatal("missing explicit fixture accepted")
	}
}
