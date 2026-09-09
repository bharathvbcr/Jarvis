package app

import (
	"encoding/json"
	"errors"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/devcouncil"
	"path/filepath"
	"sync"
)

type discoveryWriter struct {
	mu         sync.Mutex
	journal    *desktopJournal
	sequence   uint64
	artifacts  []devcouncil.EvidenceArtifact
	lastAction string
}

func (w *discoveryWriter) record(e *runEntry, r computer.Record) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.journal == nil {
		return errors.New("discovery desktop journal unavailable")
	}
	w.sequence++
	stored, artifacts, err := w.journal.append(r, w.sequence)
	if err != nil {
		return err
	}
	w.artifacts = append(w.artifacts, artifacts...)
	if r.Kind == "action_started" {
		w.lastAction = r.ActionID
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	var owned computer.Record
	if err = json.Unmarshal(raw, &owned); err != nil {
		return err
	}
	if owned.Observation != nil {
		owned.Observation.Screenshot.Base64 = ""
	}
	e.mu.Lock()
	e.view.Records = append(e.view.Records, owned)
	e.mu.Unlock()
	return nil
}
func (w *discoveryWriter) lastActionID() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastAction
}
func (w *discoveryWriter) reconciliation(report Reconciliation) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	artifact, err := storeReconciliation(w.journal.dir, report)
	if err != nil {
		return err
	}
	w.artifacts = append(w.artifacts, artifact)
	return nil
}
func (w *discoveryWriter) finish(runID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.journal.file.Close(); err != nil {
		return err
	}
	raw, err := readBounded(filepath.Join(w.journal.dir, "events.jsonl"), 64<<20)
	if err != nil {
		return err
	}
	artifacts := append([]devcouncil.EvidenceArtifact{}, w.artifacts...)
	artifacts = append(artifacts, devcouncil.EvidenceArtifact{ID: "desktop-journal", Path: "events.jsonl", SHA256: digest(raw), SizeBytes: uint64(len(raw))})
	return writeJSON(filepath.Join(w.journal.dir, "desktop-artifacts.json"), struct {
		SchemaVersion int                           `json:"schema_version"`
		RunID         string                        `json:"run_id"`
		Acceptance    string                        `json:"acceptance"`
		Artifacts     []devcouncil.EvidenceArtifact `json:"artifacts"`
	}{1, runID, "not_evaluated", artifacts})
}
