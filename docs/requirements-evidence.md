# Requirement to evidence matrix

Status vocabulary: **verified** means the named check ran; **implemented, unqualified** means code exists but the required live environment has not proved it; **open** means work or external setup remains. Current native evidence must be read together with its campaign and source revision.

| Requirement | Implementation owner | Evidence / remaining gate |
|---|---|---|
| Rust/Go first-party code | All three workspaces | Go/Rust source and build commands; no first-party Python/JS runtime |
| Native bank, two tenant layouts | Jarvis `desktop/crates/bank-demo` | 17 state/semantic tests verified; actual macOS controls and balance observed |
| Real discovery | Manvi loop + Jarvis scoped registry | Implemented, unqualified: Gemini credential and live multimodal/save-restore conformance |
| Immutable capability/compiler | Manvi `workflow` | Type/dataflow/forward-control-flow and mutation tests; exact-byte hashes |
| Live model-free replay | Manvi `computer` + `workflow` | macOS run `158b79ef3ae80a9e25bf809c1cc6ee0e` completed, balance125000USD, DevCouncil passed |
| Offline trace replay | Exported workflow reducer | Same run reconstructs exact final state; Go dependency boundary check verified |
| Independent acceptance | DevCouncil `dc-evidence` | 23 evaluator +9 CLI tests; live Go→Rust resumed/wrong-run fixture check verified |
| Saved-state oracle | Jarvis qualification CLI | Baseline40/40 bank state remained unchanged; mutation detection test; successful creation oracle remains open |
| Privacy across warm/cold/restored history | Manvi session | Escaped-string, alias, image, opaque continuation and restoration regressions |
| Responsive cancellation | Manvi registry/runner/native helpers | Cancelled admission, approval race, stale epoch, interrupted helper tests; host blocked-pipe review in progress |
| Human approval/takeover | Native broker + workbench | Implemented; successful human-approved creation on each OS/tenant remains open |
| Workbench and visual editor | Jarvis Rust workbench | Native build and semantic tests; final rendered/live control QA required |
| Optional1: catalog/invocation | Manvi catalog + workbench | Immutable storage, typed forms and promotion wiring; integration tests |
| Optional2: generated Go caller | Manvi catalog codegen | Embeds exact artifact and calls canonical executor; generated-package compile gate |
| Optional3: matching diagnostics/approval | Native resolve + workbench | Unique locator/actionability refusals, competing matches, state-diff diagnostics; score never bypasses approval |
| Optional4: one assisted recovery | Manvi runner + scoped Gemini tool | Once-per-run paused-only Back/Dismiss tests; live credentialed scenario open |
| Optional5: tenant/platform bindings | Jarvis `profiles` | Exact title/platform and subset policy checks; macOS both layouts; Windows/Linux live gates |
| Optional6: stability reports | Jarvis qualification CLI | First campaign20runs/tenant: North3passed, South17passed; retained baseline failures; hardening rerun pending |
| macOS capture/input | Manvi native | AX SetValue and Press +1760×1464 scoped capture verified on macOS27arm64 |
| Windows/Linux adapters | Manvi native | ARM64 cargo-check verified; runtime VM qualification open |
| $25 campaign accounting | Manvi `llm/budget` | Five ledger tests and per-HTTP-attempt transport tests; unknown reservations retained |
| Assignment package | Jarvis docs/evidence/build | README, required REPORT headings, baseline bundle and reproducible build command; final pins and qualification remain open |

The baseline runs are real external desktop operations. Headless egui tests, mocked workflow tests and imported verifier fixtures are distinct evidence classes. They do not establish native execution or live provider behavior.

Adversarial coverage still requiring physical qualification includes mixed DPI/negative origins, same-process windows, overlays, permission revocation, external interference, guest crashes and uncertain account-change delivery. Keep blocked or unexamined cases visible in the final report.
