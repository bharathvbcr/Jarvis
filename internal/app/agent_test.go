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

	"github.com/bharathvbcr/Manvi/manvi/serve"
)

// servedOps asks a real host-plane server built over a for its op list, the
// way the workbench and `jarvis call` learn it.
func servedOps(t *testing.T, a *App) []string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, send := io.Pipe()
	output, receive := io.Pipe()
	defer send.Close()
	defer output.Close()
	server := serve.New(receive, serve.Options{HardRules: true, Modules: []serve.Module{a}})
	go func() { _ = server.Serve(ctx, input); _ = receive.Close() }()
	go func() {
		_ = json.NewEncoder(send).Encode(serve.Request{ID: "hello", Op: serve.OpHello, Params: json.RawMessage(`{"protocol":1}`)})
	}()
	replies := make(chan serve.Response, 1)
	go func() {
		var response serve.Response
		if json.NewDecoder(output).Decode(&response) == nil {
			replies <- response
		}
	}()
	select {
	case response := <-replies:
		var hello serve.HelloResult
		if !response.OK || json.Unmarshal(response.Result, &hello) != nil {
			t.Fatalf("hello failed: %+v", response)
		}
		return hello.Ops
	case <-time.After(20 * time.Second):
		t.Fatal("hello did not answer within 20s")
	}
	return nil
}

func TestEveryServedJarvisOpIsClassified(t *testing.T) {
	a, err := New(context.Background(), Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	served := map[string]bool{}
	for _, op := range servedOps(t, a) {
		served[op] = true
		if strings.HasPrefix(op, "jarvis.") {
			if _, ok := agentOps[op]; !ok {
				t.Errorf("%s is served but has no agent classification", op)
			}
		}
	}
	for op := range agentOps {
		if !served[op] {
			t.Errorf("%s is classified but not served (stale row)", op)
		}
	}
	for _, info := range AgentOps(servedOps(t, a)) {
		if !strings.HasPrefix(info.Op, "jarvis.") && info.Op != serve.OpHello && info.Class != OpRefused {
			t.Errorf("built-in %s classified %s, want refused", info.Op, info.Class)
		}
	}
}

// readmeOpsTable parses the ops table in README.md: rows of
// "| `jarvis.X` | class | notes |". It returns op -> class.
func readmeOpsTable(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, "| `jarvis.") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 4 {
			t.Fatalf("malformed ops row: %q", line)
		}
		op := strings.Trim(strings.TrimSpace(cells[1]), "`")
		if _, dup := rows[op]; dup {
			t.Errorf("README lists %s twice", op)
		}
		rows[op] = strings.TrimSpace(cells[2])
	}
	return rows
}

// TestREADMEOpsTableMatchesAgentOps keeps the README's ops table from drifting
// from the agentOps classification: every op listed under its class, and every
// classified op listed.
func TestREADMEOpsTableMatchesAgentOps(t *testing.T) {
	rows := readmeOpsTable(t)
	for op, p := range agentOps {
		got, ok := rows[op]
		switch {
		case !ok:
			t.Errorf("%s is classified %s in agentOps but missing from the README ops table", op, p.class)
		case got != string(p.class):
			t.Errorf("README lists %s as %q, agentOps classifies it %s", op, got, p.class)
		}
	}
	for op := range rows {
		if _, ok := agentOps[op]; !ok {
			t.Errorf("README lists %s, which agentOps does not classify", op)
		}
	}
}

func agentApp(t *testing.T) (*App, string) {
	t.Helper()
	root := t.TempDir()
	a, err := New(context.Background(), Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a, root
}

func wantCode(t *testing.T, got *serve.Error, code string) {
	t.Helper()
	if code == "" {
		if got != nil {
			t.Fatalf("refused: %+v", got)
		}
		return
	}
	if got == nil || got.Code != code {
		t.Fatalf("got %+v, want code %s", got, code)
	}
}

func TestAdmitAgentRefusesHumanOnlyOps(t *testing.T) {
	a, _ := agentApp(t)
	for _, op := range []string{
		"jarvis.catalog.promote", "jarvis.run.assist", "jarvis.run.takeover", "jarvis.run.handback",
		"jarvis.run.act", "jarvis.key.set", "jarvis.inputs.put", "jarvis.inputs.get", "jarvis.evidence.export",
	} {
		t.Run(op, func(t *testing.T) { wantCode(t, a.AdmitAgent(op, nil, false, AgentModeDefault), ErrAgentRefused) })
	}
}

func TestAdmitAgentRefusesApprovalBeforeProcessScope(t *testing.T) {
	a, _ := agentApp(t)
	for params, code := range map[string]string{
		`{"run_id":"r","control":{"kind":"approve","epoch":1}}`: ErrAgentRefused,
		`{"run_id":"r","control":{"kind":"resume","epoch":1}}`:  ErrAgentRefused,
		`{"run_id":"r","Control":{"KIND":" Approve "}}`:         ErrAgentRefused,
		`{"run_id":"r","control":"approve"}`:                    ErrAgentRefused,
		`not json`:                                              ErrAgentRefused,
		`{"run_id":"r","control":{"kind":"cancel","epoch":1}}`:  ErrProcessScoped,
		`{"run_id":"r","control":{"kind":"deny","epoch":1}}`:    ErrProcessScoped,
	} {
		t.Run(params, func(t *testing.T) {
			wantCode(t, a.AdmitAgent("jarvis.run.control", json.RawMessage(params), false, AgentModeDefault), code)
		})
	}
}

func TestAdmitAgentRefusesProcessScopedAndUnknownOps(t *testing.T) {
	a, _ := agentApp(t)
	for op, code := range map[string]string{
		"jarvis.run.start":       ErrProcessScoped,
		"jarvis.run.get":         ErrProcessScoped,
		"jarvis.job.get":         ErrProcessScoped,
		"jarvis.discovery.start": ErrProcessScoped,
		"jarvis.not-an-op":       ErrAgentRefused,
		"policy.check.command":   ErrAgentRefused,
		"chat.prepare":           ErrAgentRefused,
		serve.OpHello:            "",
		"jarvis.catalog.list":    "",
		"jarvis.doctor":          "",
	} {
		t.Run(op, func(t *testing.T) { wantCode(t, a.AdmitAgent(op, nil, false, AgentModeDefault), code) })
	}
}

func TestAdmitAgentKeepsInputParamsOffTheCommandLine(t *testing.T) {
	a, root := agentApp(t)
	if err := os.WriteFile(filepath.Join(root, "t.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	params := json.RawMessage(`{"path":"t.json","capability_path":"t.json","inputs":{}}`)
	wantCode(t, a.AdmitAgent("jarvis.trace.replay", params, true, AgentModeDefault), ErrAgentRefused)
	wantCode(t, a.AdmitAgent("jarvis.trace.replay", params, false, AgentModeDefault), "")
	wantCode(t, a.AdmitAgent("jarvis.catalog.list", nil, true, AgentModeDefault), "")
}

func TestAdmitAgentRefusesPrivateAndOutsidePaths(t *testing.T) {
	a, root := agentApp(t)
	private := filepath.Join(root, ".local", "inputs")
	if err := os.MkdirAll(private, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "vani.json"), []byte(`{"values":{"pin":"1234"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "trace.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(private, "vani.json"), filepath.Join(root, "innocent.json")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for params, code := range map[string]string{
		`{"path":"trace.json"}`:                                  "",
		`{"path":".local/inputs/vani.json"}`:                     ErrAgentRefused,
		`{"PATH":".local/inputs/vani.json"}`:                     ErrAgentRefused,
		`{"path":".LOCAL/inputs/vani.json"}`:                     ErrAgentRefused,
		`{"path":"x/../.local/inputs/vani.json"}`:                ErrAgentRefused,
		`{"path":"innocent.json"}`:                               ErrAgentRefused,
		`{"path":` + mustQuote(t, outside) + `}`:                 ErrAgentRefused,
		`{"path":"missing.json"}`:                                ErrAgentRefused,
		`{"path":7}`:                                             serve.ErrBadRequest,
		`{"path":"trace.json","path":".local/inputs/vani.json"}`: serve.ErrBadRequest,
	} {
		t.Run(params, func(t *testing.T) {
			// ".LOCAL" is refused as private state on a case-insensitive
			// filesystem and as missing on a case-sensitive one: both refuse.
			wantCode(t, a.AdmitAgent("jarvis.trace.load", json.RawMessage(params), true, AgentModeDefault), code)
		})
	}
}

func mustQuote(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
