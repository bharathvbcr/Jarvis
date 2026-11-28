# Verification Plan: 002 - Discovery, Deterministic Replay, Independent Verification, and Pinned Upstreams

## 1. Automated Verification Commands

1. **Specification validation (strict)**:
   ```bash
   devcouncil spec validate 002-discovery-replay-and-independent-verification --strict --json --project-root /Users/bharath/Code/devtools/Jarvis
   go run ./cmd/dev check-specs
   ```
2. **Go suites**:
   ```bash
   CGO_ENABLED=0 GOMAXPROCS=4 go test -count=1 -p 2 ./internal/... ./cmd/...
   ```
3. **Pins** (needs bootstrapped upstreams):
   ```bash
   go run ./cmd/dev check-pins
   ```

## 2. Acceptance Proof Matrix

| Story | Criterion | Proof Artifact / Test |
| --- | --- | --- |
| US1 | `SC-001`, `SC-002` | `internal/app/discovery_lifecycle_test.go`, `internal/app/discovery_journal_test.go`, `internal/app/discovery_fixture_test.go`, `internal/app/ledger_test.go` |
| US2 | `SC-001` | `internal/trace/replay_test.go`, `internal/app/profile_test.go`, `internal/codegen/codegen_test.go` |
| US3 | `SC-001` | `internal/evidence/evidence_test.go`, `internal/evidence/formal_comparator_test.go` |
| US4 | `SC-004` | `cmd/dev/package_test.go` (`TestCheckPinsRefusesAGoModuleThatDisagreesWithThePin`) |
| US5 | `SC-003` | `cmd/qualify/main_test.go`, `cmd/qualify/oracle_test.go`, `evidence/macos-guard-partition-40.json`, `evidence/linux-x11-final-40.json` |

## 3. Result Log

- 2026-11-28: `./internal/... ./cmd/...` all `ok` on `main` with 8 uncommitted files from other sessions.
- Not run: `dev bootstrap`, `dev check-pins`, `dev build`, any desktop or live-provider run, the Rust workspace, `tests/native-platform`.
