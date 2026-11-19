package app

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/serve"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func TestParseAgentModeRefusesUnknownValues(t *testing.T) {
	for value, want := range map[string]AgentMode{"": AgentModeDefault, "default": AgentModeDefault, "yolo": AgentModeYolo, " yolo ": AgentModeYolo} {
		if got, err := ParseAgentMode(value); err != nil || got != want {
			t.Errorf("ParseAgentMode(%q) = %q, %v", value, got, err)
		}
	}
	for _, value := range []string{"YOLO!", "on", "1", "true"} {
		if _, err := ParseAgentMode(value); err == nil {
			t.Errorf("ParseAgentMode(%q) accepted", value)
		}
	}
}

func TestYoloAdmitsDiskBackedHumanOps(t *testing.T) {
	a, _ := agentApp(t)
	wantCode(t, a.AdmitAgent("jarvis.catalog.promote", json.RawMessage(`{"id":"bank.balance","revision":"1"}`), true, AgentModeYolo), "")
	wantCode(t, a.AdmitAgent("jarvis.inputs.put", json.RawMessage(`{"application":"vani","key":"k","value":{}}`), false, AgentModeYolo), "")
	// Values stay off argv even in yolo.
	wantCode(t, a.AdmitAgent("jarvis.inputs.put", json.RawMessage(`{}`), true, AgentModeYolo), ErrAgentRefused)
	// The same ops stay refused outside yolo.
	wantCode(t, a.AdmitAgent("jarvis.catalog.promote", nil, false, AgentModeDefault), ErrAgentRefused)
}

func TestYoloKeepsLiveOpsDisclosureAndConfinement(t *testing.T) {
	a, root := agentApp(t)
	for op, code := range map[string]string{
		"jarvis.run.act":         ErrProcessScoped,
		"jarvis.run.takeover":    ErrProcessScoped,
		"jarvis.key.set":         ErrProcessScoped,
		"jarvis.evidence.export": ErrProcessScoped,
		"jarvis.run.get":         ErrProcessScoped,
		"jarvis.inputs.get":      ErrAgentRefused,
		"policy.check.command":   ErrAgentRefused,
		"jarvis.not-an-op":       ErrAgentRefused,
	} {
		t.Run(op, func(t *testing.T) { wantCode(t, a.AdmitAgent(op, nil, false, AgentModeYolo), code) })
	}
	// Approval over call is not refused as human-only in yolo, but a one-shot
	// call still holds no run.
	wantCode(t, a.AdmitAgent("jarvis.run.control", json.RawMessage(`{"run_id":"r","control":{"kind":"approve"}}`), false, AgentModeYolo), ErrProcessScoped)
	private := filepath.Join(root, ".local", "inputs")
	if err := os.MkdirAll(private, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "vani.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	wantCode(t, a.AdmitAgent("jarvis.trace.load", json.RawMessage(`{"path":".local/inputs/vani.json"}`), true, AgentModeYolo), ErrAgentRefused)
}

func TestNewRefusesAnUnknownOperator(t *testing.T) {
	if _, err := New(context.Background(), Config{Root: t.TempDir(), Operator: "human"}); err == nil {
		t.Fatal("operator other than empty or agent accepted")
	}
}

func TestAttributeRecordNamesTheAgent(t *testing.T) {
	approve := &workflow.Event{Kind: "approve"}
	for _, tc := range []struct {
		in       computer.Record
		operator string
		want     string
	}{
		{computer.Record{Kind: "action_started", Actor: "human"}, OperatorAgent, OperatorAgent},
		{computer.Record{Kind: "observation", Actor: "human"}, OperatorAgent, OperatorAgent},
		{computer.Record{Kind: "event", Actor: "automation", Event: approve}, OperatorAgent, OperatorAgent},
		{computer.Record{Kind: "event", Actor: "automation", Event: &workflow.Event{Kind: "deny"}}, OperatorAgent, OperatorAgent},
		{computer.Record{Kind: "event", Actor: "automation", Event: &workflow.Event{Kind: "receipt"}}, OperatorAgent, "automation"},
		{computer.Record{Kind: "observation", Actor: "model_recovery"}, OperatorAgent, "model_recovery"},
		{computer.Record{Kind: "action_started", Actor: "human"}, "", "human"},
		{computer.Record{Kind: "event", Actor: "automation", Event: approve}, "", "automation"},
	} {
		original := tc.in
		got := attributeRecord(tc.in, tc.operator)
		if got.Actor != tc.want {
			t.Errorf("%s/%s under %q: actor %q, want %q", tc.in.Kind, tc.in.Actor, tc.operator, got.Actor, tc.want)
		}
		if tc.in.Actor != original.Actor || (tc.in.Event != nil && tc.in.Event.Kind != original.Event.Kind) {
			t.Error("attributeRecord wrote through its argument")
		}
	}
}

func journalActors(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAgentSinksNeverJournalAHumanActor(t *testing.T) {
	records := []computer.Record{
		{Kind: "action_started", Actor: "human", ActionID: "human-1", Epoch: 2},
		{Kind: "human_action", Actor: "human", ActionID: "human-1", Epoch: 2, Receipt: &computer.Receipt{Delivery: "sent"}},
		{Kind: "event", Actor: "automation", Event: &workflow.Event{Kind: "approve", Epoch: 2}},
	}
	for _, operator := range []string{OperatorAgent, ""} {
		a := &App{cfg: Config{Operator: operator}}
		recorder := testRecorder(t)
		e := &runEntry{}
		replay := a.replaySink(recorder, e)
		dir := t.TempDir()
		journal, err := openDesktopJournal(dir)
		if err != nil {
			t.Fatal(err)
		}
		d := &runEntry{view: RunView{EvidenceDir: dir}}
		discovery := a.discoverySink(&discoveryWriter{journal: journal}, d)
		for _, r := range records {
			if err := replay(r); err != nil {
				t.Fatal(err)
			}
			if err := discovery(r); err != nil {
				t.Fatal(err)
			}
		}
		journal.file.Close()
		for name, text := range map[string]string{"replay": journalActors(t, recorder.dir), "discovery": journalActors(t, dir)} {
			human := strings.Contains(text, `"actor":"human"`)
			if operator == OperatorAgent && (human || strings.Count(text, `"actor":"agent"`) != 3) {
				t.Errorf("%s journal under agent operator:\n%s", name, text)
			}
			if operator == "" && (!human || strings.Contains(text, `"actor":"agent"`)) {
				t.Errorf("%s journal under a person changed:\n%s", name, text)
			}
		}
		for _, view := range []*runEntry{e, d} {
			for _, r := range view.view.Records {
				if operator == OperatorAgent && r.Actor == "human" {
					t.Errorf("run view keeps actor human under the agent operator: %+v", r)
				}
			}
		}
	}
}

func TestOperatorSidecarMarksOnlyAgentRuns(t *testing.T) {
	dir := t.TempDir()
	if err := writeOperatorSidecar(dir, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "operator.json")); !os.IsNotExist(err) {
		t.Fatal("a person's run gained an operator sidecar")
	}
	if err := writeOperatorSidecar(dir, OperatorAgent); err != nil {
		t.Fatal(err)
	}
	var got operatorRecord
	raw, err := os.ReadFile(filepath.Join(dir, "operator.json"))
	if err != nil || json.Unmarshal(raw, &got) != nil {
		t.Fatalf("sidecar unreadable: %v %s", err, raw)
	}
	if got != (operatorRecord{SchemaVersion: 1, Operator: OperatorAgent, Mode: "yolo", HumanQualified: false}) {
		t.Fatalf("sidecar %+v", got)
	}
}

// serveCall sends one op through a real host-plane server over a.
func serveCall(t *testing.T, a *App, op string, params string) serve.Response {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, send := io.Pipe()
	output, receive := io.Pipe()
	defer send.Close()
	defer output.Close()
	go func() {
		_ = serve.New(receive, serve.Options{HardRules: true, Modules: []serve.Module{a}}).Serve(ctx, input)
		_ = receive.Close()
	}()
	go func() {
		enc := json.NewEncoder(send)
		_ = enc.Encode(serve.Request{ID: "hello", Op: serve.OpHello})
		_ = enc.Encode(serve.Request{ID: "op", Op: op, Params: json.RawMessage(params)})
	}()
	replies := make(chan serve.Response, 2)
	go func() {
		dec := json.NewDecoder(output)
		for {
			var r serve.Response
			if dec.Decode(&r) != nil {
				return
			}
			replies <- r
		}
	}()
	for {
		select {
		case r := <-replies:
			if r.ID == "op" {
				return r
			}
		case <-time.After(20 * time.Second):
			t.Fatalf("%s did not answer", op)
		}
	}
}

func TestAgentPromotionIsRecordedAsTheAgents(t *testing.T) {
	raw, err := os.ReadFile("../../examples/balance.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, operator := range []string{OperatorAgent, ""} {
		a, err := New(context.Background(), Config{Root: t.TempDir(), Operator: operator})
		if err != nil {
			t.Fatal(err)
		}
		source, _ := json.Marshal(map[string]any{"source": string(raw), "register": true})
		if r := serveCall(t, a, "jarvis.capability.compile", string(source)); !r.OK {
			t.Fatalf("register: %+v", r.Error)
		}
		if r := serveCall(t, a, "jarvis.catalog.promote", `{"id":"bank.balance","revision":"1"}`); !r.OK {
			t.Fatalf("promote: %+v", r.Error)
		}
		history, err := a.history("bank.balance")
		if err != nil || len(history) != 1 || history[0].PromotedBy != operator {
			t.Fatalf("operator %q: history %+v, %v", operator, history, err)
		}
		a.Close()
	}
}
