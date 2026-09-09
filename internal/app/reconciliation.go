package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/devcouncil"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// Reconciliation reports a fresh observation after the input fence. It cannot
// decide whether an uncertain account change committed, authorize recovery, or
// overwrite the terminal workflow state.
type Reconciliation struct {
	Status        string                     `json:"status"`
	RunID         string                     `json:"run_id"`
	SessionID     string                     `json:"session_id"`
	Epoch         uint64                     `json:"epoch"`
	ActionID      string                     `json:"action_id"`
	ObservationID string                     `json:"observation_id,omitempty"`
	Reason        string                     `json:"reason"`
	ObservedFacts map[string]json.RawMessage `json:"observed_facts"`
	Acceptance    string                     `json:"acceptance"`
}

type reconciliationObserver interface {
	Observe(context.Context, computer.Session) (computer.Observation, error)
}

func observeUnknown(ctx context.Context, observer reconciliationObserver, session computer.Session, privacy computer.PrivacyPolicy, state workflow.State, actionID string) (Reconciliation, *computer.Observation) {
	report := Reconciliation{Status: "unavailable", RunID: state.RunID, SessionID: state.SessionID, Epoch: state.Epoch, ActionID: actionID, Acceptance: "incomplete", ObservedFacts: map[string]json.RawMessage{}}
	if state.Phase != workflow.Unknown || actionID == "" || session.RunID != state.RunID || session.ID != state.SessionID || session.Epoch != state.Epoch {
		report.Reason = "reconciliation requires the exact fenced unknown run and session"
		return report, nil
	}
	// Run cancellation fences input first. A separate two-second cleanup budget
	// permits only observation of that same owned session, never another action.
	captureCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	started := uint64(time.Now().UnixMilli())
	raw, err := observer.Observe(captureCtx, session)
	if err == nil {
		err = captureCtx.Err()
	}
	if err != nil {
		report.Reason = privacy.Scrub(err.Error())
		return report, nil
	}
	if raw.ID == "" || raw.ID == state.Observation.ID {
		report.Reason = "reconciliation requires a new observation identity"
		return report, nil
	}
	if raw.Epoch != state.Epoch || raw.CapturedAtMillis < started || raw.TreeAtMillis < started {
		report.Reason = "reconciliation capture has a stale epoch or predates the request"
		return report, nil
	}
	safe, err := computer.Sanitize(raw, privacy)
	if err != nil {
		report.Reason = privacy.Scrub(err.Error())
		return report, nil
	}
	report.Status = "observed"
	report.ObservationID = safe.ID
	report.Reason = "fresh UI evidence retained; input delivery and durable account outcome remain unknown"
	report.ObservedFacts = factsFromObservation(safe)
	return report, &safe
}

func (r *Recorder) RecordReconciliation(report Reconciliation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if report.RunID != r.bundle.RunID || report.SessionID != r.bundle.SessionID || report.Acceptance != "incomplete" {
		return errors.New("reconciliation identity or acceptance mismatch")
	}
	artifact, err := storeReconciliation(r.dir, report)
	if err != nil {
		return err
	}
	r.bundle.Artifacts = append(r.bundle.Artifacts, artifact)
	return nil
}
func storeReconciliation(dir string, report Reconciliation) (devcouncil.EvidenceArtifact, error) {
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return devcouncil.EvidenceArtifact{}, err
	}
	if len(raw) > 1<<20 {
		return devcouncil.EvidenceArtifact{}, errors.New("reconciliation report exceeds one MiB")
	}
	const name = "reconciliation.json"
	if err = writeAtomic(filepath.Join(dir, name), raw); err != nil {
		return devcouncil.EvidenceArtifact{}, err
	}
	return devcouncil.EvidenceArtifact{ID: "outcome-reconciliation", Path: name, SHA256: digest(raw), SizeBytes: uint64(len(raw))}, nil
}
