package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func TestPendingApprovalTracksCanonicalRetryIdentity(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "create-subaccount.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := workflow.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	s := workflow.State{RunID: "run", StepIndex: 5, Phase: workflow.AwaitingApproval, ActionAttempt: 1}
	a.runs["run"] = &runEntry{program: p, assisted: true, view: RunView{RunID: "run", State: s, Capability: p.Capability()}}
	v, err := a.Get("run")
	if err != nil {
		t.Fatal(err)
	}
	if v.PendingActionID != s.ActionID(p) {
		t.Fatalf("approval targets %q; canonical retry is %q", v.PendingActionID, s.ActionID(p))
	}
	if !v.Assisted {
		t.Fatal("history-selected run lost admitted assistance opt-in")
	}
}

func TestRunHistoryIsBoundedMetadataWithExplicitOwner(t *testing.T) {
	a, err := New(context.Background(), Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	old := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	a.active = "pending"
	a.runs["pending"] = &runEntry{startedAt: old.Add(time.Second), view: RunView{RunID: "pending", State: workflow.State{Phase: workflow.AwaitingApproval, SessionID: "desktop-session", Epoch: 9, Outputs: map[string]workflow.Value{"secret": {Type: "string", Text: "private-output"}}}}}
	a.runs["past"] = &runEntry{startedAt: old, view: RunView{RunID: "past", Finished: true, State: workflow.State{Phase: workflow.Completed}}}
	listed := a.ListRuns()
	if listed.TotalRuns != 2 || listed.Truncated || listed.DesktopOwner != "pending" || listed.Runs[0].RunID != "pending" || listed.Runs[1].RunID != "past" || listed.Runs[0].State.Epoch != 9 {
		t.Fatalf("incorrect run metadata: %+v", listed)
	}
	raw, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private-output") || strings.Contains(string(raw), "outputs") || strings.Contains(string(raw), "records") {
		t.Fatalf("history exposed invocation detail: %s", raw)
	}
	listed.Runs[0].State.Epoch = 1
	if a.ListRuns().Runs[0].State.Epoch != 9 {
		t.Fatal("caller mutated authoritative metadata")
	}
	// Retiring an owner retains its history without claiming continued input ownership.
	a.active = ""
	a.runs["pending"].view.Finished = true
	if v := a.ListRuns(); v.DesktopOwner != "" || !v.Runs[0].Finished || v.TotalRuns != 2 {
		t.Fatalf("retirement lost history or ownership: %+v", v)
	}
}
