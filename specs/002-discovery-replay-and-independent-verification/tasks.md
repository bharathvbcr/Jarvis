# Tasks: 002 - Discovery, Deterministic Replay, Independent Verification, and Pinned Upstreams

**Spec**: `specs/002-discovery-replay-and-independent-verification/spec.md` | **Plan**: `specs/002-discovery-replay-and-independent-verification/plan.md`

## Phase 1: Shipped Discovery and Replay

- [x] [T-001] [US1] Implement scoped discovery with durable action admission and masked frames in `internal/app/discovery.go` and `internal/app/discovery_journal.go` tests: go test ./internal/app/ -run 'Discovery' -count=1
- [x] [T-002] [US1] Implement the shared campaign ledger and meterable Gemini adapter in `internal/app/ledger.go` tests: go test ./internal/app/ -run 'CampaignLedger|CampaignGemini' -count=1
- [x] [T-003] [US1] [P] Implement the fixture discovery provider and published-capability assembly in `internal/app/discovery_provider.go` and `internal/app/discovery_publish.go` depends_on: [T-001] tests: go test ./internal/app/ -run 'DiscoveryFixture|AssemblePublished' -count=1
- [x] [T-004] [US2] [P] Implement offline trace replay in `internal/trace/replay.go` tests: go test ./internal/trace/ -count=1
- [x] [T-005] [US2] [P] Implement Go caller generation delegating to the canonical executor in `internal/codegen/codegen.go` tests: go test ./internal/codegen/ -count=1
- [x] [T-006] [US2] Implement tenant overlay compilation and drift reports in `internal/app/profile.go` and `internal/app/drift.go` with `overlays/south-balance.json` tests: go test ./internal/app/ -run 'Overlay|Drift|Profile' -count=1

## Phase 2: Independent Verification and Qualification

- [x] [T-007] [US3] Implement fail-closed verifier execution and held-out lock comparison in `internal/evidence/evidence.go` and `internal/evidence/formal_comparator.go` tests: go test ./internal/evidence/ -count=1
- [x] [T-008] [US5] Implement the N-run qualification campaign, state oracle and interactive approval in `cmd/qualify/main.go`, `cmd/qualify/oracle.go` and `cmd/qualify/interactive.go` tests: go test ./cmd/qualify/ -count=1
- [x] [T-009] [US4] Implement `dev bootstrap`, `dev check-pins` and the upstream pin agreement checks in `cmd/dev/main.go` and `cmd/dev/package.go` with `upstream.lock.json` tests: go test ./cmd/dev/ -run 'CheckPins|Bootstrap' -count=1

## Phase 3: Open Qualification Work

- [ ] [T-010] [US1] Run a credentialed live Gemini discovery against the synthetic bank and retain the journal showing scoped screenshot, model tool decision, admitted native execution and fresh screenshot, following `docs/provider-qualification.md`, as a new report under `evidence/` depends_on: [T-001, T-002]
- [ ] [T-011] [US5] Qualify human-approved positive account creation, completed takeover and resume, and live visual input with `cmd/qualify --interactive`, and record the reports under `evidence/` depends_on: [T-008]
- [ ] [T-012] [US5] Reach a clean N-run stability pass on current source on macOS and Linux and record it beside `evidence/macos-guard-partition-40.json` in `docs/requirements-evidence.md` depends_on: [T-008]

## Phase 4: Specification Gate

- [x] [T-013] [US4] Expose the spec gate as `go run ./cmd/dev check-specs` in `cmd/dev/main.go`, kept out of the CI `test` plan tests: go test ./cmd/dev/ -count=1
