# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- MIT License (`LICENSE`) added to repository (`e0fb461`).

### Changed

- Go toolchain moves from 1.26.6 to 1.27.3 (`go.mod`, the workspace file `dev bootstrap` writes, and the README prerequisites). No upstream pin moves: Manvi's `go` line is 1.26.6 and gusset's is 1.26, both below 1.27.3. There is no CI, container image or linter configuration in this repository to move. The owner approved the Go 1.27 move, and the `encoding/json/v2` conversion of the journal encoders, for this run on 2026-11-09. The journal encoders call v2 with the v1 option set so their bytes do not change (`TestJournalEncodingIsStable`, `TestJournalEncodersMatchV1`). The evidence-flush batching design is in `docs/evidence-journal-flush-batching.md`; no sync has changed.
- `upstream.lock.json` pins DevCouncil `1f6d5452` and Manvi `2e2f425a`, which carry dc-store schema 11 / workbench schema 15 (CI results arrive as `run_ci_*` attention rows; a workbench 14 host refuses an upgraded profile with `schema_unsupported`). Manvi `2e2f425a` pins the same DevCouncil commit, and gusset stays at `e561435a`. `go.mod` still requires Manvi `ddec7b0c907a`, so `dev check-pins` refuses the lock until that requirement moves. Neither commit is on GitHub yet, and a pseudo-version naming `2e2f425a` breaks even the `go.work` build, which resolves it from the proxy. Once Manvi is pushed, run `GOWORK=off go get github.com/bharathvbcr/Manvi/manvi@2e2f425adf33`.
- Upstream pins updated to DevCouncil `782b9f16`, Manvi `ddec7b0c` (`v0.0.0-20261029005942-ddec7b0c907a`), and gusset `e561435a` (`0e1bbde`). Earlier in the cycle, pins advanced through DevCouncil `b1a29105`, Manvi `e60b6e01`, and gusset `320c56c2` (`ad9c335`), incorporating DevCouncil dc-store schema 11 / workbench schema 14, tokio 1.53.3/1.54.1, and gusset Rust layout checks with named refusal for unsupported targets.
- Added root module `go.sum` entry for Manvi `e60b6e01` directly from GitHub (`GOPROXY=direct`, `31f8992`).
- Configured `.claude/settings.json` and `.gitignore` to prevent Claude Code commit and PR attribution in cloud sessions (`2c736fe`).
- Updated `.gitignore` to exclude build directories, compiled binaries, build and wave reports, GitPulse tasks and state, agent handoffs, secrets/credentials, and coverage/profiling caches.

### Fixed

- `dev test` builds DevCouncil's Go host (`./cmd/devcouncil`) from the pinned checkout into a temporary directory outside the checkout under `GOWORK=off` and the gusset cgo environment, passing it as `DEVCOUNCIL_BIN` to `dc-verify`'s `json_contract.rs` test (`db5964d`). This resolves clean bootstrap test failures where `DEVCOUNCIL_BIN` was absent.
- `jarvis.key.set` calls `secret.Resolve()` immediately after `NewSecret`, failing fast on unsealable keys (`credentials.ErrLockedMemory`) with an explicit error rather than returning `configured: true` and failing during later discovery or assist runs (`41558b5`).

### Documentation

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
