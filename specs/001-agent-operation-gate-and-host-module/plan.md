# Implementation Plan: 001 - Agent Operation Gate, Host Module, and Background Jobs

**Branch**: `main` | **Date**: 2026-11-28 | **Spec**: `specs/001-agent-operation-gate-and-host-module/spec.md`

## 1. Technical Context

- **Language**: Go (`go.mod` line `go 1.27.4`, module `github.com/bharathvbcr/Jarvis`), `CGO_ENABLED=0` by default with the Gusset engine linked only by `dev build --engine` on unix (`cmd/dev/main.go`).
- **Host**: `cmd/jarvis` builds an `internal/app.App` and serves it on Manvi's `serve` host plane; `App.Configure` registers every `jarvis.*` op (`internal/app/module.go`).
- **Gate**: `internal/app/agent.go` holds `agentOps`, classes `OpAgent`, `OpHumanOnly`, `OpProcessScoped`, and the `humanControls` kinds `approve` and `resume`. `cmd/jarvis/agent.go` implements `jarvis ops` and `jarvis call`; `cmd/jarvis/watch.go` implements `--events`.
- **Persistence**: per-run evidence directories (`operator.json`, journal, bundle), `.local/inputs/` for the inputs store and `serve.log`, `.local/gemini-campaign.json` for the spend ledger (`specs/002`).
- **Consumers**: Manvi (imports `serve`, `workflow`, `computer`), Vani (inputs profile), the Rust workbench over stdio.

## 2. Constitution Check & Complexity Tracking

Judged against the nine articles of DevCouncil's `constitution.md`. That document is DevCouncil-specific; Jarvis has no repo-local principles document, so no `constitution.md` was created.

| Article | Status | Compliance Details / Justification |
| :--- | :--- | :--- |
| **I: Library-First** | PASSED | The gate and jobs live in `internal/app`; `cmd/jarvis` is a thin shell over them (`App` is built by `cmd/jarvis`, which alone sets the operator). |
| **II: CLI as First-Class Consumer** | PASSED | `jarvis ops`, `jarvis call`, `replay --events` and `discover --events` are the agent surface (`docs/agent-cli.md`). |
| **III: Test-First** | N/A | Not verifiable from the repository; tests exist for each rule but their authoring order was not examined. |
| **IV: Zero Ambient Authority & Fail-Closed** | PASSED | `agentOps` is default-deny; an unclassified op is refused and fails `TestEveryServedJarvisOpIsClassified`; `jarvis.inputs.get` is refused in every mode. |
| **V: Single Source of Truth** | PASSED | Classification lives once in `agentOps`; the README table is checked against it by `TestREADMEOpsTableMatchesAgentOps`. |
| **VI: Memory & Allocator Symmetry** | N/A | Go host code; the Rust allocator rules belong to the native crates. |
| **VII: Simplicity Limit (<= 3 modules)** | N/A | Jarvis is a product repo with several binaries (`cmd/jarvis`, `cmd/qualify`, `cmd/dev`) and Rust crates, not a library. |
| **VIII: Anti-Abstraction** | N/A | Not assessed. |
| **IX: Observability & Forensic Auditability** | PASSED | Agent-operated runs carry `operator.json` and journal actor `agent`; inputs access writes `.local/inputs/serve.log`. |

| Deviation / Seam | Why Needed Now | Simpler Alternative Rejected | Closure / Guard |
| --- | --- | --- | --- |
| Three op classes plus `refused` for Manvi built-ins | A run lives in the memory of the process that started it, so a one-shot `call` cannot address it | One flat allow-list would let an agent act on a run it cannot see | `E_PROCESS_SCOPED` points the agent at `--events` |
| Yolo mode admits some human-only ops | Lets an agent operate end to end | Refusing everything makes unattended agent use impossible | Agent attribution plus `cmd/qualify` refusal keeps such runs out of human-approval evidence |

## 3. Verified File Map

- `internal/app/agent.go`, `internal/app/module.go`, `internal/app/jobs.go`, `internal/app/operator.go`, `internal/app/inputs.go`, `internal/app/profile.go`, `internal/app/catalog_gate.go`
- `cmd/jarvis/main.go`, `cmd/jarvis/agent.go`, `cmd/jarvis/watch.go`
- `docs/agent-cli.md`, `docs/host-jobs.md`
- `profiles/vani.json`, `policies/vani.json`, `profiles/north.json`, `profiles/south.json`
