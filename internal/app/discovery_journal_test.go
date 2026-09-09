package app

import (
	"context"
	"encoding/json"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoveryActionAdmissionIsDurableBeforeReturning(t *testing.T) {
	dir := t.TempDir()
	journal, err := openDesktopJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.file.Close()
	w := discoveryWriter{journal: journal}
	e := &runEntry{view: RunView{EvidenceDir: dir}}
	if err = w.record(e, computer.Record{Kind: "action_started", ActionID: "run:confirm", Epoch: 3}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"action_started"`) || !strings.Contains(string(raw), `"run:confirm"`) {
		t.Fatalf("discovery admitted input without a durable journal record: %q", raw)
	}
}

func TestDiscoveryFramesAreDurableAndJournalFailureBlocksAdmission(t *testing.T) {
	dir := t.TempDir()
	journal, err := openDesktopJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := discoveryWriter{journal: journal}
	e := &runEntry{view: RunView{RunID: "run", EvidenceDir: dir}}
	s, p, state, o := reconciliationFixture(t)
	o.CapturedAtMillis = 1
	o.TreeAtMillis = 1
	safe, err := computer.Sanitize(o, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.record(e, computer.Record{Kind: "observation", Stage: "discovery_capture", Epoch: s.Epoch, Observation: &safe}); err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), Config{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.runs["run"] = e
	frame, err := a.Frame("run", "after")
	if err != nil || frame.Screenshot.Base64 == "" {
		t.Fatalf("discovery frame unavailable: %v", err)
	}
	if err = w.finish(state.RunID); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "desktop-artifacts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Acceptance string `json:"acceptance"`
		Artifacts  []struct{ Path, SHA256 string }
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Acceptance != "not_evaluated" || len(manifest.Artifacts) != 3 {
		t.Fatalf("bad discovery manifest: %s", raw)
	}
	for _, artifact := range manifest.Artifacts {
		b, err := os.ReadFile(filepath.Join(dir, artifact.Path))
		if err != nil {
			t.Fatal(err)
		}
		if digest(b) != artifact.SHA256 {
			t.Fatal("unbound discovery artifact")
		}
	}
	before := len(e.view.Records)
	if err = w.record(e, computer.Record{Kind: "action_started", ActionID: "later"}); err == nil || len(e.view.Records) != before {
		t.Fatal("closed journal allowed another input admission")
	}
}
