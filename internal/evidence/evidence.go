// Package evidence is Jarvis's client for DevCouncil's dc-evidence verifier
// (protocols/evidence/v1.md). The wire types project that protocol; decisions
// stay in the verifier, and this package only invokes it and validates its
// response. It lived in Manvi's devcouncil package until that was pruned of
// callers it could not see; Jarvis is its only consumer, so it lives here.
package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"time"
)

type Predicate struct {
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value,omitempty"`
}
type Criterion struct {
	ID        string    `json:"id"`
	Fact      string    `json:"fact"`
	Required  bool      `json:"required"`
	Predicate Predicate `json:"predicate"`
}
type Contract struct {
	SchemaVersion int         `json:"schema_version"`
	ID            string      `json:"id"`
	Criteria      []Criterion `json:"criteria"`
}
type Action struct {
	ID          string `json:"id"`
	Sequence    uint64 `json:"sequence"`
	Disposition string `json:"disposition"`
}
type Observation struct {
	Sequence      uint64                     `json:"sequence"`
	RunID         string                     `json:"run_id"`
	SessionID     string                     `json:"session_id"`
	Epoch         uint64                     `json:"epoch"`
	AfterActionID string                     `json:"after_action_id"`
	Facts         map[string]json.RawMessage `json:"facts"`
	ArtifactIDs   []string                   `json:"artifact_ids"`
}
type Artifact struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes uint64 `json:"size_bytes"`
}
type EpochTransition struct {
	Sequence  uint64 `json:"sequence"`
	FromEpoch uint64 `json:"from_epoch"`
	ToEpoch   uint64 `json:"to_epoch"`
	Reason    string `json:"reason"`
}
type Outcome struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}
type Intervention struct {
	ID         string `json:"id"`
	Sequence   uint64 `json:"sequence"`
	ReasonCode string `json:"reason_code"`
	Status     string `json:"status"`
}
type HumanAction struct {
	ID             string `json:"id"`
	Sequence       uint64 `json:"sequence"`
	Kind           string `json:"kind"`
	InterventionID string `json:"intervention_id"`
}
type RecoveryApplied struct {
	ID       string `json:"id"`
	Sequence uint64 `json:"sequence"`
	StepID   string `json:"step_id"`
}
type LocatorHit struct {
	Target        string `json:"target"`
	StrategyIndex uint64 `json:"strategy_index"`
	Sequence      uint64 `json:"sequence"`
}
type Bundle struct {
	SchemaVersion    int               `json:"schema_version"`
	RunID            string            `json:"run_id"`
	SessionID        string            `json:"session_id"`
	Epoch            uint64            `json:"epoch"`
	ContractSHA256   string            `json:"contract_sha256"`
	CapabilitySHA256 string            `json:"capability_sha256"`
	PolicySHA256     string            `json:"policy_sha256,omitempty"`
	Outcome          *Outcome          `json:"outcome,omitempty"`
	JournalComplete  bool              `json:"journal_complete"`
	Degraded         []string          `json:"degraded"`
	Actions          []Action          `json:"actions"`
	Observations     []Observation     `json:"observations"`
	Artifacts        []Artifact        `json:"artifacts"`
	EpochTransitions []EpochTransition `json:"epoch_transitions"`
	Interventions    []Intervention    `json:"interventions"`
	HumanActions     []HumanAction     `json:"human_actions"`
	Recoveries       []RecoveryApplied `json:"recoveries"`
	LocatorHits      []LocatorHit      `json:"locator_hits"`
}
type Issue struct {
	Code    string `json:"code"`
	Verdict string `json:"verdict"`
}
type CriterionResult struct {
	ID       string `json:"id"`
	Required bool   `json:"required"`
	Verdict  string `json:"verdict"`
	Reason   string `json:"reason"`
}
type Report struct {
	OK             bool              `json:"ok"`
	SchemaVersion  int               `json:"evidence_schema_version"`
	Verdict        string            `json:"verdict"`
	ContractSHA256 string            `json:"contract_sha256"`
	BundleSHA256   string            `json:"bundle_sha256"`
	RunID          string            `json:"run_id"`
	Criteria       []CriterionResult `json:"criteria"`
	Issues         []Issue           `json:"issues"`
}
type Request struct {
	Binary, ContractPath, BundlePath, ArtifactsRoot, ContractSHA256, CapabilitySHA256, RunID, SessionID string
	Epoch                                                                                               uint64
}

type boundedOutput struct {
	bytes.Buffer
	limit int
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("verifier output exceeds limit")
	}
	return b.Buffer.Write(p)
}

// Verify runs the verifier against a finished bundle and returns its report.
// A report whose identity does not match the request is an error, never a pass.
func Verify(ctx context.Context, r Request) (Report, error) {
	var report Report
	if r.Binary == "" || r.ContractPath == "" || r.BundlePath == "" || r.ArtifactsRoot == "" || r.RunID == "" || r.SessionID == "" || r.Epoch == 0 || len(r.ContractSHA256) != 64 || len(r.CapabilitySHA256) != 64 {
		return report, errors.New("independent evidence expectations are incomplete")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.Binary, "evidence-check", "--contract", r.ContractPath, "--bundle", r.BundlePath, "--artifacts-root", r.ArtifactsRoot, "--expected-contract-sha256", r.ContractSHA256, "--expected-capability-sha256", r.CapabilitySHA256, "--expected-run-id", r.RunID, "--expected-session-id", r.SessionID, "--expected-epoch", strconv.FormatUint(r.Epoch, 10))
	cmd.WaitDelay = 2 * time.Second
	out := boundedOutput{limit: 1 << 20}
	stderr := boundedOutput{limit: 4096}
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return report, ctx.Err()
	}
	if len(bytes.TrimSpace(out.Bytes())) == 0 {
		if err == nil {
			return report, errors.New("evidence verifier produced no report")
		}
		return report, fmt.Errorf("evidence verifier unavailable or produced no report: %w", err)
	}
	d := json.NewDecoder(bytes.NewReader(out.Bytes()))
	d.DisallowUnknownFields()
	if parseErr := d.Decode(&report); parseErr != nil {
		return report, fmt.Errorf("evidence response: %w", parseErr)
	}
	var trailing json.RawMessage
	if d.Decode(&trailing) != io.EOF {
		return report, errors.New("trailing evidence response data")
	}
	if !report.OK || report.SchemaVersion != 1 || report.RunID != r.RunID || report.ContractSHA256 != r.ContractSHA256 {
		return report, errors.New("evidence report identity or schema mismatch")
	}
	if report.Verdict != "passed" && report.Verdict != "failed" && report.Verdict != "incomplete" {
		return report, errors.New("invalid evidence verdict")
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() > 2 {
			return report, fmt.Errorf("evidence verifier process: %w", err)
		}
	}
	if report.Verdict == "passed" && err != nil {
		return report, errors.New("verifier process failed despite passed report")
	}
	return report, nil
}
