package main

import (
	"bufio"
	"bytes"
	"context"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bharathvbcr/Jarvis/internal/app"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// runHost is the part of *app.App that replay and discover drive.
type runHost interface {
	Get(id string) (app.RunView, error)
	GetSince(id string, cursor int) (app.RunView, error)
	Control(ctx context.Context, id string, c computer.Control) error
	Assist(id string) error
	Takeover(ctx context.Context, id string) error
	Handback(ctx context.Context, id string) error
	ActHuman(ctx context.Context, id, spec string) error
}

type watchOptions struct {
	// events writes one NDJSON event per line on stdout. Without answers, a
	// run that reaches a human gate is cancelled and the command exits
	// exitNeedsHuman: an unattended caller cannot answer the gate.
	events bool
	// answers are yolo-mode control lines from stdin (--events --yolo); the
	// channel closes at EOF. Nil means no caller answers gates.
	answers <-chan answer
	// lines are the operator's stdin lines in interactive mode; nil reads none.
	lines <-chan string
	// stdinTTY reports that stdin is a terminal.
	stdinTTY       bool
	stdout, stderr io.Writer
	poll           time.Duration
	// gateTimeout bounds how long a gate waits for an answer.
	gateTimeout time.Duration
	// cancelGrace bounds the wait for a run cancelled at a human gate.
	cancelGrace time.Duration
}

// runEvent is one --events line.
type runEvent struct {
	Event         string       `json:"event"`
	RunID         string       `json:"run_id"`
	Phase         string       `json:"phase,omitempty"`
	Epoch         uint64       `json:"epoch,omitempty"`
	ActionID      string       `json:"action_id,omitempty"`
	ObservationID string       `json:"observation_id,omitempty"`
	Controller    string       `json:"controller,omitempty"`
	Reason        string       `json:"reason,omitempty"`
	Answer        string       `json:"answer,omitempty"`
	Path          string       `json:"path,omitempty"`
	Run           *app.RunView `json:"run,omitempty"`
}

// controlEvent acknowledges one answer line. Sent means the control reached
// the run, not that the run applied it: a run that refuses it journals
// control_refused, which is surfaced as its own event.
type controlEvent struct {
	Event  string `json:"event"`
	RunID  string `json:"run_id"`
	Kind   string `json:"kind"`
	Sent   bool   `json:"sent"`
	Reason string `json:"reason,omitempty"`
}

// controlLine is one yolo answer on stdin. Decoding is strict: unknown or
// duplicate members and case-variant names are refused.
type controlLine struct {
	Kind          string  `json:"kind"`
	Epoch         *uint64 `json:"epoch,omitempty"`
	ActionID      string  `json:"action_id,omitempty"`
	ObservationID string  `json:"observation_id,omitempty"`
	Spec          string  `json:"spec,omitempty"`
}

type answer struct {
	line controlLine
	err  error
}

const maxAnswerLine = 64 << 10

// readAnswers decodes stdin into answers and closes the channel at EOF or on
// a read error (reported first).
func readAnswers(ctx context.Context, r io.Reader) <-chan answer {
	out := make(chan answer)
	go func() {
		defer close(out)
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 4096), maxAnswerLine)
		send := func(a answer) bool {
			select {
			case out <- a:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for scanner.Scan() {
			text := bytes.TrimSpace(scanner.Bytes())
			if len(text) == 0 {
				continue
			}
			var line controlLine
			err := jsonv2.Unmarshal(text, &line, jsonv2.RejectUnknownMembers(true))
			if !send(answer{line: line, err: err}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			send(answer{err: fmt.Errorf("stdin: %w", err)})
		}
	}()
	return out
}

func atHumanGate(phase workflow.Phase) bool {
	return phase == workflow.AwaitingApproval || phase == workflow.Paused
}

// watchRun follows run id to its end. Interactively it prompts on stderr at
// each human gate and forwards operator lines as controls. With events it
// streams NDJSON; a gate is answered from o.answers (yolo) or, with no one
// to answer, the run is cancelled and the command exits exitNeedsHuman.
func watchRun(ctx context.Context, h runHost, id string, o watchOptions) error {
	ticker := time.NewTicker(o.poll)
	defer ticker.Stop()
	var prompted, lastPhase string
	var gate *runEvent
	var gateDeadline, cancelDeadline <-chan time.Time
	answering, stdinClosed, cancelledAtGate := o.answers != nil, false, false
	seen := 0
	emit := func(v any) error {
		if !o.events {
			return nil
		}
		return writeLine(o.stdout, v)
	}
	cancelAtGate := func(epoch uint64) error {
		gateDeadline = nil
		if cancelledAtGate {
			return nil
		}
		cancelledAtGate = true
		if err := h.Control(ctx, id, computer.Control{Kind: "cancel", Epoch: epoch}); err != nil {
			return &exitError{code: exitNeedsHuman, err: fmt.Errorf("run %s reached a human gate and could not be cancelled: %w", id, err)}
		}
		cancelDeadline = time.After(o.cancelGrace)
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			v, _ := h.Get(id)
			_ = h.Control(context.Background(), id, computer.Control{Kind: "cancel", Epoch: v.State.Epoch})
			return ctx.Err()
		case <-cancelDeadline:
			return &exitError{code: exitNeedsHuman, err: fmt.Errorf("run %s stopped at %s and did not finish within %s of being cancelled", id, gate.Phase, o.cancelGrace)}
		case <-gateDeadline:
			if err := emit(runEvent{Event: "gate_timeout", RunID: id, Phase: gate.Phase, Reason: fmt.Sprintf("no answer within %s", o.gateTimeout)}); err != nil {
				return err
			}
			if err := cancelAtGate(gate.Epoch); err != nil {
				return err
			}
		case a, ok := <-o.answers:
			if !ok {
				o.answers, stdinClosed = nil, true
				if gateDeadline != nil {
					if err := cancelAtGate(gate.Epoch); err != nil {
						return err
					}
				}
				continue
			}
			kind, sent, reason := applyAnswer(ctx, h, id, a)
			if err := emit(controlEvent{Event: "control", RunID: id, Kind: kind, Sent: sent, Reason: reason}); err != nil {
				return err
			}
		case text := <-o.lines:
			if err := operatorLine(ctx, h, id, text, o); err != nil {
				return err
			}
		case <-ticker.C:
			// The poll needs the state, not the history: ask only for the
			// records that arrived since the last tick.
			v, err := h.GetSince(id, seen)
			if err != nil {
				return err
			}
			seen = v.TotalRecords
			for _, r := range v.Records {
				if r.Kind == "control_refused" {
					if err := emit(runEvent{Event: "control_refused", RunID: id, Epoch: r.Epoch, Reason: r.Reason}); err != nil {
						return err
					}
				}
			}
			if v.Finished {
				if v, err = h.Get(id); err != nil {
					return err
				}
				if o.events {
					err = writeLine(o.stdout, runEvent{Event: "finished", RunID: id, Phase: string(v.State.Phase), Run: &v})
				} else {
					err = writeJSON(o.stdout, v)
				}
				if err != nil {
					return err
				}
				if cancelledAtGate {
					return &exitError{code: exitNeedsHuman, err: fmt.Errorf("run %s reached a human gate (%s, action %q) that no one answered and was cancelled; answer it on stdin with --events --yolo, at a terminal with jarvis replay, or in the workbench", id, gate.Phase, gate.ActionID)}
				}
				if v.Error != "" {
					return errors.New(v.Error)
				}
				if v.Report != nil && v.Report.Verdict != "passed" {
					return fmt.Errorf("independent verification: %s", v.Report.Verdict)
				}
				return nil
			}
			if string(v.State.Phase) != lastPhase {
				lastPhase = string(v.State.Phase)
				if err := emit(runEvent{Event: "phase", RunID: id, Phase: lastPhase, Epoch: v.State.Epoch, Controller: v.Controller, Reason: v.State.Reason}); err != nil {
					return err
				}
			}
			if !atHumanGate(v.State.Phase) {
				// The gate was answered; its timer no longer applies.
				gateDeadline = nil
				continue
			}
			key := string(v.State.Phase) + v.PendingActionID + v.State.Observation.ID + fmt.Sprint(v.State.Epoch)
			if key == prompted {
				continue
			}
			prompted = key
			if !o.events {
				fmt.Fprintf(o.stderr, "%s: action=%s observation=%s epoch=%d controller=%s reason=%s\nType approve, deny, cancel, pause, takeover, act <kind name>, handback, refresh, focus, or resume.\n", v.State.Phase, v.PendingActionID, v.State.Observation.ID, v.State.Epoch, v.Controller, v.State.Reason)
				continue
			}
			if cancelledAtGate {
				continue
			}
			gate = &runEvent{Event: "needs_human", RunID: id, Phase: string(v.State.Phase), Epoch: v.State.Epoch, ActionID: v.PendingActionID, ObservationID: v.State.Observation.ID, Controller: v.Controller, Reason: v.State.Reason}
			if answering && !stdinClosed {
				gate.Answer = "stdin"
			}
			if err := writeLine(o.stdout, gate); err != nil {
				return err
			}
			if gate.Answer == "" {
				if err := cancelAtGate(v.State.Epoch); err != nil {
					return err
				}
				continue
			}
			gateDeadline = time.After(o.gateTimeout)
		}
	}
}

// applyAnswer checks one yolo answer against the run's current state and
// sends it. Approve and deny must name the pending gate exactly, and every
// control but assist must carry the current epoch, so an answer written for
// one gate cannot land on the next.
func applyAnswer(ctx context.Context, h runHost, id string, a answer) (kind string, sent bool, reason string) {
	if a.err != nil {
		return "", false, a.err.Error()
	}
	c := a.line
	kind = c.Kind
	v, err := h.Get(id)
	if err != nil {
		return kind, false, err.Error()
	}
	epoch := v.State.Epoch
	current := c.Epoch != nil && *c.Epoch == epoch
	stale := fmt.Sprintf("epoch must be the current epoch %d", epoch)
	switch kind {
	case "approve", "deny":
		if v.State.Phase != workflow.AwaitingApproval {
			return kind, false, "no approval is pending"
		}
		if !current || c.ActionID != v.PendingActionID || c.ObservationID != v.State.Observation.ID {
			return kind, false, fmt.Sprintf("does not match the pending gate: epoch %d, action_id %q, observation_id %q", epoch, v.PendingActionID, v.State.Observation.ID)
		}
		err = h.Control(ctx, id, computer.Control{Kind: kind, Epoch: epoch, ActionID: v.PendingActionID, ObservationID: v.State.Observation.ID})
	case "cancel", "pause", "resume", "refresh", "focus":
		if !current {
			return kind, false, stale
		}
		err = h.Control(ctx, id, computer.Control{Kind: kind, Epoch: epoch, ActionID: v.PendingActionID, ObservationID: v.State.Observation.ID})
	case "takeover", "handback", "act":
		if !current {
			return kind, false, stale
		}
		switch kind {
		case "takeover":
			err = h.Takeover(ctx, id)
		case "handback":
			err = h.Handback(ctx, id)
		default:
			if strings.TrimSpace(c.Spec) == "" {
				return kind, false, "act needs a spec, such as \"press Submit\""
			}
			err = h.ActHuman(ctx, id, strings.TrimSpace(c.Spec))
		}
	case "assist":
		err = h.Assist(id)
	default:
		return kind, false, "unknown kind; use approve, deny, cancel, pause, resume, refresh, focus, takeover, handback, act or assist"
	}
	if err != nil {
		return kind, false, err.Error()
	}
	return kind, true, ""
}

// humanOnlyLine reports an operator line that approves, resumes, acts or
// changes who controls the desktop. Those are accepted only from a terminal,
// so a caller piping stdin (an agent, a script) cannot answer a human gate.
// A pseudo-terminal still passes: this stops accidents, not a determined
// same-user process. Agents answer gates through --events --yolo instead.
func humanOnlyLine(kind string) bool {
	switch kind {
	case "approve", "resume", "assist", "takeover", "handback":
		return true
	}
	return strings.HasPrefix(kind, "act ")
}

// operatorLine forwards one stdin line as a control. Failures are reported
// and the run goes on, as they always have been.
func operatorLine(ctx context.Context, h runHost, id, text string, o watchOptions) error {
	v, err := h.Get(id)
	if err != nil {
		return err
	}
	line := strings.TrimSpace(text)
	kind := strings.ToLower(line)
	if humanOnlyLine(kind) && !o.stdinTTY {
		fmt.Fprintf(o.stderr, "refused %q: it needs a person at a terminal, and stdin is not one. Piped stdin may send deny, cancel, pause, refresh or focus; agents answer gates with --events --yolo.\n", line)
		return nil
	}
	switch {
	case kind == "assist":
		err = h.Assist(id)
	case kind == "takeover":
		err = h.Takeover(ctx, id)
	case kind == "handback":
		err = h.Handback(ctx, id)
	case strings.HasPrefix(kind, "act "):
		err = h.ActHuman(ctx, id, strings.TrimSpace(line[3:]))
	case kind == "approve" || kind == "deny" || kind == "cancel" || kind == "pause" || kind == "resume" || kind == "refresh" || kind == "focus":
		err = h.Control(ctx, id, computer.Control{Kind: kind, Epoch: v.State.Epoch, ActionID: v.PendingActionID, ObservationID: v.State.Observation.ID})
	}
	if err != nil {
		fmt.Fprintln(o.stderr, err)
	}
	return nil
}
