# Jarvis Specifications

This directory holds Jarvis's 7-layer specifications (`spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/`, `tasks.md`, `quickstart.md`, `verification.md`), validated by `devcouncil spec validate --strict`. Jarvis has no repo-local principles document, so the plans judge it against DevCouncil's `constitution.md` and mark inapplicable articles N/A.

## Index

| ID | Feature | Scope | Status |
| --- | --- | --- | --- |
| `001` | [Agent Operation Gate, Host Module, and Background Jobs](001-agent-operation-gate-and-host-module/spec.md) | `internal/app/agent.go` default-deny op classification, yolo-mode attribution, `jarvis.*` host module, bounded background jobs, inputs profiles (Vani). | Active (shipped; T-009 open: how a Manvi agent loads Jarvis) |
| `002` | [Discovery, Deterministic Replay, Independent Verification, and Pinned Upstreams](002-discovery-replay-and-independent-verification/spec.md) | Gemini discovery and the $25 soft-cap campaign ledger, offline trace replay, held-out-lock verification, `cmd/qualify` campaigns, `cmd/dev` pin checks. | Active (shipped; T-010 to T-012 open: live and human-approved qualification) |

## Validation

Run from the repository root:

```bash
go run ./cmd/dev check-specs   # devcouncil spec validate --all --strict
```

The command fails loudly when the `devcouncil` binary is missing. It is deliberately not part of `dev test` or the GitHub Actions workflow, because CI runners do not carry the binary.

## Cross-Repository Boundary Reference

- Manvi provides the host plane, workflow compiler, desktop broker and spend ledger (`/Users/bharath/Code/devtools/Manvi/specs/`).
- DevCouncil provides the independent acceptance engine (`dcverify`, `dc-evidence`).
- Pins for all three upstreams are in `upstream.lock.json`.
