# Jarvis assignment report

## Architecture

Jarvis is a thin product host over two upstream owners. Manvi owns discovery tools, the pure workflow reducer, the computer runner, catalog storage, journal privacy, and the Rust native desktop broker. DevCouncil owns the independent evidence schema and `dcverify` evaluator. Jarvis owns the bank demo, tenant profiles, reviewed policy documents, overlays, scenarios, the workbench, and packaging.

That split is deliberate. Reusable automation mechanics stay out of the product tree so a bank-specific policy or layout change cannot fork the reducer or the verifier. The Go host stays CGO-free; platform FFI lives only in Manvi’s native workspace. Reviewers build against pinned Manvi and DevCouncil commits in `upstream.lock.json` and `go.mod`, not against an ignored local `go.work` replace.

Gemini’s native `computer_use` tool surface was evaluated and **not adopted**. Typed semantic steps (observe → decide tool → admit action → observe again) produce a reviewable capability artifact with ladders, outcomes, and outputs. Pixel-level `computer_use` remains a future seam for visual anchors that lack accessibility names; it is not on the discovery path today because it would weaken the artifact contract without improving replay determinism.

## Artifact schema

Capabilities keep `schema_version: 1` and evolve in place. Each target is a locator ladder: ordered selector strategies with a robustness rationale and a stability class (`semantic`, `identifier`, or `visual`). Resolution walks rungs until one unique match; a hit above index zero is recorded drift, never a silent success. Ambiguity on a rung fails closed.

Business results are first-class. Top-level `outcomes` declare success and business kinds; terminal `conclude` steps bind an outcome id; `branch.otherwise` routes to conclude so “Member not found” is not a hard failure. An `outputs` contract lists typed fields every success path must produce. Declared `recoveries` name read-only interstitial handling (for example dismiss overlay) with a bounded `max`; applied recoveries are journaled.

Overlays compile partial ladder replacements for a tenant without widening policy. Catalog entries carry `draft` / `approved` / `revoked` status and stability signals derived from qualification reports. Unattended replay refuses anything that is not approved.

## Determinism & error handling

The reducer is pure: it consumes recorded events and emits commands. Live replay executes those commands with no model in the loop. Offline `trace replay` reconstructs state from the exported journal without a desktop client or LLM. Input dispatch, later observation, and the independent acceptance verdict remain separate facts.

Failure is typed. Results distinguish success, business outcome, recoverable exhausted, hard failure, cancelled, and outcome unknown. Unknown delivery admits one bounded same-session observation, stays incomplete, and never invents a durable mutation. Cancellation fences future input. Observation retries and predicate waits absorb asynchronous UI without replaying a delivered action. Shared durable admissions and masked frame storage cover both discovery and replay.

## Heterogeneity & multi-tenant

North Cooperative and South Mutual expose the same banking tasks in different native layouts. Semantic ladders survive most renames; when South uses `Find member` instead of `Search`, the base artifact fails dry-resolve and the South overlay restores the ladder. `jarvis drift` reports rung index and ambiguity per target without sending input.

Reviewed profiles select window titles and platforms and may only narrow the hashed policy document. macOS uses AX plus scoped ScreenCaptureKit; Windows and Linux adapters exist in Manvi but live runtime qualification for those OS guests remains deferred or open. Native qualification reports bind executable hashes so a later build cannot quietly inherit an older pass.

## Escalation & handoff

Stuck detection emits a typed `InterventionRequest` (ladder exhausted, not actionable after budget, recoveries exhausted, approval pending, deadline near) with controller metadata exposed on the host. CLI replay supports `takeover`, `act <step>`, and `handback` in the same live session; the workbench keeps the richer timeline and approval UI. Epoch, observation, and program-generation binding reject stale controls.

Assisted recovery remains explicit and narrow: one safe Back/Dismiss while paused, actor `model_recovery`, checkpoint resume required. Matching scores never grant approval. Session-expired bank faults pause for a human passcode, then hand back so the independent contract can assert `human_control_returned`.

## Safety

Policy is data: windows, editable fields, read-only targets, permitted step kinds and effects, sensitive targets, and `unattended_change: false`, hashed into admission and the evidence bundle. Profiles and overlays cannot widen that allowlist. Privacy masks selected regions before image persistence; uncertain capture is withheld. Provider-private Gemini continuation stays under `.local/discovery/`; public exports omit opaque continuation and signatures.

The broker enforces trusted input policy independently of the host. DevCouncil checks independently supplied contract and capability hashes; missing prerequisites yield `incomplete`, never `passed`. A qualification-only saved-state oracle is unavailable to discovery. Credentials never appear in capabilities, repository config, or CLI arguments—only process environment or in-memory workbench fields.

## Cuts

Wayland unattended input, arbitrary inaccessible-canvas OCR, distributed orchestration, and extra runtime frameworks are out of scope. Gemini native `computer_use` is deferred as noted above. Windows runtime qualification is deferred by the user; Linux guest qualification remains a separate gate.

**Verified gaps, not claimed complete:**

- Live Gemini discovery under `evidence/discovery/` has not been run in this checkout; use `docs/live-discovery-runbook.md` with `GEMINI_API_KEY`.
- Phase-6 curated macOS campaign bundles for the new scenarios were blocked by Screen Recording denial to the native helper; historical `evidence/examples/macos-*` bundles remain pre-outcome-field shape.
- Successful human-approved account-changing qualification across tenants is still open.
- Local `dev verify` against OpenRouter-backed DevCouncil critics may be unavailable without network or API access; task closure is therefore partial where the council cannot run.

What is implemented and offline-proven: ladder/outcome/recovery schema and tests in Manvi, policy narrowing and unattended catalog gating in Jarvis, evidence field extensions and fixtures in DevCouncil, fixture discovery (`--provider fixture`, labelled `not live`), drift reporting, bank faults (`permission-denied`, `session-expired`, `overlay`, `missing-control`) and the `renamed-controls` variant, plus independent contract files for the new scenarios.
