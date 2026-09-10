package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/devcouncil"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

type Recorder struct {
	*desktopJournal
	mu            sync.Mutex
	bundle        devcouncil.EvidenceBundle
	sequence      uint64
	sideSequence  uint64
	contract      []byte
	program       *workflow.Program
	pendingHuman  []pendingHumanAction
	seenRecovery  map[string]bool
}

type pendingHumanAction struct {
	ID   string
	Kind string
}

func NewRecorder(dir string, program *workflow.Program, contract []byte, session computer.Session, policySHA256 string) (*Recorder, error) {
	var expected devcouncil.EvidenceContract
	if err := workflow.DecodeStrict(contract, &expected); err != nil {
		return nil, err
	}
	if expected.SchemaVersion != 1 || len(expected.Criteria) == 0 {
		return nil, errors.New("acceptance contract is empty or unsupported")
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, fmt.Errorf("evidence destination must be new: %w", err)
	}
	journal, err := openDesktopJournal(dir)
	if err != nil {
		return nil, err
	}
	r := &Recorder{
		desktopJournal: journal,
		contract:       append([]byte(nil), contract...),
		program:        program,
		seenRecovery:   map[string]bool{},
		bundle: devcouncil.EvidenceBundle{
			SchemaVersion:    1,
			RunID:            session.RunID,
			SessionID:        session.ID,
			Epoch:            session.Epoch,
			ContractSHA256:   digest(contract),
			CapabilitySHA256: program.Digest(),
			PolicySHA256:     policySHA256,
			Degraded:         []string{},
			Actions:          []devcouncil.EvidenceAction{},
			Observations:     []devcouncil.EvidenceObservation{},
			Artifacts:        []devcouncil.EvidenceArtifact{},
			Interventions:    []devcouncil.EvidenceIntervention{},
			HumanActions:     []devcouncil.EvidenceHumanAction{},
			Recoveries:       []devcouncil.EvidenceRecoveryApplied{},
			LocatorHits:      []devcouncil.EvidenceLocatorHit{},
		},
	}
	if err := writeAtomic(filepath.Join(dir, "contract.json"), contract); err != nil {
		journal.file.Close()
		return nil, err
	}
	if err := writeAtomic(filepath.Join(dir, "capability.json"), program.Bytes()); err != nil {
		journal.file.Close()
		return nil, err
	}
	return r, nil
}
func (r *Recorder) appendAction(id, disposition string) {
	r.sequence++
	r.bundle.Actions = append(r.bundle.Actions, devcouncil.EvidenceAction{ID: id, Sequence: r.sequence, Disposition: disposition})
}
func (r *Recorder) actionIndex(id string) int {
	for i, a := range r.bundle.Actions {
		if a.ID == id {
			return i
		}
	}
	return -1
}
func (r *Recorder) lastActionID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.bundle.Actions) == 0 {
		return ""
	}
	return r.bundle.Actions[len(r.bundle.Actions)-1].ID
}
func (r *Recorder) Record(record computer.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if record.Kind == "action_started" {
		if r.actionIndex(record.ActionID) < 0 {
			r.appendAction(record.ActionID, "unknown")
		}
		if record.RecoveryID != "" && !r.seenRecovery[record.RecoveryID] {
			r.seenRecovery[record.RecoveryID] = true
			r.sideSequence++
			r.bundle.Recoveries = append(r.bundle.Recoveries, devcouncil.EvidenceRecoveryApplied{
				ID:       record.RecoveryID,
				Sequence: r.sideSequence,
				StepID:   stepIDFromAction(record.ActionID, r.bundle.RunID),
			})
		}
	}
	if record.Kind == "event" && record.Event != nil && record.Event.NextEpoch > record.Event.Epoch {
		r.sequence++
		r.bundle.EpochTransitions = append(r.bundle.EpochTransitions, devcouncil.EvidenceEpochTransition{Sequence: r.sequence, FromEpoch: record.Event.Epoch, ToEpoch: record.Event.NextEpoch, Reason: record.Event.Kind})
	}
	if record.Kind == "event" && record.Event != nil && record.Event.Kind == "receipt" {
		disposition := "unknown"
		switch record.Event.Delivery {
		case "sent":
			disposition = "succeeded"
		case "not_sent":
			disposition = "not_dispatched"
		}
		i := r.actionIndex(record.ActionID)
		if i < 0 {
			return errors.New("receipt has no durable action admission")
		}
		r.bundle.Actions[i].Disposition = disposition
	}
	if record.Kind == "event" && record.Event != nil && record.Event.Kind == "observed" && record.Event.Observation.Complete && record.Event.Observation.Matches == 1 {
		for _, step := range r.program.Capability().Steps {
			if record.ActionID == r.bundle.RunID+":"+step.ID && (step.Kind == "extract" || step.Kind == "assert" || step.Kind == "branch" || step.Kind == "wait") {
				i := r.actionIndex(record.ActionID)
				if i < 0 {
					return errors.New("read result has no durable admission")
				}
				r.bundle.Actions[i].Disposition = "succeeded"
			}
		}
		if target := record.Event.Observation.Target; target != "" {
			r.sideSequence++
			r.bundle.LocatorHits = append(r.bundle.LocatorHits, devcouncil.EvidenceLocatorHit{
				Target:        target,
				StrategyIndex: uint64(record.Event.Observation.StrategyIndex),
				Sequence:      r.sideSequence,
			})
		}
	}
	if record.Kind == "human_action" && record.Receipt != nil {
		disposition := "unknown"
		if record.Receipt.Delivery == "sent" {
			disposition = "succeeded"
		}
		i := r.actionIndex(record.ActionID)
		if i < 0 {
			return errors.New("human receipt has no durable action admission")
		}
		r.bundle.Actions[i].Disposition = disposition
		kind := record.StepKind
		if kind == "" {
			kind = "press"
		}
		r.pendingHuman = append(r.pendingHuman, pendingHumanAction{ID: record.ActionID, Kind: kind})
	}
	if record.Kind == "observation" && record.Observation != nil {
		r.sequence++
		if record.StrategyIndex > 0 || (record.RecoveryID == "" && record.Stage != "" && record.Stage != "outcome_reconciliation") {
			// Ladder hits are authoritative on observed events; observation
			// StrategyIndex still records drift when the runner resolves a fallback rung.
			if record.StrategyIndex > 0 {
				target := ""
				if record.ActionID != "" {
					target = stepIDFromAction(record.ActionID, r.bundle.RunID)
				}
				if target != "" {
					r.sideSequence++
					r.bundle.LocatorHits = append(r.bundle.LocatorHits, devcouncil.EvidenceLocatorHit{
						Target:        target,
						StrategyIndex: uint64(record.StrategyIndex),
						Sequence:      r.sideSequence,
					})
				}
			}
		}
	}
	stored, artifacts, err := r.desktopJournal.append(record, r.sequence)
	if err != nil {
		return err
	}
	r.bundle.Artifacts = append(r.bundle.Artifacts, artifacts...)
	if stored.Kind == "observation" && stored.Observation != nil {
		o := *stored.Observation
		isRead := record.StepKind == "extract" || record.StepKind == "assert" || record.StepKind == "branch" || record.StepKind == "wait"
		if record.Stage == "observe_after" || record.Stage == "human_after" || record.Stage == "outcome_reconciliation" || isRead {
			r.sequence++
			facts := factsFromObservation(o)
			r.bundle.Observations = append(r.bundle.Observations, devcouncil.EvidenceObservation{Sequence: r.sequence, RunID: r.bundle.RunID, SessionID: r.bundle.SessionID, Epoch: record.Epoch, AfterActionID: record.ActionID, Facts: facts, ArtifactIDs: []string{artifacts[0].ID, artifacts[1].ID}})
		}
	}
	return nil
}
func stepIDFromAction(actionID, runID string) string {
	prefix := runID + ":"
	if strings.HasPrefix(actionID, prefix) {
		return strings.TrimPrefix(actionID, prefix)
	}
	if i := strings.LastIndex(actionID, ":"); i >= 0 {
		return actionID[i+1:]
	}
	return actionID
}
func factsFromObservation(o computer.Observation) map[string]json.RawMessage {
	facts := map[string]json.RawMessage{}
	counts := map[string]int{}
	for _, n := range o.Nodes {
		if n.Role == "text_field" {
			counts[n.Name]++
		}
	}
	for _, n := range o.Nodes {
		// Bank facts bind to value fields. AT-SPI also exposes their separate
		// labels with the same name; labels cannot supply financial values.
		if n.Role != "text_field" || counts[n.Name] != 1 || n.Value == nil || strings.Contains(*n.Value, "[redacted]") {
			continue
		}
		text := *n.Value
		data, err := json.Marshal(text)
		if err != nil {
			continue
		}
		facts["ui."+n.Name] = data
		if n.Name == "Balance" {
			v, err := computer.ParseValue(text, "money", "USD")
			if err == nil {
				number, err := json.Marshal(v.Integer)
				if err == nil {
					facts["balance_minor"] = number
				}
				facts["currency"] = json.RawMessage(`"USD"`)
			}
		}
	}
	return facts
}
func (r *Recorder) Finish(result computer.RunResult, interventions []computer.InterventionRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	terminalOK := result.Error == "" && (result.State.Phase == workflow.Completed || result.State.Phase == workflow.Concluded)
	r.bundle.JournalComplete = terminalOK
	if r.bundle.EpochTransitions == nil {
		r.bundle.EpochTransitions = []devcouncil.EvidenceEpochTransition{}
	}
	if !r.bundle.JournalComplete {
		r.bundle.Degraded = append(r.bundle.Degraded, string(result.State.Phase))
	}
	if outcomeID := result.State.Outcome; outcomeID != "" {
		kind := "success"
		for _, o := range r.program.Capability().Outcomes {
			if o.ID == outcomeID {
				kind = o.Kind
				break
			}
		}
		if kind != "success" && kind != "business" {
			kind = "success"
		}
		r.bundle.Outcome = &devcouncil.EvidenceOutcome{ID: outcomeID, Kind: kind}
	}
	returnedID := ""
	for _, iv := range interventions {
		r.sideSequence++
		status := iv.Status
		if status == "" {
			status = computer.InterventionRequested
		}
		id := iv.ID
		if id == "" {
			id = fmt.Sprintf("%s:intervention:%d", r.bundle.RunID, r.sideSequence)
		}
		r.bundle.Interventions = append(r.bundle.Interventions, devcouncil.EvidenceIntervention{
			ID:         id,
			Sequence:   r.sideSequence,
			ReasonCode: iv.ReasonCode,
			Status:     status,
		})
		if status == computer.InterventionReturned {
			returnedID = id
		}
	}
	if returnedID != "" {
		for _, h := range r.pendingHuman {
			r.sideSequence++
			r.bundle.HumanActions = append(r.bundle.HumanActions, devcouncil.EvidenceHumanAction{
				ID:             h.ID,
				Sequence:       r.sideSequence,
				Kind:           h.Kind,
				InterventionID: returnedID,
			})
		}
	}
	if err := r.file.Close(); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(r.dir, "trace.json"), result); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(r.dir, "bundle.json"), r.bundle); err != nil {
		return err
	}
	return writeJSON(filepath.Join(r.dir, "perfetto.json"), traceEvents(result))
}

type traceEvent struct {
	Name      string `json:"name"`
	Category  string `json:"cat"`
	Phase     string `json:"ph"`
	Timestamp int64  `json:"ts"`
	PID       int    `json:"pid"`
	TID       int    `json:"tid"`
	Scope     string `json:"s"`
}

func traceEvents(result computer.RunResult) []traceEvent {
	out := make([]traceEvent, 0, len(result.Events))
	for _, e := range result.Events {
		out = append(out, traceEvent{Name: e.Kind, Category: "workflow", Phase: "i", Timestamp: e.ElapsedMillis * 1000, PID: 1, TID: 1, Scope: "t"})
	}
	return out
}
