# Evidence inventory

The pinned build is recorded in [build-verification.json](build-verification.json).
[macos-pinned-40.json](macos-pinned-40.json) contains 23 successful replays and
17 suspensions after native input-counter changes; all 40 saved-state checks passed.
North passed 14/20 and South 9/20. This campaign is not a clean stability pass.
[macos-pinned-denial.json](macos-pinned-denial.json) reached and denied approval on
South; North suspended before approval. Both saved-state checks passed.
Those 17 suspensions were attributed to bare pointer motion and the guard was
narrowed. [guard-partition/](guard-partition/) records the controlled experiment:
with one variable changed on one host under identical injected pointer motion, the
strict guard cancelled the run and escalated to a human operator while the
partitioned guard completed it and recorded the motion.
[macos-guard-partition-40.json](macos-guard-partition-40.json) is the campaign that
followed — 35/40 passed, all 40 independent saved-state checks unchanged, zero model
requests. It is **not** a clean 40/40 pass. Its four `external_interaction` refusals
are now attributed by category (two `pointer_button`, one `keyboard`, one
`keyboard, pointer_button`) and are all genuine human input during the run; none was
pointer motion. A fifth run stopped on an unsatisfied wait predicate.
The exact current-binary [success bundle](examples/macos-pinned-balance/) and
[input-counter refusal bundle](examples/macos-input-counter-refusal/) were copied
without changing artifact bytes and rechecked with the independent verifier.

Older all-success reports below establish results for their recorded executables.

These reports and bundles record genuine macOS and Linux X11 desktop activity through Manvi's native broker. They are separate from deterministic fixtures, semantic egui tests and cross-compilation. Campaign provenance binds the exact admitted executable bytes. The macOS readiness report records dirty source metadata; Linux records unavailable Git metadata. Later source changes require a fresh build and qualification, even where a historical filename contains `final`.

| Report | Recorded result |
| --- | --- |
| [macos-stability-baseline.json](macos-stability-baseline.json) | Original 40-run campaign: North 3/20 and South 17/20 passed; all 40 saved-state checks unchanged. `macos-stability.json` retains the same baseline. |
| [macos-checkpoint-probe.json](macos-checkpoint-probe.json) | Five of six fresh launches passed after bounded predicate waits; one input was refused after Stage Manager changed window geometry. |
| [macos-stability-hardened.json](macos-stability-hardened.json) | North 20/20 and South 16/20 passed after bounded confirmed-not-sent recovery; capture-window ambiguity remained. |
| [macos-stability-final.json](macos-stability-final.json) | North 20/20 and South 19/20 passed; one startup attachment remained blocked. This is a historical report. |
| [macos-stability-readiness.json](macos-stability-readiness.json) | North 20/20 and South 20/20 passed; all 40 independent saved-state checks unchanged. Forty observation retries, no input retries, zero first-attempt successes and zero model requests/cost. Executables predate later visual/HID and lifecycle changes. |
| [macos-denial-readiness.json](macos-denial-readiness.json) | Both tenants reached actual pending approval and were denied; workflows cancelled and saved seeds remained unchanged. Expected denial behavior passed 2/2; task acceptance stayed blocked/incomplete. The earlier `macos-denial.json` did not reach approval and is not denial qualification. |
| [linux-initial-probe.json](linux-initial-probe.json) | Six real guest attempts blocked during AT-SPI attachment, with all saved-state checks unchanged. The failed baseline is preserved. |
| [linux-x11-final-40.json](linux-x11-final-40.json) | North 20/20 and South 20/20 passed with all 40 saved-state checks unchanged; 39 observation retries, no input retries, one first-attempt success and zero model requests/cost. The guest executables are hash-bound; this does not attest to later rebuilt source. |
| [linux-denial-final.json](linux-denial-final.json) | Both tenants reached actual pending approval and were denied. Expected denial behavior passed 2/2, workflows cancelled, seeds unchanged; task acceptance stayed blocked/incomplete. |
| [linux-overlay-final.json](linux-overlay-final.json) | Overlay observed for both tenants; targets were not actionable, runs cancelled and seeds unchanged. Expected safe behavior passed 2/2; task acceptance remained blocked and verification unavailable. |
| [linux-missing-control-final.json](linux-missing-control-final.json) | Zero matching targets observed for both tenants; runs cancelled and seeds unchanged. Expected safe behavior passed 2/2; task acceptance remained blocked and verification unavailable. |
| [linux-duplicate-control-final.json](linux-duplicate-control-final.json) | Two matching targets observed for both tenants; runs cancelled and seeds unchanged. Expected safe behavior passed 2/2; task acceptance remained blocked and verification unavailable. |
| [linux-refresh-blocked.json](linux-refresh-blocked.json) | Refresh against committed sources stopped before guest startup. UTM failed on the macOS host and the VM remained stopped; 0/40 trials attempted, no guest commands or approvals. The successful native Linux cross-check is recorded separately from runtime execution. |

Additional `macos-*-probe.json` reports and Linux build/probe text logs retain intermediate diagnostics. Raw terminal logs can contain terminal control characters and are not structured success verdicts. A successful safety campaign means its expected refusal/denial occurred; it does not mean an account change was accepted.

| Curated bundle | Scope |
| --- | --- |
| [examples/macos-balance](examples/macos-balance/) | Complete sanitized North run `aa8e64f3844a7a03233c793bf09586df`: output 125000 minor USD and passed independent evidence. Includes exact capability/contract, masked pixels, accessibility snapshots, receipts, trace and Perfetto timeline. |
| [examples/linux-balance](examples/linux-balance/) | Complete sanitized North run `09a8bb6c81691e2e5b8c16d6e275e61d`, trial 1 of the Linux 40-run campaign. Twenty original evidence files copied byte-for-byte; all three evidence criteria passed and seed remained unchanged. Its README records X11/AT-SPI/XTest and renderer readiness. |
| [examples/macos-window-refusal](examples/macos-window-refusal/) | Preserved South run `5c25ec24dd665d2febe30710061ad87e`: geometry changed from 880×732 to 102×125 before dispatch. The failure predates bounded confirmed-not-sent recovery and remains unchanged. |
| [examples/macos-visual-no-grant](examples/macos-visual-no-grant/) | Genuine protected crop creation and unique native visual matching in a 1760×1464 bank frame. An unapproved visual click returned `approval_required/not_sent`; saved state was unchanged. No human grant or live click was exercised. |

Curated observations contain only the synthetic bank. Member-input pixels are masked and semantic values redacted before persistence. Original artifact bytes remain unchanged when copied; verify each bundle against its hashes. Provider-private continuation and local credentials are excluded. Checksums establish consistency of supplied bytes, not truthful observation independently of the trusted coordinator and native workers.

Offline playback must use the capability **inside its bundle**, because current examples can contain a newer revision:

```text
./build/jarvis trace replay --capability evidence/examples/macos-balance/capability.json --trace evidence/examples/macos-balance/trace.json
./build/jarvis trace replay --capability evidence/examples/linux-balance/capability.json --trace evidence/examples/linux-balance/trace.json
```

No live Gemini discovery/save-restore bundle, human-approved successful account-change bundle, live false-ack/uncertain-delivery mutation bundle or approved visual-click bundle has been produced. Windows runtime execution is deferred by the user. See the [requirement matrix](../docs/requirements-evidence.md) for remaining gates; no missing check is a pass.
