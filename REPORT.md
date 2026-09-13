# Jarvis assignment report

## Architecture

Three owners keep reusable behavior upstream: Manvi controls discovery, deterministic execution, journal privacy and the native broker; DevCouncil independently evaluates evidence; Jarvis composes the bank, tenant profiles, scenarios and native workbench. The Go host remains CGO-free. A separate Rust workspace confines platform FFI to macOS/Windows/Linux adapters. Each interactive desktop has one input owner; independent guest desktops may run concurrently.

## Artifact schema

An application contains capabilities with immutable revisions. Each revision declares typed parameters, semantic or bounded visual targets, ordered bounded steps, explicit business effects and output types. Money uses integer minor units and currency. Visual templates contain exact hashed PNG bytes and geometry; they support approved clicks, not text extraction. Tenant/platform bindings can narrow reviewed input policy. An invocation adds run/session identities, control epoch, independent expected contract and a byte hash of the exact capability. No credentials, executable expressions or arbitrary code appear in capabilities.

## Determinism & error handling

Recorded state has to survive serialization exactly, because replay compares a decoded
trace against a recomputed canonical admission. `workflow.State` briefly did not:
`NewState` allocated an `omitempty` map eagerly, so encoding dropped it and decoding
returned nil, and offline replay rejected its own valid journals. The fix is at the
canonical owner, with a reflective invariant test that fails any `omitempty` map or
slice allocated empty. `check-pins` now also refuses a `go.mod` that names a different
Manvi revision than `upstream.lock.json`, because the Go host and the native Rust
workspace had drifted onto different revisions of the same dependency.

The pure reducer consumes recorded events and emits commands. Live replay executes its commands without model decisions; offline replay reconstructs state without importing a desktop client or provider. Input dispatch, subsequent observation and acceptance verdict remain separate facts. Cancellation fences future input and late responses. Unknown delivery permits a bounded same-session observation, remains incomplete, and stops automatic recovery. The bank survives action cancellation until reconciliation and bounded process cleanup finish. Bounded observation retries and predicate waits handle asynchronous UI updates without replaying a delivered action. Discovery and replay share durable action admissions and masked frame storage.

## Heterogeneity & multi-tenant

North Cooperative and South Mutual present the same canonical banking tasks in distinct native layouts. Reviewed tenant profiles select the exact window and supported platforms without widening policy. macOS uses AX and scoped ScreenCaptureKit; Windows uses UI Automation and per-monitor/virtual-desktop handling; Linux uses AT-SPI/X11, with checked XTest input for editable fields when semantic setting is unavailable. Genuine read-only campaigns completed 20/20 runs per tenant on macOS and Linux, with unchanged saved-state oracles and zero model requests/cost. Observation retries meant first-attempt success was 0/40 on macOS and 1/40 on Linux. These reports bind historical executables, not later source rebuilds. Windows runtime qualification is deferred by the user.

## Escalation & handoff

The workbench exposes typed invocation, structured editing through the canonical compiler, revision history, current desktop ownership, pending approvals, a sanitized live view, timeline frames, accessibility differences, locator refusals and evidence gaps. Human takeover uses the broker in the same live session. Epoch/observation/program-generation binding rejects stale controls. Optional assisted recovery is explicitly enabled per invocation, admits at most one safe Back/Dismiss action and requires checkpoint-based resume. A score cannot replace required human approval.

## Safety

Privacy operates before projections, observers, persisted evidence and provider input. Selected sensitive regions are masked before image persistence; uncertain capture is withheld. Provider-private continuation remains in a separate private journal, while public exports omit it. Approval binds the reviewed action and form state, is single-use, and is revalidated after focus changes. The broker independently applies its trusted input policy. DevCouncil checks independently supplied expectations and missing evidence cannot pass. The qualification oracle checks saved state independently of UI acknowledgment. The bank's `false-ack` fault displays real success without writing state; semantic tests verify both tenants and unchanged exact file bytes. The qualifier keeps UI evidence PASS separate from the failed durable task outcome; its live human-approved campaign remains open. Native denial passed its expected safety behavior for both tenants on macOS/Linux; Linux overlay, missing-control and duplicate-control scenarios each safely terminated for both tenants without accepted account changes.

## Cuts

The pinned build, consolidated tests and offline source-bundle restoration passed.
Its fresh macOS campaign passed 23/40, with 17 input-counter suspensions and all 40
saved-state checks unchanged. A controlled experiment attributed those suspensions to
bare pointer motion: the admission guard demanded exact equality across fifteen
`HIDSystemState` counters, `MouseMoved` among them, so any person touching the mouse
aborted a valid run. The guard now partitions counters by what the input behind them
can commit — buttons, keys, modifiers, scroll and drags still refuse; bare motion is
tolerated on element-addressed accessibility work, recorded as `pointer_motion`, and
still revalidated before dispatch — while coordinate-addressed input keeps the strict
contract. The campaign that followed passed 35/40 with all 40 saved-state checks
unchanged and every refusal attributed by input category. It does not pass clean
stability qualification.
The Linux refresh is blocked before guest startup by a UTM crash. Earlier 40/40
campaigns remain valid for their recorded binaries. The stable workbench still
reports Accessibility and Screen Recording denied under its own application identity.

Wayland unattended input, arbitrary inaccessible-canvas text extraction, distributed orchestration and extra runtime frameworks are excluded. All six optional features have implemented surfaces; qualification status is tracked in [the requirement matrix](docs/requirements-evidence.md). Live Gemini discovery/save-restore and assisted recovery require a locally supplied credential. Successful creation, false acknowledgment, uncertain post-commit delivery, completed takeover and approved visual clicks still require genuine native qualification. macOS has a bounded HID-counter guard, but hardware-versus-injected classification is unqualified; continuous monitoring and Windows/X11 external-input listeners are not implemented. Packaging does not complete those acceptance gates. Fixtures and semantic UI tests do not replace these checks.

The two optional features highlighted for submission are the native capability catalog with typed invocation, and generated Go callers that delegate to the canonical executor. Matching diagnostics and approval policy, assisted recovery, tenant bindings, and N-run diagnostic reports provide the other four source surfaces. The Linux refresh is currently blocked before guest startup by a recorded UTM host failure, with zero new trials attempted. The packaged workbench requires its own Accessibility and Screen Recording grants. Submission readiness depends on completing the evidence matrix, not on feature names or a successful build alone.
