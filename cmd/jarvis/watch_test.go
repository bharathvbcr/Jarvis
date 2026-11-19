package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bharathvbcr/Jarvis/internal/app"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// fakeHost plays a scripted run: GetSince steps through views and holds the
// last; a cancel control ends the run when finishOnCancel is set.
type fakeHost struct {
	mu             sync.Mutex
	views          []app.RunView
	i              int
	finishOnCancel bool
	finishOn       string // a control kind that completes the run
	finishAfter    int    // complete after this many polls; 0 never
	polls          int
	finished       bool
	finalPhase     workflow.Phase
	controls       []computer.Control
}

func view(phase workflow.Phase, epoch uint64, action, observation string) app.RunView {
	v := app.RunView{RunID: "run-1", PendingActionID: action}
	v.State.Phase, v.State.Epoch, v.State.Observation.ID = phase, epoch, observation
	return v
}

func (f *fakeHost) current() app.RunView {
	if f.finished {
		phase := f.finalPhase
		if phase == "" {
			phase = workflow.Cancelled
		}
		v := view(phase, f.views[f.i].State.Epoch, "", "")
		v.Finished = true
		return v
	}
	return f.views[f.i]
}
func (f *fakeHost) Get(string) (app.RunView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current(), nil
}
func (f *fakeHost) GetSince(string, int) (app.RunView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls++
	if f.finishAfter > 0 && f.polls > f.finishAfter && !f.finished {
		f.finished, f.finalPhase = true, workflow.Completed
	}
	v := f.current()
	if f.i < len(f.views)-1 {
		f.i++
	}
	return v, nil
}
func (f *fakeHost) record(c computer.Control) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.controls = append(f.controls, c)
	if c.Kind == "cancel" && f.finishOnCancel {
		f.finished = true
	}
	if f.finishOn != "" && c.Kind == f.finishOn {
		f.finished, f.finalPhase = true, workflow.Completed
	}
}
func (f *fakeHost) Control(_ context.Context, _ string, c computer.Control) error {
	f.record(c)
	return nil
}
func (f *fakeHost) Assist(string) error { f.record(computer.Control{Kind: "assist"}); return nil }
func (f *fakeHost) Takeover(context.Context, string) error {
	f.record(computer.Control{Kind: "takeover"})
	return nil
}
func (f *fakeHost) Handback(context.Context, string) error {
	f.record(computer.Control{Kind: "handback"})
	return nil
}
func (f *fakeHost) ActHuman(context.Context, string, string) error {
	f.record(computer.Control{Kind: "act"})
	return nil
}
func (f *fakeHost) kinds() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.controls {
		out = append(out, c.Kind)
	}
	return out
}

func exitCode(err error) int {
	var exit *exitError
	if errors.As(err, &exit) {
		return exit.code
	}
	if err != nil {
		return exitFailed
	}
	return 0
}

func TestEventsModeCancelsAtApprovalAndExitsNeedsHuman(t *testing.T) {
	h := &fakeHost{finishOnCancel: true, views: []app.RunView{
		view(workflow.Observing, 1, "", "o-0"),
		view(workflow.AwaitingApproval, 3, "act-1", "o-1"),
	}}
	var stdout, stderr bytes.Buffer
	err := watchRun(context.Background(), h, "run-1", watchOptions{events: true, stdout: &stdout, stderr: &stderr, poll: time.Millisecond, cancelGrace: 5 * time.Second})
	if exitCode(err) != exitNeedsHuman {
		t.Fatalf("exit %d (%v), want %d", exitCode(err), err, exitNeedsHuman)
	}
	if len(h.controls) != 1 || h.controls[0].Kind != "cancel" || h.controls[0].Epoch != 3 {
		t.Fatalf("controls %+v, want one cancel at epoch 3", h.controls)
	}
	var events []runEvent
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		var e runEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("stdout line is not one JSON event: %q", line)
		}
		events = append(events, e)
	}
	var names []string
	for _, e := range events {
		names = append(names, e.Event+":"+e.Phase)
	}
	want := "phase:observing phase:awaiting_approval needs_human:awaiting_approval finished:cancelled"
	if strings.Join(names, " ") != want {
		t.Fatalf("events %v, want %s", names, want)
	}
	if gate := events[2]; gate.ActionID != "act-1" || gate.ObservationID != "o-1" || gate.Epoch != 3 {
		t.Fatalf("needs_human event %+v does not name the gate", gate)
	}
	if events[3].Run == nil || !events[3].Run.Finished {
		t.Fatal("finished event does not carry the final run view")
	}
}

func TestEventsModeBoundsTheWaitAfterCancelling(t *testing.T) {
	h := &fakeHost{views: []app.RunView{view(workflow.Paused, 2, "", "o-1")}}
	var stdout bytes.Buffer
	start := time.Now()
	err := watchRun(context.Background(), h, "run-1", watchOptions{events: true, stdout: &stdout, stderr: &stdout, poll: time.Millisecond, cancelGrace: 50 * time.Millisecond})
	if exitCode(err) != exitNeedsHuman || time.Since(start) > 5*time.Second {
		t.Fatalf("exit %d after %s (%v), want %d promptly", exitCode(err), time.Since(start), err, exitNeedsHuman)
	}
}

func operatorRun(t *testing.T, tty bool, lines ...string) (*fakeHost, string) {
	t.Helper()
	h := &fakeHost{finishOnCancel: true, views: []app.RunView{view(workflow.AwaitingApproval, 4, "act-9", "o-9")}}
	input := make(chan string, len(lines))
	for _, line := range lines {
		input <- line
	}
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// A long poll lets every queued line reach the run before the first tick.
	if err := watchRun(ctx, h, "run-1", watchOptions{lines: input, stdinTTY: tty, stdout: &stdout, stderr: &stderr, poll: 200 * time.Millisecond, cancelGrace: time.Second}); err != nil {
		t.Fatalf("run: %v", err)
	}
	return h, stderr.String()
}

func TestPipedStdinCannotApproveOrAct(t *testing.T) {
	h, stderr := operatorRun(t, false, "approve", "APPROVE", "resume", "takeover", "handback", "assist", "act click Confirm", "cancel")
	if got := strings.Join(h.kinds(), ","); got != "cancel" {
		t.Fatalf("piped stdin sent controls %s, want only cancel", got)
	}
	if strings.Count(stderr, "refused") != 7 {
		t.Fatalf("each human-only line should be refused on stderr; got:\n%s", stderr)
	}
}

func TestTerminalStdinStillApproves(t *testing.T) {
	h, _ := operatorRun(t, true, "approve", "cancel")
	if got := strings.Join(h.kinds(), ","); got != "approve,cancel" {
		t.Fatalf("terminal stdin sent %s, want approve,cancel", got)
	}
	if c := h.controls[0]; c.ActionID != "act-9" || c.ObservationID != "o-9" || c.Epoch != 4 {
		t.Fatalf("approve %+v is not bound to the pending action", c)
	}
}
