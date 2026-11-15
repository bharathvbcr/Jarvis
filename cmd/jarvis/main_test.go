package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/bharathvbcr/Jarvis/internal/evidence"
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

func sha256HexForTest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestRunVerifyEnforcesHeldOutLockComparatorFromCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub verifier runs on POSIX")
	}
	dir := t.TempDir()
	artRoot := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(artRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	artPayload := []byte("cli-evidence-log\n")
	if err := os.WriteFile(filepath.Join(artRoot, "run.log"), artPayload, 0o600); err != nil {
		t.Fatal(err)
	}

	contract := evidence.Contract{
		SchemaVersion: 1,
		ID:            "cli-qualify",
		Criteria: []evidence.Criterion{
			{ID: "crit-1", Fact: "ok", Required: true, Predicate: evidence.Predicate{Op: "eq", Value: json.RawMessage(`true`)}},
		},
	}
	contractRaw, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(dir, "contract.json")
	if err := os.WriteFile(contractPath, contractRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	contractSHA := sha256HexForTest(contractRaw)
	capSHA := strings.Repeat("a", 64)
	polSHA := strings.Repeat("b", 64)

	bundle := evidence.Bundle{
		SchemaVersion:    1,
		RunID:            "run-cli-1",
		SessionID:        "sess-cli-1",
		Epoch:            2,
		ContractSHA256:   contractSHA,
		CapabilitySHA256: capSHA,
		PolicySHA256:     polSHA,
		JournalComplete:  true,
		Degraded:         []string{},
		Artifacts: []evidence.Artifact{
			{ID: "log", Path: "run.log", SHA256: sha256HexForTest(artPayload), SizeBytes: uint64(len(artPayload))},
		},
	}
	bundleRaw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(dir, "bundle.json")
	if err := os.WriteFile(bundlePath, bundleRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	bundleSHA := sha256HexForTest(bundleRaw)

	rep := evidence.Report{
		OK:             true,
		SchemaVersion:  1,
		Verdict:        "passed",
		ContractSHA256: contractSHA,
		BundleSHA256:   bundleSHA,
		RunID:          "run-cli-1",
		Criteria:       []evidence.CriterionResult{{ID: "crit-1", Required: true, Verdict: "passed", Reason: "ok"}},
		Issues:         []evidence.Issue{},
	}
	repRaw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	verifierPath := filepath.Join(dir, "dcverify-stub")
	script := "#!/bin/sh\ncat <<'EOF'\n" + string(repRaw) + "\nEOF\nexit 0\n"
	if err := os.WriteFile(verifierPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	verifierSHA := sha256HexForTest([]byte(script))

	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })

	// 1. Valid held-out lock verification succeeds over the CLI without broker/bank.
	os.Args = []string{
		"jarvis", "verify",
		"--root", dir,
		"--verifier", verifierPath,
		"--verifier-sha256", verifierSHA,
		"--contract", contractPath,
		"--contract-sha256", contractSHA,
		"--capability-sha256", capSHA,
		"--policy-sha256", polSHA,
		"--bundle", bundlePath,
		"--artifacts-root", artRoot,
		"--run-id", "run-cli-1",
		"--session-id", "sess-cli-1",
		"--epoch", "2",
	}
	out, err := captureStdout(t, run)
	if err != nil {
		t.Fatalf("expected valid CLI held-out verify to pass, got err=%v out=%s", err, string(out))
	}

	// 2. Tampering the verifier binary (even if it still prints a valid passed report)
	// fails closed against --verifier-sha256!
	tamperedScript := script + "# tampered\n"
	if err := os.WriteFile(verifierPath, []byte(tamperedScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := captureStdout(t, run); err == nil || !strings.Contains(err.Error(), "verifier binary sha256") {
		t.Fatalf("expected tampered verifier binary to be rejected by held-out lock CLI, got %v", err)
	}
}
