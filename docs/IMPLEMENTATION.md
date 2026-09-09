# Implementation ledger

The approved design is being implemented in three repositories. This ledger records gates, not inferred completion.

| Gate | Status |
| --- | --- |
| Isolated upstream worktrees | Created |
| Manvi privacy, cancellation, replay and provider regressions | Implemented; focused and race suites passed, final audit running |
| DevCouncil independent evidence evaluator |23 evaluator+9 CLI tests passed; real Go/Rust compatibility passed |
| Rust native broker and platform adapters | macOS native execution verified; ARM Windows/Linux cross-checks passed |
| Deterministic workflow compiler and executor | Implemented and adversarially tested; real macOS live/offline replay passed |
| Rust bank and native workbench | Built;17 bank tests and15 workbench tests passed; rendered qualification pending |
| Live Gemini multi-turn conformance | Pending credential readiness |
| Physical macOS qualification | Baseline40runs retained;20passed/10failed/10incomplete; timing hardening under test |
| Windows ARM64 qualification | User deferred execution; build/qualification instructions prepared |
| Linux ARM64/X11 qualification | UTM guest booted; guest readiness and native execution pending |
| All six optional features | Implemented surfaces; live assisted mode and final workbench integration still require qualification |
| Submission evidence and report | Genuine macOS bundles and baseline report exist; final pins and complete qualification gates remain |

Sources are maintained in their canonical owning repositories. Local integration uses an ignored Go workspace; release integration pins reviewed revisions. Existing shared checkout edits are preserved.

No fixture run, synthetic input event, or build result counts as native execution. A missing check is not a pass. Discovery and live replay use only external desktop APIs. Evaluator-only state inspection is unavailable to the automation tool registry.
