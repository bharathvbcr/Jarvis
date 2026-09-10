# Native input-counter refusal

Run `3a005356846d0b300c8e220819b99c9c` is North attempt 6 in
`../../macos-pinned-40.json`. The native guard reported `external_interaction`
after its private HID counters changed. The workflow paused for reconciliation;
the qualification harness then cancelled it. The independent saved-state oracle
remained unchanged and the task was not accepted.

These are exact copied artifacts from the real run, not an injected fixture.
The event-counter change was observed; its hardware-versus-synthetic source was
not independently classified. Do not infer a specific person or device from the
refusal. Binary identities are recorded in the campaign and build report.
