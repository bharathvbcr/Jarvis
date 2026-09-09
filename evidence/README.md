# Evidence inventory

These are genuine local macOS bank runs, captured through the native desktop
broker. They are separate from deterministic fixtures and semantic UI tests.

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
- `examples/macos-balance`: complete, sanitized North run
  `aa8e64f3844a7a03233c793bf09586df`, output125000minorUSD; independent verifier
  passed. Includes exact capability/contract, masked pixels, accessibility
  snapshots, input receipts, trace and Perfetto timeline.
- `examples/macos-window-refusal`: preserved South run
  `5c25ec24dd665d2febe30710061ad87e`; geometry changed from880×732 to102×125
  before dispatch. This predates bounded confirmed-not-sent recovery. Its
  failure report remains unchanged as historical evidence.

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
