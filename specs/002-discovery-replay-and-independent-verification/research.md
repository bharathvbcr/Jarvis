# Research & Evidence Audit: 002 - Discovery, Deterministic Replay, Independent Verification, and Pinned Upstreams

**Date**: 2026-11-28 | **Scope**: `internal/app/` (discovery, ledger, drift), `internal/{trace,evidence,codegen}/`, `cmd/{qualify,dev}/`, `upstream.lock.json`, `docs/requirements-evidence.md`, `docs/provider-qualification.md` at HEAD `e38d0b55` on `main`.

## 1. Evidence Observed

1. **Tests run in this session** (verified): `CGO_ENABLED=0 GOMAXPROCS=4 go test -count=1 -p 2 ./internal/... ./cmd/...` all `ok`, including `cmd/dev` (17.9s) and `cmd/qualify`.
2. **Pins** (verified by reading `upstream.lock.json`): Manvi `a86369b6b554ff36c30091ee41583de1b4e6e719`, DevCouncil `2acef200df69622b459278e1d88745bfb393d043`, gusset `19383ad1`. `go.mod` requires `github.com/bharathvbcr/Manvi/manvi v0.0.0-20261125003843-a86369b6b554`. The Manvi checkout at `/Users/bharath/Code/devtools/Manvi` is at `785d9883` and has moved past the pin; the pin was not changed.
3. **Ledger cap** (verified by reading `internal/app/ledger.go`): `campaignLedgerSoftCap = 25_000_000_000` nano-USD, rates 1500 input / 7500 output / 1500 cache write / 1500 cache read nano-USD per token, `campaignPriceRevision = "google-standard-2027-conservative-2026-09-09"`, `DiscoveryModel = "gemini-3.8-flash"` (`internal/app/discovery.go`).
4. **Retained campaign reports** (verified by reading `docs/requirements-evidence.md`): macOS current-source guard-partition campaign 35 of 40 with all 40 saved-state checks unchanged; macOS pinned 40-run 23 passed and 17 suspended; Linux X11 40-run 40 successful with 1 first-attempt success; denial campaign 4 of 4 denied. The statement that there is no clean stability pass on the macOS final pinned build is the document's own. The JSON reports themselves were listed but not re-parsed except `evidence/macos-guard-partition-40.json` for its top-level keys.
5. **Where the docs and code differ** (verified): `README.md` says "$25 hard spend cap"; the code names it a soft cap. The code is followed.

## 2. Open work (evidence in `docs/` and `BUILD_REPORT.md`)

- T-010: credentialed live Gemini discovery and the multimodal tool-decision sequence (`docs/provider-qualification.md`: "No credentialed Gemini qualification has run in this checkout"; `docs/requirements-evidence.md`: discovery is "Implemented, unqualified").
- T-011: human-approved positive creation, completed takeover/resume and live visual input (`docs/requirements-evidence.md`, "Human approval and same-session takeover" row: Unqualified).
- T-012: a clean N-run stability pass on the current source (`docs/requirements-evidence.md`: 35/40 is "an improvement, not a clean stability pass").
- `BUILD_REPORT.md` (2026-11-07) lists a deferred discovery-payload card and a Lappi caller card; their current status was not checked, so they are not turned into tasks.

## 3. Unverified

- `go run ./cmd/dev check-pins` and `dev bootstrap` were not run: they need fresh upstream checkouts at the pinned revisions (`.local/upstream/`) and the Manvi checkout currently differs from its pin.
- The Rust workspace under `desktop/`, `tests/native-platform` and `cmd/qualify` against a real desktop were not run; no live provider call was made.
- Campaign counts above are as stated in `docs/requirements-evidence.md`; only one report's keys were opened.
- `scenarios/*.contract.json` content was not read; they are cited as the home of acceptance contracts from the directory listing and the documentation.
- `README.md` has uncommitted edits from another session; its architecture diagrams were not relied on.
