# Data Model: 001 - Agent Operation Gate, Host Module, and Background Jobs

**Date**: 2026-11-28 | **Source Verification**: `internal/app/agent.go`, `internal/app/operator.go`, `internal/app/jobs.go`, `docs/host-jobs.md`.

## 1. Operation Classification (`internal/app/agent.go`)

| Entity | Fields | Invariants |
| --- | --- | --- |
| `opPolicy` | `class` (`OpAgent` / `OpHumanOnly` / `OpProcessScoped`), `inline` (params may be inline JSON), `paths` (param names confined to the product root), `async`, `yolo` (admitted in yolo mode), `live` (needs a live run or credential) | An op absent from `agentOps` is refused. `jarvis.inputs.get` is human-only with `yolo` false. |
| Error codes | `E_AGENT_REFUSED`, `E_PROCESS_SCOPED` | Refusal has no side effect. |
| `humanControls` | `approve`, `resume` | Only a person may send them; `deny`, `cancel`, `pause`, `refresh`, `focus` may only stop or re-observe. |

## 2. Operator Record (`internal/app/operator.go`)

| Entity | Fields | Invariants |
| --- | --- | --- |
| `operatorRecord` (`operator.json`) | `schema_version`, `operator`, `mode`, `human_qualified` | Written before the run accepts its first control; the authority on who operated the run, because `bundle.json` still files agent acts under `human_actions`. |

## 3. Background Jobs (`internal/app/jobs.go`)

| Entity | Fields / Bounds | Invariants |
| --- | --- | --- |
| `JobView` | `job_id`, `operation`, `status` (`pending` / `running` / `succeeded` / `failed` / `cancelled`), `result`, `error` | Only `succeeded` carries `result`; failed or cancelled carry `error`. At most 16 jobs retained; results `<= 16 MiB`, total `<= 64 MiB`. |

## 4. Run Exit Codes (`docs/agent-cli.md`)

| Code | Meaning |
| --- | --- |
| 0 | ok |
| 1 | operation or run failed, or verification did not pass |
| 2 | usage error |
| 3 | refused by the agent gate |
| 4 | a run reached an unanswered human gate under `--events` and was cancelled |
| 5 | `--timeout` passed |
