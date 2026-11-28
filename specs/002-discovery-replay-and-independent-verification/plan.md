# Implementation Plan: 002 - Discovery, Deterministic Replay, Independent Verification, and Pinned Upstreams

**Branch**: `main` | **Date**: 2026-11-28 | **Spec**: `specs/002-discovery-replay-and-independent-verification/spec.md`

## 1. Technical Context

- **Languages**: Go host (`internal/app`, `internal/trace`, `internal/evidence`, `internal/codegen`, `cmd/jarvis`, `cmd/qualify`, `cmd/dev`) and Rust (`desktop/crates/{bank-demo,desktop-ui,workbench}`, the synthetic bank and egui workbench).
- **Flow**: `jarvis discover` (model turns, optional) -> frozen `capability.json` -> `jarvis replay` (zero model turns, Manvi reducer plus desktop broker) -> `jarvis verify --contract C --bundle B` (DevCouncil `dcverify`, `internal/evidence`).
- **Contracts**: `scenarios/*.contract.json` (acceptance, independent of the capability), `scenarios/*.inputs.json`, `profiles/{north,south,vani}.json`, `policies/jarvis-bank.json`, `overlays/south-balance.json`, `examples/{balance,create-subaccount}.json`.
- **Persistence**: `.local/gemini-campaign.json` (ledger), per-run evidence directories, `evidence/*.json` retained campaign reports.
- **Build**: `go run ./cmd/dev {bootstrap,workspace,build,test,gusset,qualify,check-pins,check-specs,bundle-sources,package}`; CI (`.github/workflows/ci.yml`) runs `dev bootstrap` then `dev test`.

## 2. Constitution Check & Complexity Tracking

Judged against the nine articles of DevCouncil's `constitution.md`. That document is DevCouncil-specific; Jarvis has no repo-local principles document, so no `constitution.md` was created.

| Article | Status | Compliance Details / Justification |
| :--- | :--- | :--- |
| **I: Library-First** | PASSED | `internal/trace`, `internal/evidence` and `internal/codegen` are importable packages the CLI wraps. |
| **II: CLI as First-Class Consumer** | PASSED | `jarvis replay|discover|verify|codegen|compile|drift|catalog` and `cmd/qualify`. |
| **III: Test-First** | N/A | Authoring order not verifiable from the repository. |
| **IV: Zero Ambient Authority & Fail-Closed** | PASSED | Verifier output and exit are bounded and fail closed (`TestVerifyFailsClosed`); held-out locks reject unlocked verifiers; `check-pins` refuses dirty or mismatched upstreams. |
| **V: Single Source of Truth** | PASSED | Pins live once in `upstream.lock.json`; `checkUpstreamPinsAgree` compares Manvi's and DevCouncil's own pin files to it. |
| **VI: Memory & Allocator Symmetry** | N/A | Go host; allocator rules belong to the native crates. |
| **VII: Simplicity Limit (<= 3 modules)** | N/A | Product repo with several binaries. |
| **VIII: Anti-Abstraction** | N/A | Not assessed. |
| **IX: Observability & Forensic Auditability** | PASSED | Immutable journal, masked frames, Perfetto trace, retained campaign reports under `evidence/`. |

| Deviation / Seam | Why Needed Now | Simpler Alternative Rejected | Closure / Guard |
| --- | --- | --- | --- |
| Soft ledger cap with conservative prices | Cached input is charged at the full rate and reasoning on top of output, so a charge can only be over-stated | Pricing from provider usage alone could under-count | `campaignPriceRevision` names the rate table in the discovery record |
| Held-out lock beside `dcverify`'s own checks | The agent must not be able to rewrite verifier, contract or capability between run and judgement | Trusting hashes inside the report | `VerifyAgainstHeldOutLock` takes the expectations from outside the run |

## 3. Verified File Map

- **Discovery**: `internal/app/discovery.go`, `internal/app/discovery_provider.go`, `internal/app/discovery_journal.go`, `internal/app/discovery_publish.go`, `internal/app/ledger.go`, `docs/live-discovery-runbook.md`, `docs/provider-qualification.md`
- **Replay and codegen**: `internal/trace/replay.go`, `internal/codegen/codegen.go`, `internal/app/drift.go`, `internal/app/reconciliation.go`
- **Verification**: `internal/evidence/evidence.go`, `internal/evidence/formal_comparator.go`, `docs/requirements-evidence.md`
- **Qualification**: `cmd/qualify/main.go`, `cmd/qualify/oracle.go`, `cmd/qualify/interactive.go`, `cmd/qualify/provenance.go`, `cmd/qualify/README.md`
- **Build and pins**: `cmd/dev/main.go`, `cmd/dev/package.go`, `cmd/dev/steps.go`, `cmd/dev/gusset.go`, `upstream.lock.json`, `go.mod`, `.github/workflows/ci.yml`
