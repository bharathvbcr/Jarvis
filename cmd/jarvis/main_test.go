package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func TestOutputRejectsUnsupportedType(t *testing.T) {
	if err := output(make(chan struct{})); err == nil {
		t.Fatal("unsupported type was accepted by output encoder")
	}
}

func TestRunRequiresCommand(t *testing.T) {
	original := os.Args
	os.Args = []string{"jarvis"}
	t.Cleanup(func() { os.Args = original })

	if err := run(); err == nil || !strings.Contains(err.Error(), "commands:") {
		t.Fatalf("unexpected command validation: %v", err)
	}
}

func TestRunRequiresTraceReplaySubcommand(t *testing.T) {
	original := os.Args
	os.Args = []string{"jarvis", "trace"}
	t.Cleanup(func() { os.Args = original })

	if err := run(); err == nil || err.Error() != "use trace replay" {
		t.Fatalf("missing trace replay still accepted: %v", err)
	}
}

func TestRunTraceReplayWritesReconstructedState(t *testing.T) {
	root := t.TempDir()
	expected, capability, trace, inputs := replayFixture(t, root)

	original := os.Args
	os.Args = []string{"jarvis", "trace", "replay", "--root", root, "--capability", capability, "--trace", trace, "--inputs", inputs}
	t.Cleanup(func() { os.Args = original })

	raw, err := captureStdout(t, run)
	if err != nil {
		t.Fatal(err)
	}
	var got workflow.State
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("replay reconstruction changed: %#v != %#v", got, expected)
	}
}

func captureStdout(t *testing.T, fn func() error) ([]byte, error) {
	t.Helper()
	output, err := os.CreateTemp(t.TempDir(), "jarvis-stdout-*")
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = output
	runErr := fn()
	closeErr := output.Close()
	os.Stdout = original
	raw, readErr := os.ReadFile(output.Name())
	if runErr == nil && closeErr == nil && readErr == nil {
		return raw, nil
	}
	return raw, errors.Join(runErr, closeErr, readErr)
}

type replayJournal struct {
	Initial workflow.State   `json:"initial"`
	State   workflow.State   `json:"state"`
	Events  []workflow.Event `json:"events"`
	Error   string           `json:"error,omitempty"`
}

func replayFixture(t *testing.T, root string) (workflow.State, string, string, string) {
	t.Helper()
	rootPath := filepath.Join(sourceRoot(t), "examples", "balance.json")
	capability, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	capabilityPath := filepath.Join(root, "capability.json")
	if err = os.WriteFile(capabilityPath, capability, 0600); err != nil {
		t.Fatal(err)
	}
	inputsPath := filepath.Join(root, "inputs.json")
	inputsRaw := map[string]workflow.Value{"member_id": {Type: "string", Text: "M-1001"}}
	encodedInputs, err := json.Marshal(inputsRaw)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(inputsPath, encodedInputs, 0600); err != nil {
		t.Fatal(err)
	}

	packageValue, err := workflow.Compile(capability)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := workflow.NewState(packageValue, "run", "session", 1)
	if err != nil {
		t.Fatal(err)
	}
	event := workflow.Event{Sequence: 1, RunID: "run", SessionID: "session", Kind: "cancel", Epoch: 1}
	state, _, err := workflow.Reduce(packageValue, initial, event, inputsRaw)
	if err != nil {
		t.Fatal(err)
	}
	journal := replayJournal{Initial: initial, State: state, Events: []workflow.Event{event}}
	rawJournal, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	tracePath := filepath.Join(root, "trace.json")
	if err = os.WriteFile(tracePath, rawJournal, 0600); err != nil {
		t.Fatal(err)
	}
	return state, capabilityPath, tracePath, inputsPath
}

func sourceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}
