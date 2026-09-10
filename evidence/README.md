# Evidence inventory

These are genuine local macOS bank runs, captured through the native desktop
broker. They are separate from deterministic fixtures and semantic UI tests.

## Historical 40-run campaigns (keep as documents)

- `macos-stability-baseline.json`: original40-run campaign, North3/20 and
  South17/20 passed; all40 independent saved-state checks remained unchanged.
- `macos-checkpoint-probe.json`: six fresh launches after bounded predicate waits,
  five passed; one refused input after a Stage Manager geometry change.
- `macos-stability-hardened.json`: North20/20 and South16/20 after bounded
  confirmed-not-sent recovery; capture-window ambiguity remained.
- `macos-stability-final.json`: North20/20 and South19/20 after capture retry
  correction; one startup attachment remained blocked. The historical filename
  does not make this the final source revision.
- `macos-stability-readiness.json`: North20/20 and South20/20 after bounded
  attachment readiness. All40 evidence/oracle checks passed, model requests and
  cost were zero. All runs needed observation retries, so first-attempt success
  remained zero. This report identifies its exact pre-visual-anchor executables.
- `macos-denial-readiness.json`: both tenants reached actual approval and were
  denied; workflows cancelled and saved state remained unchanged. These are
  successful denial scenarios, not accepted account changes.
- `linux-initial-probe.json`: six real guest attempts blocked during AT-SPI
  attachment, all saved-state oracles unchanged. Later Linux qualification is
  tracked separately; this baseline remains intact.

## Curated example bundles

- `examples/macos-balance`: complete, sanitized North run
  `aa8e64f3844a7a03233c793bf09586df`, output125000minorUSD; independent verifier
  passed against its **embedded** contract/capability (pre-outcome-field shape).
  Kept as historical curated evidence until re-recorded under the Phase-6 schema.
- `examples/macos-window-refusal`: preserved South run
  `5c25ec24dd665d2febe30710061ad87e`; geometry changed from880×732 to102×125
  before dispatch. Historical failure evidence; unchanged.
- `examples/linux-balance`, `examples/macos-visual-no-grant`: historical platform
  samples; unchanged.

## Phase 6 scenarios (contracts ready; live re-record blocked 2026-09-10)

New independent contracts under `scenarios/`:

| Scenario | Contract | Inputs / flags | Expected |
|---|---|---|---|
| success (existing) | `balance-m1001.contract.json` | `m1001.inputs.json` | `outcome.id=success`, balance 125000 |
| not_found | `balance-m9999.contract.json` | `m9999.inputs.json` | `outcome.id=not_found` |
| permission_denied | `balance-permission-denied.contract.json` | `m1001.inputs.json --fault permission-denied` | `outcome.id=permission_denied` |
| overlay recovery | `balance-overlay.contract.json` | `m1001.inputs.json --fault overlay` | success after `dismiss_overlay` |
| south tenant overlay | `balance-south-overlay.contract.json` | `m1001.inputs.json --tenant south --variant renamed-controls` | success via Find member overlay |
| hard failure | `missing-control.contract.json` | `subaccount.inputs.json --fault missing-control` | success contract incomplete/failed |
| session handoff | `subaccount-session-expired.contract.json` | `subaccount.inputs.json --fault session-expired` | `human_control_returned=true` + created |

**Blocker verified:** live `jarvis replay` and `jarvis drift` fail with
`capture_permission: Screen Recording permission is not granted to the native helper`.
Grant Screen Recording (and Accessibility) to `build/manvi-desktop` / Terminal /
Cursor, then re-run the commands below. Do not invent substitute bundles.

### Exact re-record commands (after permissions)

```text
go run ./cmd/dev build --manvi /Users/bharath/Code/devtools/Manvi --devcouncil /Users/bharath/Code/devtools/DevCouncil

# success → copy evidence/private/RUN_ID → evidence/examples/macos-balance
./build/jarvis replay --capability examples/balance.json \
  --contract scenarios/balance-m1001.contract.json --inputs scenarios/m1001.inputs.json --tenant north

# not_found → evidence/examples/macos-balance-m9999
./build/jarvis replay --capability examples/balance.json \
  --contract scenarios/balance-m9999.contract.json --inputs scenarios/m9999.inputs.json --tenant north

# permission_denied → evidence/examples/macos-balance-permission-denied
./build/jarvis replay --capability examples/balance.json \
  --contract scenarios/balance-permission-denied.contract.json --inputs scenarios/m1001.inputs.json \
  --tenant north --fault permission-denied

# overlay recovery → evidence/examples/macos-balance-overlay
./build/jarvis replay --capability examples/balance.json \
  --contract scenarios/balance-overlay.contract.json --inputs scenarios/m1001.inputs.json \
  --tenant north --fault overlay

# south renamed-controls + overlay → evidence/examples/macos-balance-south-overlay
./build/jarvis replay --capability examples/balance.json \
  --contract scenarios/balance-south-overlay.contract.json --inputs scenarios/m1001.inputs.json \
  --tenant south --variant renamed-controls

# hard failure missing-control → evidence/examples/macos-missing-control
./build/jarvis replay --capability examples/create-subaccount.json \
  --contract scenarios/missing-control.contract.json --inputs scenarios/subaccount.inputs.json \
  --tenant north --fault missing-control

# session-expired handoff (CLI): when paused, type:
#   takeover
#   set_value Passcode BRANCH-7741
#   press Continue
#   handback
# then approve Confirm creation when prompted
./build/jarvis replay --capability examples/create-subaccount.json \
  --contract scenarios/subaccount-session-expired.contract.json --inputs scenarios/subaccount.inputs.json \
  --tenant north --fault session-expired
```

### Reduced 10/tenant campaign (blocked until Screen Recording)

```text
go run ./cmd/qualify --n 10 --tenants north,south --scenario balance \
  --out evidence/macos-phase6-balance-10x2.json
```

Offline checks already run for this phase: capability compile, scenario contract
decode, south overlay profile bind, and `dcverify evidence-check` on DevCouncil
`contract-not-found` / `bundle-not-found` plus the historical `examples/macos-balance`
bundle (legacy contract inside the bundle still passes under additive fields).

The curated bundles contain only synthetic-bank observations. Member input is
masked in PNGs and redacted in semantic data before persistence. Exact artifact
bytes remain unchanged when copied here; verify against each bundle's hashes.
Provider-private continuation and local credentials are excluded.

Run offline playback with the capability **inside the bundle**, since current
examples can have newer revisions:

```text
./build/jarvis trace replay --capability evidence/examples/macos-balance/capability.json --trace evidence/examples/macos-balance/trace.json
```

No live Gemini discovery bundle or human-approved account-change bundle has been
produced yet. Do not interpret their absence as successful qualification. Windows
execution is deferred by the user; Linux guest execution remains a separate gate.
