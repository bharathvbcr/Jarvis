package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bharathvbcr/Jarvis/internal/app"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func TestExactReliabilityBound(t *testing.T) {
	if got := lowerBound(20, 20); math.Abs(got-.8608916593) > 1e-9 {
		t.Fatalf("wrong20/20bound %.12f", got)
	}
	if lowerBound(0, 20) != 0 || lowerBound(19, 20) >= lowerBound(20, 20) {
		t.Fatal("incorrect mixed-case bound")
	}
}

func completeReport() report {
	r := report{Mode: "fresh_process", Scenario: "balance", RequestedPerTenant: 20, Tenants: []string{"north", "south"}}
	for _, tenant := range r.Tenants {
		for index := 1; index <= 20; index++ {
			r.Results = append(r.Results, result{RunID: fmt.Sprintf("%s-%d", tenant, index), Tenant: tenant, Index: index, Attempted: true, Outcome: "passed", Phase: workflow.Completed, Verdict: "passed", OracleVerified: true})
		}
	}
	return r
}
func TestQualificationRequiresBothTenantsUniqueRunsAndCompleteEvidence(t *testing.T) {
	r := completeReport()
	summarize(&r)
	if !r.Qualified || !r.MatrixComplete || r.ByTenant["north"].Passed != 20 {
		t.Fatalf("complete report rejected: %+v", r)
	}
	cases := []struct {
		name   string
		change func(*report)
	}{
		{"single tenant", func(r *report) { r.Tenants = r.Tenants[:1]; r.Results = r.Results[:20] }},
		{"same process", func(r *report) { r.Mode = "same_process" }},
		{"missing attempt", func(r *report) { r.Results = r.Results[:39] }},
		{"duplicate index", func(r *report) { r.Results[39].Index = 1 }},
		{"duplicate run", func(r *report) { r.Results[39].RunID = r.Results[0].RunID }},
		{"unexpected tenant", func(r *report) { r.Results[39].Tenant = "other" }},
		{"unexamined oracle", func(r *report) { r.Results[39].OracleVerified = false }},
		{"reported success without execution", func(r *report) { r.Results[39].Attempted = false }},
		{"fault campaign", func(r *report) { r.Scenario = "commit-noop" }},
		{"blocked attempt", func(r *report) { r.Results[39].Outcome = "blocked" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := completeReport()
			tc.change(&r)
			summarize(&r)
			if r.Qualified {
				t.Fatal("incomplete/malformed campaign promoted")
			}
		})
	}
}
func TestValidateOptionsRejectsAmbiguousAttachmentAndDuplicateTenants(t *testing.T) {
	for _, tc := range []struct {
		tenants, scenario, oracle string
		pid                       uint
	}{
		{"north,north", "balance", "", 0}, {"north,south", "balance", "state.json", 1},
		{"north", "balance", "", 1}, {"north", "commit-noop", "state.json", 1}, {"north", "unknown", "", 0},
	} {
		if _, err := validateOptions(20, tc.tenants, tc.scenario, tc.pid, tc.oracle, time.Minute); err == nil {
			t.Fatalf("invalid options accepted: %+v", tc)
		}
	}
	if _, err := validateOptions(20, "north", "balance", 123, "state.json", time.Minute); err != nil {
		t.Fatal(err)
	}
}
func TestUnexaminedOracleAndStatusCannotPass(t *testing.T) {
	v := app.RunView{Finished: true, State: workflow.State{Phase: workflow.Completed}}
	if got := classify(v, "passed", "", os.ErrNotExist); got != "blocked" {
		t.Fatal(got)
	}
	if got := classify(v, "passed", "transport failed", nil); got != "blocked" {
		t.Fatal(got)
	}
	if got := classify(v, "passed", "", oracleMismatch{"changed state"}); got != "failed" {
		t.Fatal(got)
	}
	if got := classify(v, "passed", "", nil); got != "passed" {
		t.Fatal(got)
	}
}
func TestReportPreservesFailureAndRefusesExistingPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "report.json")
	r := completeReport()
	r.Results[0].Outcome = "blocked"
	r.Results[0].Error = "cancelled"
	summarize(&r)
	if err := reserveReport(p, r); err != nil {
		t.Fatal(err)
	}
	if err := reserveReport(p, report{}); err == nil {
		t.Fatal("existing report overwritten")
	}
	if err := save(p, r); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err = json.Unmarshal(raw, &got); err != nil || got.Qualified || got.Results[0].Error != "cancelled" {
		t.Fatalf("failed record lost: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("report mode: %v %v", info, err)
	}
}

type waitingHost struct {
	controls []string
	waited   bool
	view     app.RunView
	getError error
}

func (h *waitingHost) Get(string) (app.RunView, error) { return h.view, h.getError }
func (h *waitingHost) Control(_ context.Context, _ string, c computer.Control) error {
	h.controls = append(h.controls, c.Kind)
	return nil
}
func (h *waitingHost) Wait(ctx context.Context, _ string) (app.RunView, error) {
	h.waited = true
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 8*time.Second {
		return h.view, errors.New("missing bounded cleanup deadline")
	}
	return h.view, context.DeadlineExceeded
}
func TestCancelledTrialHasOneBoundedCleanupWithoutRetry(t *testing.T) {
	h := &waitingHost{view: app.RunView{State: workflow.State{Phase: workflow.Observing}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, _, err := waitTrial(ctx, h, "run")
	if !errors.Is(err, context.Canceled) || !h.waited || len(h.controls) != 1 || h.controls[0] != "cancel" || time.Since(start) > time.Second {
		t.Fatalf("unbounded/repeated cancellation: %+v %v", h, err)
	}
}
func TestDeniedPostCommitFaultIsNotMarkedReached(t *testing.T) {
	status := "Creation did not complete"
	records := []computer.Record{{Observation: &computer.Observation{Complete: true, Nodes: []computer.Node{{Name: "Status", Role: "text_field", Value: &status}}}}}
	if faultReached("commit-noop", records, true) || faultReached("crash-after-commit", nil, true) {
		t.Fatal("denial was reported as post-commit execution")
	}
	if !faultReached("denial", nil, true) {
		t.Fatal("actual denial was lost")
	}
}
func TestOracleDetectsSuccessfulUIWithChangedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	raw := `{"schema_version":1,"tenant":"north","revision":0,"members":[{"id":"M-1001","name":"Alex Morgan","currency":"USD","balance_cents":125000,"subaccounts":[]},{"id":"M-1002","name":"Jordan Lee","currency":"USD","balance_cents":84050,"subaccounts":[]}]}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := oracle(path, "north"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"tenant":"north","revision":1,"members":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := oracle(path, "north"); err == nil {
		t.Fatal("changed backing state accepted independently of UI acknowledgment")
	}
}
