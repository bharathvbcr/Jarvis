package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bharathvbcr/Jarvis/internal/app"
	"github.com/bharathvbcr/Manvi/manvi/serve"
)

func call(t *testing.T, stdin string, command string, args ...string) ([]byte, int) {
	t.Helper()
	var stdout bytes.Buffer
	err := agentCommand(context.Background(), command, args, strings.NewReader(stdin), &stdout)
	return stdout.Bytes(), exitCode(err)
}

func envelopeOf(t *testing.T, raw []byte) serve.Response {
	t.Helper()
	if bytes.Count(raw, []byte("\n")) != 1 {
		t.Fatalf("stdout is not exactly one line: %q", raw)
	}
	var r serve.Response
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("stdout is not a response envelope: %q", raw)
	}
	return r
}

// serveLine is the raw response line `jarvis serve` itself writes for op.
func serveLine(t *testing.T, root, op string, params json.RawMessage) []byte {
	t.Helper()
	a, err := app.New(context.Background(), app.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var out bytes.Buffer
	requests := []serve.Request{{ID: "hello", Op: serve.OpHello}, {ID: "call", Op: op, Params: params}}
	var in bytes.Buffer
	for _, r := range requests {
		if err := json.NewEncoder(&in).Encode(r); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := serve.New(&out, serve.Options{HardRules: true, Modules: []serve.Module{a}}).Serve(ctx, &in); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	for _, line := range bytes.SplitAfter(out.Bytes(), []byte("\n")) {
		if bytes.HasPrefix(line, []byte(`{"id":"call"`)) {
			return line
		}
	}
	t.Fatalf("serve wrote no response for %s: %s", op, out.Bytes())
	return nil
}

func TestCallPassesTheHostLineThroughByteForByte(t *testing.T) {
	root := t.TempDir()
	trace := filepath.Join(root, "trace.json")
	if err := os.WriteFile(trace, []byte(`{"z":1,"a":[true,null]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"jarvis.catalog.list", "jarvis.trace.load"} {
		params := `{}`
		if op == "jarvis.trace.load" {
			params = `{"path":"trace.json"}`
		}
		got, code := call(t, "", "call", op, "--root", root, "--params", params)
		want := serveLine(t, root, op, json.RawMessage(params))
		if !bytes.Equal(got, want) {
			t.Fatalf("%s: call wrote\n%s\nserve wrote\n%s", op, got, want)
		}
		if r := envelopeOf(t, got); r.OK != (code == 0) {
			t.Fatalf("%s: ok=%t but exit %d", op, r.OK, code)
		}
	}
}

func TestCallRefusesHumanOnlyOpsWithExit3(t *testing.T) {
	root := t.TempDir()
	key := "sk-test-0123456789abcdef"
	params := filepath.Join(root, "key.json")
	if err := os.WriteFile(params, []byte(`{"key":"`+key+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for op, file := range map[string]string{"jarvis.key.set": "@" + params, "jarvis.run.act": "", "jarvis.catalog.promote": "", "jarvis.inputs.get": ""} {
		args := []string{"--root", root, op}
		if file != "" {
			args = append(args, "--params", file)
		}
		got, code := call(t, "", "call", args...)
		r := envelopeOf(t, got)
		if code != exitRefused || r.OK || r.Error == nil || r.Error.Code != app.ErrAgentRefused {
			t.Fatalf("%s: exit %d, %s", op, code, got)
		}
		if strings.Contains(string(got), key) {
			t.Fatalf("%s: refusal echoes the credential", op)
		}
	}
}

func TestCallRefusesApprovalControlFromStdin(t *testing.T) {
	got, code := call(t, `{"run_id":"r","control":{"kind":"approve","epoch":1}}`, "call", "jarvis.run.control", "--root", t.TempDir(), "--params", "-")
	if r := envelopeOf(t, got); code != exitRefused || r.Error.Code != app.ErrAgentRefused {
		t.Fatalf("exit %d: %s", code, got)
	}
}

func TestCallRefusesProcessScopedOps(t *testing.T) {
	got, code := call(t, "", "call", "jarvis.run.get", "--root", t.TempDir(), "--params", `{"run_id":"r"}`)
	if r := envelopeOf(t, got); code != exitRefused || r.Error.Code != app.ErrProcessScoped {
		t.Fatalf("exit %d: %s", code, got)
	}
}

func TestCallKeepsTraceReplayParamsOffTheCommandLine(t *testing.T) {
	got, code := call(t, "", "call", "jarvis.trace.replay", "--root", t.TempDir(), "--params", `{"path":"t.json","capability_path":"c.json","inputs":{}}`)
	if r := envelopeOf(t, got); code != exitRefused || r.Error.Code != app.ErrAgentRefused {
		t.Fatalf("exit %d: %s", code, got)
	}
}

func TestCallCannotReadPrivateState(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".local", "inputs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vani.json"), []byte(`{"values":{"pin":"s3cret-pin"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, code := call(t, "", "call", "jarvis.trace.load", "--root", root, "--params", `{"path":".local/inputs/vani.json"}`)
	if code != exitRefused || strings.Contains(string(got), "s3cret") {
		t.Fatalf("exit %d: %s", code, got)
	}
}

func TestCallDoctorPollsTheJobAndPrintsTheDiagnostic(t *testing.T) {
	got, code := call(t, "", "call", "jarvis.doctor", "--root", t.TempDir())
	r := envelopeOf(t, got)
	if code != 0 || !r.OK {
		t.Fatalf("exit %d: %s", code, got)
	}
	var d app.Diagnostic
	if err := json.Unmarshal(r.Result, &d); err != nil || d.Platform == "" {
		t.Fatalf("result is not a diagnostic: %s", r.Result)
	}
	if strings.Contains(string(r.Result), "job_id") {
		t.Fatalf("result is the job record, not the diagnostic: %s", r.Result)
	}
}

func TestCallUsageAndTimeoutExits(t *testing.T) {
	root := t.TempDir()
	for name, tc := range map[string]struct {
		args []string
		code int
		err  string
	}{
		"no op":         {[]string{"--root", root}, exitUsage, serve.ErrBadRequest},
		"two ops":       {[]string{"--root", root, "a", "b"}, exitUsage, serve.ErrBadRequest},
		"array params":  {[]string{"--root", root, "jarvis.catalog.list", "--params", "[1]"}, exitUsage, serve.ErrBadRequest},
		"missing file":  {[]string{"--root", root, "jarvis.catalog.list", "--params", "@" + filepath.Join(root, "nope")}, exitUsage, serve.ErrBadRequest},
		"zero timeout":  {[]string{"--root", root, "--timeout", "0s", "jarvis.catalog.list"}, exitUsage, serve.ErrBadRequest},
		"expired timer": {[]string{"--root", root, "--timeout", "1ns", "jarvis.catalog.list"}, exitTimeout, errTimeout},
	} {
		t.Run(name, func(t *testing.T) {
			got, code := call(t, "", "call", tc.args...)
			if r := envelopeOf(t, got); code != tc.code || r.Error == nil || r.Error.Code != tc.err {
				t.Fatalf("exit %d: %s", code, got)
			}
		})
	}
}

func TestOpsClassifiesTheServedOps(t *testing.T) {
	got, code := call(t, "", "ops", "--root", t.TempDir())
	r := envelopeOf(t, got)
	if code != 0 || !r.OK {
		t.Fatalf("exit %d: %s", code, got)
	}
	var result struct {
		Ops []app.OpInfo `json:"ops"`
	}
	if err := json.Unmarshal(r.Result, &result); err != nil {
		t.Fatal(err)
	}
	classes := map[string]app.OpClass{}
	for _, op := range result.Ops {
		classes[op.Op] = op.Class
	}
	for op, want := range map[string]app.OpClass{
		"jarvis.catalog.list":  app.OpAgent,
		"jarvis.doctor":        app.OpAgent,
		"jarvis.key.set":       app.OpHumanOnly,
		"jarvis.run.get":       app.OpProcessScoped,
		"policy.check.command": app.OpRefused,
	} {
		if classes[op] != want {
			t.Errorf("%s classified %q, want %q", op, classes[op], want)
		}
	}
}

func TestRunDispatchesCall(t *testing.T) {
	root := t.TempDir()
	original := os.Args
	os.Args = []string{"jarvis", "call", "--root", root, "jarvis.catalog.list"}
	t.Cleanup(func() { os.Args = original })
	raw, err := captureStdout(t, run)
	if err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	if r := envelopeOf(t, raw); !r.OK {
		t.Fatalf("catalog.list failed: %s", raw)
	}
}
