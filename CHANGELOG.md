# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-10-17

First version cut for publish. No tag has been published. The desktop
workspace version moves from 0.1.0 to 0.2.0. Notes below are the seven
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
