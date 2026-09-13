# External-interaction guard: strict equality versus partition

## What this answers

The pinned build's [40-run macOS campaign](../macos-pinned-40.json) passed 23/40 and
suspended 17 times with `external_interaction`. The refusal text named no cause, so
the suspensions could not be attributed. This directory records the controlled
experiment that attributed them and the change that followed.

## The defect

The macOS admission guard snapshots fifteen `HIDSystemState` counters and demanded
exact equality across all of them. `MouseMoved` is one of those counters, so any
person moving the mouse aborted an otherwise valid run. Jarvis does not trip its own
guard — its input uses a private event source — but a human at the machine does.

## The experiment

[`experiment.json`](experiment.json) records both arms. One variable changed in one
source tree: the observe path used the strict comparison in the control arm and the
partitioned classification in the treatment arm. Both ran `examples/balance.json`
against tenant `north` on the same host while synthetic pointer motion was posted
continuously to the HID event tap.

| Arm | Phase | Result |
| --- | --- | --- |
| Strict (pre-change) | `cancelled` | `external_interaction: Hardware input changed since admission`, an intervention request raised to a human operator, then abandoned at the deadline |
| Partitioned | `completed` | `balance = 125000` minor USD, with `pointer_motion` recorded on seven observations |

The control arm's refusal text matches the 17 suspended runs in the pinned campaign.

The motion was injected, not physical. It is used because it demonstrably moves the
same `MouseMoved` counter the guard reads — the probe observed index 4 advancing
656774 → 658710 over 45 s with no other counter moving. This does **not** qualify
hardware-versus-injected classification, which remains untested.

## The change

Counters are partitioned by what the input behind them can do. Buttons, keys,
modifiers, scroll and drags can commit an application state change and stay a hard
refusal; drags are state-changing because they carry a held button. `MouseMoved` and
`TabletPointer` cannot press, type, scroll or drag, so element-addressed
accessibility work reports them instead of aborting, and still revalidates the
window, the scoped accessibility fingerprint and the target before dispatch.
Coordinate-addressed input keeps the strict contract, because there the pointer
position is part of the action.

Tolerated motion is recorded, not dropped: `pointer_motion` appears on the
observation and receipt so evidence still shows a person was present. The field is
omitted when nothing was observed, so quiet runs serialize exactly as before.

## What it did not fix

[`macos-guard-partition-40.json`](../macos-guard-partition-40.json) passed 35/40 with
all 40 independent saved-state checks unchanged and zero model requests. It is not a
clean 40/40 stability pass. The four `external_interaction` refusals in it are now
attributed by category — two `pointer_button`, one `keyboard`, one
`keyboard, pointer_button` — and every one is genuine human input that could have
changed application state. No refusal was `pointer_motion`, which is what the
partition intends. A fifth run stopped on `wait predicate not yet satisfied`, which
is unrelated to the guard.

A campaign on a machine someone is actively using will legitimately suspend. That is
the guard working, not failing.
