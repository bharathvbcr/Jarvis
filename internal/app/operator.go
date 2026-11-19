package app

import (
	"errors"
	"path/filepath"

	"github.com/bharathvbcr/Manvi/manvi/computer"
)

// OperatorAgent marks an App whose runs an agent drives (`jarvis --yolo`).
// The empty operator is the default: a person at the workbench or terminal.
// Only cmd/jarvis sets it; the stdio host the workbench spawns never does, so
// it cannot be set over the wire.
const OperatorAgent = "agent"

func validOperator(operator string) error {
	if operator != "" && operator != OperatorAgent {
		return errors.New("operator must be empty or agent")
	}
	return nil
}

// operatorRecord is operator.json, written into an agent-operated run's
// evidence directory before the run can accept its first control. It is the
// authority on who operated the run: bundle.json still files agent acts
// under human_actions (DevCouncil's Bundle has no actor field), and the
// runner still reports controller "human" after an agent takeover.
type operatorRecord struct {
	SchemaVersion  int    `json:"schema_version"`
	Operator       string `json:"operator"`
	Mode           string `json:"mode"`
	HumanQualified bool   `json:"human_qualified"`
}

func writeOperatorSidecar(dir, operator string) error {
	if operator == "" {
		return nil
	}
	return writeJSON(filepath.Join(dir, "operator.json"), operatorRecord{SchemaVersion: 1, Operator: operator, Mode: "yolo", HumanQualified: false})
}

// attributeRecord names the operator on records Manvi attributes to a person
// or to no one. Manvi's runner hard-codes actor "human" for human_act and its
// observations, and journals approve/deny as "automation" because a Control
// carries no actor. Under an agent operator neither may stand: an agent's
// approval is never journaled as a human's, nor as nobody's. Records are
// copied, never written through, so the caller's record is unchanged.
func attributeRecord(record computer.Record, operator string) computer.Record {
	if operator == "" {
		return record
	}
	switch {
	case record.Actor == "human":
		record.Actor = operator
	case record.Kind == "event" && record.Event != nil && (record.Event.Kind == "approve" || record.Event.Kind == "deny"):
		record.Actor = operator
	}
	return record
}

// replaySink is the record sink of a frozen replay: attribution first, so the
// hashed journal and the run view agree.
func (a *App) replaySink(recorder *Recorder, e *runEntry) func(computer.Record) error {
	return func(record computer.Record) error {
		record = attributeRecord(record, a.cfg.Operator)
		if err := recorder.Record(record); err != nil {
			return err
		}
		return e.appendRecord(record)
	}
}

// discoverySink is the record sink of a discovery run.
func (a *App) discoverySink(writer *discoveryWriter, e *runEntry) func(computer.Record) error {
	return func(record computer.Record) error {
		return writer.record(e, attributeRecord(record, a.cfg.Operator))
	}
}
