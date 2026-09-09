package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/bharathvbcr/Jarvis/internal/app"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

type trialApprovals struct {
	Denied  bool
	Granted []string
}
type expectedForm struct{ Tenant, MemberID, SubaccountName string }
type formField struct {
	ID, Name, Value   string
	Enabled, Editable bool
}
type approvalForm struct {
	RunID, SessionID, ActionID, ObservationID, TargetID string
	Epoch                                               uint64
	Window                                              computer.Window
	Expected                                            expectedForm
	Captured                                            []formField
}
type approvalPrompt struct {
	Form            approvalForm
	Digest, Command string
}
type approver interface {
	Decide(context.Context, approvalPrompt) (bool, error)
}

func makeApprovalPrompt(view app.RunView, id string, expected expectedForm) (approvalPrompt, error) {
	s, o := view.State, view.Observation
	if view.Finished || view.RunID != id || s.RunID != id || s.SessionID == "" || s.Epoch == 0 || s.Phase != workflow.AwaitingApproval || view.PendingActionID == "" || s.Observation.ID == "" || s.Observation.TargetID == "" || !s.Observation.Complete || !s.Observation.Actionable || s.Observation.Matches != 1 || o == nil || !o.Complete || o.Window.PID == 0 || o.ID != s.Observation.ID || o.Epoch != s.Epoch || o.TruncatedReason != "" || len(o.Nodes) > 2000 {
		return approvalPrompt{}, errors.New("approval requires a complete, current, uniquely bound native observation")
	}
	if s.StepIndex < 0 || s.StepIndex >= len(view.Capability.Steps) {
		return approvalPrompt{}, errors.New("approval step is unavailable")
	}
	step := view.Capability.Steps[s.StepIndex]
	if view.Capability.Application != "jarvis-bank" || step.Effect != "change" || step.Kind != "press" || view.Capability.Targets[step.Target].Name != "Confirm creation" {
		return approvalPrompt{}, errors.New("qualification only admits explicit approval of the synthetic bank's Confirm creation action")
	}
	form := approvalForm{RunID: id, SessionID: s.SessionID, ActionID: view.PendingActionID, ObservationID: o.ID, TargetID: s.Observation.TargetID, Epoch: s.Epoch, Window: o.Window, Expected: expected, Captured: []formField{}}
	targets := 0
	for _, n := range o.Nodes {
		if n.ID == form.TargetID && n.Role == "button" && n.Name == "Confirm creation" && n.Enabled {
			targets++
		}
		if n.Role == "text_field" && n.Value != nil {
			if len(n.ID)+len(n.Name)+len(*n.Value) > 8192 {
				return approvalPrompt{}, errors.New("approval form field exceeds display bound")
			}
			form.Captured = append(form.Captured, formField{n.ID, n.Name, *n.Value, n.Enabled, n.Editable})
		}
	}
	if targets != 1 || len(form.Captured) > 64 {
		return approvalPrompt{}, errors.New("approval target or form is ambiguous")
	}
	sort.Slice(form.Captured, func(i, j int) bool { return form.Captured[i].ID < form.Captured[j].ID })
	raw, err := json.Marshal(form)
	if err != nil {
		return approvalPrompt{}, err
	}
	if len(raw) > 65536 {
		return approvalPrompt{}, errors.New("approval form exceeds 64 KiB")
	}
	digest := sha256.Sum256(raw)
	bound := hex.EncodeToString(digest[:])
	return approvalPrompt{form, bound, fmt.Sprintf("approve %s %s %d %s %s", id, form.ActionID, form.Epoch, form.ObservationID, bound)}, nil
}

type inputLine struct {
	text string
	err  error
}
type terminalApprover struct {
	lines  <-chan inputLine
	output io.Writer
}

// One bounded reader lives for the campaign. Cancellation never creates a new
// blocked stdin reader, and a stale/prefetched line cannot match a future identity.
func newTerminalApprover(ctx context.Context, input *os.File, output io.Writer) (*terminalApprover, error) {
	info, err := input.Stat()
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return nil, errors.New("--interactive requires an attached terminal; piped approval is refused")
	}
	lines := make(chan inputLine, 1)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 256), 4096)
		for scanner.Scan() {
			select {
			case lines <- inputLine{text: scanner.Text()}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case lines <- inputLine{err: err}:
			case <-ctx.Done():
			}
		}
	}()
	return &terminalApprover{lines, output}, nil
}

func (t *terminalApprover) Decide(ctx context.Context, prompt approvalPrompt) (bool, error) {
	raw, err := json.MarshalIndent(prompt.Form, "", "  ")
	if err != nil {
		return false, err
	}
	if _, err = fmt.Fprintf(t.output, "\nSynthetic bank approval checkpoint\n%s\nCaptured sensitive fields may be redacted. Inspect the native bank form and compare the independently requested member/name above. This authorizes one action only; the broker revalidates the captured form.\nType exactly:\n%s\nOr type deny or cancel. The run deadline still applies.\n> ", raw, prompt.Command); err != nil {
		return false, err
	}
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case line, ok := <-t.lines:
			if !ok {
				return false, io.EOF
			}
			if line.err != nil {
				return false, line.err
			}
			switch strings.TrimSpace(line.text) {
			case prompt.Command:
				return true, nil
			case "deny":
				return false, nil
			case "cancel":
				return false, context.Canceled
			default:
				if _, err = fmt.Fprintln(t.output, "No approval sent: input did not match the current full identity. Type the displayed command, deny, or cancel."); err != nil {
					return false, err
				}
			}
		}
	}
}

func waitTrial(ctx context.Context, host trialHost, id string) (app.RunView, bool, error) {
	view, approvals, err := waitTrialWithApproval(ctx, host, id, nil, expectedForm{})
	return view, approvals.Denied, err
}

func waitTrialWithApproval(ctx context.Context, host trialHost, id string, human approver, expected expectedForm) (app.RunView, trialApprovals, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	approvals := trialApprovals{Granted: []string{}}
	lastSubmitted := ""
	for {
		view, err := host.Get(id)
		if err != nil {
			return view, approvals, err
		}
		if view.Finished {
			return view, approvals, nil
		}
		if err := ctx.Err(); err != nil {
			v, e := cancelTrial(host, id, view, err)
			return v, approvals, e
		}
		if view.State.Phase == workflow.AwaitingApproval && !approvals.Denied {
			kind := "deny"
			if human != nil {
				prompt, err := makeApprovalPrompt(view, id, expected)
				if err != nil {
					v, e := cancelTrial(host, id, view, err)
					return v, approvals, e
				}
				if prompt.Command == lastSubmitted {
					select {
					case <-ctx.Done():
					case <-ticker.C:
					}
					continue
				}
				approved, err := human.Decide(ctx, prompt)
				if err != nil {
					v, e := cancelTrial(host, id, view, err)
					return v, approvals, e
				}
				fresh, err := host.Get(id)
				if err != nil {
					v, e := cancelTrial(host, id, view, err)
					return v, approvals, e
				}
				current, err := makeApprovalPrompt(fresh, id, expected)
				if err != nil || current.Command != prompt.Command {
					continue
				}
				view = fresh
				lastSubmitted = current.Command
				if approved {
					kind = "approve"
				}
			}
			if err = host.Control(ctx, id, computer.Control{Kind: kind, Epoch: view.State.Epoch, ActionID: view.PendingActionID, ObservationID: view.State.Observation.ID}); err != nil {
				v, e := cancelTrial(host, id, view, err)
				return v, approvals, e
			}
			if kind == "approve" {
				approvals.Granted = append(approvals.Granted, view.PendingActionID)
			} else {
				approvals.Denied = true
			}
		}
		if view.State.Phase == workflow.Paused {
			v, e := cancelTrial(host, id, view, errors.New("native run paused: "+view.State.Reason))
			return v, approvals, e
		}
		select {
		case <-ctx.Done():
			v, e := cancelTrial(host, id, view, ctx.Err())
			return v, approvals, e
		case <-ticker.C:
		}
	}
}
