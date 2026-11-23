# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Agent CLI (`docs/agent-cli.md`):
  - `jarvis ops` lists the served host-plane operations with their agent class.
  - `jarvis call OP [--params JSON|@FILE|-]` runs one operation per process through the same `serve` server and handlers, and writes one envelope line; `jarvis.doctor` is polled to its result.
  - `replay`/`discover --events` stream NDJSON run events and cancel at the first human gate (exit 4).
  - Exit codes: 2 usage, 3 refused, 4 needs a human, 5 timeout.
  - A default-deny table (`internal/app/agent.go`) classifies every `jarvis.*` op and refuses agents approval (`run.control` approve/resume), act, takeover, handback, assist, promotion, `key.set`, `inputs.get`/`put` and export.
  - Path params must resolve inside the root and outside `.local/`.
- Agent yolo mode, opt-in by owner direction (`--yolo` or `JARVIS_AGENT_MODE=yolo`; `docs/agent-cli.md#yolo-mode`):
  - `replay`/`discover --events --yolo` let an agent answer gates with strict NDJSON controls on stdin. Approve and deny must name the pending gate's epoch, action and observation. EOF or `--gate-timeout` at a gate cancels the run (exit 4).
  - `call --yolo` admits `catalog.promote` and `inputs.put`. `inputs.get` and ops that need a live run stay refused.
  - Agent-operated runs get an `operator.json` sidecar before the first control, journal actor `agent` in place of `human` and in place of `automation` for approve and deny, `RunView.operator`, and promotion `promoted_by`. `cmd/qualify` refuses them.
  - `--export FILE.zip` on replay and discover writes the finished run's evidence.
- `dev test` overlaps independent legs: one Go step and one Cargo step run at a time (`cmd/dev/steps.go`), a step waits only for the steps it names in `needs` (dc-verify waits for the DevCouncil host build), the first failure cancels the rest, output is prefixed per step, and a per-step wall-time table is printed. `--serial` keeps plan order, one step at a time. `qualify --check-boundaries` is now a plan step instead of a tail command. Test steps no longer inherit stdin.
- Held-out verification lock comparator and safety-first Pareto frontier selection (`internal/evidence/formal_comparator.go`: `VerifyAgainstHeldOutLock`, `StrictlyDominates`, `SelectAdmissibleFrontier`), exposed via `jarvis verify --verifier-sha256`, `--policy-sha256`, and `--artifacts-root` executing out-of-band before broker/bank initialization.
- Profile-driven sensitive parameter catalogs and non-bank `kind: "inputs"` support (`profiles/vani.json`, `policies/vani.json`, `internal/app/inputs.go`), exposing `jarvis.inputs.put`, `jarvis.inputs.list-keys`, and `jarvis.inputs.get` over stdio while refusing desktop admission via `RequireDesktopProfile` and scoping bank `Confirm creation` approval rules to `BankApplication`.
- Asynchronous visual anchor construction operation (`jarvis.anchor.create`, 15s deadline) and synthetic bank `--fault` modes `false-ack`, `permission-denied`, `session-expired` plus `--variant renamed-controls` (`south` tenant).
- MIT License (`LICENSE`) added to repository (`e0fb461`).

### Changed

- Security: `jarvis replay` and `jarvis discover` accept `approve`, `resume`, `assist`, `takeover`, `handback` and `act` only when stdin is a terminal. Piped stdin keeps `deny`, `cancel`, `pause`, `refresh` and `focus`. Before this change, a piped `approve` could answer an account-changing approval. The interactive run loop moved into `cmd/jarvis/watch.go` with its behaviour otherwise unchanged.
- `upstream.lock.json` pins Manvi `66d17fa7` (a fast-forward of `5e32e6ad`, 79 commits) and DevCouncil `d938f7f8` (a fast-forward of `230bbf04`, 37 commits, DevCouncil's `origin/main`). gusset stays at `19383ad1`. Manvi `66d17fa7`'s `scripts/module-pins.txt` names the same DevCouncil and gusset commits, so Manvi and Jarvis build against one DevCouncil revision. No schema constant changes value in `230bbf04..d938f7f8`: workbench schema stays 17 and dc-store schema stays 11. In the Manvi packages Jarvis imports, the changes add: the native DevCouncil tools (`devcouncil_batch`, `devcouncil_code`, `devcouncil_script`) with a read-before-overwrite guard in `tools`; batch and script turns in `agent`; an end-of-turn verification verdict that the exit status reports; and Anthropic hosted web search, off unless `SetWebSearch`, with search pricing in `llm/budget`. `work.assist.complete` answers a rejected Anthropic key with `E_PROVIDER_AUTH`. `go.mod` requires `v0.0.0-20261123172547-66d17fa7a46b`, and the two `go.sum` lines equal what `go mod download` reports for it from GitHub (`GOPROXY=direct`, fresh module cache) (owner-approved bump, 2026-11-23).
- `upstream.lock.json` pins Manvi `5e32e6ad`, a fast-forward of `524a7dad` (11 commits). DevCouncil `230bbf04` and gusset `19383ad1` are unchanged (Manvi's `scripts/module-pins.txt` is unchanged). What Jarvis calls changed in one place: `computer.AttachFocusedReady` lets an attempt in flight finish when its ten-second window closes (perf1109 H2/M1/M2), bounded at 25 seconds; Jarvis no longer wraps it in its own 10-second deadline. The rest is Manvi's `--approve-fd` pre-scan, CI and test-gate changes, bench and docs. Manvi CI at the pinned commit: branch run 44615050912, all eight gate jobs plus `--race` green; windows-latest red (the DevCouncil testsupport `.exe` defect, DevCouncil card ft-8424979f). `go.mod` requires `v0.0.0-20261119210359-5e32e6ad9184` (2026-11-19).
- `upstream.lock.json` pins Manvi `524a7dad`, a fast-forward of `8a7bada4` (68 commits). DevCouncil stays at `230bbf04` and gusset at `19383ad1`: Manvi `524a7dad`'s `scripts/module-pins.txt` is unchanged, so workbench and dc-store schemas are unchanged too. Manvi brings `verify.sh` building and vetting Jarvis against every Manvi checkout (`scripts/check-consumers.sh`), one additive serve op (`work.assist.complete`; `contracts/serve.ops.schema.json` +77/−0), asymmetric checker/worker endpoints, gitpulse and devmap MCP servers for managed coding-agent sessions, and `golang.org/x/sys` 0.50.0, which `go get` raised here as well. `go.mod` requires `v0.0.0-20261119165510-524a7dad9b46`, and `go.sum` gains the hash lines `go get` recorded (owner-approved bump, 2026-11-19).
- `upstream.lock.json` pins Manvi `8a7bada4` and DevCouncil `230bbf04`. Both are fast-forwards of the previous pins, and workbench schema stays 17 and dc-store schema stays 11. Manvi brings the w21 campaign: refguard refuses the admin-directory ref shapes its snapshot does not read, grep stamps every directory under the search path, rustix is locked at 1.1.6 for RUSTSEC-2026-0486, and the store guard's `ps` listing runs without provider keys in its environment and under a timeout. DevCouncil brings only rustfmt, dc-store clippy and dc-proc test-race fixes under Rust 1.100.0. gusset stays at `19383ad1`. Manvi `8a7bada4`'s `scripts/module-pins.txt` names the same DevCouncil and gusset commits. `go.mod` requires `v0.0.0-20261115001930-8a7bada404db`, and `go.sum` gains the hash lines that `go get` recorded for it (owner-approved bump, 2026-11-14).
- `upstream.lock.json` pins Manvi `44cf392f` and DevCouncil `d9c7a512`, which carry workbench schema 17. Schema 17 adds a cross-repository execution queue, GitHub issue sync state, a confirmed outbox and `items.readiness`. dc-store schema stays 11, and a workbench 16 host refuses an upgraded profile. The DevCouncil pin also brings dc-redact's bare Google OAuth shapes (`ya29.`, `1//`, `GOCSPX-`). gusset stays at `19383ad1`. Manvi `44cf392f`'s `scripts/module-pins.txt` names the same DevCouncil and gusset commits, and `go.mod` requires `v0.0.0-20261114185916-44cf392fa6f6` (owner-approved bump, 2026-11-14).
- `upstream.lock.json` pins Manvi `5f2eb796` and DevCouncil `c6788883`, which carry workbench schema 16 (the task board's `inbox` status is merged into `backlog`; dc-store schema stays 11; a workbench 15 host refuses an upgraded profile with `schema_unsupported`). gusset stays at `19383ad1`. Manvi `5f2eb796`'s `scripts/module-pins.txt` names the same DevCouncil and gusset commits, and `go.mod` requires `v0.0.0-20261113161946-5f2eb7966187`, so `dev check-pins` passes (owner-approved bump, 2026-11-13).
- Go 1.27.4 (owner decision 2026-11-12): `go.mod`, the `go.work` that `cmd/dev bootstrap` writes and the README prerequisites move from Go 1.26.6 to 1.27.4, the latest stable release. DevCouncil from `c8e526ab` on declares `go 1.27.4` in `backend/go_orchestrator`, which the Go host builds through Manvi's replace, so Go 1.26.x can no longer build it.
- Rust 1.100 migration (owner decision 2026-11-12): the `desktop` workspace declares `rust-version = "1.100"` (was 1.95) and gains `desktop/.cargo/config.toml` with `[registry] global-min-publish-age = "3 days"`.
- `upstream.lock.json` pins Manvi `c0944c06` (Rust 1.100, Go 1.27.4), DevCouncil `9658b687` and gusset `19383ad1`, all Rust 1.100 (owner-approved bump, 2026-11-12). Manvi `c0944c06`'s `scripts/module-pins.txt` names the same DevCouncil and gusset commits, and `go.mod` requires `v0.0.0-20261113155013-c0944c06f110`, so `dev check-pins` passes. Manvi's requirement also moves `golang.org/x/sys` 0.47.0 to 0.49.0 (indirect). dc-store schema 11 and workbench schema 15 are unchanged at the new DevCouncil pin. gusset `19383ad1` is on GitHub as branch `rust-1.100` (PR #65), because gusset's main only takes pull requests.
- The evidence journal encoders call `encoding/json/v2` with the v1 option set, so their bytes do not change (`TestJournalEncodingIsStable`, `TestJournalEncodersMatchV1`); the owner approved that conversion for the 2026-11-09 performance run. Run views and records are copied without a JSON round trip, and polling uses a cursor. The evidence-flush batching design is in `docs/evidence-journal-flush-batching.md`; no sync has changed. The Go 1.27.3 toolchain move that run recorded is superseded by the Go 1.27.4 entry above.
- `upstream.lock.json` pins DevCouncil `1f6d5452` and Manvi `2e2f425a`, which carry dc-store schema 11 / workbench schema 15 (CI results arrive as `run_ci_*` attention rows; a workbench 14 host refuses an upgraded profile with `schema_unsupported`). Manvi `2e2f425a` pins the same DevCouncil commit, and gusset stays at `e561435a`. `go.mod` still requires Manvi `ddec7b0c907a`, so `dev check-pins` refuses the lock until that requirement moves. Neither commit is on GitHub yet, and a pseudo-version naming `2e2f425a` breaks even the `go.work` build, which resolves it from the proxy. Once Manvi is pushed, run `GOWORK=off go get github.com/bharathvbcr/Manvi/manvi@2e2f425adf33`.
- Upstream pins updated to DevCouncil `782b9f16`, Manvi `ddec7b0c` (`v0.0.0-20261029005942-ddec7b0c907a`), and gusset `e561435a` (`0e1bbde`). Earlier in the cycle, pins advanced through DevCouncil `b1a29105`, Manvi `e60b6e01`, and gusset `320c56c2` (`ad9c335`), incorporating DevCouncil dc-store schema 11 / workbench schema 14, tokio 1.53.3/1.54.1, and gusset Rust layout checks with named refusal for unsupported targets.
- Added root module `go.sum` entry for Manvi `e60b6e01` directly from GitHub (`GOPROXY=direct`, `31f8992`).
- Configured `.claude/settings.json` and `.gitignore` to prevent Claude Code commit and PR attribution in cloud sessions (`2c736fe`).
- Updated `.gitignore` to exclude build directories, compiled binaries, build and wave reports, GitPulse tasks and state, agent handoffs, secrets/credentials, and coverage/profiling caches.

### Fixed

- `jarvis replay`/`discover` without `--events` cancel at a human gate once stdin reaches EOF, instead of waiting there for the run's own timeout (`cmd/jarvis/watch.go`), and a gate's identity is joined with a separator, so two different gates cannot share a key and leave the second unprompted (`gateKey`).
- The inputs audit line names its actor (`actor=agent` for a yolo-mode operator, `actor=person` otherwise). `cmd/qualify` refuses to start with `JARVIS_AGENT_MODE` set; `docs/agent-cli.md` no longer claims its per-run operator check is what keeps agents out, says what `--export` and the `call` path check do and do not cover.
- `jarvis replay --unattended` refuses a capability revision an agent last promoted in yolo mode (`internal/app/catalog_gate.go`: `refuseAgentPromotion`). The promote handler set the catalog status to approved for an agent as for a person, and only `.local/promotions/<id>.json` recorded who, so an agent-promoted capability (one it registered itself, even) ran unattended in default mode as if a person had approved it. A person promoting the same revision again admits it; an unreadable history refuses. Security impact: narrows what runs unattended; nothing newly runs.
- Jarvis no longer wraps `AttachFocusedReady` in a 10-second deadline of its own (`app.go`, `discovery.go`, `drift.go`). The call bounds itself, and from Manvi with perf1109 H2 fixed it lets an attempt in flight finish; a shorter deadline here cut that attempt off and retired the broker for a slow window.
- `dev bootstrap` moves an existing upstream checkout to its pin instead of skipping it (`cmd/dev/package.go`: `advanceCheckout`). A lock bumped after the first bootstrap used to leave the old revision checked out, and every later `check-pins` refused it: `.local/upstream/Manvi` stayed at 5f2eb796 through two Manvi pin bumps. Only a clean checkout moves, and only forward (its HEAD must be an ancestor of the pin); a dirty checkout, a checkout on a branch, or a pin that does not contain HEAD is refused and left where it is.
- `dev test` builds DevCouncil's Go host (`./cmd/devcouncil`) from the pinned checkout into a temporary directory outside the checkout under `GOWORK=off` and the gusset cgo environment, passing it as `DEVCOUNCIL_BIN` to `dc-verify`'s `json_contract.rs` test (`db5964d`). This resolves clean bootstrap test failures where `DEVCOUNCIL_BIN` was absent.
- `jarvis.key.set` calls `secret.Resolve()` immediately after `NewSecret`, failing fast on unsealable keys (`credentials.ErrLockedMemory`) with an explicit error rather than returning `configured: true` and failing during later discovery or assist runs (`41558b5`).

### Documentation

- Reframed Jarvis as the computer-use agent arm for Manvi and Vani, with more agents to follow (owner direction, 2026-11-22). `README.md` now opens on that role, lists every `jarvis.*` op by class as `agentOps` classifies it, describes the Manvi and Vani consumers from what the code shows, states as an open gap that no documented path lets a Manvi agent load Jarvis at runtime, and points to `upstream.lock.json` for pins. `TestREADMEOpsTableMatchesAgentOps` fails when the README table and `agentOps` disagree.
- Removed assignment and submission framing from `README.md`, `REPORT.md` (now "Jarvis design notes", same path), `docs/IMPLEMENTATION.md`, `docs/requirements-evidence.md`, `docs/provider-qualification.md` and `docs/live-discovery-runbook.md`. Dated evidence and its figures are unchanged; stale pin mentions are marked historical.
- Moved the self-superseded `docs/publish-status.md` to `docs/archive/publish-status.md`.
- Untracked the installed skill copies under `.agents/skills`, `.claude/skills` and `.cursor/skills` and `.devcouncil-skills.json`; `.gitignore` keeps a reinstall (`devmap skills install`, `devcouncil skills scaffold`) untracked.
- Clarified across `README.md`, `REPORT.md`, `docs/platforms.md`, `docs/publish-status.md`, and `docs/requirements-evidence.md` that automated build verification in `evidence/build-verification.json` was recorded against earlier pins (Manvi `4818dc2`, DevCouncil `4ccadc6`) and not re-recorded at current `upstream.lock.json` pins (`067c4a2`).
- Detailed the 35/40 narrowed-guard macOS campaign outcomes in `evidence/macos-guard-partition-40.json`: four human input refusals attributed by category (two `pointer_button`, one `keyboard`, one `keyboard, pointer_button`) and one stopped on an unsatisfied wait predicate (`067c4a2`).

## [0.2.0] - 2026-10-17

First version cut for publish. Tagged as `v0.2.0`. The desktop
workspace version moves from 0.1.0 to 0.2.0. Notes below are the
commits on `main` since `origin/main` (`d475cf5`) and the earlier behavior
on `main` that those commits sit on.

### Added

- Gusset policy engine through Manvi (`fad29d1`). `jarvis gusset-check` runs
  Manvi's `gussetcheck.SelfTest` (CPython fnmatch parity, batched match-any,
  and a Rust panic caught at the boundary) before the app starts, and exits
  non-zero when the engine is not linked. `doctor` reports `gusset` as ok,
  not_linked, or failed. `dev gusset` is the cgo build-and-test leg.
  `upstream.lock.json` records a gusset pin.
- On Linux and macOS, `dev build` links that engine into `build/jarvis` and
  runs `gusset-check` before the build counts (`d6da1e3`). A linked build
  decides policy pattern questions through the engine and denies when the
  engine cannot answer (`99ee758`). Windows stays cgo-off; `--engine=false`
  opts out.
- DevCouncil evidence client and catalog Go codegen live in this repo as
  `internal/evidence` and `internal/codegen` (`7723174`). `FindByDigest`
  scans `catalog.Store.List`.
- DevMap skills, including devmap-debugging, devmap-exploring,
  devmap-impact, and devmap-refactoring, with an installation receipt in
  `.devcouncil-skills.json` (`d475cf5`).

### Changed

- Upstream pins are DevCouncil `1ae3bc15`, Manvi `5dbf47c7`, and gusset
  `984209b5`, in agreement with Manvi's and DevCouncil's own pins
  (`c0b6646`).
- The campaign ledger follows Manvi's lifecycle API (`7723174`, `33c83fd`).
  Gemini is metered through a pricer because `budget.Provider` takes prices
  from the provider. `ledger.Close` is gone. `Snapshot` returns an error
  that fails the run. `openCampaignLedger` takes the caller's context, so a
  cancelled discover or assist run stops before the shared ledger opens.
  The end-of-run evidence snapshot uses `context.WithoutCancel`, so a
  cancelled run still records what it spent, and `Snapshot` bounds its own
  lock wait.
- `doctor` calls `gussetcheck.SelfTest` under a 5 second bound (`7723174`,
  `eff361a`). `gussetcheck.Run` was retired as a subset of `SelfTest`.
- `jarvis serve` and `jarvis gusset-check` release the engine with
  `gussetcheck.Shutdown` (2 seconds) and drain the engine log ring to
  stderr (`fbc216d`, `ce514cb`).
- `check-pins` refuses a lock whose Manvi module pins or DevCouncil action
  ref name a different DevCouncil or gusset revision than
  `upstream.lock.json` (`2edbb04`).
- DevCouncil skill front matter has titles and trigger keywords or globs,
  the stack skills point at the language-policy skill, and
  `.devcouncil-skills.json` records the hash of every installed skill file
  (69 files, all matching the tree) (`bd16702`).
- Agent docs point the repo map and code graph at `.devmap/`, and describe
  GitPulse Insights pre-flight checks, collision risk, DevMap invocation,
  and repository hygiene (`74c0cde`).

### Fixed

- Provider credentials are withheld from the two Jarvis child processes that
  previously inherited the whole environment (`1ff4705`). `dcverify`
  (`internal/evidence`.Verify) and `jarvis-bank` (`startOwnedBank`) set
  `cmd.Env` to `childenv.Inherit()`: `os.Environ()` minus
  `credentials.ProviderEnv`, never nil. That list includes `GEMINI_API_KEY`,
  which `jarvis discover` reads from the environment. The dev and qualify
  tooling spawn sites are unchanged.
- Shutdown closes every run, and a close during admission cannot start
  work (`65d2a89`). Visual policy equality includes search-region and
  click-point geometry, so a larger search region or a different click
  point does not compare equal by digest alone.
