# Publish and verification status

Recorded 2026-09-10 for TASK-010 / `p7-docs-publish`.

## Public pins (verified resolvable)

| Repo | SHA | Notes |
|---|---|---|
| https://github.com/bharathvbcr/Jarvis | `e816e5a1e03f1d89c51506e4862ec8abbabcd0ea` (`main`) | Public; Assignment PDF, `.local/`, `evidence/private/` excluded by `.gitignore` |
| https://github.com/bharathvbcr/Manvi | `adeb253a76f20c279b820f63cc340743bec25ef2` | Schema/runner/catalog/evidence Go changes pushed; unrelated local dirty files remain uncommitted |
| https://github.com/bharathvbcr/DevCouncil | `5818ac55e51ef4209d305758fa9034ceb99faee5` | `dc-evidence` / `dc-verify` / protocol only; local `rust-port` dirty tree left alone |

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
- Jarvis: `go test ./internal/app/...` and `GOWORK=off go build ./cmd/jarvis` (pass)

## Remaining user actions

1. **Screen Recording** — grant to Terminal/Cursor and `build/manvi-desktop`, then re-record Phase-6 curated bundles and the reduced 10/tenant campaign (`evidence/README.md`).
2. **Live Gemini discovery** — set `GEMINI_API_KEY` and follow `docs/live-discovery-runbook.md`; copy sanitized output to `evidence/discovery/<RUN_ID>/`.
3. **Optional council close** — with OpenRouter configured, lease and `dev verify` remaining tasks from a clean diff window if a requirement→task coverage report is required for submission.
