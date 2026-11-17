package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxHeldOutFileBytes = 8 << 20

// HeldOutLock pins the out-of-band cryptographic expectations (from
// upstream.lock.json and frozen qualification manifests) that an agent or
// child verifier is never permitted to rewrite.
type HeldOutLock struct {
	VerifierSHA256   string
	ContractSHA256   string
	CapabilitySHA256 string
	PolicySHA256     string // optional; enforced when non-empty
}

func isLowerHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func hashRegularFileBounded(path string, maxBytes int64) (string, []byte, uint64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, 0, fmt.Errorf("path %q must be a regular file (got mode %v)", path, info.Mode())
	}
	if info.Size() > maxBytes {
		return "", nil, 0, fmt.Errorf("file %q exceeds %d-byte limit (%d bytes)", path, maxBytes, info.Size())
	}
	f, err := os.Open(path)
	if err != nil {
		return "", nil, 0, err
	}
	defer func() { _ = f.Close() }()

	raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", nil, 0, err
	}
	if int64(len(raw)) > maxBytes {
		return "", nil, 0, fmt.Errorf("file %q grew past %d-byte limit during read", path, maxBytes)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), raw, uint64(len(raw)), nil
}

func decodeStrictJSON(raw []byte, dst any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return errors.New("empty or null JSON document")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var trailing json.RawMessage
	if dec.Decode(&trailing) != io.EOF {
		return errors.New("trailing data after JSON document")
	}
	return nil
}

func verifyArtifactConfinementAndDigests(artifactsRoot string, artifacts []Artifact) error {
	absRoot, err := filepath.Abs(artifactsRoot)
	if err != nil {
		return fmt.Errorf("resolve artifacts root: %w", err)
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return fmt.Errorf("eval artifacts root symlinks: %w", err)
	}
	realRoot = filepath.Clean(realRoot)

	seenIDs := make(map[string]struct{}, len(artifacts))
	for _, art := range artifacts {
		if art.ID == "" {
			return errors.New("bundle artifact has empty id")
		}
		if _, dup := seenIDs[art.ID]; dup {
			return fmt.Errorf("duplicate bundle artifact id %q", art.ID)
		}
		seenIDs[art.ID] = struct{}{}
		if !isLowerHex64(art.SHA256) {
			return fmt.Errorf("artifact %q has malformed sha256 %q", art.ID, art.SHA256)
		}
		if art.Path == "" || filepath.IsAbs(art.Path) {
			return fmt.Errorf("artifact %q path %q must be relative to artifacts root", art.ID, art.Path)
		}
		joined := filepath.Join(realRoot, art.Path)
		relCheck, relErr := filepath.Rel(realRoot, joined)
		if relErr != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
			return fmt.Errorf("artifact %q path %q escapes artifacts root", art.ID, art.Path)
		}
		realArt, evalErr := filepath.EvalSymlinks(joined)
		if evalErr != nil {
			return fmt.Errorf("artifact %q eval symlinks: %w", art.ID, evalErr)
		}
		relReal, relErr := filepath.Rel(realRoot, realArt)
		if relErr != nil || relReal == ".." || strings.HasPrefix(relReal, ".."+string(filepath.Separator)) {
			return fmt.Errorf("artifact %q symlink escapes artifacts root", art.ID)
		}
		gotSHA, _, gotSize, hashErr := hashRegularFileBounded(realArt, maxHeldOutFileBytes)
		if hashErr != nil {
			return fmt.Errorf("artifact %q read: %w", art.ID, hashErr)
		}
		if gotSize != art.SizeBytes {
			return fmt.Errorf("artifact %q size mismatch: disk=%d declared=%d", art.ID, gotSize, art.SizeBytes)
		}
		if gotSHA != art.SHA256 {
			return fmt.Errorf("artifact %q sha256 mismatch: disk=%s declared=%s", art.ID, gotSHA, art.SHA256)
		}
	}
	return nil
}

// stagePinnedVerifier writes the already-hashed verifier bytes to a new private
// (0700) directory and returns the executable's path with a function that
// removes it. The file is closed before it is returned so it can be executed.
// ext keeps the original file extension, which Windows needs to execute it.
func stagePinnedVerifier(raw []byte, ext string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "jarvis-verifier-")
	if err != nil {
		return "", nil, err
	}
	discard := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "verifier"+ext)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		discard()
		return "", nil, err
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		discard()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		discard()
		return "", nil, err
	}
	return path, discard, nil
}

// VerifyAgainstHeldOutLock wraps Verify with pre- and post-execution
// cryptographic lock verification (verifier binary digest, contract digest,
// capability/policy digests, strict bundle + artifact confinement, TOCTOU
// re-hashing, and non-vacuous passed-verdict completeness).
func VerifyAgainstHeldOutLock(ctx context.Context, r Request, lock HeldOutLock) (Report, error) {
	var empty Report
	if !isLowerHex64(lock.VerifierSHA256) || !isLowerHex64(lock.ContractSHA256) || !isLowerHex64(lock.CapabilitySHA256) {
		return empty, errors.New("held-out lock digests must be 64-character lowercase hex SHA-256")
	}
	if lock.PolicySHA256 != "" && !isLowerHex64(lock.PolicySHA256) {
		return empty, errors.New("held-out policy digest must be 64-character lowercase hex SHA-256")
	}
	if r.ContractSHA256 != lock.ContractSHA256 || r.CapabilitySHA256 != lock.CapabilitySHA256 {
		return empty, errors.New("request digests diverge from held-out lock")
	}

	// 1. Pre-execution cryptographic snapshot of verifier binary, contract, and bundle.
	// The verifier is pinned by content: the bytes that are hashed are the bytes
	// that run. Hashing a path and later executing the same path would let a
	// writer swap the file in between and put it back afterwards, so a private
	// copy of the hashed bytes is what Verify executes.
	binSHA, binRaw, _, err := hashRegularFileBounded(r.Binary, 64<<20)
	if err != nil {
		return empty, fmt.Errorf("hash verifier binary: %w", err)
	}
	if binSHA != lock.VerifierSHA256 {
		return empty, fmt.Errorf("verifier binary sha256 %s does not match held-out lock %s", binSHA, lock.VerifierSHA256)
	}

	contractSHA, contractRaw, _, err := hashRegularFileBounded(r.ContractPath, maxHeldOutFileBytes)
	if err != nil {
		return empty, fmt.Errorf("hash contract: %w", err)
	}
	if contractSHA != lock.ContractSHA256 {
		return empty, fmt.Errorf("on-disk contract sha256 %s does not match held-out lock %s", contractSHA, lock.ContractSHA256)
	}
	var contract Contract
	if err := decodeStrictJSON(contractRaw, &contract); err != nil {
		return empty, fmt.Errorf("parse contract: %w", err)
	}
	if contract.SchemaVersion != 1 || contract.ID == "" {
		return empty, errors.New("invalid contract schema_version or empty id")
	}

	bundleSHA, bundleRaw, _, err := hashRegularFileBounded(r.BundlePath, maxHeldOutFileBytes)
	if err != nil {
		return empty, fmt.Errorf("hash bundle: %w", err)
	}
	var bundle Bundle
	if err := decodeStrictJSON(bundleRaw, &bundle); err != nil {
		return empty, fmt.Errorf("parse bundle: %w", err)
	}
	if bundle.SchemaVersion != 1 || bundle.RunID != r.RunID || bundle.SessionID != r.SessionID || bundle.Epoch != r.Epoch {
		return empty, errors.New("bundle identity or schema mismatch against request")
	}
	if bundle.ContractSHA256 != lock.ContractSHA256 || bundle.CapabilitySHA256 != lock.CapabilitySHA256 {
		return empty, errors.New("bundle contract/capability digest mismatch against held-out lock")
	}
	if lock.PolicySHA256 != "" && bundle.PolicySHA256 != lock.PolicySHA256 {
		return empty, errors.New("bundle policy digest mismatch against held-out lock")
	}
	if err := verifyArtifactConfinementAndDigests(r.ArtifactsRoot, bundle.Artifacts); err != nil {
		return empty, err
	}

	// 2. Run the child verifier from the pinned private copy. It is staged only
	// now, after every check that can reject without running it.
	pinnedBinary, discardPinned, err := stagePinnedVerifier(binRaw, filepath.Ext(r.Binary))
	if err != nil {
		return empty, fmt.Errorf("stage verifier binary: %w", err)
	}
	defer discardPinned()
	pinned := r
	pinned.Binary = pinnedBinary
	report, err := Verify(ctx, pinned)
	if err != nil {
		return report, err
	}

	// 3. Post-execution TOCTOU re-hash + cross-report consistency check. The
	// binary re-hash is defense in depth: what ran was the private copy, but a
	// verifier path that changed mid-run still fails closed.
	postBinSHA, _, _, err := hashRegularFileBounded(r.Binary, 64<<20)
	if err != nil || postBinSHA != binSHA {
		return empty, errors.New("TOCTOU violation: verifier binary changed during execution")
	}
	postContractSHA, _, _, err := hashRegularFileBounded(r.ContractPath, maxHeldOutFileBytes)
	if err != nil || postContractSHA != contractSHA {
		return empty, errors.New("TOCTOU violation: contract file changed during execution")
	}
	postBundleSHA, _, _, err := hashRegularFileBounded(r.BundlePath, maxHeldOutFileBytes)
	if err != nil || postBundleSHA != bundleSHA {
		return empty, errors.New("TOCTOU violation: bundle file changed during execution")
	}
	if report.BundleSHA256 != bundleSHA {
		return empty, fmt.Errorf("report bundle_sha256 %q does not match hashed bundle %q", report.BundleSHA256, bundleSHA)
	}

	// 4. Passed-verdict soundness gate: a passed report cannot rest on an
	// incomplete journal, degraded subsystems, failed required criteria, or blocking issues.
	if report.Verdict == "passed" {
		if !bundle.JournalComplete || len(bundle.Degraded) > 0 {
			return empty, errors.New("passed report rejected: bundle journal incomplete or degraded")
		}
		// Which criteria are required is decided by the held-out contract, never by
		// the Required flag the verifier's own report carries, and a report may
		// neither repeat a criterion nor name one the contract does not contain.
		contractRequired := make(map[string]bool, len(contract.Criteria))
		for _, c := range contract.Criteria {
			if _, dup := contractRequired[c.ID]; dup {
				return empty, fmt.Errorf("contract declares criterion %q more than once", c.ID)
			}
			contractRequired[c.ID] = c.Required
		}
		reported := make(map[string]bool, len(report.Criteria))
		for _, cr := range report.Criteria {
			if reported[cr.ID] {
				return empty, fmt.Errorf("passed report repeats criterion %q", cr.ID)
			}
			reported[cr.ID] = true
			required, inContract := contractRequired[cr.ID]
			if !inContract {
				return empty, fmt.Errorf("passed report names criterion %q that the contract does not declare", cr.ID)
			}
			if required && cr.Verdict != "passed" {
				return empty, fmt.Errorf("passed report contains non-passed required criterion %q (%s)", cr.ID, cr.Verdict)
			}
		}
		for id, required := range contractRequired {
			if required && !reported[id] {
				return empty, fmt.Errorf("passed report omitted required contract criterion %q", id)
			}
		}
		for _, iss := range report.Issues {
			if iss.Verdict == "failed" || iss.Verdict == "incomplete" {
				return empty, fmt.Errorf("passed report carries blocking issue %q (%s)", iss.Code, iss.Verdict)
			}
		}
	}
	return report, nil
}

// DominanceTuple represents a candidate qualification vector for Pareto /
// dominance comparison. SafetyGateOK and Regressions are non-compensable: no
// latency reduction or optional pass count may ever offset a failed safety gate
// or an extra regression.
type DominanceTuple struct {
	CandidateID    string
	SafetyGateOK   bool
	Regressions    uint32
	RequiredPassed uint32
	OptionalPassed uint32
	LatencyMillis  uint64
}

// StrictlyDominates reports whether candidate a strictly dominates candidate b
// under Jarvis's safety-first partial order (a ≻ b).
//
// An unsafe candidate (!a.SafetyGateOK) never dominates any candidate, ensuring
// two broken builds cannot rank-order one another into promotion. A safe
// zero-regression candidate strictly dominates an unsafe candidate; among safe
// candidates, dominance is Pareto over (-Regressions, +RequiredPassed,
// +OptionalPassed, -LatencyMillis).
func StrictlyDominates(a, b DominanceTuple) bool {
	if !a.SafetyGateOK {
		return false
	}
	if !b.SafetyGateOK {
		return a.Regressions == 0
	}
	noWorse := a.Regressions <= b.Regressions &&
		a.RequiredPassed >= b.RequiredPassed &&
		a.OptionalPassed >= b.OptionalPassed &&
		a.LatencyMillis <= b.LatencyMillis
	if !noWorse {
		return false
	}
	strictlyBetter := a.Regressions < b.Regressions ||
		a.RequiredPassed > b.RequiredPassed ||
		a.OptionalPassed > b.OptionalPassed ||
		a.LatencyMillis < b.LatencyMillis
	return strictlyBetter
}

// SelectAdmissibleFrontier returns all safety-admissible (SafetyGateOK &&
// Regressions == 0) candidates that are not strictly dominated by any peer.
func SelectAdmissibleFrontier(candidates []DominanceTuple) []DominanceTuple {
	var frontier []DominanceTuple
	for i, c := range candidates {
		if !c.SafetyGateOK || c.Regressions > 0 {
			continue
		}
		dominated := false
		for j, other := range candidates {
			if i != j && StrictlyDominates(other, c) {
				dominated = true
				break
			}
		}
		if !dominated {
			frontier = append(frontier, c)
		}
	}
	return frontier
}
