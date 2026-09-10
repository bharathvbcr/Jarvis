package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/devcouncil"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func TestEvidenceRejectsUnadmittedReceipt(t *testing.T) {
	r := testRecorder(t)
	if err := r.Record(computer.Record{Kind: "human_action", ActionID: "human-1", Receipt: &computer.Receipt{Delivery: "sent"}}); err == nil {
		t.Fatal("unadmitted human action accepted")
	}
}

func TestNotSentAttemptRemainsDistinctFromFailedOrUnknownDelivery(t *testing.T) {
	r := testRecorder(t)
	for _, delivery := range []string{"not_sent", "unknown", "sent"} {
		if err := r.Record(computer.Record{Kind: "action_started", ActionID: delivery}); err != nil {
			t.Fatal(err)
		}
		if err := r.Record(computer.Record{Kind: "event", ActionID: delivery, Event: &workflow.Event{Kind: "receipt", Delivery: delivery}}); err != nil {
			t.Fatal(err)
		}
	}
	for i, want := range []string{"not_dispatched", "unknown", "succeeded"} {
		if r.bundle.Actions[i].Disposition != want {
			t.Fatalf("delivery distinction lost: %v", r.bundle.Actions)
		}
	}
}
func testRecorder(t *testing.T) *Recorder {
	t.Helper()
	raw, err := os.ReadFile("../../examples/balance.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := workflow.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := os.ReadFile("../../scenarios/balance-m1001.contract.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRecorder(filepath.Join(t.TempDir(), "run"), p, contract, computer.Session{ID: "desktop", RunID: "run", Epoch: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.file.Close() })
	return r
}
func TestHumanReceiptUpdatesAdmissionAndEpochChain(t *testing.T) {
	r := testRecorder(t)
	for _, record := range []computer.Record{{Kind: "action_started", Actor: "human", ActionID: "human-1", Epoch: 2}, {Kind: "human_action", Actor: "human", ActionID: "human-1", Epoch: 2, Receipt: &computer.Receipt{Delivery: "sent"}}, {Kind: "event", Event: &workflow.Event{Kind: "pause", Epoch: 1, NextEpoch: 2}}, {Kind: "event", Event: &workflow.Event{Kind: "resume", Epoch: 2, NextEpoch: 3}}} {
		if err := r.Record(record); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.bundle.Actions) != 1 {
		t.Fatal("human action receipt duplicated journal action")
	}
	if len(r.bundle.EpochTransitions) != 2 || r.bundle.Epoch != 1 {
		t.Fatal("admission epoch identity changed")
	}
	if err := r.Finish(computer.RunResult{State: workflow.State{Phase: workflow.Completed, Epoch: 3}}, nil); err != nil {
		t.Fatal(err)
	}
	var bundle devcouncil.EvidenceBundle
	raw, err := os.ReadFile(filepath.Join(r.dir, "bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &bundle); err != nil {
		t.Fatal(err)
	}
	if len(bundle.Degraded) != 0 {
		t.Fatal("valid epoch transition marked blanket degraded")
	}
}
func TestFactsAreIndependentTypedValues(t *testing.T) {
	balance, status, redacted := "1250.00 USD", "Member found", "[redacted]"
	o := computer.Observation{Nodes: []computer.Node{{Role: "text_field", Name: "Balance", Value: &balance}, {Role: "text_field", Name: "Status", Value: &status}, {Role: "text_field", Name: "Member ID", Value: &redacted}}}
	facts := factsFromObservation(o)
	if string(facts["balance_minor"]) != "125000" || string(facts["currency"]) != `"USD"` {
		t.Fatalf("invalid integer money facts %v", facts)
	}
	if _, ok := facts["ui.Member ID"]; ok {
		t.Fatal("masked input became acceptance fact")
	}
	o.Nodes = append(o.Nodes, computer.Node{Role: "text_field", Name: "Balance", Value: &balance})
	if _, ok := factsFromObservation(o)["balance_minor"]; ok {
		t.Fatal("ambiguous duplicate balance accepted")
	}
}

func TestEvidenceFactsDistinguishLabelsFromValueFields(t *testing.T) {
	label, balance, statusLabel, status := "Balance", "1250.00 USD", "Status", "Member found"
	o := computer.Observation{Nodes: []computer.Node{{Role: "text", Name: "Balance", Value: &label}, {Role: "text_field", Name: "Balance", Value: &balance}, {Role: "text", Name: "Status", Value: &statusLabel}, {Role: "text_field", Name: "Status", Value: &status}}}
	facts := factsFromObservation(o)
	if string(facts["balance_minor"]) != "125000" || string(facts["ui.Status"]) != `"Member found"` {
		t.Fatalf("genuine fields lost because their labels share names: %v", facts)
	}
	o.Nodes = append(o.Nodes, computer.Node{Role: "text_field", Name: "Balance", Value: &balance})
	if facts := factsFromObservation(o); facts["balance_minor"] != nil || facts["ui.Balance"] != nil {
		t.Fatal("duplicate value fields accepted")
	}
}
