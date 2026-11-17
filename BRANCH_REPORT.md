# BRANCH_REPORT — branch consolidation (2026-11-15)

## Update 2026-11-17 (later) — the last branch, `claude/pin-manvi-8a7bada-c8f201`, deleted on the owner's instruction

This supersedes the "Kept" sentence in the section below. Main (`eeb4241`) is now the only branch and the only worktree [V].

- **Checks immediately before deleting [V]:** tip `bdd9bec` is an ancestor of main; backup ref `refs/backup/2026-11-15/claude/pin-manvi-8a7bada-c8f201` is at that exact commit; the worktree had no tracked or untracked changes. Its ignored content was all replaceable: `.claude/skills/*` (the same auto-installed set as the primary checkout), `build/` (build output) and the generated `go.work`.
- **Done with** `git worktree remove` (no `--force`) and `git branch -d`; `git fsck` is clean afterwards.
- **A live session was attached and was not closed.** `claude` pid 32443 (up 2 days 11 hours, last activity about two days old, never answered the wind-down message) still had its working directory in that worktree. It was not killed, because it was not started by the session that deleted the branch; the owner asked for the cleanup knowing the session was still there. Its working directory no longer exists, so that session's shell tools will fail until it is closed. No work was lost: everything it had committed is on main and its worktree was clean.
- **Restore** the branch with `git branch claude/pin-manvi-8a7bada-c8f201 refs/backup/2026-11-15/claude/pin-manvi-8a7bada-c8f201`; a worktree has to be re-created with `git worktree add`.
- **The pin bump itself is still not fully verified:** `ft-0994172b` on the board tracks the consumer checks (`manvi doctor`, `devmap --version`, GitPulse `check:schema-parity`). Its "close out the branch" part is now done.

## Update 2026-11-17 — wind-down, four branches merged, merged branches pruned

Evidence labels as below: **[V]** verified by a command in this session, **[P]** peer-reported (session a7e8b48b) and re-checked only where stated, **[I]** inferred.

**Merged into main** (one fast-forward from `integrate/1117`; every branch had a backup ref `refs/backup/2026-11-17/<branch>` first):

| Branch | What | Review |
|---|---|---|
| `fix/held-out-lock-criticals` | the verifier that runs is a private copy of the bytes that were hashed; a held-out pass takes `required` from the contract, not the report, and rejects repeated or undeclared criterion ids | two Opus reviews of the original code (peer) and an independent Opus review of the fix: safe to merge [V for mine, P for the first two] |
| `fix/inputs-log-redaction` | a stored input value never enters the serve log line | same |
| `fix/discovery-log-redaction` | the discovery log scrub masked watched values one at a time in map order and left fragments of overlapping ones; matches are now merged before replacing. Reproduced first: 3 cases and the order property failed on the old algorithm [V] | independent Opus review: safe to merge [V] |
| `perf/dev-test-lanes` | `dev test` overlaps one Go and one Cargo step, cancels whole process groups, bounds the fixture build by the package timeout; a never-run CI workflow (`.github/workflows/ci.yml`) | independent Opus review: no blockers, 10 non-critical findings, filed on the board [V] |

**Verification** [V]: on the integrated tree `gofmt`, `go vet ./...`, `go test` for `internal/*`, `cmd/jarvis`, `cmd/qualify`, `tests/*`, and `cmd/dev` under `-race` (cargo fixture test skipped) all pass. The real gate (`dev bootstrap`, which ends in `check-pins`, then `dev test`) ran in a fresh worktree against fresh clones at the lock's pins (Manvi `8a7bada4`, DevCouncil `230bbf04`, gusset `19383ad1`): bootstrap and `check-pins` passed; every plan step passed except one upstream Manvi test (`serve` `TestVerifyJobDeadlineDegradesASlowCriticAndLeavesTheServerResponsive`, a 5s bound) that failed at host load 70-96 and passed alone 3/3 and in a full `serve` rerun at load 16 [V]. The three steps the failure skipped were run by hand as `testSteps` defines them and passed. Not verified: the Manvi/DevCouncil consumer checks the pin rule names (`manvi doctor`, `devmap --version`, GitPulse `check:schema-parity`), and any speedup from the new scheduler (the host was loaded).

**Pruned**: the merged `fix/*`, `perf/dev-test-lanes` and `integrate/1117` branches and their worktrees. **Kept**: `claude/pin-manvi-8a7bada-c8f201` and its worktree, because session pid 32443 (last activity about two days old) has not answered the wind-down request and still has its cwd there; it is merged and backed up, so delete it once that session is closed (`git worktree remove`, then `git branch -d`).

**Open items** are on the GitPulse board; none is critical.

## Update 2026-11-16 — merges done on the owner's instruction ("commit all branches and merge them")

**Result: all four branches are merged into main. Nothing deleted, nothing pushed.** The sections below this one are the 2026-11-15 record and describe the pre-merge state; where they say "not merged", "blocked" or "kept pending", this section supersedes them. This instruction replaced the earlier "pin: not now" and "owner finishes winddown's merge" answers.

| Step | Commit | What happened |
|---|---|---|
| 1 | `6ae21cf` | Concluded winddown's parked merge of `perf1109/jarvis-records` (4 conflicts, resolved below). Made with a file archive taken first. |
| 2 | `b7e2462` | `git merge --no-ff winddown/jarvis-candidate` into main. **No conflicts.** Brings in all 6 perf1109 commits. |
| 3 | `16252d6` | `git merge --no-ff claude/pin-manvi-8a7bada-c8f201` into main. **No conflicts.** Pin bump. |

Every branch tip is an ancestor of main [V]: `winddown/jarvis-candidate` 6ae21cf, `perf1109/jarvis-records` 6dd459f, `claude/pin-manvi-8a7bada-c8f201` bdd9bec, `perf/dev-test-lanes` 3226fe9. `git branch --no-merged main` is empty. `git fsck`: clean.

### Conflict resolutions (step 1), reviewed by Opus before the commit
| File | Resolution | Why |
|---|---|---|
| `go.mod` | kept `go 1.27.4` | main's `d0b8ffa` bump; perf1109's only change was the stale 1.27.3 number. Already hand-resolved in the parked worktree. |
| `cmd/dev/main.go` | kept `go 1.27.4` in the `go.work` text | same; perf1109's only change was the version number |
| `README.md` (2 hunks) | kept "Go 1.27.4" | same |
| `CHANGELOG.md` | kept main's five Changed bullets; replaced perf1109's bullet | perf1109's bullet mixed a superseded Go 1.27.3 claim with real content. The new bullet keeps the `encoding/json/v2` encoders (v1 option set, so bytes don't change), the run-view copy and cursor polling, the owner's 2026-11-09 approval and the flush-batching doc, and notes that 1.27.3 is superseded by the 1.27.4 entry. |

After resolution `go.mod`, `cmd/dev/main.go` and `README.md` were identical to main, so nothing of perf1109's was lost [V]. Opus verdict: resolutions correct; encoder, copy and cursor claims in the bullet checked against the code; `gofmt` clean; no leftover `1.27.3` outside that bullet. Opus could not run the `internal/app` tests (cgo link); I ran them with cgo off, below.

### Verification of the merged tree (cgo off, `go.work` in use)
- After step 2, before committing: `gofmt` clean, `go build ./...`, `go vet ./...`, and `go test` for `internal/*`, `cmd/jarvis`, `cmd/qualify`, `tests/*` all **pass** [V]. `cmd/dev` with the cargo-building test skipped: **one failure**, `TestBootstrapRunsOfflineWithoutWorkspaceOrUpstreamModules` (`signal: killed`). It fails the same way on pre-merge main `3226fe9` in a clean worktree [V], and the merge changes zero `cmd/dev` files [V], so it predates the merge.
- After step 3: the same build, vet and tests pass, but see the pin caveat below.

### Pin bump (step 3): deltas and what is NOT verified
- Manvi `44cf392f` → `8a7bada4` (go.mod `v0.0.0-20261115001930-8a7bada404db`); DevCouncil `d9c7a512` → `230bbf04`; gusset unchanged [V from the commit diff].
- The pin commit states workbench schema stays 17 and dc-store stays 11; both are fast-forwards. **Unverified:** I could not inspect the upstream commits, because the local checkouts don't contain them.
- **The pin is not verified end to end.** `go.work` replaces Manvi with the stale local checkout (`5f2eb796`), so the passing tests say nothing about `8a7bada4`. `dev check-pins` was not re-run; it fails here already because `.local/upstream/*` lag the pins. A `GOWORK=off GOPROXY=off` build resolves the pinned Manvi from the module cache but fails on a missing `go.sum` entry for a DevCouncil package Manvi imports; that is inferred to be the same gap main already had, but I did not run it on main.
- Main's `go.sum` had **no** entries for its previous pin (Manvi `44cf392f`) [V]. The pin commit adds entries for `8a7bada4`, which closes that gap for the new pin only.
- Per your rules "done" for a pin bump means each consumer reports the new numbers (`dev check-pins`, `manvi doctor`, `devmap --version`, GitPulse `bun run check:schema-parity`). **None of that has run.** Treat the pin as merged, not done.
- Live session pid 32443 (idle) is attached to the pin worktree; the merge didn't touch that worktree.

### New branch found and deliberately NOT touched: `perf/dev-test-lanes`
- A worktree `/Users/bharath/Code/devtools/Jarvis.perf-dev-test-lanes` appeared after the inventory (branch `perf/dev-test-lanes`, created at `3226fe9`, 0 commits of its own). It holds **uncommitted, actively edited work**: 4 modified files (CHANGELOG.md, README.md, `cmd/dev/main.go`, `cmd/dev/package_test.go`) and 5 new files in `cmd/dev` (`steps.go`, `procgroup_unix.go`, `procgroup_other.go`, `steps_test.go`, `steps_cancel_test.go`), with files written in the last 30 minutes [V]. It looks like work on the `cmd/dev` test hang found above [I].
- I could not confirm a live session (GitPulse's sessions facet was rate-limited) [U]. Committing another agent's in-progress work would capture a half-finished state and could disrupt it, and the branch did not exist when you gave the instruction, so I left it alone. Its branch is already merged (no commits of its own), so merging added nothing.
- It is now 8 commits behind main and edits CHANGELOG.md, README.md and `cmd/dev/main.go`, so whoever finishes it will need to merge main in and resolve those files.

### Backups taken for this step [V]
`refs/backup/2026-11-16/main`, `refs/backup/2026-11-16/winddown/jarvis-candidate`, and `/Users/bharath/Code/devtools/_branch-backups/jarvis-all-2026-11-16-after-merges.bundle` (all 5 branches, verified). The earlier 2026-11-15 refs, bundles and the winddown merge-state archive are unchanged.

### State after this step
- Main = `16252d6` (then one more commit adding this report), 22 commits ahead of `origin/main` [V]; **not pushed**.
- Worktrees all still present and checked out on their branches (winddown now clean with its merge concluded). I removed only the two temporary worktrees I created.
- Branches are **kept**, not deleted: you asked to commit and merge, not delete. All four are fully merged and each has a backup ref, so they are now deletion candidates whenever you ask.
- To undo the three merges: `git reset --hard 3226fe9` on main (discards the merges; every branch tip still exists), or restore from the 2026-11-15-post backup.

## Update 2026-11-16 (later) — stale branches deleted on the owner's instruction

Rules applied: tip reachable from main, a backup ref at the tip, worktree clean, no process with its cwd there; `git worktree remove` without `--force`; `git branch -d` (refuses unmerged), never `-D`. Re-checked immediately before deleting [V].

| Branch | Backup ref | Worktree removed |
|---|---|---|
| `winddown/jarvis-candidate` (was `6ae21cf`) | `refs/backup/2026-11-16/winddown/jarvis-candidate` | `.antigravity/worktrees/winddown-jarvis-candidate` (clean, nothing ignored) |
| `perf1109/jarvis-records` (was `6dd459f`) | `refs/backup/2026-11-15/perf1109/jarvis-records` | `.claude/worktrees/perf1109-records` (clean; only the regenerable `go.work` and `go.work.sum` were ignored files) |

Both tips are still reachable from main, and `git fsck` is clean [V].

**Kept, with the reason:**
| Branch | Reason |
|---|---|
| `main` | trunk |
| `claude/pin-manvi-8a7bada-c8f201` | **live `claude` process (pid 32443, up 1d+) has its cwd in the worktree** [V]; the branch is fully merged and backed up, so delete it once that session is closed |
| `perf/dev-test-lanes` | worktree has 9 uncommitted, actively edited paths [V]; someone appears to be mid-work on the `cmd/dev` test hang |
| `refs/backup/*`, `origin/main` | backups are kept on purpose; no remote branches were touched |

The earlier backup `refs/backup/2026-11-15/winddown/jarvis-candidate` points at the old tip `ab65f14`; the current one is the 2026-11-16 ref.

**Restore a deleted branch:**
```
git branch winddown/jarvis-candidate refs/backup/2026-11-16/winddown/jarvis-candidate
git branch perf1109/jarvis-records   refs/backup/2026-11-15/perf1109/jarvis-records
# or from the bundle (all five branches as of 2026-11-16):
git fetch /Users/bharath/Code/devtools/_branch-backups/jarvis-all-2026-11-16-after-merges.bundle refs/heads/<name>:<name>
# worktrees are not restored by this; recreate with: git worktree add <path> <name>
```
Restoring a branch does **not** bring back the winddown worktree's pre-commit files; those are in `/Users/bharath/Code/devtools/_branch-backups/winddown-merge-state-2026-11-15.tar`, though the merge they led to is now commit `6ae21cf`.

**Status of the earlier (2026-11-15) record below: Phases 1–3 done; nothing merged at that point.**
Main's pending work was committed as three commits (owner confirmed it was theirs). Everything else is blocked on the items under "Blocked", none of which I can clear myself.

Decisions the owner gave this session: commit main's dirty work (done); pin bump **not now**; perf1109 lands by **finishing winddown's merge** (owner or its session concludes it; I do not touch that worktree); gate is `dev check-pins` + `dev test` + `go test` in a clean worktree.

Evidence labels: **[V]** verified by me with a command or tool result; **[F]** reported by the Fable review agent and not independently re-run by me; **[I]** inferred; **[U]** unverified.

## Phase 1 — Inventory

Only local branches exist. The sole remote ref is `origin/main` at 9a5f25a (2026-10-30). Local `main` is ahead of it and was not pushed. [V]

| Branch | Ahead / behind main | Last commit | Merged into main? | Worktree | Worktree state | Live session | Bound GitPulse task |
|---|---|---|---|---|---|---|---|
| `main` @ ab65f14 (now d5eac1a) | — | 2026-11-14 | — | primary checkout | **dirty at inventory time: 34 paths** (27 modified, 7 untracked), owner unknown then; owner confirmed, committed in Phase 3 | claude pid 5027 (this session) [V] | — |
| `winddown/jarvis-candidate` @ ab65f14 | 0 / 0 | 2026-11-14 | yes, by ref only (listed by `--merged`) | `.antigravity/worktrees/winddown-jarvis-candidate` | **unfinished merge** of `perf1109/jarvis-records` (MERGE_HEAD 6dd459f, started 2026-11-14 17:38); `UU` in CHANGELOG.md, README.md, cmd/dev/main.go, go.mod; 17 dirty paths [V] | none now (antigravity ok, 0 sessions) [V]; merge looks parked [I] | [U] |
| `perf1109/jarvis-records` @ 6dd459f | 6 / 3 | 2026-11-09 | no; all 6 commits `+` in `git cherry` [V] | `.claude/worktrees/perf1109-records` | clean [V] | none [V] | probably ft-7899e808 (perf audit, backlog, label `decision`) [I] |
| `claude/pin-manvi-8a7bada-c8f201` @ bdd9bec | 1 / 0 | 2026-11-14 | no; `+` in `git cherry` [V] | `.claude/worktrees/pin-manvi-8a7bada-c8f201` | clean [V] | **live claude pid 32443, idle, since 2026-11-15 02:40** [V] | possibly ft-65ea22c4 "Fail closed when upstream pins do not match" (review) [I] |

All four authors are the same person. Branch-to-task bindings were not looked up through provenance or the ledger [U].

Ahead/behind figures are as of inventory, when main was `ab65f14`. Now that main is `d5eac1a` [V]: `perf1109/jarvis-records` is 6 ahead / 6 behind, `claude/pin-manvi-8a7bada-c8f201` is 1 ahead / 3 behind, `winddown/jarvis-candidate` is 0 ahead / 3 behind.

### GitPulse facets
- sessions: ok after two failed attempts (rate-limit deferral). [V]
- `changes` and `collisions` facets: **failed on every attempt** (git spawn rate limit). They are not "clean"; `dirty_unknown` was 3. Dirty counts above come from `git status`, not from those facets.

### Classification (Fable review agreed with it; its reasoning is [F])
| Branch | Class | Reason |
|---|---|---|
| `winddown/jarvis-candidate` | **in use** | `--merged` is true only because the ref holds no commits. Its worktree holds a half-resolved merge that exists nowhere else. |
| `perf1109/jarvis-records` | **mergeable, kept and flagged** | Real unmerged work, not superseded. Its 4 conflicts are already being resolved, unfinished, in winddown, and the owner chose to let that merge finish first. |
| `claude/pin-manvi-8a7bada-c8f201` | **in use** and needs approval | Live session attached. Merging it is a pin bump, which needs owner approval under the global rules. |
| `main` | keep | |

## Phase 2 — Backups (done) [V]

Backup refs (all four local branches):
```
refs/backup/2026-11-15/main                              ab65f1412b70c48f76c95756aca3ad407e9634ab
refs/backup/2026-11-15/winddown/jarvis-candidate         ab65f1412b70c48f76c95756aca3ad407e9634ab
refs/backup/2026-11-15/perf1109/jarvis-records           6dd459f4a064fb46b08bf4e037ec0049135c335b
refs/backup/2026-11-15/claude/pin-manvi-8a7bada-c8f201   bdd9becc09046086c0592cd798dc43cbf4b1a3fe
```
Bundle: `/Users/bharath/Code/devtools/_branch-backups/jarvis-2026-11-15.bundle`. `git bundle verify` passed and it records complete history.

Note: the harness write gate flagged this directory as outside the project root and let the write through only because the session is in bypass mode. I chose the path, since the brief said "a path outside the repo".

**Follow-up backups, taken later the same day [V]:**
- `refs/backup/2026-11-15-post/main` = `3226fe9`, plus `/Users/bharath/Code/devtools/_branch-backups/jarvis-main-2026-11-15-post-commits.bundle` (verified). These cover the four commits made on main this session (`d1c6267`, `d7d892d`, `d5eac1a`, `3226fe9`), which are unpushed. The first set above predates them.
- `/Users/bharath/Code/devtools/_branch-backups/winddown-merge-state-2026-11-15.tar`: the 17 dirty files from winddown's worktree as they sit on disk (conflict markers included for the `UU` files), plus `MERGE_HEAD`, `MERGE_MSG`, `MERGE_MODE` and the worktree `index`. Made read-only; the worktree and its merge were not touched. It is a file copy, not a commit: the resolution survives, but restoring it means copying files back and re-creating the merge state by hand, so aborting winddown's merge is still not free.

## Phase 3 — Commit pending work (main done; others untouched)
The owner confirmed main's 34 dirty paths were theirs; I committed with explicit paths, no `git add -A`.

**What I read, and what I did not.** Read in full: every diff of a tracked source file, the full docs diff, `internal/evidence/formal_comparator.go`, and both `vani` JSON files. Not read line by line: `internal/app/inputs.go` (508 lines; secret-scanned, and its three registered operations confirmed at `inputs.go:475-491`), and the four new test files (`inputs_test.go`, `admission_test.go`, `formal_comparator_test.go`, plus the added tests in tracked files), which were run and pass but not reviewed for strength. The secret scan found nothing but redaction code in `inputs.go`. I found no defect in the code I read; that is not a security review.

**Docs claims checked against the repo:** the pins in the docs match `upstream.lock.json`; every evidence file the docs link exists and is tracked; `--provider`/`--fixture` are real flags (`cmd/jarvis/main.go:65-66`); `jarvis.anchor.create` is at `internal/app/module.go:134`; the bank `--fault`/`--variant` modes are in HEAD; `StrictlyDominates`, `SelectAdmissibleFrontier` and the fail-closed verifier-hash check (`formal_comparator.go:161-163`) exist as described in the commit messages. **Not verified:** the Rust test counts quoted in `desktop/README.md` and `docs/IMPLEMENTATION.md` (23 bank / 62 desktop tests); I ran no Rust.

**One docs line was wrong when committed in `d5eac1a`; corrected afterwards in `3226fe9`:** `docs/publish-status.md` says `dev check-pins` is "passing at `ab65f14`" and describes it as checking agreement between `upstream.lock.json` and `go.mod`. The code (`cmd/dev/package.go:110-135`) also requires each upstream checkout's HEAD to equal its pin and to be clean, and it fails on this machine (see "The gate"). The "passing at `ab65f14`" claim is unverified. Say whether to correct it or leave the owner's wording.

| Commit | Content |
|---|---|
| `d1c6267` feat(evidence): verify against a held-out lock out-of-band | `internal/evidence/formal_comparator{,_test}.go`, `cmd/jarvis/main{,_test}.go` (4 files, +904/−16) |
| `d7d892d` feat(app): admit inputs profiles apart from the bank | `internal/app/*` (profile, drift, discovery, module, app, new `inputs.go`, `inputs_test.go`, `admission_test.go`), `cmd/qualify/*`, `profiles/{north,south,vani}.json`, `policies/vani.json` (18 files, +1300/−40) |
| `d5eac1a` docs: describe the held-out lock, inputs profiles and bank fault modes | CHANGELOG, README, REPORT, desktop, docs/, evidence/ (12 files) |

Main is now at `d5eac1a`, 3 commits past `ab65f14`. Only this untracked `BRANCH_REPORT.md` remains in its working tree.

These commits were **not** produced by a merge and add nothing to `upstream.lock.json`, `go.mod` or `go.sum`, so they are not a pin bump.

- winddown: a half-resolved merge is in-progress work. Not touched.
- The other two worktrees are clean; nothing to commit.

## Phase 4 — Merges: none performed

### Conflicts, re-run against `d5eac1a` (`git merge-tree --write-tree`, read-only) [V]
- `main` + pin branch: **clean three-way merge, no conflicts**, but no longer a fast-forward (pin is 1 ahead / 3 behind). Both sides edit CHANGELOG.md in different regions and auto-merge.
- `main` + perf1109: conflicts in exactly the same four files as before: `CHANGELOG.md`, `README.md`, `cmd/dev/main.go`, `go.mod`. `cmd/jarvis/main.go`, `internal/app/app.go` and `internal/app/module.go` **auto-merge cleanly** against the new main. My earlier guess that the conflict set would grow was wrong.

Fable's original read against `ab65f14` [F], kept for the record:
- `main` + pin branch: clean, fast-forward. Touches CHANGELOG.md, go.mod, go.sum, upstream.lock.json.
- `main` + perf1109: content conflicts in CHANGELOG.md, README.md, cmd/dev/main.go and go.mod.
  - `go.mod` and `cmd/dev/main.go` conflict over the Go line. Main has `go 1.27.4`. perf1109 sets `1.27.3` (Fable says it moved from 1.26.6 up to 1.27.3, not down from 1.27.4). The resolution must keep **1.27.4** in both files. Both sides of each conflict still need reading before resolving; none is resolved here.
- Symbol overlap with dirty main [F, DevMap]. Moot now: that work is committed and the merge-tree above shows these three files merge cleanly. Kept for the record:
  - `module.go`: same function (`App.Configure`), distant hunks.
  - `app.go`: different symbols.
  - `cmd/jarvis/main.go`: both sides edit inside `run()`.
  - DevMap's blast on `app.go:32` returned `complete:false`, so that overlap is a lower bound. I did not record a gap with `devmap gap-record`.

### Why nothing was merged
1. ~~Main was dirty~~ — cleared by the Phase 3 commits. The conflict lists were re-run against `d5eac1a` (see above): perf1109 still conflicts in the same four files, the pin merges cleanly.
2. Pin bump: the owner said **not now**, and its worktree has a live session.
3. perf1109: the owner chose to let winddown's parked merge finish first. Merging it directly now would duplicate that resolution.
4. winddown's own branch cannot be merged or deleted: it has no commits, and its worktree holds the unfinished merge.

### The gate
`ci:local` is **not defined in this repo** (searched code, CLAUDE.md and AGENTS.md) [V]. The owner chose `dev check-pins` + `dev test` + `go test`.

What I could and could not run [V unless noted]:

| Check | Result |
|---|---|
| `go build ./...`, `go vet`, `go test` for `./internal/...`, `./cmd/jarvis/...`, `./cmd/qualify/...`, run in a clean throwaway worktree at each of `d1c6267`, `d7d892d`, `d5eac1a` with `CGO_ENABLED=0` | **pass** at all three, so each commit builds and passes on its own |
| Same, with cgo on | **cannot link**: `ld: library 'gusset' not found`. The Rust gusset engine is not built in this checkout. Environmental; compile stage passed. |
| `go run ./cmd/dev check-pins` | **fails**: `upstream HEAD differs from pin: .local/upstream/Manvi`. The local Manvi checkout is at `5f2eb796` and DevCouncil at `c6788883`, the pins from two bumps ago. Main pins `44cf392f` and `d9c7a512`; gusset matches. Not caused by these commits; they touch no pin file. |
| `CGO_ENABLED=0 go test ./...` on main (`d5eac1a`) | **FAIL, one package.** `internal/*`, `cmd/jarvis`, `cmd/qualify` and `tests/native-platform` pass. `cmd/dev` **hit the 600s timeout and panicked**; a re-run with `-timeout 75s` shows it stuck in `TestBuildFindsBinariesWithInheritedCargoTargetDirectory`, a test that runs a real `dev build` (cargo). The harness reported "exit 0" for the first run only because the command was piped through `tail`; the output contains `FAIL github.com/bharathvbcr/Jarvis/cmd/dev 600.151s`. |
| Cause of the `cmd/dev` hang | **Inferred, not proven.** The test's `cargo` is a wrapper (`~/.claude-bin/cargo`) sitting in a `sleep` loop while another session's `cargo clippy` (pid 9878, not mine) was running, so it looks queued behind that. My commits changed **zero** files under `cmd/dev` [V]. I did not run `cmd/dev` tests at `ab65f14` to prove it fails there too. |
| `go run ./cmd/dev test` | **not run.** It rewrites `go.work`, needs the upstream checkouts at the pinned revisions, and needs the Rust and DevCouncil legs. It would fail on the stale checkouts above. |
| `git fsck --no-dangling` | clean, no output |

**Cleanup note:** the two `cmd/dev` test runs left orphaned `dev build` / `cargo` wrapper processes (PIDs 7970, 8007, 31673, 31717). They were mine, identified by a test-named `--target-dir` and their start times, and I killed exactly those four PIDs. The other session's clippy (pid 9878) was not touched.

**Caveat on the passing tests:** `go.work` makes the build use the local Manvi checkout, so those tests ran against Manvi `5f2eb796`, not the pinned `44cf392f`. A pass is real evidence about the code but not about the pinned build. The full gate remains unproven.

**Unblocking the gate (needs the owner):** advance `.local/upstream/{Manvi,DevCouncil}` to the pins in `upstream.lock.json` and build the gusset engine, then run `dev check-pins`, `dev test`. I did not touch those checkouts: they are shared with other sessions, and a pin-bump session (pid 32443) is live next door.

## Phase 5 — Deleted: none

## Kept
| Branch / worktree | Reason |
|---|---|
| `main` | trunk; now clean apart from this untracked report, 3 commits ahead of where inventory started |
| `winddown/jarvis-candidate` + worktree | unfinished merge; holds unsaved resolution |
| `perf1109/jarvis-records` + worktree | unmerged work (6 commits `+`); contended |
| `claude/pin-manvi-8a7bada-c8f201` + worktree | live session; pin bump needs approval |

## Blocked, and what unblocks it
| Item | Needed |
|---|---|
| Pin branch | the owner deferred the bump ("not now"). To proceed: report the Manvi 8a7bada4 / DevCouncil 230bbf04 deltas, close session 32443, then run `dev check-pins` and the doctor checks (`manvi doctor`, `devmap --version`, GitPulse `bun run check:schema-parity`) |
| perf1109 | winddown's merge concluded by its owner or session (chosen path). Then merge winddown into main; it will conflict again where it overlaps the three Phase 3 commits, so expect to resolve CHANGELOG.md, README.md, `cmd/dev/main.go` and `go.mod`, keeping the Go line at 1.27.4 |
| The gate, for any merge | `.local/upstream` checkouts advanced to the pins and gusset built; `cmd/dev` test hang explained (cargo contention suspected) |
| Deleting winddown | its merge concluded or aborted, then `git worktree remove` |

## Restore commands
Nothing was deleted, so nothing needs restoring today. Should a branch be deleted later:
```
# from the backup ref
git branch <name> refs/backup/2026-11-15/<name>
# or from the bundle
git fetch /Users/bharath/Code/devtools/_branch-backups/jarvis-2026-11-15.bundle refs/heads/<name>:<name>
```

## Phase 6 — Verification
- Final gate on main: **not green.** `check-pins` fails on stale upstream checkouts; `go test ./...` fails in `cmd/dev` (600s timeout in a cargo-building test, cause inferred as cargo contention, not caused by files my commits touched); `dev test` was not run. Every other Go package passes with cgo off, against the stale local Manvi checkout (see "The gate").
- `git fsck`: clean.
- Final `gitpulse_insights`: the `changes` facet came back ok (1 untracked file, this report); the **sessions facet was rate-limited and failed**, so GitPulse did not confirm that no live session lost its branch. I checked it directly instead: `git for-each-ref` shows all four branches at their original tips, and `git worktree list` shows all three worktrees still on their branches. That shows no branch or checkout was removed or moved; it does not show what a session is doing. The DevMap index is now marked stale (source differs from generation 225) because main changed; I did not rebuild it.
- The three other worktrees are at the same HEADs as before (winddown ab65f14, perf1109 6dd459f, pin bdd9bec). The temp worktree I created was removed with `git worktree remove`.
- Net changes this session: 4 `refs/backup/` entries, the bundle file, 3 commits on main, and this report.

## Environment notes
- The harness gate (GitPulse/MANVI) reported it **would have refused** some calls and did not because the session is in bypass mode: writing the bundle outside the project root, and one read-only command whose task binding could not be resolved under the git rate limit. None changed repo state beyond what is listed above, but the gate was advisory, not enforcing, for this run.
- GitPulse's pre-op snapshot was unavailable for every git write (`os error 35`), so the three commits and four refs are **not** undoable through GitPulse's op-log. Undo them with git: `git reset --soft ab65f14` (keeps the work staged) while main is at `d5eac1a`, and `git update-ref -d refs/backup/2026-11-15/<name>` for the refs.
- GitPulse `changes` and `collisions` facets never returned `ok:true`; session data did.

## Open items (all need the owner)
1. Conclude or abort winddown's parked merge (its staged resolution is not backed up anywhere). Once it is concluded, merge it into main, then recompute the conflicts against `d5eac1a`, resolve with the Go line kept at 1.27.4, run the gate, and only then delete.
2. Pin bump (Manvi 8a7bada4 / DevCouncil 230bbf04): deferred by the owner. Needs the deltas reported, session 32443 closed, and the consumer checks afterwards.
3. Advance `.local/upstream` checkouts to the current pins and build gusset so the full gate can run.
4. Branch-to-task bindings (provenance/ledger) are still unverified. This needs no owner decision, only a GitPulse call that was not made; no board task is `in_progress`, so it cannot change a classification.
5. ~~Correct the `docs/publish-status.md` check-pins line~~ — done in `3226fe9` ("docs: describe what check-pins actually enforces"). It now describes what `cmd/dev/package.go:110-135` enforces, and drops the unverified "passing at `ab65f14`" claim.
6. Push: not done and not requested. `main` is now 11 commits ahead of `origin/main` by my count (8 before + 3) [I].
