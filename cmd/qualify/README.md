# Native qualification

Run this command from the Jarvis root after the normal build. The default is a
read-only balance campaign with a fresh synthetic bank process for each trial:

```sh
go run ./cmd/qualify --n 20 --out evidence/platform-stability.json
```

Existing report paths are refused. `--tenants north` or `south` selects a partial
matrix; `--pid` requires one tenant, `--oracle-state`, and the read-only balance
scenario. Attached-process runs never become a full fresh-process qualification.
Linux requires the logged-in X11 session. macOS permissions must belong to the
actual process launch. Permission refusal, absent state, and unfinished bounded
cancellation remain blocked; they do not count as success.

## Explicit human approval

Creation and post-commit faults need a human at a terminal:

```sh
go run ./cmd/qualify --scenario creation --interactive --n 1 --tenants north \
  --run-timeout 3m --out evidence/creation-north.json
go run ./cmd/qualify --scenario commit-noop --interactive --n 1 --tenants north \
  --run-timeout 3m --out evidence/commit-noop-north.json
go run ./cmd/qualify --scenario crash-after-commit --interactive --n 1 --tenants south \
  --run-timeout 3m --out evidence/crash-after-commit-south.json
```

The bank uses member `M-1001` and savings subaccount name `Qualification` in every
fresh trial. The terminal prints the requested inputs, captured form, target,
run/session/action/observation IDs, epoch and a SHA-256 binding. Sensitive captured
values remain redacted: inspect the visible native bank form and compare it with
the requested synthetic inputs. Type the entire displayed `approve …` command
to authorize exactly that checkpoint. `deny` sends a denial; `cancel`, EOF and
the run timeout cause bounded cancellation. Cleanup has an additional eight
second bound. Piped input is refused in interactive mode.

No approval is sent because a flag was selected, a line contained only `approve`,
or an earlier checkpoint was approved. The qualifier re-reads the coordinator
after input; changed form, epoch, observation or action ID discards that approval.
The broker independently focuses and revalidates before native input. A safe
pre-dispatch retry gets a different action-attempt ID and needs another explicit
approval. The qualifier does not use an API shortcut to mutate bank state.

Without `--interactive`, every risky action is denied, including a requested
creation/fault campaign. The `denial` scenario always denies. Overlay, missing
and duplicate controls are observed and then safely cancelled; this command does
not automatically dismiss overlays or resolve ambiguous targets.

## Independent outcomes

The saved-state oracle is private to this executable and unavailable to discovery
or automation tools. It checks the version/tenant/revision, both exact members,
their names/currencies/balances, and every subaccount. Starting balances must
remain `125000` and `84050` minor units. Creation requires exactly one Savings
account named `Qualification` for `M-1001`, with member `M-1002` unchanged.

Reports keep native workflow outcome separate from expected fault behavior:

| Scenario | Required independent result |
| --- | --- |
| `balance` | Completed workflow, passed evidence and unchanged saved seed |
| `creation` | Explicit approval, completed/passed workflow and exactly one saved creation |
| `denial` | Actual denial, cancelled workflow, no acceptance pass and unchanged saved seed |
| `delay` | Delay actually observed, explicit approval, completed creation and exact saved state |
| `overlay`, `missing-control`, `duplicate-control` | Fault visible in a complete observation, no approval, safe termination and unchanged seed |
| `commit-noop` | Approved input was dispatched, the same action's post-observation shows no completion, no acceptance pass and no saved write |
| `crash-after-commit` | Approved input was dispatched, exactly one durable creation, and terminal `outcome_unknown` with no acceptance pass |

`expected_behavior_verified` can be true for a caught no-op or correctly unknown
post-commit outcome; it never changes that run's failed/incomplete acceptance into
passed. `campaign_passed` means every requested trial verified its expected
behavior. `qualified` is narrower: both tenants, fresh processes, at least twenty
balance trials each, and complete passing workflow/evidence/oracle results.

Observation retries and proven-not-sent input retries are separate counts. A
first-attempt success requires both to be zero. The one-sided 95% bound describes
the observed acceptance count under an independence assumption; repeated trials
in this controlled setup do not establish independent production reliability.

## Reproducibility and checks

Schema-3 reports include exact path, resolved path, size and SHA-256 bindings for
capability, acceptance contract, broker, bank, verifier and this executable.
Bindings are checked before each trial; replacing an artifact stops later trials.
Git revision and clean/dirty/unavailable source status are recorded when possible.
A dirty source tree is allowed. Source revision is context; the executable hashes
identify what was actually admitted. These hashes do not authenticate artifacts
against a malicious writer racing the trusted local workspace.

```sh
go test ./cmd/qualify
go test -race ./cmd/qualify
go vet ./cmd/qualify
```

The tests simulate approval input and coordinator state to attack stale grants,
then exercise independent files for oracle and artifact-binding failures. They
do not claim a person approved a real creation, nor establish macOS/Windows/Linux
physical execution. Interactive creation/no-op/crash campaigns must be run with
the actual human and native target before those platform gates can be marked met.
