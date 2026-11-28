# Research & Evidence Audit: 001 - Agent Operation Gate, Host Module, and Background Jobs

**Date**: 2026-11-28 | **Scope**: `internal/app/`, `cmd/jarvis/`, `docs/agent-cli.md`, `docs/host-jobs.md` at HEAD `e38d0b55` on `main`.

## 1. Evidence Observed

1. **Classification table** (verified by reading `internal/app/agent.go:95-130`): nine `agent` ops, nine `human-only` ops (including `jarvis.inputs.get`, refused in every mode) and eleven `process-scoped` ops; `jarvis.catalog.promote` and `jarvis.inputs.put` carry `yolo: true`; `jarvis.run.assist`, `takeover`, `handback`, `act`, `key.set` and `evidence.export` carry `live: true`.
2. **Tests run in this session** (verified): `CGO_ENABLED=0 GOMAXPROCS=4 go test -count=1 -p 2 ./internal/... ./cmd/...` returned `ok` for `internal/app`, `internal/codegen`, `internal/evidence`, `internal/trace`, `cmd/dev`, `cmd/jarvis` and `cmd/qualify`.
3. **Operator attribution** (verified by reading `internal/app/operator.go`): `OperatorAgent = "agent"`, `validOperator` rejects anything else, and the stdio host spawned by the workbench never sets it.
4. **Background jobs** (verified from `docs/host-jobs.md`): 16 retained jobs, five-minute expiry, 16 MiB per result, 64 MiB total, outer deadlines of 10, 15, 15, 15 and 30 seconds for doctor, verification, frame, anchor and export.
5. **README gap** (verified: `git show HEAD:README.md` contains "Open gap"): no documented path yet for a Manvi agent to load Jarvis at runtime.

## 2. Open work

- T-009: document or implement how a Manvi agent loads Jarvis at runtime. Evidence: the committed `README.md` "Consumers" section.

## 3. Unverified

- The README op table is another session's uncommitted edit (`README.md` shows modified in `git status`); the spec relies on the code and on `TestREADMEOpsTableMatchesAgentOps` passing, not on the README text.
- `scripts/check-consumers.sh` lives in the Manvi repository and was not run.
- Windows behaviour (`cmd/dev/procgroup_other.go`) was not run; this host is macOS.
- `devmap` was available (Jarvis index fresh, generation 243) but symbol lookups were not run; paths were verified with `test -e` and by reading source.
- The Rust workbench's handling of job results was not examined.
