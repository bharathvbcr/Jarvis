# Jarvis audit — 2026-11-01

Read-only audit of `/Users/bharath/Code/devtools/Jarvis`. No source, config, or dependency files were modified. The only writes are this report and eight new GitPulse backlog cards. Nothing was moved to done. The pre-existing Lappi card was not edited, merged, or deleted.

Repository record checked on every board call: `d62555e9-4917-423e-a368-0fb66f0a8bb7`, name Jarvis.

## Severity × dimension

Counts are confirmed findings (nine). Eight cards cover them. C4 is an acceptance criterion on the pin-check card, not its own card. Empty cells are zero.

| Dimension | Critical | High | Medium | Low |
| --- | ---: | ---: | ---: | ---: |
| Modular architecture | 0 | 0 | 0 | 0 |
| Strict typing | 0 | 0 | 0 | 0 |
| Defensive validation at trust boundaries | 0 | 0 | 2 | 1 |
| Async error boundaries | 0 | 1 | 1 | 0 |
| Deterministic testability | 0 | 0 | 0 | 0 |
| Performance and benchmarks | 0 | 0 | 0 | 0 |
| Correctness (Windows / macOS / Linux) | 0 | 0 | 2 | 0 |
| Security | 0 | 0 | 2 | 0 |

Medium defensive-validation findings are C1 and D2. The low one is C4. High async is D1. Medium async is C2. Medium correctness is A1 and A2. Medium security is B1 (evidence disposition) and A3 (credential replacement).

No performance finding was confirmed, so no benchmark-baseline card was filed. The 300 ms interactive deadline already lives on the Lappi card.

## Top risks

Nine confirmed findings, in build-relevant order. There is no tenth confirmed risk.

1. **D1, high, priority 1.** Workbench `control_pending` survives disconnect, channel death, the deadline stop, and reconnect. Approve, Deny, Pause, Resume, and Cancel stay disabled when epoch, sequence, and capture id do not change. `desktop/crates/workbench/src/app.rs` (disconnect 268–272, channel death 275–281, deadline 295–299, reconnect 931–941, hello 558, idle gate 1402–1405). A failed control reply does clear the lock (383). A dead pipe never delivers that reply.
2. **A2, medium, priority 1.** Human action ids are `runID + ":human:" + name` (`internal/app/intervention.go:98`). The broker treats that id as single-use. A second attempt conflicts or returns a cached response and does not dispatch. `Run.Send` returns nil at enqueue, so the caller is told the act was accepted.
3. **B1, medium, priority 1.** A `human_action` receipt with delivery `not_sent` stays `unknown` (`internal/app/evidence.go:159–168`). Event receipts already map `not_sent` to `not_dispatched` (126–132). DevCouncil treats `unknown` as incomplete and still requires a post-observation. A refused human attempt can block an otherwise acceptable bundle.
4. **C1, medium, priority 1** (auditor said high; downgraded). `dev build` prints an unreadable lock or a HEAD mismatch (`cmd/dev/gusset.go:91–106`) and still exits 0. `gusset-check: ok` is the engine self-test (`cmd/jarvis/gusset_check.go:38`), not a pin match. `check-pins` fails closed; `build` does not call it.
5. **A1, medium, priority 1.** `set_value` and `type_text` keep only the first token as the control name (`internal/app/intervention.go:132–138`). `Member ID` and `Subaccount name` cannot be named. `press` and `click` already keep the rest of the line.
6. **C2, medium, priority 1.** `jarvis replay` and `discover` drop stdin EOF and scanner errors (`cmd/jarvis/main.go:226–236`). The select loop (240–304) keeps reprinting the prompt. A token over 64 KiB does the same.
7. **D2, medium, priority 1.** `jarvis.discovery.get` returns the whole view, including a live screenshot of up to 16 MiB of base64 (`privacy.go:55`). The workbench line cap is 20 MiB (`transport.rs:15`, 231–232) and polls every 600 ms (`app.rs:331–336`). `read_responses` PNG-decodes that frame (254–270) even though the Discovery tab does not show it.
8. **C4, low, priority 2.** `checkUpstreamPinsAgree` (`cmd/dev/package.go:160–166`) accepts the pinned revision anywhere `ref: <rev>\n` occurs, including a comment, while the step ref differs. Filed as the second criterion of the pin-check card.
9. **A3, medium, priority 2.** `jarvis.key.set` calls `credentials.Set` before it knows the seal worked (`internal/app/module.go:209–213`). A failed seal is `Present()` and `Resolve` returns it without falling through, so a later failed replacement removes a working Gemini key.

## New cards

All status `backlog`, kind `bug`, label `audit-2026-11-01`. Re-listed after filing: 9 open tasks, `has_more: false`. No card was overwritten. No fold into an existing card.

| Wave | Finding | item_id | task_id | Priority | Severity |
| --- | --- | --- | --- | ---: | --- |
| 1 | D1 workbench control lock | `ft-63c4cc812f3f17afd0e8cb69a54611d8` | `gp-audit-2026-11-01-workbench-control-lock` | 1 | high |
| 1 | A2 unique broker action id | `ft-b2c56d57c9a9b0e0820eb0efe5b92001` | `gp-audit-2026-11-01-human-action-id` | 1 | medium |
| 1 | B1 human `not_sent` → `not_dispatched` | `ft-aabf1eb5dd36236a081a1cfdd521d015` | `gp-audit-2026-11-01-human-not-sent` | 1 | medium |
| 1 | C1 + C4 pin checks fail closed | `ft-65ea22c411f3b06b84e133a7bfc266b0` | `gp-audit-2026-11-01-pin-checks` | 1 | medium |
| 2 | A1 multi-word act-spec names | `ft-2c82904b7f9a6cf526afed02eb5e9dfe` | `gp-audit-2026-11-01-act-spec-names` | 1 | medium |
| 2 | C2 stdin EOF | `ft-179313b3b8160b558d64a80c8cad7a00` | `gp-audit-2026-11-01-stdin-eof` | 1 | medium |
| 2 | D2 discovery payload under 20 MiB | `ft-ac1c6cf0776795e9f538993ccf96991c` | `gp-audit-2026-11-01-discovery-payload` | 1 | medium |
| 2 | A3 seal failure must keep the previous key | `ft-aed99e87d008e1f080459282298c432b` | `gp-audit-2026-11-01-key-set-seal` | 2 | medium |

## Folded cards

None. Related-task refusals were title-word collisions under the shared `audit-2026-11-01` label (`when`, `fail`, `a`, `the`, `control`, `keep`). Each listed card was read and judged a different concern. Creates were retried with `reviewed_related` naming every colliding id. The Lappi card was not in those lists.

C1 and C4 were grouped into one new card before the first successful create. That is grouping, not a fold of a card that was already on the board.

## Verified-stale cards

None. No acceptance criterion was marked `[MET — verified in source]`.

`ft-0fb664e1b22b216354a84e7278f68fb6` (Lappi caller, backlog, priority 3, severity low, revision 1) is not stale. First-party `*.go`, `*.rs`, and `*.md` have no Lappi reference. Discovery stays on `gemini-3.8-flash` (`internal/app/discovery.go:26`). None of its three criteria are implemented. It was not marked done and its priority and severity were not lowered.

## Fable waves

Followed. No disagreement with the verifier.

Wave 1, disjoint files:

- D1: `desktop/crates/workbench/src/app.rs`
- A2: `internal/app/intervention.go`, `internal/app/intervention_test.go`. Do not edit `cmd/jarvis/main.go` unless a broker refusal cannot be returned from the act path. If it must, do it in this wave, before the stdin card.
- B1: `internal/app/evidence.go`, `internal/app/evidence_test.go`
- C1 + C4: `cmd/dev/main.go`, `cmd/dev/gusset.go`, `cmd/dev/package.go`, `cmd/dev/package_test.go`, `cmd/dev/gusset_test.go`

Wave 2, disjoint from each other. Two paths repeat a wave-1 owner, so they wait:

- A1: second editor of `internal/app/intervention.go` and `intervention_test.go`
- C2: `cmd/jarvis/main.go`, `cmd/jarvis/main_test.go`
- D2: `internal/app/discovery.go`, `desktop/crates/workbench/src/transport.rs`, second editor of `desktop/crates/workbench/src/app.rs`
- A3: `internal/app/module.go`, `internal/app/key_set_test.go`

## Rejected findings

- **C3.** Qualify records `qualified` / `campaign_passed` when `source_status` is `unavailable` or `dirty`. Documented contract: `cmd/qualify/README.md` (git revision and clean/dirty/unavailable are context; executable SHA-256 identifies what was admitted) and `docs/IMPLEMENTATION.md` (dirty macOS campaign and unavailable Linux guest, both bound by SHA-256). Windows ARM64 execution is deferred by the product notes. No Windows-qualification card.
- **Absolute `App.resolve` / `trace.load`.** `App.resolve` returns absolute paths unchanged. Serve is a local stdio child. The same principal can already read the file.
- **Dead-symbol rows at confidence 0.4.** `campaignGemini.PriceOf` is the `budget.Pricer` method. `App.Configure` is `serve.Module.Configure`. `PolicySelector.validate` is called from `PolicyDocument.validate`. `BankState::validate` is a Rust namesake of the Go method. `internal/evidence` `Contract` is used as a type. Not dead.
- **Lappi absence.** Already the open Lappi card. No new evidence.
- **Catalog gate only on `--unattended`.** `RequireApprovedForUnattended` is the CLI replay path. Start and `jarvis.run.start` do not call it. Behavior matches the current gate, so it was not filed as a bug.
- **`ActHuman` effect `"read"`.** Not an approval bypass. The broker still requires approval when the target is outside the read-only list. A Confirm click that is `not_sent` is the B1 mislabel, not an authorization skip.
- **`validateBankProgram` empty `UnattendedChange` branch.** Does not auto-approve.
- **`extract` / `assert` / `branch` / `wait` continuing past the profile target list.** Load-bearing for `examples/balance.json`.
- **First enabled-name match.** Current matcher behavior. The multi-word bug is A1; the first-match rule itself was not filed.
- **`findByDigest` I/O errors still refuse.** Fail closed.
- **`Get` holding the mutex across `json.Marshal` of the observation.** Unmeasured. Not a confirmed performance finding.
- **`jobs.go` storing a late success after the deadline, and async handlers using `a.ctx`.** A late success after the deadline should not be stored as the job result. Background jobs discarding the request context is the existing design. Not filed.
- **Discovery `run.Wait(context.Background())`.** Bounded by the run context (discovery timeout plus the runner intervention limit). Not an unbounded wait.
- **`discovery` effect when the control name is `Confirm creation`.** Workflow pauses only for an effect change. The broker still requires a token when the node is outside `read_only_targets`.
- **`Catalog().Put` during capability publish.** Writes a draft. Unattended replay still goes through `RequireApprovedForUnattended`.
- **`ownedBank` nil admission error with a non-zero PID.** Safe on the path that was read.
- **`childenv.Inherit` stripping only `credentials.ProviderEnv`.** Tests assert `GEMINI_API_KEY` is absent.
- **Windows case-folding.** Not re-run. Sources are `//go:build !windows`. No card invented.
- **`codegen.Go` identifier emission.** Constrained by `namePattern` and `strconv.Quote`. Name collisions are rejected.
- **`trace.Replay` ignoring `Journal.Error`.** A terminal state that does not match the reduction is still refused.
- **Fixture `evidence_class: "not live"`.** Intentional in the live-discovery runbook.
- **`internal/evidence.Verify` returning a failed verdict with a nil error when `report.OK` is true.** Callers check the verdict. Returning the report is the contract.
- **Duplicated `sourceRoot` helpers, quoted `-append`, `extractRoot` README skip, `jarvis doctor` exit 0 with a JSON error field, TOCTOU, `checkBoundaries` with no timeout.** Not confirmed as defects worth a card.
- **`jarvis verify` omitting `.exe`.** Go `exec.LookPath` applies `lookExtensions`.
- **Desktop: no listening socket, bank-demo as the synthetic target, `SESSION_PASSCODE` fixture constant, `TypedValue` omitempty, editor drag `1..=8` versus compiler `1..=5` (fails at compile), Wayland disabled, `unsafe_code` forbid, `total_records` `unwrap_or` on a field that is always marshaled, unix-only fsync, existing approval-binding tests.** Not filed.
- **Clones noted, not filed:** `PolicyDocument.Allows*` in `policy.go`; duplicated `sourceRoot`; bank-demo `Fixture` new/drop; `Tenant.parse` versus `Variant.parse`.

## Evidence limits

Phase 1 GitPulse insights, all facets ok: one worktree, branch `main`, HEAD `13d77586e2d2241116684b501d1d23ebbb531ffb`, 0 dirty files, 0 live sessions, 0 collisions, ledger fresh. In-flight risk at that snapshot: none. Insights was not re-run during filing. No source file was edited, so no new in-flight collision was introduced by this audit.

DevMap at phase 1: generation 183, schema 29, fresh, query-ready, 925 nodes, 2880 edges, 0 pending. Not degraded. Resolution is **partial**: gross permille 118 (1944 resolved / 14464 unresolved). Net 695. Go net 734, Rust net 644. Dead-symbol `walk_incomplete`: 850 of 14464 unresolved attribution sites have no indexed target. Empty caller lists from that index are **unknown**, not proof of no callers. `devmap ask_evidence` was truncated, `walk_incomplete`, and reported a coverage gap of at least 22 unbound call sites; the walk stopped at depth 3. The index was not rebuilt.

The parent re-read the lines cited for the eight filed concerns, including the Manvi broker action-id check, `Run.Send`, credential `Set`/`Resolve`/`Present`, DevCouncil evidence dispositions, and the 16 MiB screenshot cap. Tests were **not executed**. `go test`, `go vet`, `cargo test`, and `cargo clippy` were not run.

Two card sentences rest on slice-audit citations the parent did not re-read: qualify's approver forwarding `scanner.Err` (the jarvis loop itself was re-read), and strict JSON rejecting unknown fields (the discovery parameter struct having only `runID` was re-read). `packageBuild`'s zip path was not re-read; the pin card does not claim it. The discovery preview's 128 KiB stop was not re-read and is not a filed criterion.

## Decisions made without asking

- This host has no model selector. Slice audits and the Fable pass were Grok `code-reader` subagents. The parent verified every filed finding against the code. Fable's grouping was followed. Disagreements: none.
- Slice subagents could not write finding files. Verified findings are this report and the board cards.
- C1 severity lowered from high to medium because the mismatch is printed and `gusset-check: ok` does not claim the pin matched. Priority stays 1. C4 stays an acceptance criterion on that card and does not lower the card.
- D2 filed at medium / priority 1 after the 16 MiB cap was verified. The failure is a disconnect.
- A1 and A2 are two cards in different waves. Both edit `intervention.go`.
- No benchmark card.
- Human-act grammar (quoted name or longest enabled name) is an implementation choice inside the A1 card.
- D2's protocol choice (omit the live screenshot, or fetch frames only through `jarvis.frame.get`) is stated on the card. The Go handler and the desktop change together because unknown JSON fields are rejected.
- The pin-check behavior change is recorded on that card (below). It is a `dev build` exit-code change, not a breaking library API, so filing continued.
- `related_tasks_exist` collisions were kept as separate cards via `reviewed_related`. None were folded.
- All new cards are `backlog`. Building waits for an explicit go-ahead.
- DevMap was not rebuilt.

## Owner decisions already written on cards

These do not block filing. Building should follow them.

- **Pin card.** Fail `dev build` when the lock is unreadable or HEAD differs from the pin, and add an explicit flag if a working-tree proof must still build. Do not change `dev gusset`'s documented warn-only behavior. `reportRevisions` takes a fail-closed versus warn mode from the caller.
- **Act-spec card.** Quoted names or longest enabled-name match are both acceptable. The card requires one of them, a hard error for an unknown name, and `press` / `click` keeping the rest of the line.
- **Discovery-payload card.** Omit the live screenshot from `jarvis.discovery.get`, or fetch frames only through `jarvis.frame.get`. A Go change is required with the desktop change.

No separate product stop was left open. No in-progress incident card existed. No user-authored card was merged or deleted.

## Stop

Board filing is complete. Waiting for a go-ahead before any implementation.
