# Publish and verification status

Recorded 2026-09-10 for TASK-010 / `p7-docs-publish`. Superseded for pins: `upstream.lock.json` and `go.mod` hold the current Manvi, DevCouncil and gusset revisions.

## Public pins (verified resolvable)

| Repo | SHA | Notes |
|---|---|---|
| https://github.com/bharathvbcr/Jarvis | `v0.2.0` (`0994985`) | Public release 0.2.0 |
| https://github.com/bharathvbcr/Manvi | `5dbf47c78cc5a8637b222b4dc72d23b2906041f1` | Host module, reducer, and lifecycle API |
| https://github.com/bharathvbcr/DevCouncil | `1ae3bc15a62a3875f9dd827899ba774aa1c50ab0` | `dc-evidence` / `dc-verify` independent verification |
| https://github.com/bharathvbcr/gusset | `984209b542885b10b43e3d9aaf2073fd871e9eb9` | In-process Rust policy matching engine |

Jarvis `go.mod` requires `github.com/bharathvbcr/Manvi/manvi v0.0.0-20260910143105-adeb253a76f2`. Smoke `go get` against that pseudo-version succeeded with `GOPROXY=direct`.

`go run ./cmd/dev check-pins` fails in this developer checkout while Manvi/DevCouncil working trees still have unrelated dirty files. Clean clones at the pinned revisions match.

## DevCouncil tasks (local `.devcouncil` state)

| Task | Status after publish attempt |
|---|---|
| TASK-002-p1-policy-document | verified |
| TASK-001-p1-schema-core | blocked (AC evidence gaps from earlier cross-repo work) |
| TASK-003 … TASK-010 | planned / not closed by council |

`dev verify TASK-010-p7-docs-publish` ran in coarse mode with an empty post-commit diff (`diff_empty: true`), so it could not attribute README/REPORT/go.mod/upstream.lock edits to a leased execution and did not mark the task verified. OpenRouter is unset (`OPENROUTER_SET=no`), so critic/reviewer roles that need that provider cannot run.

Offline Go/Rust suites used for this publish path:

- Manvi: `go test ./workflow/... ./computer/... ./devcouncil/...` (pass)
- DevCouncil: `cargo test --locked -p dc-evidence -p dc-verify` (pass)
- Jarvis: `go test ./internal/app/...` and `GOWORK=off go build ./cmd/jarvis` (pass at the Manvi pin of that run)

Since the gusset pin, Manvi's `go.mod` replaces DevCouncil and gusset with
sibling checkouts, and `replace` applies only to the main module or a
workspace, so `GOWORK=off go build` no longer resolves; build through the
workspace `dev bootstrap` or `dev workspace` writes.

## Remaining user actions

1. **Screen Recording** — grant to Terminal/Cursor and `build/manvi-desktop`, then re-record Phase-6 curated bundles and the reduced 10/tenant campaign (`evidence/README.md`).
2. **Live Gemini discovery** — set `GEMINI_API_KEY` and follow `docs/live-discovery-runbook.md`; copy sanitized output to `evidence/discovery/<RUN_ID>/`.
3. **Optional council close** — with OpenRouter configured, lease and `dev verify` remaining tasks from a clean diff window if a requirement→task coverage report is required for submission.
