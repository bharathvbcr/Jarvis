# Jarvis native desktop applications

This Rust workspace contains the synthetic bank and native automation workbench.
`crates/bank-demo` owns bank UI and persisted state; `crates/workbench` calls the
Jarvis backend over bounded NDJSON pipes; `crates/desktop-ui` owns shared accessible
text fields. The Jarvis runner and native broker own automation and action approval.
See [WORKBENCH.md](WORKBENCH.md) for the operator workflow and protocol boundaries.

The workspace pins eframe/egui 0.36.2 and egui_kittest 0.36.2. They require Rust
1.95 or newer. AccessKit, built-in fonts, OpenGL and X11 are enabled explicitly.
eframe inspection and its default feature set are disabled. There is no local
HTTP server, automation command API or application-state tool exposed to agents.

From this directory:

```sh
cargo build -p jarvis-bank
cargo test -p jarvis-bank
cargo clippy -p jarvis-bank --all-targets -- -D warnings
target/debug/jarvis-bank --tenant north --state /absolute/run-directory/north.json
```

Use a separate state path for each tenant/run. The application creates the state
directory and seeds a missing state file. Supplying existing state resumes that
state and requires the same tenant. An OS file lock prevents concurrent bank
instances from opening the same state. Close the bank before independently
inspecting or seeding its oracle state for a qualification test.

## Banking workflow

Both tenants have synthetic member `M-1001` with 125000 cents (`1250.00 USD`)
and `M-1002` with 84050 cents (`840.50 USD`). North Cooperative has a two-column
lookup/details layout. South Mutual arranges those controls vertically. The
window title is `Jarvis Bank — North Cooperative` or `Jarvis Bank — South Mutual`.

Enter a member ID, click **Search**, then **New subaccount**. Enter a name, click
**Review**, and **Confirm creation** to create a savings subaccount. Review and
Back do not persist changes. Creation leaves the existing balance unchanged.
Repeated creation of the same case-insensitive name does not create another
account. Editing the member ID invalidates any existing selection or review.

| Accessible role | Exact accessible labels |
|---|---|
| Text input | Member ID; Subaccount name |
| Read-only text input with value | Balance; Status; Subaccount name on review |
| Button | Search; New subaccount; Review; Confirm creation; Back |

The label widgets explicitly label their corresponding text inputs. Balance and
Status use immutable text buffers while exposing selectable textual values.
The shared field adapter consumes native AccessKit `SetValue` for enabled editable
fields by exact widget ID. This fills egui 0.36.2's missing text SetValue handling;
read-only fields explicitly advertise AccessKit's read-only flag. Native providers
may expose additional platform metadata, so the broker also enforces its policy.
The bank's manual Confirm button performs the transaction; Jarvis approval of an
automated press must occur in the external broker before any native input.

## Controlled fault modes

Pass `--fault` with one of the following values (default `none`):

| Mode | Observable behavior |
|---|---|
| `overlay` | A Session notice modal disables underlying controls until Dismiss |
| `delay` | Search remains pending for 1500 ms before publishing a member |
| `missing-control` | The creation form omits Review |
| `duplicate-control` | The review has two buttons labeled Confirm creation |
| `commit-noop` | Confirmation changes no state and reports Creation did not complete |
| `crash-after-commit` | Native process exits with code 86 after a persisted creation |

Faults act through the real UI/state flow. A fault does not create a fabricated
successful result. The process-crash flag is observed after drawing in the native
eframe host; headless tests inspect that flag and disk state without exiting.

## Persistence and verification boundaries

The JSON oracle has `schema_version`, `tenant`, `revision`, and `members`.
Each member has `id`, `name`, `balance_cents`, `currency`, and `subaccounts`;
subaccounts have `id`, `name`, and `kind: "savings"`. `BankState::seed` is the
canonical fixture generator. The oracle file is solely application persistence
and an independent test assertion input; it must never be registered as an agent
observation or discovery tool. UI acceptance facts come from native capture.

Transactions validate the next state, write a new file in the same directory,
sync it and rename it over the previous state. Unix also syncs the parent
directory. A write error leaves further writes disabled until reopening and
reconciling persisted state. Existing state/lock paths must be regular files;
input state is capped at 1 MiB, 128 members and 64 subaccounts per member.
This is a trusted local synthetic state directory, not an adversarial filesystem
sandbox or financial service.

Locally verified on macOS: compilation/linking, seven state tests and ten
headless egui accessibility interaction tests, plus Clippy with warnings denied.
Tests exercise both tenant layouts, read-only values, staged creation, persistent
commit, idempotent retries, lock ownership, invalid state, symlink refusal and
all fault modes. Those tests do not establish native OS accessibility delivery,
physical input, actual process termination, Windows/Linux execution or LLM task
reliability. The runner's separate platform qualification provides those gates.
