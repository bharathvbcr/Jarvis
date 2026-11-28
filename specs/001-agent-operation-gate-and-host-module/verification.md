# Verification Plan: 001 - Agent Operation Gate, Host Module, and Background Jobs

## 1. Automated Verification Commands

1. **Specification validation (strict)**:
   ```bash
   devcouncil spec validate 001-agent-operation-gate-and-host-module --strict --json --project-root /Users/bharath/Code/devtools/Jarvis
   go run ./cmd/dev check-specs
   ```
2. **Go suites**:
   ```bash
   CGO_ENABLED=0 GOMAXPROCS=4 go test -count=1 -p 2 ./internal/... ./cmd/...
   ```

## 2. Acceptance Proof Matrix

| Story | Criterion | Proof Artifact / Test |
| --- | --- | --- |
| US1 | `SC-002` | `internal/app/agent_test.go` (`TestEveryServedJarvisOpIsClassified`, `TestREADMEOpsTableMatchesAgentOps`), `cmd/jarvis/agent_test.go` |
| US2 | `SC-003` | `internal/app/yolo_test.go`, `internal/app/agent_test.go`, `cmd/jarvis/yolo_test.go` |
| US3 | `SC-004` | `internal/app/jobs_test.go` |
| US4 | `SC-001` | `internal/app/inputs_test.go`, `internal/app/profile_test.go` |

## 3. Result Log

- 2026-11-28: `./internal/... ./cmd/...` all `ok` on `main` with 8 uncommitted files from other sessions (`LICENSE`, `README.md`, `desktop/Cargo.toml` and five new licence and contribution files).
- Not run: the Rust workspace under `desktop/`, `tests/native-platform`, and any live desktop or provider run.
