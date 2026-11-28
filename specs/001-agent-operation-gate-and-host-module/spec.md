# Feature: 001 - Agent Operation Gate, Host Module, and Background Jobs

**Status**: Active Specification | **Canonical Owner**: Jarvis (`github.com/bharathvbcr/Jarvis`) | **Date**: 2026-11-28 | **Implementation State**: Shipped (`internal/app/agent.go`, `internal/app/module.go`, `internal/app/jobs.go`, `internal/app/inputs.go`, `internal/app/operator.go`, `cmd/jarvis/`); one documented gap is open (T-009).

## 1. Problem Statement & Motivation

Jarvis is the computer-use arm that Manvi-based agents call (`README.md`, "What Jarvis is"). An agent reaches it by the `jarvis` CLI or by the same operations on the Manvi host plane. An agent must be able to read, compile and diagnose, but it must never approve an account-changing action, take over a run, read a stored secret or promote a capability by default. Without a binding specification, a newly added `jarvis.*` operation could land unclassified and be reachable by an agent, a "yolo" agent run could be counted as a human approval, or a long diagnostic could block the host's serial request loop.

## 2. Canonical Ownership & Non-Overlap Boundary

| Layer / Capability | Canonical Owner | Jarvis Role | Explicit Non-Goals / Delegations |
| --- | --- | --- | --- |
| **Host plane transport (`serve.Server`, `Router`, `Module`)** | **Manvi** (`manvi/serve/`, `specs/001` in the Manvi repository) | Registers `jarvis.*` ops as a `serve.Module` in `App.Configure` (`internal/app/module.go`). | Does not reimplement the NDJSON protocol or the built-in `policy.*` and `chat.*` ops; those are `refused` in `jarvis ops`. |
| **Operation classification and the agent gate** | **Jarvis** (`internal/app/agent.go`, `cmd/jarvis/agent.go`) | Owns the default-deny `agentOps` table and the `E_AGENT_REFUSED` / `E_PROCESS_SCOPED` refusals. | Manvi owns the approval and takeover mechanics themselves (`manvi/computer`). |
| **Agent attribution (yolo mode)** | **Jarvis** (`internal/app/operator.go`) | Marks agent-operated runs with `operator.json`, journal actor `agent`, and `"operator":"agent"` in the run view; such runs never count as human approval. | Does not fabricate a human grant. `cmd/qualify` refuses agent-operated runs. |
| **Inputs catalog for non-desktop profiles (Vani)** | **Jarvis** (`internal/app/inputs.go`, `internal/app/profile.go`, `profiles/vani.json`, `policies/vani.json`) | Serves `jarvis.inputs.{put,list-keys,get}` for `kind: "inputs"` profiles, which the desktop loader refuses. | Vani itself is a separate consumer. |
| **Background jobs** | **Jarvis** (`internal/app/jobs.go`, `docs/host-jobs.md`) | Admits bounded async jobs so the serial stdio host stays responsive. | Rust workbench rendering of job results is `desktop/crates/workbench`. |
| **Evidence acceptance** | **DevCouncil** (`rust/dc-evidence`, `dcverify`) | Jarvis only calls it (`specs/002`). | Jarvis never decides acceptance itself. |

## 3. User Stories & Acceptance Scenarios

### US1: Every Served Operation Is Classified, Default-Deny (Priority: P1)

As an agent operator, an agent can call only operations that were deliberately classified for it, and an operation nobody classified is refused.

- **Scenario 1: Unclassified operation is refused**
  - **Given** a `jarvis.*` operation registered by `App.Configure` with no entry in `agentOps` (`internal/app/agent.go`)
  - **When** an agent calls it through `jarvis call`
  - **Then** the call is refused, and `TestEveryServedJarvisOpIsClassified` fails the build until the operation is classified.
- **Scenario 2: Human-only and process-scoped operations are refused to a one-shot call**
  - **Given** an agent running `jarvis call jarvis.run.act` (human-only) or `jarvis call jarvis.run.get` (process-scoped)
  - **When** the gate evaluates the call without yolo mode
  - **Then** the human-only call returns `E_AGENT_REFUSED` and the process-scoped call returns `E_PROCESS_SCOPED`, with no side effect.
- **Scenario 3: The README op table matches the code**
  - **Given** the table of operations in `README.md`
  - **When** `TestREADMEOpsTableMatchesAgentOps` runs
  - **Then** it fails if the table and `agentOps` disagree.

### US2: Yolo Mode Lets an Agent Operate End to End Without Faking Human Approval (Priority: P1)

As an agent operator who opts in with `--yolo` or `JARVIS_AGENT_MODE=yolo`, the agent can answer gates, act, take over, promote and store inputs, and every such run is recorded as the agent's.

- **Scenario 1: Agent runs are attributed**
  - **Given** an `App` created with `OperatorAgent` (only `cmd/jarvis` sets it; the stdio host the workbench spawns never does)
  - **When** a run starts
  - **Then** `operator.json` is written to the run's evidence directory before the run accepts its first control, journal records carry actor `agent`, and `HumanQualified` is false.
- **Scenario 2: Secrets are never disclosed, even in yolo mode**
  - **Given** yolo mode and `jarvis.inputs.get`
  - **When** an agent calls it
  - **Then** it is refused in every mode, because it would print a stored secret into the transcript; and `approve` and `resume` run controls remain human-only (`humanControls` in `internal/app/agent.go`).
- **Scenario 3: Unknown agent mode is refused**
  - **Given** `JARVIS_AGENT_MODE` set to a value other than the recognized ones
  - **When** `cmd/jarvis` resolves the mode
  - **Then** `ParseAgentMode` returns an error rather than defaulting to yolo (`TestParseAgentModeRefusesUnknownValues`).

### US3: Long Operations Run as Bounded Background Jobs (Priority: P1)

As the workbench or an agent, a slow diagnostic or export does not block status and control requests on the serial stdio host.

- **Scenario 1: Admission is not success**
  - **Given** `jarvis.doctor`, `jarvis.evidence.verify`, `jarvis.frame.get`, `jarvis.evidence.export` or `jarvis.anchor.create`
  - **When** the host admits one
  - **Then** the first response carries `job_id`, the original `operation` and `status: "pending"`, and only `jarvis.job.get` returning `succeeded` carries `result`.
- **Scenario 2: Retention and deadlines are bounded**
  - **Given** repeated timeouts and many completed jobs
  - **When** `jobRegistry` admits more work (`internal/app/jobs.go`)
  - **Then** at most 16 jobs are retained, completed jobs expire after five minutes, each result is bounded to 16 MiB of JSON, and a timed-out worker keeps its admission slot until it returns (`TestJobsBoundPhysicalWorkersAfterDeadline`).

### US4: Inputs Profiles Serve Contact Fields Without Granting Desktop Access (Priority: P2)

As the Vani consumer, I can store and list profile contact fields through `jarvis.inputs.*` while the desktop loader refuses my profile.

- **Scenario 1: Inputs profile is not a desktop profile**
  - **Given** `profiles/vani.json` with `kind: "inputs"`
  - **When** the desktop loader runs (`RequireDesktopProfile` in `internal/app/profile.go`)
  - **Then** it refuses the profile, while `jarvis.inputs.put` and `jarvis.inputs.list-keys` accept it under `policies/vani.json`.

## 4. Success Metrics & Testability Criteria

- **SC-001**: `CGO_ENABLED=0 go -C /Users/bharath/Code/devtools/Jarvis test -count=1 ./internal/app/ ./cmd/jarvis/` passes (run 2026-11-28 as part of `./internal/... ./cmd/...`: `internal/app` ok in 7.7s, `cmd/jarvis` ok in 1.1s).
- **SC-002**: `TestEveryServedJarvisOpIsClassified` and `TestREADMEOpsTableMatchesAgentOps` in `internal/app/agent_test.go` pass.
- **SC-003**: `internal/app/yolo_test.go` passes (`TestYoloAdmitsDiskBackedHumanOps`, `TestYoloKeepsLiveOpsDisclosureAndConfinement`, `TestOperatorSidecarMarksOnlyAgentRuns`).
- **SC-004**: `internal/app/jobs_test.go` passes the physical-worker and result-ownership tests.
- **SC-005**: `devcouncil spec validate --all --strict` passes from the repository root.

## 5. Edge Cases, Exclusions & Open Questions

- **Exclusions**:
  - Jarvis does not load into a Manvi agent at runtime today: `README.md` records an open gap that nothing shows an agent loading Jarvis as a module, and Manvi only build-checks Jarvis through `scripts/check-consumers.sh`. This spec tracks it as T-009 and does not claim it.
  - The Rust workbench (`desktop/crates/workbench`) and the native bank (`desktop/crates/bank-demo`) are not specified here beyond their host-plane contract.
- **Edge Cases**:
  - `jarvis.inputs.list-keys` appends an audit line to `.local/inputs/serve.log` and `jarvis.capability.compile` with `"register":true` adds a draft catalog revision, so `agent` class is not strictly read-only (`docs/agent-cli.md`).
  - `jarvis call --timeout` must be at most 10 minutes.
- **Open Questions**: None; the one known gap is tracked as T-009.
