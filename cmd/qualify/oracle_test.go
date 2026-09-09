package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bharathvbcr/Jarvis/internal/app"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func independentState(created bool) savedState {
	s := savedState{SchemaVersion: 1, Tenant: "north", Members: []savedMember{{ID: "M-1001", Name: "Alex Morgan", Currency: "USD", Balance: 125000, Subaccounts: []savedAccount{}}, {ID: "M-1002", Name: "Jordan Lee", Currency: "USD", Balance: 84050, Subaccounts: []savedAccount{}}}}
	if created {
		s.Revision = 1
		s.Members[0].Subaccounts = []savedAccount{{ID: "S-M-1001-1", Name: "Qualification", Kind: "savings"}}
	}
	return s
}
func writeOracle(t *testing.T, state savedState) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestCreationOracleRequiresExactlyOneNamedWriteAndUnchangedMembers(t *testing.T) {
	want := oracleExpectation{"Qualification"}
	if err := oracleWithExpectation(writeOracle(t, independentState(true)), "north", want); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*savedState){
		func(s *savedState) { s.Revision = 0 }, func(s *savedState) { s.Members[0].Subaccounts = nil },
		func(s *savedState) {
			s.Members[0].Subaccounts = append(s.Members[0].Subaccounts, s.Members[0].Subaccounts[0])
		},
		func(s *savedState) { s.Members[0].Subaccounts[0].Name = "Different" }, func(s *savedState) { s.Members[0].Subaccounts[0].Kind = "checking" },
		func(s *savedState) { s.Members[0].Balance++ }, func(s *savedState) { s.Members[1].Balance++ },
		func(s *savedState) { s.Members[1].Name = "Changed" }, func(s *savedState) { s.Members[1].Currency = "" },
		func(s *savedState) { s.Members[1].Subaccounts = s.Members[0].Subaccounts }, func(s *savedState) { s.Members[1].ID = s.Members[0].ID },
	} {
		s := independentState(true)
		mutate(&s)
		if err := oracleWithExpectation(writeOracle(t, s), "north", want); err == nil {
			t.Fatal("mutated creation state accepted")
		}
	}
}
func TestSuccessfulUIAcknowledgmentCannotSubstituteForSavedCreation(t *testing.T) {
	err := oracleWithExpectation(writeOracle(t, independentState(false)), "north", oracleExpectation{"Qualification"})
	v := app.RunView{Finished: true, State: workflow.State{Phase: workflow.Completed}}
	if err == nil || classify(v, "passed", "", err) != "failed" {
		t.Fatal("UI acknowledgment overrode independent unchanged disk state")
	}
	item := result{Outcome: "passed", Verdict: "passed", ApprovedActionIDs: []string{"run:confirm"}}
	if expectedBehavior("creation", v, &item, err, err) {
		t.Fatal("false creation was qualified")
	}
}
func TestFalseAckNeedsRealApprovedDispatchAndIndependentDisagreement(t *testing.T) {
	view, item := falseAckEvidence()
	faultErr, taskErr := scenarioOracles(writeOracle(t, independentState(false)), "north", "false-ack", true)
	item.Outcome = classify(view, item.Verdict, item.Error, taskErr)
	if faultErr != nil || taskErr == nil || item.Outcome != "failed" || !expectedBehavior("false-ack", view, &item, faultErr, taskErr) || !item.UIAcknowledgmentObserved || !item.FaultReached {
		t.Fatalf("UI evidence and failed business task were conflated: %+v, %v, %v", item, faultErr, taskErr)
	}
	if item.Verdict != "passed" {
		t.Fatal("independent UI evidence verdict was overwritten")
	}
	for _, mutate := range []func(*app.RunView, *result){
		func(_ *app.RunView, i *result) { i.ApprovedActionIDs = nil },
		func(_ *app.RunView, i *result) { i.ApprovalDenied = true },
		func(v *app.RunView, _ *result) { v.Finished = false },
		func(v *app.RunView, _ *result) { v.State.Phase = workflow.Unknown },
		func(_ *app.RunView, i *result) { i.Verdict = "incomplete" },
		func(_ *app.RunView, i *result) { i.Outcome = "passed" },
		func(_ *app.RunView, i *result) { i.Error = "transport failed" },
		func(v *app.RunView, _ *result) { v.Records[0].ActionID = "other" },
		func(v *app.RunView, _ *result) { v.Records[1].Event.Delivery = "unknown" },
		func(v *app.RunView, _ *result) { v.Records[1].Event.Delivery = "not_sent" },
		func(v *app.RunView, _ *result) { v.Records[0], v.Records[1] = v.Records[1], v.Records[0] },
		func(v *app.RunView, _ *result) { v.Records[1], v.Records[2] = v.Records[2], v.Records[1] },
		func(v *app.RunView, _ *result) { v.Records[2].ActionID = "stale" },
		func(v *app.RunView, _ *result) { v.Records[2].Stage = "observe" },
		func(v *app.RunView, _ *result) { v.Records[2].Observation.Complete = false },
		func(v *app.RunView, _ *result) { v.Records[2].Observation.Epoch++ },
		func(v *app.RunView, _ *result) { v.Records[1].Epoch++ },
		func(v *app.RunView, _ *result) {
			v.Records[2].Observation.Nodes = append(v.Records[2].Observation.Nodes, v.Records[2].Observation.Nodes[0])
		},
		func(v *app.RunView, _ *result) { *v.Records[2].Observation.Nodes[0].Value = "Ready" },
	} {
		v, i := falseAckEvidence()
		mutate(&v, &i)
		if expectedBehavior("false-ack", v, &i, faultErr, taskErr) {
			t.Fatal("incomplete or misbound false-ack evidence qualified")
		}
	}
	for _, taskErr := range []error{nil, os.ErrNotExist} {
		v, i := falseAckEvidence()
		if expectedBehavior("false-ack", v, &i, nil, taskErr) {
			t.Fatal("unexamined task oracle substituted for real saved-state disagreement")
		}
	}
	for _, created := range []bool{false, true} {
		state := independentState(created)
		if !created {
			state.Members[1].Balance++
		}
		faultErr, taskErr := scenarioOracles(writeOracle(t, state), "north", "false-ack", true)
		v, i := falseAckEvidence()
		if expectedBehavior("false-ack", v, &i, faultErr, taskErr) {
			t.Fatal("mutated saved state qualified as unchanged false acknowledgment")
		}
	}
}
func falseAckEvidence() (app.RunView, result) {
	status := "Subaccount created"
	records := confirmedRecords()
	for index := range records {
		records[index].Epoch = 1
	}
	records = append(records, computer.Record{Epoch: 1, Stage: "observe_after", ActionID: "run:confirm", Observation: &computer.Observation{ID: "after", Epoch: 1, Complete: true, Nodes: []computer.Node{{Role: "text_field", Name: "Status", Value: &status}}}})
	view := app.RunView{Finished: true, State: workflow.State{Phase: workflow.Completed}, Records: records}
	item := result{Verdict: "passed", Outcome: "failed", ApprovedActionIDs: []string{"run:confirm"}}
	return view, item
}
func confirmedRecords() []computer.Record {
	return []computer.Record{{Kind: "action_started", ActionID: "run:confirm"}, {Event: &workflow.Event{Kind: "receipt", ActionID: "run:confirm", Delivery: "sent"}}}
}
func TestCommitNoopAndCrashRemainDistinctFromSuccessfulAcceptance(t *testing.T) {
	noop := app.RunView{Finished: true, State: workflow.State{Phase: workflow.Cancelled}, Records: confirmedRecords()}
	status := "Creation did not complete"
	noop.Records = append(noop.Records, computer.Record{Stage: "observe_after", ActionID: "run:confirm", Observation: &computer.Observation{Complete: true, Nodes: []computer.Node{{Role: "text_field", Name: "Status", Value: &status}}}})
	item := result{Verdict: "failed", Outcome: "failed", ApprovedActionIDs: []string{"run:confirm"}}
	err := oracleWithExpectation(writeOracle(t, independentState(false)), "north", expectation("commit-noop", true))
	if !expectedBehavior("commit-noop", noop, &item, err, err) || !item.FaultReached || item.Outcome != "failed" {
		t.Fatal("expected no-op was converted into workflow success or lost")
	}
	crash := app.RunView{Finished: true, State: workflow.State{Phase: workflow.Unknown}, Records: confirmedRecords()}
	item = result{Verdict: "incomplete", Outcome: "blocked", ApprovedActionIDs: []string{"run:confirm"}}
	err = oracleWithExpectation(writeOracle(t, independentState(true)), "north", expectation("crash-after-commit", true))
	if !expectedBehavior("crash-after-commit", crash, &item, err, err) || !item.FaultReached || item.Outcome != "blocked" {
		t.Fatal("post-commit interruption lost unknown acceptance classification")
	}
	item.Verdict = "passed"
	if expectedBehavior("crash-after-commit", crash, &item, err, err) {
		t.Fatal("unknown action was accepted")
	}
	item.Verdict = "incomplete"
	item.ApprovedActionIDs = nil
	if expectedBehavior("crash-after-commit", crash, &item, err, err) {
		t.Fatal("crash flag substituted for actual approved input")
	}
}
func TestFaultObservationBeforeDispatchCannotQualifyCommitNoop(t *testing.T) {
	status := "Creation did not complete"
	v := app.RunView{Finished: true, State: workflow.State{Phase: workflow.Cancelled}, Records: []computer.Record{{Stage: "observe", ActionID: "run:confirm", Observation: &computer.Observation{Complete: true, Nodes: []computer.Node{{Role: "text_field", Name: "Status", Value: &status}}}}}}
	item := result{ApprovedActionIDs: []string{"run:confirm"}, Verdict: "incomplete"}
	if expectedBehavior("commit-noop", v, &item, nil, nil) {
		t.Fatal("pre-action text was treated as post-commit fault execution")
	}
	v.Records[0].Stage = "observe_after"
	v.Records = append(v.Records, confirmedRecords()...)
	if expectedBehavior("commit-noop", v, &item, nil, nil) {
		t.Fatal("a post-action label with wrong journal order qualified a fault")
	}
}
func TestInputRetriesAreSeparateAndRemoveFirstAttemptSuccess(t *testing.T) {
	obs, input := retryCounts([]computer.Record{{Event: &workflow.Event{Kind: "observe_failed"}}, {Event: &workflow.Event{Kind: "receipt", Delivery: "not_sent", FailureCode: "window_changed"}}, {Event: &workflow.Event{Kind: "receipt", Delivery: "not_sent", FailureCode: "state_changed"}}, {Event: &workflow.Event{Kind: "receipt", Delivery: "unknown", FailureCode: "state_changed"}}, {Event: &workflow.Event{Kind: "receipt", Delivery: "not_sent", FailureCode: "accessibility_denied"}}})
	if obs != 1 || input != 2 {
		t.Fatalf("retry counts: %d %d", obs, input)
	}
	r := completeReport()
	r.Results[0].InputRetries = 1
	summarize(&r)
	if r.FirstAttemptSuccesses != 39 {
		t.Fatal("safe input retry counted as first-attempt success")
	}
}
