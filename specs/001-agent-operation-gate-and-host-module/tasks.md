# Tasks: 001 - Agent Operation Gate, Host Module, and Background Jobs

**Spec**: `specs/001-agent-operation-gate-and-host-module/spec.md` | **Plan**: `specs/001-agent-operation-gate-and-host-module/plan.md`

## Phase 1: Shipped Gate and Host Module

- [x] [T-001] [US1] Implement the default-deny `agentOps` classification and the `E_AGENT_REFUSED` / `E_PROCESS_SCOPED` refusals in `internal/app/agent.go` tests: go test ./internal/app/ -run 'TestEveryServedJarvisOpIsClassified|TestREADMEOpsTableMatchesAgentOps' -count=1
- [x] [T-002] [US1] Implement `jarvis ops` and `jarvis call` over the host plane in `cmd/jarvis/agent.go` depends_on: [T-001] tests: go test ./cmd/jarvis/ -count=1
- [x] [T-003] [US1] Register every `jarvis.*` op as a Manvi `serve.Module` in `internal/app/module.go` depends_on: [T-001] tests: go test ./internal/app/ -count=1
- [x] [T-004] [US2] [P] Implement yolo mode parsing, agent attribution and the `operator.json` sidecar in `internal/app/operator.go` and `cmd/jarvis/agent.go` depends_on: [T-001] tests: go test ./internal/app/ -run 'Yolo|Operator|AgentSinks|AttributeRecord' -count=1
- [x] [T-005] [US2] [P] Implement the unattended-approval and agent-promotion gate in `internal/app/catalog_gate.go` depends_on: [T-001] tests: go test ./internal/app/ -run 'AgentPromotion' -count=1
- [x] [T-006] [US2] Stream NDJSON events and cancel at the first human gate (exit 4) in `cmd/jarvis/watch.go` depends_on: [T-002] tests: go test ./cmd/jarvis/ -count=1

## Phase 2: Background Jobs and Inputs

- [x] [T-007] [US3] Implement the bounded background job registry in `internal/app/jobs.go` per `docs/host-jobs.md` tests: go test ./internal/app/ -run 'TestJobs' -count=1
- [x] [T-008] [US4] [P] Implement the inputs catalog and the `kind: "inputs"` profile admission in `internal/app/inputs.go` and `internal/app/profile.go` with `profiles/vani.json` and `policies/vani.json` tests: go test ./internal/app/ -count=1

## Phase 3: Open Work

- [ ] [T-009] [US1] Document, and where needed implement, how a Manvi agent loads Jarvis at runtime; the committed `README.md` records that nothing shows an agent loading Jarvis as a module and that Manvi only build-checks Jarvis; record the result in `docs/agent-cli.md` depends_on: [T-003]

## Phase 4: Specification Gate

- [x] [T-010] [US1] Expose the spec gate as `go run ./cmd/dev check-specs`, running `devcouncil spec validate --all --strict`, in `cmd/dev/main.go` tests: go test ./cmd/dev/ -count=1
