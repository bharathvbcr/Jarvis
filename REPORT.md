# Jarvis assignment report

## Architecture

Three owners keep reusable behavior upstream: Manvi controls discovery, deterministic execution, journal privacy and the native broker; DevCouncil independently evaluates evidence; Jarvis composes the bank, tenant profiles, scenarios and native workbench. The Go host remains CGO-free. A separate Rust workspace confines platform FFI to macOS/Windows/Linux adapters. Each interactive desktop has one input owner; independent guest desktops may run concurrently.

## Artifact schema

An application contains capabilities with immutable revisions. Each revision declares typed parameters, semantic or bounded visual targets, ordered bounded steps, explicit business effects and output types. Money uses integer minor units and currency. Visual templates contain exact hashed PNG bytes and geometry; they support approved clicks, not text extraction. Tenant/platform bindings can narrow reviewed input policy. An invocation adds run/session identities, control epoch, independent expected contract and a byte hash of the exact capability. No credentials, executable expressions or arbitrary code appear in capabilities.

## Determinism & error handling

The pure reducer consumes recorded events and emits commands. Live replay executes its commands without model decisions; offline replay reconstructs state without importing a desktop client or provider. Input dispatch, subsequent observation and acceptance verdict remain separate facts. Cancellation fences future input and late responses. Unknown delivery permits a bounded same-session observation, remains incomplete, and stops automatic recovery. The bank survives action cancellation until reconciliation and bounded process cleanup finish. Bounded observation retries and predicate waits handle asynchronous UI updates without replaying a delivered action. Discovery and replay share durable action admissions and masked frame storage.

## Heterogeneity & multi-tenant

North Cooperative and South Mutual present the same canonical banking tasks in distinct native layouts. Semantic role/name locators survive those layout changes. Reviewed tenant profiles select the exact window and supported platforms without widening policy. macOS uses AX and scoped ScreenCaptureKit; Windows uses UI Automation and per-monitor/virtual-desktop handling; Linux targets AT-SPI and X11 first. Native runtime qualification is recorded separately from cross-compilation.

## Escalation & handoff

The workbench exposes typed invocation, structured editing through the canonical compiler, revision history, current desktop ownership, pending approvals, a sanitized live view, timeline frames, accessibility differences, locator refusals and evidence gaps. Human takeover uses the broker in the same live session. Epoch/observation/program-generation binding rejects stale controls. Optional assisted recovery is explicitly enabled per invocation, admits at most one safe Back/Dismiss action and requires checkpoint-based resume. A score cannot replace required human approval.

## Safety

Privacy operates before projections, observers, persisted evidence and provider input. Selected sensitive regions are masked before image persistence; uncertain capture is withheld. Provider-private continuation remains in a separate private journal, while public exports omit it. Approval binds the reviewed action and form state, is single-use, and is revalidated after focus changes. The broker independently applies its trusted input policy. DevCouncil checks independently supplied expectations and missing evidence cannot pass. The qualification oracle checks saved state rather than accepting a successful UI acknowledgment.

## Cuts

Wayland unattended input, arbitrary inaccessible-canvas text extraction, distributed orchestration and extra runtime frameworks are excluded. All six optional features have implemented surfaces; qualification status is tracked explicitly in `docs/requirements-evidence.md`. Live Gemini conformance requires a locally supplied credential; successful account-changing native tests require human approval. Windows runtime execution is deferred by the user. Hardware-input monitoring and arbitrary synthetic-input classification have separate platform limits, documented with native capability probes. These gates are not replaced with fixtures or headless UI tests.

The two optional features highlighted for submission are the native capability catalog with typed invocation, and generated Go callers that delegate to the canonical executor. Matching diagnostics and approval policy, assisted recovery, tenant bindings, and N-run diagnostic reports cover the other four optional tasks. Submission readiness depends on completing the evidence matrix, not on feature names or a successful build alone. Windows runtime qualification was explicitly deferred by the user.
