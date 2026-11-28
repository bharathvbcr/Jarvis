# Feature: 002 - Discovery, Deterministic Replay, Independent Verification, and Pinned Upstreams

**Status**: Active Specification | **Canonical Owner**: Jarvis (`github.com/bharathvbcr/Jarvis`) | **Date**: 2026-11-28 | **Implementation State**: Shipped for fixture, offline replay, verification, qualification tooling and pin checks; live credentialed discovery and several human-approved qualification gates are open (T-010 to T-012).

## 1. Problem Statement & Motivation

Jarvis discovers desktop capabilities with a Gemini model, freezes them into typed capability artifacts, replays them through Manvi with zero model decisions, and has DevCouncil check an acceptance contract that the agent did not write against the recorded evidence (`README.md`, "What Jarvis is"). Three things go wrong without a binding specification: discovery spend runs past its cap or is uncounted when an attempt is lost; a replay trace is accepted though it was tampered with or produced by a different capability; and a verifier or contract is swapped between the run and its judgement. A fourth is build drift: Jarvis builds against Manvi, DevCouncil and gusset checkouts, and a Go host and a native Rust workspace could come from different Manvi revisions (`docs/requirements-evidence.md`, "Upstream revision agreement").

## 2. Canonical Ownership & Non-Overlap Boundary

| Layer / Capability | Canonical Owner | Jarvis Role | Explicit Non-Goals / Delegations |
| --- | --- | --- | --- |
| **Workflow compiler, reducer, catalog, desktop broker, journal privacy** | **Manvi** (`manvi/workflow`, `manvi/computer`, `native/`) | Calls them; compiles per-tenant overlays (`overlays/south-balance.json`). | Does not reimplement a workflow engine; generated callers embed the artifact and delegate to the canonical executor (`internal/codegen/codegen.go`). |
| **Spend ledger** | **Manvi** (`manvi/llm/budget`, Manvi `specs/003`) | Opens one campaign ledger (`.local/gemini-campaign.json`) with a soft cap of `25_000_000_000` nano-USD ($25) and conservative prices (`internal/app/ledger.go`). | The cap is soft in the code, and the ledger governs only its own tally. |
| **Discovery and published capabilities** | **Jarvis** (`internal/app/discovery.go`, `discovery_provider.go`, `discovery_publish.go`) | Runs scoped desktop tools with `gemini-3.8-flash` (`DiscoveryModel`), admits and journals each action before returning, publishes a proposed capability. | Does not promote it: promotion is human-only (spec 001). |
| **Offline trace replay and drift reports** | **Jarvis** (`internal/trace/replay.go`, `internal/app/drift.go`) | Rejects foreign events, a wrong final state or tampered state. | The reducer itself is Manvi's. |
| **Independent acceptance** | **DevCouncil** (`rust/dc-evidence`, `dcverify evidence-check`) | `internal/evidence/evidence.go` runs the verifier; `internal/evidence/formal_comparator.go` enforces held-out SHA-256 locks and dominance ordering. | Acceptance predicates are supplied by contracts under `scenarios/`, not by the agent. |
| **Qualification campaigns** | **Jarvis** (`cmd/qualify/`) | N-run stability reports, a durable state oracle, interactive human approval. | Never counts agent-operated (yolo) runs. |
| **Pinned upstreams** | **Jarvis** (`upstream.lock.json`, `cmd/dev/`) | `dev bootstrap`, `check-pins`, `bundle-sources`, `package`. Pins Manvi `a86369b6`, DevCouncil `2acef200`, gusset `19383ad1`. | Bumping a pin is a decision for the owner, not part of this spec. |

## 3. User Stories & Acceptance Scenarios

### US1: Discovery Is Scoped, Metered and Durably Journalled (Priority: P1)

As an operator, a discovery run observes only the scoped window, every model-proposed action is admitted and journalled before it executes, and its spend counts against one ledger.

- **Scenario 1: Admission before execution**
  - **Given** a discovery run with a provider decision to act
  - **When** `App.Discover` admits the action (`internal/app/discovery.go`)
  - **Then** the action and its masked frames are durably stored before the call returns, and a journal failure blocks admission (`TestDiscoveryActionAdmissionIsDurableBeforeReturning`, `TestDiscoveryFramesAreDurableAndJournalFailureBlocksAdmission`).
- **Scenario 2: Assist and discovery share one ledger**
  - **Given** assisted recovery and discovery in one campaign
  - **When** either opens the ledger
  - **Then** both use `openCampaignLedger`, the Gemini adapter is meterable through `campaignGemini.PriceOf`, and a cancelled context stops opening (`TestOpenCampaignLedgerUnifiesAssistAndDiscovery`, `TestCampaignGeminiIsMeterable`, `TestOpenCampaignLedgerStopsOnCancelledContext`).
- **Scenario 3: Fixture discovery needs no credential**
  - **Given** the fixture provider
  - **When** discovery runs
  - **Then** it loads without a Gemini key and an unknown provider is rejected without a credential (`TestDiscoveryFixtureProviderLoadsWithoutGeminiKey`, `TestDiscoverRejectsUnknownProviderWithoutCredential`).

### US2: Replay Runs With Zero Model Decisions and Verifies Offline (Priority: P1)

As a verifier, a recorded trace replays offline against the capability that produced it, and a foreign or tampered trace is rejected.

- **Scenario 1: Foreign and tampered state rejected**
  - **Given** a recorded trace and capability (`examples/balance.json`)
  - **When** `trace.Replay` runs (`internal/trace/replay.go`)
  - **Then** a foreign event, a wrong final state or tampered state is refused (`TestOfflineTraceRejectsForeignAndTamperedState`).
- **Scenario 2: Tenant overlays cannot widen policy**
  - **Given** a tenant overlay such as `overlays/south-balance.json`
  - **When** the capability is compiled for the tenant
  - **Then** only the overlay naming that capability applies, two overlays naming one revision are refused, and an overlay cannot widen policy (`TestCompileForTenantAppliesOnlyTheOverlayNamingThatCapability`, `TestProfileRefusesTwoOverlaysForOneCapabilityRevision`, `TestOverlayCannotWidenPolicy`).

### US3: Acceptance Is Judged Independently Against Held-Out Locks (Priority: P1)

As an auditor, the verifier that judged a run is the one whose bytes were hashed, the contract came from outside the run, and an agent cannot forge a pass.

- **Scenario 1: Held-out lock holds**
  - **Given** a `HeldOutLock` of verifier, contract, capability and optional policy SHA-256 values (`internal/evidence/formal_comparator.go`)
  - **When** `VerifyAgainstHeldOutLock` runs
  - **Then** a mismatch, a repeated or undeclared criterion id, or a report claiming pass without the contract's required criteria is rejected, and the verifier that runs is a private copy of the hashed bytes (`TestHeldOutLockComparatorAcceptsValidAndRejectsEveryBypassClass`, `TestHeldOutLockRunsAPrivateCopyOfTheHashedVerifier`, `TestHeldOutLockPassedGateUsesContractNotReportFlags`).
- **Scenario 2: Verification fails closed**
  - **Given** a missing, oversized or cancelled verifier run
  - **When** `evidence.Verify` runs (`internal/evidence/evidence.go`)
  - **Then** it returns a failure, not a pass (`TestVerifyFailsClosed`, `TestVerifyHonoursCancellation`, `TestVerifyBoundsVerifierOutput`).

### US4: Upstream Pins Agree or the Build Refuses (Priority: P1)

As a maintainer, the Go host, the native Rust workspace and the pinned upstream checkouts come from the revisions in `upstream.lock.json`.

- **Scenario 1: Pin mismatch refused**
  - **Given** `upstream.lock.json` and the Manvi, DevCouncil and gusset checkouts
  - **When** `dev check-pins` runs (`cmd/dev/package.go`)
  - **Then** it refuses a checkout whose `HEAD` differs from its pin or that has changes outside the pinned revision, a Manvi `go.mod` version that disagrees with the pin, and upstream pin files that name other revisions (`TestCheckPinsRefusesAGoModuleThatDisagreesWithThePin`).

### US5: Qualification Reports Distinguish Success From Everything Else (Priority: P2)

As a release owner, a campaign counts a run as a success only when it truly was one, and records the rest by cause.

- **Scenario 1: Campaign accounting**
  - **Given** `go run ./cmd/qualify --n 40` per tenant
  - **When** the campaign ends
  - **Then** the report separates requested from attempted runs, first-attempt outcomes, input refusals, model requests and independent saved-state checks, and existing report paths are refused; the retained report `evidence/macos-guard-partition-40.json` is the 35/40 result named in `docs/requirements-evidence.md`.

## 4. Success Metrics & Testability Criteria

- **SC-001**: `CGO_ENABLED=0 go test -count=1 -p 2 ./internal/... ./cmd/...` passes (run 2026-11-28: `internal/app`, `internal/codegen`, `internal/evidence`, `internal/trace`, `cmd/dev`, `cmd/jarvis`, `cmd/qualify` all `ok`).
- **SC-002**: The campaign ledger soft cap constant is `25_000_000_000` nano-USD in `internal/app/ledger.go`.
- **SC-003**: The retained reports `evidence/macos-guard-partition-40.json` and `evidence/linux-x11-final-40.json` exist, and `docs/requirements-evidence.md` states their counts without claiming a clean stability pass.
- **SC-004**: `go run ./cmd/dev check-pins` passes in a bootstrapped workspace (not run in this session; see `research.md`).
- **SC-005**: `devcouncil spec validate --all --strict` passes from the repository root.

## 5. Edge Cases, Exclusions & Open Questions

- **Exclusions**:
  - No credentialed Gemini qualification has run in this checkout (`docs/provider-qualification.md`); fixture discovery is the verified path.
  - Windows runtime qualification is deferred (`docs/requirements-evidence.md`, Optional 5).
  - Replay, drift and verification evidence under `evidence/` binds historical executable hashes; a final source rebuild needs its own qualification.
- **Edge Cases**:
  - `README.md` calls the $25 limit a hard cap; `internal/app/ledger.go` names it `campaignLedgerSoftCap`. The code is followed here.
  - Interactive qualification refuses piped input and never sends an approval unless the full displayed `approve ...` command is typed (`cmd/qualify/README.md`).
- **Open Questions**: None; open gates are tasks T-010 to T-012.
