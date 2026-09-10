<!-- Managed by dev map: keep this file in sync with .devcouncil/repo_map.json. -->

# Agent Workspace Guide

Use `.devcouncil/repo_map.json` as the primary file index for this workspace.
Repo map: `.devcouncil/repo_map.json`
Code graph: `.devcouncil/graph/code_graph.json` (symbol-level; query with `dev map`).

Workflow for agents:
1. Open `.devcouncil/repo_map.json` before guessing at file locations.
2. Use the `files` list to resolve module ownership and nearby siblings.
3. Use `subsystems` for subsystem-level navigation.
4. In `subsystems`, use `entry_points` + `critical_files` for entry points and starting context.
5. Use `role_files` in `subsystems` for subsystem role buckets (tests, entry, api, models, services, config, docs, other). Each bucket is a capped **sample** for orientation, not an inventory — `role_file_counts` carries the real per-role total, and `files` is the complete list.
6. Use `neighbors` and `handoff_paths` in `subsystems` to follow cross-subsystem flow. `handoff_paths` names the ordered file pairs a subsystem reaches other subsystems through; both are capped, and `liveness_meta.subsystems` reports what was cut.
7. Prefer `dev map dead --confidence extracted` + file greps for dead code. Treat `inferred` as unconfirmed. Prefer `unwired_candidates` / `dead_symbol_candidates` over `unreachable_files` (static BFS is often noisy for routers / dynamic imports / JSX). If `entry_roots` are empty / `liveness_unreachable_unreliable`, ignore `unreachable_files` and mass inferred dead. Check `unwired_candidates` / `dead_symbol_candidates` before creating new modules — wire what you create into a real caller.
8. Prefer DevMap MCP tools (`devmap_explore`, `devmap_impact`, `devmap_trace`, …) over GitNexus. CLI: `dev map query <name>` / `dev map trace <a> <b>` / `dev map dead`; `dev map graph-html` (or `dev map html --symbols`) for the symbol visualizer. The kernel store (`.devcouncil/codeintel/devmap.sqlite`) is canonical — prefer `dev map` / `devmap_*` when `code_graph.json` is missing or a size-capped stub. When DevMap cannot answer, record a gap in `.devcouncil/codeintel/sessions/gaps.jsonl`.
9. Run `dev map` (or `dev map --watch`) after large refactors; `dev map status` / `dev map doctor` report engine, store and freshness.

DevCouncil loop:
- Prefer DevCouncil MCP tools (`devcouncil_status`, `devcouncil_checkout_task`, `devcouncil_verify_task`, …) for task state; do not guess.
- Interactive Shell does not need a lease under assist (`hook_gate.mode=off`); checkout before writes only when write-gates / contain mode are active.
- Engineering skills live under `.claude/skills/`, `.cursor/skills/`, and `.agents/skills/` (`dev skills scaffold` / `dev integrate cursor --apply`).

Important surfaces:
1. `cmd/jarvis/` — cmd/jarvis: 1 file, go (1 entry)
2. `desktop/crates/bank-demo/src/` — desktop/crates/bank-demo/src: 4 files, rust (1 entry)
3. `cmd/qualify/` — cmd/qualify: 9 files, mostly go, markdown (5 entry, 4 tests)
4. `cmd/dev/` — cmd/dev: 3 files, go (2 entry, 1 tests)
5. `tests/native-platform/` — tests/native-platform: 3 files, go (3 tests)
6. `internal/app/` — internal/app: 38 files, mostly go, json (1 entry, 17 tests)

If the map and source disagree, trust the source and regenerate the map.
