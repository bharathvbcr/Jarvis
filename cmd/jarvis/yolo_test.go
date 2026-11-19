package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bharathvbcr/Jarvis/internal/app"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func answersOf(t *testing.T, lines ...string) chan answer {
	t.Helper()
	ch := make(chan answer, len(lines))
	for _, line := range lines {
		ch <- <-readAnswers(context.Background(), strings.NewReader(line+"\n"))
	}
	return ch
}

func events(t *testing.T, out string) []map[string]any {
	t.Helper()
	var list []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("not an event line: %q", line)
		}
		list = append(list, e)
	}
	return list
}

func yoloRun(t *testing.T, h *fakeHost, answers <-chan answer, gateTimeout time.Duration) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := watchRun(ctx, h, "run-1", watchOptions{events: true, answers: answers, stdout: &stdout, stderr: &stderr, poll: time.Millisecond, gateTimeout: gateTimeout, cancelGrace: 2 * time.Second})
	return stdout.String(), err
}

func TestYoloAnswersAGateFromStdin(t *testing.T) {
	h := &fakeHost{finishOn: "approve", views: []app.RunView{view(workflow.AwaitingApproval, 3, "act-1", "o-1")}}
	answers := make(chan answer, 1)
	go func() {
		// Answer after the gate is announced, as an agent reading events would.
		time.Sleep(20 * time.Millisecond)
		answers <- <-readAnswers(context.Background(), strings.NewReader(`{"kind":"approve","epoch":3,"action_id":"act-1","observation_id":"o-1"}`+"\n"))
	}()
	out, err := yoloRun(t, h, answers, 5*time.Second)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if got := strings.Join(h.kinds(), ","); got != "approve" {
		t.Fatalf("controls %s, want approve", got)
	}
	c := h.controls[0]
	if c.Epoch != 3 || c.ActionID != "act-1" || c.ObservationID != "o-1" {
		t.Fatalf("approve %+v is not bound to the gate", c)
	}
	var sawGate, sawSent bool
	for _, e := range events(t, out) {
		sawGate = sawGate || (e["event"] == "needs_human" && e["answer"] == "stdin")
		sawSent = sawSent || (e["event"] == "control" && e["kind"] == "approve" && e["sent"] == true)
	}
	if !sawGate || !sawSent {
		t.Fatalf("events do not show the answered gate:\n%s", out)
	}
}

func TestYoloRefusesAnswersThatDoNotMatchTheGate(t *testing.T) {
	h := &fakeHost{finishOnCancel: true, views: []app.RunView{view(workflow.AwaitingApproval, 3, "act-1", "o-1")}}
	answers := answersOf(t,
		`{"kind":"approve","epoch":2,"action_id":"act-1","observation_id":"o-1"}`,
		`{"kind":"approve","epoch":3,"action_id":"act-0","observation_id":"o-1"}`,
		`{"kind":"approve","action_id":"act-1","observation_id":"o-1"}`,
		`{"kind":"approve","epoch":3,"action_id":"act-1","observation_id":"o-1","actor":"human"}`,
		`{"Kind":"approve","epoch":3,"action_id":"act-1","observation_id":"o-1"}`,
		`{"kind":"act","epoch":3}`,
		`{"kind":"launch","epoch":3}`,
		`not json`,
		`{"kind":"cancel","epoch":3}`,
	)
	out, err := yoloRun(t, h, answers, 5*time.Second)
	if exitCode(err) != 0 && exitCode(err) != exitNeedsHuman {
		t.Fatalf("run: %v", err)
	}
	if got := strings.Join(h.kinds(), ","); got != "cancel" {
		t.Fatalf("controls %s, want only cancel", got)
	}
	refused := 0
	for _, e := range events(t, out) {
		if e["event"] == "control" && e["sent"] == false {
			refused++
		}
	}
	if refused != 8 {
		t.Fatalf("%d answers refused, want 8:\n%s", refused, out)
	}
}

func TestYoloStdinEOFBeforeAGateLetsTheRunFinish(t *testing.T) {
	refused := view(workflow.Observing, 1, "", "o-0")
	refused.Records = []computer.Record{{Kind: "control_refused", Epoch: 1, Reason: "stale control epoch"}}
	h := &fakeHost{finishAfter: 3, views: []app.RunView{refused, view(workflow.Observing, 1, "", "o-0")}}
	closed := make(chan answer)
	close(closed)
	out, err := yoloRun(t, h, closed, 5*time.Second)
	if err != nil || len(h.controls) != 0 {
		t.Fatalf("exit %v with controls %v:\n%s", err, h.kinds(), out)
	}
	if !strings.Contains(out, `"event":"control_refused"`) || !strings.Contains(out, "stale control epoch") {
		t.Fatalf("the run's own refusal was not surfaced:\n%s", out)
	}
}

func TestYoloStdinEOFAtAGateCancels(t *testing.T) {
	for name, delay := range map[string]time.Duration{"eof first": 0, "gate first": 50 * time.Millisecond} {
		t.Run(name, func(t *testing.T) {
			h := &fakeHost{finishOnCancel: true, views: []app.RunView{view(workflow.Paused, 2, "", "o-1")}}
			answers := make(chan answer)
			go func() { time.Sleep(delay); close(answers) }()
			out, err := yoloRun(t, h, answers, 5*time.Second)
			if exitCode(err) != exitNeedsHuman || strings.Join(h.kinds(), ",") != "cancel" {
				t.Fatalf("exit %d controls %v:\n%s", exitCode(err), h.kinds(), out)
			}
		})
	}
}

func TestYoloGateTimeoutCancels(t *testing.T) {
	h := &fakeHost{finishOnCancel: true, views: []app.RunView{view(workflow.AwaitingApproval, 1, "a", "o")}}
	out, err := yoloRun(t, h, make(chan answer), 30*time.Millisecond)
	if exitCode(err) != exitNeedsHuman || !strings.Contains(out, `"event":"gate_timeout"`) {
		t.Fatalf("exit %d:\n%s", exitCode(err), out)
	}
}

func TestReadAnswersBoundsLinesAndEnds(t *testing.T) {
	long := `{"kind":"act","epoch":1,"spec":"` + strings.Repeat("x", maxAnswerLine) + `"}`
	var got []answer
	for a := range readAnswers(context.Background(), strings.NewReader("\n"+`{"kind":"focus","epoch":1}`+"\n"+long+"\n")) {
		got = append(got, a)
	}
	if len(got) != 2 || got[0].err != nil || got[0].line.Kind != "focus" || got[1].err == nil {
		t.Fatalf("answers %+v", got)
	}
}

func TestAgentModeComesFromFlagOrEnvironment(t *testing.T) {
	t.Setenv("JARVIS_AGENT_MODE", "")
	if m, err := resolveAgentMode(false); err != nil || m != app.AgentModeDefault {
		t.Fatalf("%q %v", m, err)
	}
	if m, _ := resolveAgentMode(true); m != app.AgentModeYolo {
		t.Fatal("--yolo ignored")
	}
	t.Setenv("JARVIS_AGENT_MODE", "yolo")
	if m, _ := resolveAgentMode(false); m != app.AgentModeYolo {
		t.Fatal("JARVIS_AGENT_MODE ignored")
	}
	t.Setenv("JARVIS_AGENT_MODE", "Yolo!")
	if _, err := resolveAgentMode(true); err == nil {
		t.Fatal("unknown mode accepted")
	}
}

func TestCallYoloPromotesAsTheAgent(t *testing.T) {
	t.Setenv("JARVIS_AGENT_MODE", "")
	root := t.TempDir()
	source, err := os.ReadFile(filepath.Join(sourceRoot(t), "examples", "balance.json"))
	if err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": string(source), "register": true})
	file := filepath.Join(root, "compile.json")
	if err := os.WriteFile(file, params, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, code := call(t, "", "call", "--root", root, "jarvis.capability.compile", "--params", "@"+file); code != 0 {
		t.Fatalf("register exit %d: %s", code, got)
	}
	promote := `{"id":"bank.balance","revision":"1"}`
	if got, code := call(t, "", "call", "--root", root, "jarvis.catalog.promote", "--params", promote); code != exitRefused {
		t.Fatalf("promote without yolo: exit %d: %s", code, got)
	}
	if got, code := call(t, "", "call", "--root", root, "--yolo", "jarvis.catalog.promote", "--params", promote); code != 0 {
		t.Fatalf("promote --yolo: exit %d: %s", code, got)
	}
	history, err := os.ReadFile(filepath.Join(root, ".local", "promotions", "bank.balance.json"))
	if err != nil || !strings.Contains(string(history), `"promoted_by": "agent"`) {
		t.Fatalf("promotion history does not name the agent: %s %v", history, err)
	}
	if got, code := call(t, "", "call", "--root", root, "--yolo", "jarvis.inputs.get", "--params", `{"application":"vani","key":"k"}`); code != exitRefused {
		t.Fatalf("inputs.get --yolo: exit %d: %s", code, got)
	}
	t.Setenv("JARVIS_AGENT_MODE", "bogus")
	if got, code := call(t, "", "call", "--root", root, "jarvis.catalog.list"); code != exitUsage {
		t.Fatalf("bogus mode: exit %d: %s", code, got)
	}
}

func TestOtherCommandsIgnoreTheAgentMode(t *testing.T) {
	t.Setenv("JARVIS_AGENT_MODE", "bogus")
	root := t.TempDir()
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	os.Args = []string{"jarvis", "catalog", "--root", root}
	if raw, err := captureStdout(t, run); err != nil {
		t.Fatalf("catalog read JARVIS_AGENT_MODE: %v %s", err, raw)
	}
	os.Args = []string{"jarvis", "catalog", "--root", root, "--yolo"}
	if _, err := captureStdout(t, run); exitCode(err) != exitUsage {
		t.Fatalf("catalog --yolo: %v", err)
	}
}
