//go:build !windows

package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha256HexBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type heldOutFixture struct {
	dir        string
	artRoot    string
	contractP  string
	bundleP    string
	binP       string
	req        Request
	lock       HeldOutLock
	bundleBody Bundle
}

func buildCleanHeldOutFixture(t *testing.T) heldOutFixture {
	t.Helper()
	dir := t.TempDir()
	artRoot := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(artRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	artPayload := []byte("deterministic-test-log\n")
	artFile := filepath.Join(artRoot, "run.log")
	if err := os.WriteFile(artFile, artPayload, 0o644); err != nil {
		t.Fatal(err)
	}

	contract := Contract{
		SchemaVersion: 1,
		ID:            "qualify-core",
		Criteria: []Criterion{
			{ID: "crit-tests", Fact: "tests.green", Required: true, Predicate: Predicate{Op: "eq", Value: json.RawMessage(`true`)}},
		},
	}
	contractRaw, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	contractP := filepath.Join(dir, "contract.json")
	if err := os.WriteFile(contractP, contractRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	contractSHA := sha256HexBytes(contractRaw)
	capSHA := strings.Repeat("c", 64)
	polSHA := strings.Repeat("d", 64)

	bundle := Bundle{
		SchemaVersion:    1,
		RunID:            "run-formal-1",
		SessionID:        "sess-1",
		Epoch:            3,
		ContractSHA256:   contractSHA,
		CapabilitySHA256: capSHA,
		PolicySHA256:     polSHA,
		JournalComplete:  true,
		Degraded:         []string{},
		Artifacts: []Artifact{
			{
				ID:        "art-log",
				Path:      "run.log",
				SHA256:    sha256HexBytes(artPayload),
				SizeBytes: uint64(len(artPayload)),
			},
		},
	}
	bundleRaw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	bundleP := filepath.Join(dir, "bundle.json")
	if err := os.WriteFile(bundleP, bundleRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	bundleSHA := sha256HexBytes(bundleRaw)

	rep := Report{
		OK:             true,
		SchemaVersion:  1,
		Verdict:        "passed",
		ContractSHA256: contractSHA,
		BundleSHA256:   bundleSHA,
		RunID:          "run-formal-1",
		Criteria: []CriterionResult{
			{ID: "crit-tests", Required: true, Verdict: "passed", Reason: "ok"},
		},
		Issues: []Issue{},
	}
	repRaw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	binP := filepath.Join(dir, "dc-evidence-stub")
	script := "#!/bin/sh\ncat <<'EOF'\n" + string(repRaw) + "\nEOF\nexit 0\n"
	if err := os.WriteFile(binP, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	binSHA := sha256HexBytes([]byte(script))

	req := Request{
		Binary:           binP,
		ContractPath:     contractP,
		BundlePath:       bundleP,
		ArtifactsRoot:    artRoot,
		ContractSHA256:   contractSHA,
		CapabilitySHA256: capSHA,
		RunID:            "run-formal-1",
		SessionID:        "sess-1",
		Epoch:            3,
	}
	lock := HeldOutLock{
		VerifierSHA256:   binSHA,
		ContractSHA256:   contractSHA,
		CapabilitySHA256: capSHA,
		PolicySHA256:     polSHA,
	}
	return heldOutFixture{
		dir:        dir,
		artRoot:    artRoot,
		contractP:  contractP,
		bundleP:    bundleP,
		binP:       binP,
		req:        req,
		lock:       lock,
		bundleBody: bundle,
	}
}

func TestHeldOutLockComparatorAcceptsValidAndRejectsEveryBypassClass(t *testing.T) {
	// 1. Reachability / non-vacuity witness: authentic held-out lock + matching bundle/artifacts pass.
	clean := buildCleanHeldOutFixture(t)
	rep, err := VerifyAgainstHeldOutLock(context.Background(), clean.req, clean.lock)
	if err != nil || rep.Verdict != "passed" {
		t.Fatalf("expected valid held-out verification to pass, got rep=%+v err=%v", rep, err)
	}

	// 2. Disproof 1: tampered verifier binary rejected BEFORE trust.
	t.Run("tampered_verifier_binary_digest", func(t *testing.T) {
		fx := buildCleanHeldOutFixture(t)
		if err := os.WriteFile(fx.binP, []byte("#!/bin/sh\necho forged\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyAgainstHeldOutLock(context.Background(), fx.req, fx.lock); err == nil {
			t.Fatal("accepted tampered verifier binary")
		}
	})

	// 3. Disproof 2: on-disk contract weakened after hash recorded.
	t.Run("weakened_contract_on_disk", func(t *testing.T) {
		fx := buildCleanHeldOutFixture(t)
		weak := Contract{SchemaVersion: 1, ID: "qualify-core", Criteria: nil}
		raw, _ := json.Marshal(weak)
		if err := os.WriteFile(fx.contractP, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyAgainstHeldOutLock(context.Background(), fx.req, fx.lock); err == nil {
			t.Fatal("accepted weakened on-disk contract")
		}
	})

	// 4. Disproof 3: report.BundleSHA256 does not match actual bundle.json bytes.
	t.Run("bundle_sha256_mismatch_in_report", func(t *testing.T) {
		fx := buildCleanHeldOutFixture(t)
		fx.bundleBody.PolicySHA256 = fx.lock.PolicySHA256
		fx.bundleBody.Degraded = []string{} // re-marshal with extra whitespace so sha changes
		mutated := append([]byte(" \n"), mustMarshal(t, fx.bundleBody)...)
		if err := os.WriteFile(fx.bundleP, mutated, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyAgainstHeldOutLock(context.Background(), fx.req, fx.lock); err == nil {
			t.Fatal("accepted report whose bundle_sha256 disagrees with on-disk bundle")
		}
	})

	// 5. Disproof 4: TOCTOU mutation of bundle.json by child process during Verify.
	t.Run("toctou_bundle_mutation_during_verify", func(t *testing.T) {
		fx := buildCleanHeldOutFixture(t)
		bundleRaw, _ := os.ReadFile(fx.bundleP)
		bundleSHA := sha256HexBytes(bundleRaw)
		rep := Report{
			OK:             true,
			SchemaVersion:  1,
			Verdict:        "passed",
			ContractSHA256: fx.lock.ContractSHA256,
			BundleSHA256:   bundleSHA,
			RunID:          fx.req.RunID,
			Criteria:       []CriterionResult{{ID: "crit-tests", Required: true, Verdict: "passed"}},
		}
		repRaw := mustMarshal(t, rep)
		// Child verifier appends a space to bundle.json during execution!
		evilScript := "#!/bin/sh\nprintf ' ' >> '" + fx.bundleP + "'\ncat <<'EOF'\n" + string(repRaw) + "\nEOF\nexit 0\n"
		if err := os.WriteFile(fx.binP, []byte(evilScript), 0o700); err != nil {
			t.Fatal(err)
		}
		fx.lock.VerifierSHA256 = sha256HexBytes([]byte(evilScript))
		if _, err := VerifyAgainstHeldOutLock(context.Background(), fx.req, fx.lock); err == nil {
			t.Fatal("accepted TOCTOU bundle mutation during verifier execution")
		}
	})

	// 6. Disproof 5: artifact path traversal ("../outside.txt") rejected.
	t.Run("artifact_path_traversal_escape", func(t *testing.T) {
		fx := buildCleanHeldOutFixture(t)
		outside := filepath.Join(fx.dir, "outside.txt")
		payload := []byte("outside")
		if err := os.WriteFile(outside, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		fx.bundleBody.Artifacts = []Artifact{{
			ID:        "escape",
			Path:      "../outside.txt",
			SHA256:    sha256HexBytes(payload),
			SizeBytes: uint64(len(payload)),
		}}
		rewriteBundleAndStub(t, &fx, fx.bundleBody, []CriterionResult{{ID: "crit-tests", Required: true, Verdict: "passed"}}, nil)
		if _, err := VerifyAgainstHeldOutLock(context.Background(), fx.req, fx.lock); err == nil {
			t.Fatal("accepted ../ artifact traversal escape")
		}
	})

	// 7. Disproof 6: artifact symlink pointing outside ArtifactsRoot rejected.
	t.Run("artifact_symlink_escape", func(t *testing.T) {
		fx := buildCleanHeldOutFixture(t)
		outside := filepath.Join(fx.dir, "secret.txt")
		payload := []byte("secret")
		if err := os.WriteFile(outside, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		sym := filepath.Join(fx.artRoot, "link.log")
		if err := os.Symlink(outside, sym); err != nil {
			t.Fatal(err)
		}
		fx.bundleBody.Artifacts = []Artifact{{
			ID:        "symlink-escape",
			Path:      "link.log",
			SHA256:    sha256HexBytes(payload),
			SizeBytes: uint64(len(payload)),
		}}
		rewriteBundleAndStub(t, &fx, fx.bundleBody, []CriterionResult{{ID: "crit-tests", Required: true, Verdict: "passed"}}, nil)
		if _, err := VerifyAgainstHeldOutLock(context.Background(), fx.req, fx.lock); err == nil {
			t.Fatal("accepted symlink escaping artifacts root")
		}
	})

	// 8. Disproof 7: passed verdict with JournalComplete == false or Degraded != empty rejected.
	t.Run("passed_verdict_with_incomplete_journal", func(t *testing.T) {
		fx := buildCleanHeldOutFixture(t)
		fx.bundleBody.JournalComplete = false
		rewriteBundleAndStub(t, &fx, fx.bundleBody, []CriterionResult{{ID: "crit-tests", Required: true, Verdict: "passed"}}, nil)
		if _, err := VerifyAgainstHeldOutLock(context.Background(), fx.req, fx.lock); err == nil {
			t.Fatal("accepted passed verdict on incomplete bundle journal")
		}
	})

	// 9. Disproof 8: passed verdict omitting a required contract criterion rejected.
	t.Run("passed_verdict_omitting_required_criterion", func(t *testing.T) {
		fx := buildCleanHeldOutFixture(t)
		rewriteBundleAndStub(t, &fx, fx.bundleBody, []CriterionResult{}, nil)
		if _, err := VerifyAgainstHeldOutLock(context.Background(), fx.req, fx.lock); err == nil {
			t.Fatal("accepted passed verdict that omitted required contract criterion")
		}
	})
}

// The lock hashes the verifier and must then run exactly those bytes. A path an
// attacker can rewrite between the hash and the exec is not the hashed verifier,
// so the verifier has to run from a private copy that is removed afterwards.
func TestHeldOutLockRunsAPrivateCopyOfTheHashedVerifier(t *testing.T) {
	fx := buildCleanHeldOutFixture(t)
	marker := filepath.Join(fx.dir, "ran-from")
	orig, err := os.ReadFile(fx.binP)
	if err != nil {
		t.Fatal(err)
	}
	script := strings.Replace(string(orig), "#!/bin/sh\n", "#!/bin/sh\nprintf '%s' \"$0\" > '"+marker+"'\n", 1)
	if err := os.WriteFile(fx.binP, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	fx.lock.VerifierSHA256 = sha256HexBytes([]byte(script))

	rep, err := VerifyAgainstHeldOutLock(context.Background(), fx.req, fx.lock)
	if err != nil || rep.Verdict != "passed" {
		t.Fatalf("expected the locked verifier to pass, got rep=%+v err=%v", rep, err)
	}
	ranFromRaw, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("verifier did not run: %v", err)
	}
	ranFrom := string(ranFromRaw)
	if ranFrom == fx.binP || strings.HasPrefix(ranFrom, fx.dir+string(filepath.Separator)) {
		t.Fatalf("verifier ran from %q, a path a writer can swap after it was hashed; it must run a private copy", ranFrom)
	}
	if _, err := os.Stat(ranFrom); !os.IsNotExist(err) {
		t.Fatalf("private verifier copy %q was not removed (stat err=%v)", ranFrom, err)
	}
}

// Which criteria are required comes from the held-out contract, never from flags
// the verifier's own report carries, and a report may not repeat or invent IDs.
func TestHeldOutLockPassedGateUsesContractNotReportFlags(t *testing.T) {
	cases := []struct {
		name string
		crit []CriterionResult
	}{
		{"duplicate_id_masks_required_failure", []CriterionResult{
			{ID: "crit-tests", Required: false, Verdict: "failed"},
			{ID: "crit-tests", Required: true, Verdict: "passed"},
		}},
		{"duplicate_id_even_when_both_pass", []CriterionResult{
			{ID: "crit-tests", Required: true, Verdict: "passed"},
			{ID: "crit-tests", Required: true, Verdict: "passed"},
		}},
		{"criterion_not_in_contract", []CriterionResult{
			{ID: "crit-tests", Required: true, Verdict: "passed"},
			{ID: "ghost", Required: false, Verdict: "failed"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := buildCleanHeldOutFixture(t)
			rewriteBundleAndStub(t, &fx, fx.bundleBody, tc.crit, nil)
			if _, err := VerifyAgainstHeldOutLock(context.Background(), fx.req, fx.lock); err == nil {
				t.Fatal("accepted a passed report whose criteria disagree with the held-out contract")
			}
		})
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func rewriteBundleAndStub(t *testing.T, fx *heldOutFixture, b Bundle, crit []CriterionResult, issues []Issue) {
	t.Helper()
	bRaw := mustMarshal(t, b)
	if err := os.WriteFile(fx.bundleP, bRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	bSHA := sha256HexBytes(bRaw)
	rep := Report{
		OK:             true,
		SchemaVersion:  1,
		Verdict:        "passed",
		ContractSHA256: fx.lock.ContractSHA256,
		BundleSHA256:   bSHA,
		RunID:          fx.req.RunID,
		Criteria:       crit,
		Issues:         issues,
	}
	repRaw := mustMarshal(t, rep)
	script := "#!/bin/sh\ncat <<'EOF'\n" + string(repRaw) + "\nEOF\nexit 0\n"
	if err := os.WriteFile(fx.binP, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	fx.lock.VerifierSHA256 = sha256HexBytes([]byte(script))
}

func TestFormalDominanceStrictPartialOrderAndSafetyNonCompensability(t *testing.T) {
	// Enumerate finite state grid: 2 safety states * 3 regression counts * 3 req * 3 opt * 3 latency = 162 tuples,
	// checking all 162^3 = 4,251,528 triples exhaustively!
	var grid []DominanceTuple
	id := 0
	for _, safe := range []bool{false, true} {
		for _, reg := range []uint32{0, 1, 2} {
			for _, reqP := range []uint32{0, 1, 2} {
				for _, optP := range []uint32{0, 1, 2} {
					for _, lat := range []uint64{10, 50, 200} {
						id++
						grid = append(grid, DominanceTuple{
							CandidateID:    "c",
							SafetyGateOK:   safe,
							Regressions:    reg,
							RequiredPassed: reqP,
							OptionalPassed: optP,
							LatencyMillis:  lat,
						})
					}
				}
			}
		}
	}

	n := len(grid)
	if n != 162 {
		t.Fatalf("expected 162 grid tuples, got %d", n)
	}

	rel := make([][]bool, n)
	edgeCount := 0
	for i := range n {
		rel[i] = make([]bool, n)
		for j := range n {
			d := StrictlyDominates(grid[i], grid[j])
			rel[i][j] = d
			if d {
				edgeCount++
			}
			// Safety non-compensability theorem:
			// 1) Unsafe candidate never dominates anything.
			if d && !grid[i].SafetyGateOK {
				t.Fatalf("unsafe candidate %+v dominated %+v", grid[i], grid[j])
			}
			// 2) Candidate with more regressions never dominates a safe candidate with fewer regressions.
			if d && grid[j].SafetyGateOK && grid[i].Regressions > grid[j].Regressions {
				t.Fatalf("regression-heavier candidate %+v dominated %+v", grid[i], grid[j])
			}
		}
	}
	if edgeCount == 0 {
		t.Fatal("non-vacuity failure: zero dominance edges in grid")
	}

	// Exhaustive algebraic verification of Strict Partial Order axioms over all 162^3 triples:
	// 1. Irreflexivity: ∀x, ¬(x ≻ x)
	// 2. Asymmetry: ∀x,y, (x ≻ y) ⇒ ¬(y ≻ x)
	// 3. Transitivity: ∀x,y,z, (x ≻ y) ∧ (y ≻ z) ⇒ (x ≻ z)
	for i := range n {
		if rel[i][i] {
			t.Fatalf("irreflexivity violated at %+v", grid[i])
		}
		for j := range n {
			if rel[i][j] && rel[j][i] {
				t.Fatalf("asymmetry violated between %+v and %+v", grid[i], grid[j])
			}
			if !rel[i][j] {
				continue
			}
			for k := range n {
				if rel[j][k] && !rel[i][k] {
					t.Fatalf("transitivity violated: %+v ≻ %+v and %+v ≻ %+v, but ¬(%+v ≻ %+v)",
						grid[i], grid[j], grid[j], grid[k], grid[i], grid[k])
				}
			}
		}
	}

	// Disproof of naive linear scalarization:
	// A naive scalar score S = 100*OptionalPassed - 50*Regressions - LatencyMillis
	// ranks fastBuggy (1 regression, 2 optional, 5ms -> S = 145) ABOVE slowSafe
	// (0 regressions, 0 optional, 100ms -> S = -100), promoting a regression!
	// Verify SelectAdmissibleFrontier rejects fastBuggy and retains slowSafe.
	slowSafe := DominanceTuple{CandidateID: "slow-safe", SafetyGateOK: true, Regressions: 0, RequiredPassed: 5, OptionalPassed: 0, LatencyMillis: 100}
	fastBuggy := DominanceTuple{CandidateID: "fast-buggy", SafetyGateOK: true, Regressions: 1, RequiredPassed: 5, OptionalPassed: 2, LatencyMillis: 5}
	naiveScore := func(c DominanceTuple) int64 {
		return int64(c.OptionalPassed)*100 - int64(c.Regressions)*50 - int64(c.LatencyMillis)
	}
	if naiveScore(fastBuggy) <= naiveScore(slowSafe) {
		t.Fatal("disproof setup error: naive scalarizer should mistakenly prefer fastBuggy")
	}
	frontier := SelectAdmissibleFrontier([]DominanceTuple{slowSafe, fastBuggy})
	if len(frontier) != 1 || frontier[0].CandidateID != "slow-safe" {
		t.Fatalf("expected admissible frontier [slow-safe], got %+v", frontier)
	}
}
