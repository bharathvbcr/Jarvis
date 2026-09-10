# Live discovery runbook

One instrumented command per stage for the end-of-project live Gemini discovery
run. Fixture rehearsal (`--provider fixture`) is labelled `not live` and does
not satisfy this runbook. Do not put credentials in capabilities, command
arguments, repository files, or chat — use the workbench memory field or
`GEMINI_API_KEY` in the process environment.

Sanitized evidence destination for a completed live run:

```text
evidence/discovery/<RUN_ID>/
```

Copy only the public, scrubbed artifacts listed in stage 6. Private Gemini
continuation under `.local/discovery/` is never a submission artifact.

## Stage 0 — Build and doctor

```sh
go run ./cmd/dev build --manvi /Users/bharath/Code/devtools/Manvi --devcouncil /Users/bharath/Code/devtools/DevCouncil
./build/jarvis doctor
```

Must prove: binaries exist under `build/`, accessibility and selected-window
capture are available in this launch environment. Credential presence alone is
not proof of provider access.

## Stage 1 — Offline fixture rehearsal (optional, not live)

```sh
./build/jarvis discover --provider fixture --tenant north \
  --task 'Observe the bank, find the member using the member_id parameter reference, search, extract Balance as USD money, declare outcomes/outputs/ladder, and publish bank.balance at revision 1.'
```

Must prove: discovery path loads Manvi's recorded tool transcript without
`GEMINI_API_KEY`; `evidence/private/<RUN_ID>/discovery.json` carries
`"provider":"fixture"`, `"live":false`, `"evidence_class":"not live"`. This
stage never counts as live discovery qualification.

Default transcript:
`internal/app/testdata/discovery/balance-tool-transcript.json`
(override with `--fixture PATH`).

## Stage 2 — Live Gemini discovery

```sh
./build/jarvis discover --provider gemini --tenant north \
  --task 'Observe the bank, find the member using the member_id parameter reference, search, extract Balance as USD money, declare outcomes outputs and ladder rationale, and publish bank.balance at revision 1.'
```

Must prove, in order:

1. Scoped screenshot via `desktop_observe`
2. Actual model tool decisions for `desktop_step` (ladder rationale + stability)
3. Admitted and dispatched native execution
4. Fresh screenshot returned as the tool result
5. `capability_publish` with `outputs`, `outcomes`, targets that carry
   `strategies` / `rationale` / `stability`, and conclude routing
   (`schema_version` stays `1`)
6. `discovery.json` with `"provider":"gemini"`, `"live":true`,
   `"evidence_class":"live"`, model `gemini-3.8-flash`, prompt revision
   `jarvis-desktop-v2`

Note the printed `run <RUN_ID>`. Private journal:
`.local/discovery/<RUN_ID>/session.json`.

## Stage 3 — Optional live continuation

```sh
./build/jarvis discover --provider gemini --tenant north \
  --resume-session .local/discovery/<RUN_ID>/session.json \
  --task 'Continue from a fresh scoped observation. Look up the same member again and publish bank.balance_continued at revision 1 after extracting the balance.'
```

Must prove: a new live provider request after restoration and another native
observe → step → result cycle. Local deserialization alone is insufficient.

## Stage 4 — Register and independent replay

```sh
./build/jarvis compile --capability evidence/private/<RUN_ID>/discovered-capability.json --register
./build/jarvis replay --capability evidence/private/<RUN_ID>/discovered-capability.json \
  --tenant north \
  --inputs scenarios/m1001.inputs.json \
  --contract scenarios/balance-m1001.contract.json
```

Must prove: zero model requests during replay; independent contract evaluation
runs against the fresh bundle; saved-state oracle (qualification harness) still
matches when you run it separately.

## Stage 5 — Verify the replay bundle

```sh
./build/jarvis verify \
  --contract scenarios/balance-m1001.contract.json \
  --bundle evidence/private/<REPLAY_RUN_ID>/bundle.json \
  --contract-sha256 <EXPECTED_CONTRACT_HASH> \
  --capability-sha256 <EXPECTED_CAPABILITY_HASH> \
  --run-id <REPLAY_RUN_ID> \
  --session-id <EXPECTED_SESSION_ID> \
  --epoch 1
```

Must prove: DevCouncil `dcverify evidence-check` verdict is `passed` with
independently supplied identities. Missing prerequisites remain `incomplete`,
never a silent pass.

## Stage 6 — Sanitize into `evidence/discovery/<RUN_ID>`

```sh
mkdir -p evidence/discovery/<RUN_ID>
cp evidence/private/<RUN_ID>/discovered-capability.json evidence/discovery/<RUN_ID>/capability.json
cp evidence/private/<RUN_ID>/discovery.json evidence/discovery/<RUN_ID>/discovery.json
cp evidence/private/<RUN_ID>/model-session.json evidence/discovery/<RUN_ID>/model-session.json
cp evidence/private/<RUN_ID>/desktop-artifacts.json evidence/discovery/<RUN_ID>/desktop-artifacts.json
# Also copy masked frames / journal excerpts only after confirming redaction.
```

Must prove: no raw member IDs, no API keys, no provider-private continuation or
thought signatures; `discovery.json` still says `"evidence_class":"live"`;
capability bytes still compile with outcomes, outputs, ladder targets, and
`schema_version: 1`.

Retain exact model/prompt revision, usage and ledger snapshot, capability
digest, and the independent replay verification report alongside the sanitized
tree.
