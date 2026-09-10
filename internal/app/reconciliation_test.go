package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/devcouncil"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

type observerFunc func(context.Context, computer.Session) (computer.Observation, error)

func (f observerFunc) Observe(ctx context.Context, s computer.Session) (computer.Observation, error) {
	return f(ctx, s)
}

func reconciliationFixture(t *testing.T) (computer.Session, computer.PrivacyPolicy, workflow.State, computer.Observation) {
	t.Helper()
	window := computer.Window{PID: 1, ID: 2, Bounds: computer.Bounds{Width: 100, Height: 80}}
	s := computer.Session{ID: "desktop", RunID: "run", Epoch: 1, Window: window}
	p := bankPrivacy(PolicyDocument{
		SensitiveTargets: []PolicySelector{
			{Role: "text_field", Name: "Member ID"},
			{Role: "text_field", Name: "Subaccount name"},
		},
	}, map[string]workflow.Value{"member_id": {Type: "string", Text: "synthetic-secret"}})
	p.PID, p.WindowID = window.PID, window.ID
	state := workflow.State{RunID: s.RunID, SessionID: s.ID, Epoch: s.Epoch, Phase: workflow.Unknown, Observation: workflow.Observation{ID: "before"}}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 100, 80))); err != nil {
		t.Fatal(err)
	}
	secret, status := "synthetic-secret", "Created successfully"
	o := computer.Observation{ID: "after", Epoch: 1, Complete: true, Window: window, Screenshot: computer.Screenshot{MIMEType: "image/png", Base64: base64.StdEncoding.EncodeToString(b.Bytes()), Width: 100, Height: 80}, Nodes: []computer.Node{{ID: "secret", Name: "Member ID", Role: "text_field", Value: &secret, Bounds: &computer.Bounds{X: 10, Y: 10, Width: 20, Height: 10}}, {ID: "status", Name: "Status", Role: "text_field", Value: &status}}}
	return s, p, state, o
}

func TestUnknownReobservationKeepsDispositionAndMasksBeforeEvidence(t *testing.T) {
	s, p, state, o := reconciliationFixture(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	observer := observerFunc(func(ctx context.Context, got computer.Session) (computer.Observation, error) {
		if got != s || ctx.Err() != nil {
			t.Fatal("lost fenced identity or cleanup observation budget")
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			t.Fatal("unbounded cleanup")
		}
		o.CapturedAtMillis = uint64(time.Now().UnixMilli())
		o.TreeAtMillis = o.CapturedAtMillis
		return o, nil
	})
	report, safe := observeUnknown(cancelled, observer, s, p, state, "change")
	if report.Status != "observed" || report.Acceptance != "incomplete" || safe == nil || state.Phase != workflow.Unknown {
		t.Fatalf("incorrect reconciliation %+v", report)
	}
	raw, err := json.Marshal(struct {
		Report      Reconciliation
		Observation *computer.Observation
	}{report, safe})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "synthetic-secret") {
		t.Fatal("raw private value escaped reconciliation")
	}
	if string(report.ObservedFacts["ui.Status"]) != `"Created successfully"` {
		t.Fatal("observed UI acknowledgment missing")
	}
	r := testRecorder(t)
	if err = r.Record(computer.Record{Kind: "action_started", ActionID: "change"}); err != nil {
		t.Fatal(err)
	}
	if err = r.Record(computer.Record{Kind: "observation", Stage: "outcome_reconciliation", ActionID: "change", Epoch: 1, Observation: safe}); err != nil {
		t.Fatal(err)
	}
	if err = r.RecordReconciliation(report); err != nil {
		t.Fatal(err)
	}
	if err = r.Finish(computer.RunResult{State: state}, nil); err != nil {
		t.Fatal(err)
	}
	var bundle devcouncil.EvidenceBundle
	raw, err = os.ReadFile(filepath.Join(r.dir, "bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.JournalComplete || bundle.Actions[0].Disposition != "unknown" || len(bundle.Observations) != 1 || bundle.Observations[0].AfterActionID != "change" {
		t.Fatalf("reconciliation changed dispatch or acceptance: %+v", bundle)
	}
	artifact := bundle.Artifacts[len(bundle.Artifacts)-1]
	raw, err = os.ReadFile(filepath.Join(r.dir, artifact.Path))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SHA256 != digest(raw) || artifact.SizeBytes != uint64(len(raw)) {
		t.Fatal("reconciliation bytes not bound to bundle")
	}
}

func TestUnknownReobservationRejectsStaleForeignIncompleteAndMissingCapture(t *testing.T) {
	for _, kind := range []string{"same_observation", "old_epoch", "old_capture", "wrong_surface", "incomplete", "error", "wrong_session", "completed"} {
		t.Run(kind, func(t *testing.T) {
			s, p, state, o := reconciliationFixture(t)
			if kind == "wrong_session" {
				s.ID = "foreign"
			}
			if kind == "completed" {
				state.Phase = workflow.Completed
			}
			report, safe := observeUnknown(context.Background(), observerFunc(func(context.Context, computer.Session) (computer.Observation, error) {
				o.CapturedAtMillis = uint64(time.Now().UnixMilli())
				o.TreeAtMillis = o.CapturedAtMillis
				switch kind {
				case "same_observation":
					o.ID = "before"
				case "old_epoch":
					o.Epoch = 0
				case "old_capture":
					o.TreeAtMillis = 0
				case "wrong_surface":
					o.Window.ID = 99
				case "incomplete":
					o.Complete = false
				case "error":
					return o, errors.New("synthetic-secret capture denied")
				}
				return o, nil
			}), s, p, state, "change")
			if report.Status != "unavailable" || safe != nil || len(report.ObservedFacts) != 0 || strings.Contains(report.Reason, "synthetic-secret") {
				t.Fatalf("accepted untrusted reconciliation: %+v", report)
			}
		})
	}
}
