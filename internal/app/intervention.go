package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// ListInterventions returns typed intervention requests for a run.
func (a *App) ListInterventions(runID string) ([]computer.InterventionRequest, error) {
	e, err := a.entry(runID)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	run := e.run
	e.mu.Unlock()
	if run == nil {
		return []computer.InterventionRequest{}, nil
	}
	return run.Interventions(), nil
}

// Takeover transfers controller to the human on a live paused/pausable run.
func (a *App) Takeover(ctx context.Context, runID string) error {
	e, err := a.entry(runID)
	if err != nil {
		return err
	}
	e.mu.Lock()
	run := e.run
	done := e.view.Finished
	e.mu.Unlock()
	if done || run == nil {
		return errors.New("takeover requires a live run")
	}
	return run.Send(ctx, computer.Control{Kind: "takeover", Epoch: run.Snapshot().Epoch})
}

// Handback returns controller to automation after a human intervention.
func (a *App) Handback(ctx context.Context, runID string) error {
	e, err := a.entry(runID)
	if err != nil {
		return err
	}
	e.mu.Lock()
	run := e.run
	done := e.view.Finished
	e.mu.Unlock()
	if done || run == nil {
		return errors.New("handback requires a live run")
	}
	s := run.Snapshot()
	return run.Send(ctx, computer.Control{Kind: "handback", Epoch: s.Epoch})
}

// ActHuman performs one human action against a named control while paused.
// Spec is "kind name" e.g. "press Dismiss", or "set_value Passcode BRANCH-7741".
func (a *App) ActHuman(ctx context.Context, runID, spec string) error {
	kind, name, text, err := parseActSpec(spec)
	if err != nil {
		return err
	}
	e, err := a.entry(runID)
	if err != nil {
		return err
	}
	e.mu.Lock()
	run := e.run
	done := e.view.Finished
	e.mu.Unlock()
	if done || run == nil {
		return errors.New("act requires a live run")
	}
	s := run.Snapshot()
	if s.Phase != workflow.Paused {
		return errors.New("act requires a paused run")
	}
	o := run.Observation()
	if o == nil || o.Epoch != s.Epoch {
		return errors.New("fresh paused observation required; refresh first")
	}
	var target *computer.Node
	for i := range o.Nodes {
		n := &o.Nodes[i]
		if n.Name == name && n.Enabled {
			target = n
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no enabled control named %q in the current observation", name)
	}
	actionID := s.RunID + ":human:" + name
	action := computer.Action{
		ActionID:      actionID,
		ObservationID: o.ID,
		TargetID:      target.ID,
		Kind:          kind,
		Effect:        "read",
	}
	if text != nil {
		action.Text = text
	}
	return run.Send(ctx, computer.Control{
		Kind:          "human_act",
		Epoch:         s.Epoch,
		ActionID:      actionID,
		ObservationID: o.ID,
		Action:        action,
	})
}

func parseActSpec(spec string) (kind, name string, text *string, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", "", nil, errors.New("act requires kind and control name")
	}
	parts := strings.SplitN(spec, " ", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", "", nil, errors.New("act requires kind and control name, e.g. press Dismiss")
	}
	kind = strings.ToLower(strings.TrimSpace(parts[0]))
	rest := strings.TrimSpace(parts[1])
	switch kind {
	case "press", "click":
		return kind, rest, nil, nil
	case "set_value", "type_text":
		nameAndValue := strings.SplitN(rest, " ", 2)
		if len(nameAndValue) != 2 || strings.TrimSpace(nameAndValue[1]) == "" {
			return "", "", nil, errors.New("set_value requires control name and text, e.g. set_value Passcode BRANCH-7741")
		}
		value := strings.TrimSpace(nameAndValue[1])
		return kind, strings.TrimSpace(nameAndValue[0]), &value, nil
	default:
		return "", "", nil, fmt.Errorf("unsupported act kind %q", kind)
	}
}
