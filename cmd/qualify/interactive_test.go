package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bharathvbcr/Jarvis/internal/app"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func reviewedView() app.RunView {
	member, name, status := "[redacted]", "[redacted]", "Ready for confirmation"
	return app.RunView{RunID: "run-1", PendingActionID: "run-1:confirm", State: workflow.State{RunID: "run-1", SessionID: "session-1", Epoch: 3, Phase: workflow.AwaitingApproval, Observation: workflow.Observation{ID: "obs-1", TargetID: "confirm", Complete: true, Actionable: true, Matches: 1}}, Capability: workflow.Capability{Application: "jarvis-bank", Targets: map[string]workflow.Selector{"confirm": {Role: "button", Name: "Confirm creation"}}, Steps: []workflow.Step{{ID: "confirm", Target: "confirm", Kind: "press", Effect: "change"}}}, Observation: &computer.Observation{ID: "obs-1", Epoch: 3, Complete: true, Window: computer.Window{PID: 123, Title: "Jarvis Bank"}, Nodes: []computer.Node{{ID: "member", Role: "text_field", Name: "Member ID", Value: &member}, {ID: "name", Role: "text_field", Name: "Subaccount name", Value: &name}, {ID: "status", Role: "text_field", Name: "Status", Value: &status}, {ID: "confirm", Role: "button", Name: "Confirm creation", Enabled: true}}}}
}

type scriptedHost struct {
	views    []app.RunView
	index    int
	controls []computer.Control
}

func (s *scriptedHost) Get(string) (app.RunView, error) {
	i := s.index
	if i < len(s.views)-1 {
		s.index++
	}
	return s.views[i], nil
}
func (s *scriptedHost) Control(_ context.Context, _ string, c computer.Control) error {
	s.controls = append(s.controls, c)
	return nil
}
func (s *scriptedHost) Wait(ctx context.Context, _ string) (app.RunView, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 8*time.Second {
		return app.RunView{}, errors.New("unbounded cleanup")
	}
	return app.RunView{Finished: true, State: workflow.State{Phase: workflow.Cancelled}}, nil
}

type approveFunc func(context.Context, approvalPrompt) (bool, error)

func (f approveFunc) Decide(ctx context.Context, p approvalPrompt) (bool, error) { return f(ctx, p) }

func TestTerminalApprovalRequiresExactIdentityAndRejectsPipedInput(t *testing.T) {
	prompt, err := makeApprovalPrompt(reviewedView(), "run-1", expectedForm{"north", "M-1001", "Qualification"})
	if err != nil {
		t.Fatal(err)
	}
	lines := make(chan inputLine, 3)
	lines <- inputLine{text: "approve"}
	lines <- inputLine{text: strings.Replace(prompt.Command, "obs-1", "obs-old", 1)}
	lines <- inputLine{text: prompt.Command}
	close(lines)
	var output bytes.Buffer
	term := terminalApprover{lines, &output}
	approved, err := term.Decide(context.Background(), prompt)
	if err != nil || !approved || strings.Count(output.String(), "No approval sent") != 2 {
		t.Fatalf("identity was not enforced: %t %v %s", approved, err, output.String())
	}
	if !strings.Contains(output.String(), "Qualification") || !strings.Contains(output.String(), "[redacted]") {
		t.Fatal("independent and observed form were not displayed")
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	if _, err = newTerminalApprover(context.Background(), read, io.Discard); err == nil {
		t.Fatal("piped automatic approval accepted")
	}
}
func TestApprovalRejectsIncompleteOrAmbiguousNativeBinding(t *testing.T) {
	for _, mutate := range []func(*app.RunView){func(v *app.RunView) { v.State.Observation.Matches = 2 }, func(v *app.RunView) { v.Observation.Complete = false }, func(v *app.RunView) { v.PendingActionID = "" }, func(v *app.RunView) { v.State.Epoch++ }, func(v *app.RunView) { v.Capability.Steps[0].Effect = "read" }, func(v *app.RunView) { v.State.RunID = "other" }} {
		view := reviewedView()
		mutate(&view)
		if _, err := makeApprovalPrompt(view, "run-1", expectedForm{}); err == nil {
			t.Fatal("unsafe binding accepted")
		}
	}
}
func TestStaleApprovalIsReReadBeforeAnyInputControl(t *testing.T) {
	for _, mutate := range []func(*app.RunView){
		func(v *app.RunView) { v.PendingActionID = "run-1:confirm:attempt:2" },
		func(v *app.RunView) { v.State.Epoch++; v.Observation.Epoch++ },
		func(v *app.RunView) { v.State.Observation.ID = "obs-2"; v.Observation.ID = "obs-2" },
		func(v *app.RunView) { changed := "Changed form"; v.Observation.Nodes[2].Value = &changed },
	} {
		initial, changed := reviewedView(), reviewedView()
		mutate(&changed)
		h := &scriptedHost{views: []app.RunView{initial, changed, {Finished: true, State: workflow.State{Phase: workflow.Cancelled}}}}
		_, approvals, err := waitTrialWithApproval(context.Background(), h, "run-1", approveFunc(func(context.Context, approvalPrompt) (bool, error) { return true, nil }), expectedForm{})
		if err != nil || len(approvals.Granted) != 0 || len(h.controls) != 0 {
			t.Fatalf("stale approval sent input: %+v %v", h.controls, err)
		}
	}
}
func TestExactApprovalUsesBackendAttemptAndObservationIDs(t *testing.T) {
	view := reviewedView()
	view.PendingActionID = "run-1:confirm:attempt:2"
	h := &scriptedHost{views: []app.RunView{view, view, {Finished: true, State: workflow.State{Phase: workflow.Completed}}}}
	calls := 0
	_, approvals, err := waitTrialWithApproval(context.Background(), h, "run-1", approveFunc(func(_ context.Context, p approvalPrompt) (bool, error) {
		calls++
		if p.Form.ActionID != view.PendingActionID {
			t.Fatal("invented action ID")
		}
		return true, nil
	}), expectedForm{})
	if err != nil || calls != 1 || len(approvals.Granted) != 1 || len(h.controls) != 1 {
		t.Fatalf("approval flow: %+v %v", h.controls, err)
	}
	c := h.controls[0]
	if c.Kind != "approve" || c.ActionID != view.PendingActionID || c.ObservationID != "obs-1" || c.Epoch != 3 {
		t.Fatalf("wrong binding: %+v", c)
	}
}
func TestApprovalEOFAndCancellationNeverGrantAndUseBoundedCleanup(t *testing.T) {
	for _, cause := range []error{io.EOF, context.Canceled, context.DeadlineExceeded} {
		h := &scriptedHost{views: []app.RunView{reviewedView()}}
		_, approvals, err := waitTrialWithApproval(context.Background(), h, "run-1", approveFunc(func(context.Context, approvalPrompt) (bool, error) { return false, cause }), expectedForm{})
		if !errors.Is(err, cause) || len(approvals.Granted) != 0 || len(h.controls) != 1 || h.controls[0].Kind != "cancel" {
			t.Fatalf("failed approval was not cancelled: %+v %v", h.controls, err)
		}
	}
}
