# Data Model: 002 - Discovery, Deterministic Replay, Independent Verification, and Pinned Upstreams

**Date**: 2026-11-28 | **Source Verification**: `internal/app/ledger.go`, `internal/app/discovery.go`, `internal/evidence/formal_comparator.go`, `cmd/dev/main.go`, `upstream.lock.json`.

## 1. Upstream Lock (`upstream.lock.json`, `cmd/dev/main.go`)

| Entity | Fields | Invariants |
| --- | --- | --- |
| `lock` | `schema_version`, `manvi`, `devcouncil`, `gusset` (each `{url, revision}`) | Gusset is required because Manvi's `go.mod` replaces gusset with the sibling of its checkout. `check-pins` refuses a `HEAD` differing from `revision`, a dirty checkout, and a Manvi `go.mod` version that disagrees with the pin. |

## 2. Campaign Ledger (`internal/app/ledger.go`)

| Entity | Fields / Bounds | Invariants |
| --- | --- | --- |
| Campaign ledger | file `.local/gemini-campaign.json`, limit `25_000_000_000` nano-USD | One ledger for assisted recovery and discovery; Manvi's `budget.Snapshot` shape (Manvi `specs/003`). |
| Campaign prices | input 1500, output 7500, cache write 1500, cache read 1500 nano-USD per token | Revision `google-standard-2027-conservative-2026-09-09`; charges can only be over-stated. |

## 3. Held-Out Lock (`internal/evidence/formal_comparator.go`)

| Entity | Fields | Invariants |
| --- | --- | --- |
| `HeldOutLock` | `VerifierSHA256`, `ContractSHA256`, `CapabilitySHA256`, `PolicySHA256` (optional) | Out-of-band expectations the agent or child verifier cannot rewrite; hashed files are bounded to 8 MiB (`maxHeldOutFileBytes`). |
| `DominanceTuple` | compared by `StrictlyDominates`; `SelectAdmissibleFrontier` selects the frontier | Safety is non-compensable in the dominance order (`TestFormalDominanceStrictPartialOrderAndSafetyNonCompensability`). |

## 4. Qualification Report (`evidence/macos-guard-partition-40.json`)

Top-level keys read from the file: `schema_version`, `platform`, `architecture`, `desktop_session`, `started_at`, `mode`, `scenario`, `requested_per_tenant`, `tenants`, `results`, `by_tenant`, `matrix_complete`. Existing report paths are refused by `cmd/qualify`.
