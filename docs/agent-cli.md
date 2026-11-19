# Agent CLI

Agents drive Jarvis from a shell. They do not need an MCP server or a
long-lived `jarvis serve` pipe. Two shapes cover this:

- **`jarvis ops` and `jarvis call`** run one host-plane operation per process.
  They use the same `serve` server and handlers the workbench reaches over
  stdio, so no behaviour is implemented twice.
- **`jarvis replay --events` and `jarvis discover --events`** run a workflow
  unattended. They stream NDJSON and stop at the first human gate.
- **Yolo mode** (`--yolo`, or `JARVIS_AGENT_MODE=yolo` in the agent's
  environment) lets an agent operate Jarvis as a person would. It answers gates
  on stdin, acts, takes over, promotes, and stores inputs. Every run and
  promotion it makes is recorded as the agent's. See [Yolo mode](#yolo-mode).

Code: `cmd/jarvis/agent.go`, `cmd/jarvis/watch.go`, and `internal/app/agent.go`
(the classification table and the gate), and `internal/app/operator.go`
(agent attribution).

## `jarvis ops`

```sh
jarvis ops [--root DIR]
```

This lists the operations this build serves, as reported by the server's
`hello`, with each one's class:

| Class | Meaning | Can `call` it? |
|---|---|---|
| `agent` | Reads, compiles or diagnoses from disk. Not strictly read-only: `inputs.list-keys` appends an audit line to `.local/inputs/serve.log`, and `capability.compile` with `"register":true` adds a draft catalog revision, as `jarvis compile --register` already does. | yes |
| `human-only` | Approves, acts, takes over, promotes, sets a credential, reads or writes a sensitive input, or exports an existing run (`jarvis.evidence.export`) | never: `E_AGENT_REFUSED` |
| `process-scoped` | Addresses a run or job held in the memory of the process that started it | no: `E_PROCESS_SCOPED`. Use `--events` |
| `refused` | A Manvi host-plane built-in (`policy.*`, `chat.*`, …) or an unclassified op | no: use the `manvi` CLI |

A `jarvis.*` op added without a classification is refused. It also fails
`TestEveryServedJarvisOpIsClassified`.

## `jarvis call`

```sh
jarvis call OP [--params JSON|@FILE|-] [--root DIR] [--timeout 35s]
jarvis call jarvis.capability.read --params '{"path":"examples/balance.json"}'
jarvis call jarvis.trace.replay --params @replay.json
jarvis call jarvis.doctor
```

stdout is exactly one JSON line, the host plane's response envelope:
`{"id":"call","ok":true,"result":…}` or
`{"id":"call","ok":false,"error":{"code":…,"message":…}}`.

- **Synchronous ops** write the server's own bytes unchanged.
- **Background ops** (`jarvis.doctor`) are polled with `jarvis.job.get` until
  they end. Their `result` is the operation's result, not the job record.
- **`--timeout`** bounds the whole call, polling included. It must be at most
  10 minutes.

The admit check enforces these limits:

- **`run.control`:** the `approve` and `resume` kinds are refused as
  human-only. They are checked before process scope, so a later transport that
  can reach live runs cannot let them through.
- **Path params** (`path`, `capability_path`), matched case-insensitively as
  `encoding/json` matches them:
  - They must resolve through symlinks to an existing file inside `--root`.
  - They must not resolve under `.local/`, which holds sensitive inputs,
    provider-private discovery sessions and promotion records.
- **`jarvis.trace.replay`** takes params only from `@FILE` or `-` (stdin), so
  input values stay out of argv and shell history.

## `--events` on replay and discover

```sh
jarvis replay --events --capability examples/balance.json --tenant north
```

stdout carries one JSON object per line. stderr keeps its human-readable lines.

| `event` | Fields |
|---|---|
| `started` | `run_id` |
| `phase` | `phase`, `epoch`, `controller`, `reason`, on every phase change |
| `needs_human` | `phase` (`awaiting_approval` or `paused`), `action_id`, `observation_id`, `epoch` |
| `finished` | `phase` and `run`, the final run view (the shape `jarvis.run.get` returns) |

In `--events` mode stdin is not read. At the first human gate the run is
cancelled at the gate's epoch, the run finishes, and the command exits 4. If
the cancelled run has not finished within 30 seconds, the command still exits
4. To approve, rerun at a terminal without `--events`, or use the workbench.
SIGINT cancels the run as before.

## Yolo mode

Yolo is the owner's choice to let agents drive Jarvis end to end. It is off by
default.

**Turning it on.** Pass `--yolo` to `call`, `ops`, `replay` or `discover`, or
set `JARVIS_AGENT_MODE=yolo` in the agent's environment.

- Any value other than `yolo`, `default` or empty is a usage error (exit 2).
- Only `cmd/jarvis` reads the mode. `jarvis serve` (the workbench host),
  `cmd/qualify` and every other command ignore it.

### Answering gates

```sh
jarvis replay --events --yolo --capability examples/create-subaccount.json \
  --inputs scenarios/subaccount.inputs.json --contract scenarios/subaccount.contract.json \
  --export evidence/out.zip
```

At each gate, the `needs_human` event carries `"answer":"stdin"` along with the
gate's `epoch`, `action_id` and `observation_id`. The agent answers by writing
one JSON object per line to stdin:

```
{"kind":"approve","epoch":3,"action_id":"…","observation_id":"…"}
{"kind":"deny","epoch":3,"action_id":"…","observation_id":"…"}
{"kind":"cancel|pause|resume|refresh|focus|takeover|handback","epoch":3}
{"kind":"act","epoch":3,"spec":"press Submit"}
{"kind":"assist"}
```

The rules:

- **Strict decoding.** Unknown, duplicate and case-variant member names are
  refused. Lines are capped at 64 KiB.
- **Exact match.** `approve` and `deny` must name the pending gate exactly.
  Every kind except `assist` must carry the current epoch, so an answer written
  for one gate cannot land on the next.
- **Acknowledgement.** Each line gets a `{"event":"control","kind":…,"sent":…}`
  reply. `sent` means the control reached the run, not that the run applied
  it. A refusal by the run arrives as a `control_refused` event.
- **No answer.** If stdin reaches EOF while a gate is waiting, or
  `--gate-timeout` (default 5m) passes, the run is cancelled and the command
  exits 4. EOF before any gate is fine: a read-only run finishes normally.
- **Export.** `--export FILE.zip` writes the evidence of the run this command
  just ran, in every mode, including after an exit-4 cancel. That differs from
  `jarvis.evidence.export` over `call`, which can export any run and stays
  human-only. The destination may be any path the user can write, outside the
  root included; an existing file is never overwritten.

Without `--events`, yolo does not answer gates. The interactive loop keeps its
terminal-only rule for `approve`, `act` and the rest.

### One-shot calls

`jarvis call --yolo` also admits `jarvis.catalog.promote` and
`jarvis.inputs.put`. `inputs.put` takes params only from `@FILE` or `-`.

These stay refused even in yolo:

- **Ops that need a live run or credential:** `run.*`, `key.set`, `export`,
  `frame` and `anchor` (`E_PROCESS_SCOPED`). Use `--events --yolo` instead,
  and `GEMINI_API_KEY` in the environment for discovery.
- **`jarvis.inputs.get`:** it prints a stored secret into the agent's
  transcript, and no run needs it.
- **Unclassified ops, Manvi built-ins, and private paths:** path params under
  `.local/` or outside the root are refused.

Promotion history and `inputs.put` are read-modify-write files guarded only by
an in-process lock. A yolo call that races a live workbench writing the same
file can lose one of the two updates: the last writer wins.

### Attribution

An agent-operated run never claims a human approval. These records attribute
the run to the agent:

- **`operator.json`** is written into the run's evidence directory before the
  run can accept a control:
  `{"schema_version":1,"operator":"agent","mode":"yolo","human_qualified":false}`.
- **The journal** (`events.jsonl`) and the run view record the agent in place
  of `"human"`. Manvi's runner writes "human" for acts and takeovers. It also
  journals approve and deny as `"automation"`, because a control carries no
  actor. Both become `"agent"`.
- **The run view** carries `"operator":"agent"`, and promotion history carries
  `"promoted_by":"agent"`.
- **Unattended replay** (`jarvis replay --unattended`) refuses a capability
  revision whose latest promotion an agent made, whoever starts the replay. The
  catalog status alone says only "approved"; a person promoting the same
  revision again is what admits it.
- **`cmd/qualify`** refuses to start when `JARVIS_AGENT_MODE` is set, and its
  interactive approver requires a terminal. It builds its own App with no
  operator, so the per-run refusal of an operator-marked run is defence in depth
  for a host that marks runs, not what keeps agents out.

Two places still say "human" and are known residue:

- **`bundle.json`** files agent acts under `human_actions`, because DevCouncil's
  Bundle has no actor field.
- **The run's `controller`** reads `human` after an agent takeover. This is
  Manvi's term for "not automation".

`operator.json` is the authority for both. The verifier reads neither actors
nor unlisted files. That conclusion comes from reading the `dc-evidence` and
`dc-verify` sources, not from a verified run.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | ok |
| 1 | The operation ran and failed, the run failed, or verification did not pass |
| 2 | Usage error, or `E_BAD_REQUEST` / `E_TOO_LARGE` |
| 3 | Refused by the agent gate (`E_AGENT_REFUSED`, `E_PROCESS_SCOPED`) |
| 4 | A run reached a human gate under `--events` that no one answered, and was cancelled |
| 5 | `--timeout` passed (`E_TIMEOUT`) |

The other commands keep exiting 1 on any failure.

## Security

**In yolo mode:**

- Any process running as the owner that sets one flag or environment variable
  can approve account-changing actions, type into the attached application,
  take over the desktop, promote capabilities for unattended replay, and
  overwrite stored sensitive inputs.
- Nothing technical separates an agent from a person here. The mode is intent
  plus audit, not a boundary.
- What it preserves is honesty: agent-operated runs are marked before the first
  control and never journal a human actor, and qualification refuses them.
- Credentials still never enter argv. `inputs.get` stays refused.

**Outside yolo mode,** the human approval gate is also enforced on the
interactive CLI:

- `jarvis replay` and `jarvis discover` accept `approve`, `resume`, `assist`,
  `takeover`, `handback` and `act …` only when stdin is a terminal.
- Piped stdin may still send `deny`, `cancel`, `pause`, `refresh` and `focus`,
  which can only stop or re-observe.
- Before this change, a process piping `approve` into `jarvis replay` could
  answer an account-changing approval.

These are same-user fences against accidental self-approval and transcript
leakage, not a security boundary:

- A pseudo-terminal passes the terminal check.
- A same-user process can still read `.local/` directly or pipe NDJSON into
  `jarvis serve`, which is unchanged for the workbench.
- The path check on `jarvis call` params resolves symlinks once, and the
  handler opens the path again, so a symlink swapped in between is followed. A
  hard link into `.local` is not seen as `.local` at all. The bound applies
  only to `jarvis call`; other subcommands' path flags are not bounded.

A real boundary needs an approval credential the agent cannot hold. That is a
change to Manvi's broker.

## When the tool MCP servers are down

These CLIs answer from the same stores as their MCP servers. Both binaries
were found on PATH on 2026-11-18.

| MCP tool | CLI fallback |
|---|---|
| `devmap_*` | `devmap`. Ran on 2026-11-18: `paths --json`, `status --json`, `impact FILE::SYMBOL --json`, `affected FILE --json`, `gap-record`. Listed in `AGENTS.md` but not run: `explore`, `search`, `ask --evidence`. |
| DevCouncil MCP | `devcouncil`, also installed as `dev` and `devc`. Taken from `devcouncil --help`, not run: `gate status --json`, `task show ID --json`, `verify TASK_ID --json`, `doctor --json`. |
| `gitpulse_*` | No query CLI was found; `gitpulse-hook` is the hook entry point. Report the gap rather than substituting another tool. |

If a devmap MCP entry points at a missing binary, the CLI on PATH still
answers. `devmap paths --json` reports the dead registration as
`missing_binary_warning` and names the config file that holds it. On
2026-11-18 that was a user-level Claude config, not a file in this repository.
