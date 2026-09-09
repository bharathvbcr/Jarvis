# Native automation workbench

Build the product backend/broker/verifier/bank using the root build commands,
then build and launch the Rust host from `desktop/`:

```sh
cargo build -p jarvis-workbench
target/debug/jarvis-workbench --backend /absolute/Jarvis/build/jarvis --root /absolute/Jarvis
```

On macOS, the stable app bundle is `build/JarvisWorkbench.app`, with metadata in
`desktop/packaging/macos/Info.plist` and identifier `dev.jarvis.workbench`.
Its executable is the same Rust workbench binary, placed in
`Contents/MacOS/jarvis-workbench`. The stable bundle and `build/jarvis-workbench`
infer the product root and sibling backend, so Finder launch needs no arguments.
Explicit `--backend` and `--root` override these defaults; other executable
locations require explicit paths. Keep the bundle identity and path stable when rebuilding. Local builds
are unsigned development artifacts; this is not a signed distribution workflow.
Screen Recording and Accessibility authorization must be checked from the actual
workbench launch using **Run doctor**. Permission granted to a terminal-launched
helper does not establish permission for a helper launched through an app bundle.

The window opens a child `jarvis serve --root ROOT`. The backend resolves its
broker, verifier and bank from `ROOT/build/`. The workbench has no alternate
desktop-control driver, local compiler or acceptance evaluator. Its UI remains
usable while backend requests run; errors and pending work are visible at the top.

## Operator workflow

- **Catalog:** refresh immutable revisions, inspect promoted entries, load a
  revision, or load the balance/subaccount examples. Example loading supplies
  the matching scenario contract and synthetic sample inputs for review.
- **Editor & invocation:** use structured identity, execution-limit, parameter,
  target and ordered-step forms, or edit the underlying JSON. Step forms expose
  typed input/output/literal references, predicates, output types and forward
  edges. Steps can be added, removed and reordered; target renaming updates its
  step references only after an explicit rename action. Compile with the canonical Manvi compiler,
  register a new immutable revision, compare lines against the loaded revision,
  promote it and generate a Go binding. Compiler error text comes from the
  backend. Editing source or changing its path invalidates the previous compiled
  identity. The line comparison is positional, bounded and explicitly marked if
  truncated; it is not a minimal edit algorithm.
  Forms open only after the canonical compiler accepts the source; unsupported
  fields are refused rather than silently erased. Every form edit invalidates
  the old compiled identity. Editing forms never invokes a capability or approves
  input. A locally serialized form is not a validation result.
- **Typed invocation:** edit strings, booleans, integers and money amounts.
  Money uses integer minor units and a separate ISO currency. Sensitive fields
  are masked. Select a tenant, supply an independent acceptance contract, and
  either attach a PID or let the backend launch its native bank. Assisted
  recovery is disabled by default and may be enabled for that invocation.
- **Run & takeover:** inspect session ownership, epoch, phase, outputs and
  before/action/after records. Select an observation record to fetch its sanitized
  historical frame; the current capture and accessibility tree are also available.
  The refusal inspector preserves refusal reasons and unresolved outcomes.
  Candidate tables show exact-match counts and displayed/total candidates. Their
  score is matched requested semantic fields divided by required fields; it is
  neither a probability of success nor authorization to act.
  Session history uses the backend's existing bounded registry, with current
  input ownership and an approval-queue count. Selecting a run clears old
  controls and fetches a fresh full snapshot; registry summaries cannot authorize
  actions. Ownership becomes explicitly stale after three seconds without a
  fresh registry response. The UI retains one full run timeline at a time.
  Selecting a captured tree also compares it with the preceding capture from
  that window. Changes cover role, name, value, enabled/editable state, identifiers,
  geometry and supported actions. Incomplete, foreign-window and duplicate-identity
  trees produce a comparison error. At most 200 changes are displayed alongside
  the total count; sanitized values are the only values compared.
- **Approval:** a button submits exactly the backend's pending action ID, current
  observation ID and epoch. Missing, incomplete or ambiguous binding disables
  approval. No render, poll, timeout, discovery response or assistant action
automatically approves anything. Another click remains disabled until a new
  coordinator state/capture or explicit refusal is observed.
  A failed or malformed latest run snapshot disables previously visible controls
  until a valid current snapshot arrives. Runs that fail during admission show
  **Finished with error** while preserving the coordinator's last phase.
- **Visual target authoring:** open a complete latest capture, choose a crop,
  bounded search region and click offset in captured-image pixels, then request
  **Create and insert visual anchor**. The backend owns capture provenance,
  sensitive-region rejection and immutable template construction. The workbench
  inserts the returned inline PNG and hash into a new target and requires a new
  canonical compilation. It cannot create an accessibility node from image data.
  Visual targets support approved clicks only; they cannot extract canvas text.
  Templates are opaque and nonuniform, 8–128 pixels per side, and at most 96 KiB
  of encoded PNG bytes. Matching is exhaustive exact RGBA equality within the
  search region, limited to one million placements and 64 million compared
  pixels; exceeding a limit is a refusal. Frame dimensions, window geometry and
  scale are pinned. Changed pixels or ambiguous matches require fresh review.
  These controls do not imply physical visual-input qualification on an OS.
  Authoring requires an active session; pause and capture it before selecting
  coordinates. Both the crop and search region must avoid protected pixels.
  Approval shows the bound match and a magnified crop from the latest decoded
  capture. Missing image data or a different selected historical frame disables
  visual approval even when a match was reported.
  The displayed frame digest covers sanitized pixels. Raw frame hashes remain
  private to the broker/worker revalidation cache and are not exported to the UI.
- **Same-session human control:** pause to fence automation, focus or refresh
  the attached window, select a captured enabled control and explicitly press it
  or replace its editable value. The backend owns the native approval and action
  boundary. Resume requests a new checkpoint through the same session. Each
  human action carries the current epoch and observation; read-only controls
  cannot be edited through this UI.
  If uncertain delivery ends a run, an available final reconciliation capture is
  shown separately. The original outcome remains unknown and acceptance incomplete;
  a later UI status is not proof of durable saved state. The UI never enables
  Resume or another approval for that terminal run.
- **Assisted recovery:** when the invocation opted in, a paused run exposes one
  explicit recovery request. The backend admits a bounded Gemini recovery under
  its policy. The UI leaves resumption to the operator after inspecting its result.
- **Discovery:** enter a task and typed inputs, start discovery and inspect its
  status. When publication finishes, refresh/open the registered revision for
  review using its original bytes. Discovery and native execution have separate
  start actions and the backend enforces the desktop owner.
  **Inspect live discovery session** opens its current workflow in the same
  approval/takeover view. Explicit program generations allow discovery's next
  microprogram to begin a new workflow sequence; epochs never regress and an
  older response cannot replace the new generation.
- **Diagnostics:** run the backend doctor. Enter a Gemini credential in the
  password field and send it to the backend's memory store. The field clears
  after submission, including queue/connection refusal. Credential-setting errors
  do not echo backend message text. This is not a claim of secure memory erasure.
- **Evidence & replay:** verify a finished run, export its artifacts to a new
  `.zip` path, inspect a trace, or replay it with the compiled capability and
  typed inputs to compare the reconstructed final state. Trace inspection and
  deterministic replay use different operations; neither invokes native input
  or a provider. Export and verification errors remain visible.

## Protocol and resource ownership

Requests use Manvi's version-1 envelope `{id,op,params}` and responses use
`{id,ok,result?,error?:{code,message}}`. `hello` negotiates version 1 and advertised
operations before features can submit work. Additive response fields are ignored;
malformed envelopes, unknown correlations and incompatible versions fail closed.

Doctor, evidence verification/export, anchor construction and historical-frame loading may return a
background job receipt. The UI retains the original operation, run/observation
binding and deadline, and polls `jarvis.job.get` at least 600 ms apart per job.
Pending/running jobs display their status without applying a result. Only
`succeeded` jobs with matching identity/operation are applied; failed, cancelled,
expired and inconsistent jobs remain errors. A pending diagnostics job leaves
explicit approval and cancellation usable. Historical PNG decoding stays on the
reader thread after unwrapping a successful frame job. Evidence returned for an
older run cannot replace the currently selected run's report.

Pipe writer, reader/image-decoder and process-supervisor threads perform work
outside the UI. Queues and in-flight requests are capped at eight; requests at
4 MiB; response lines at 20 MiB; each pending call at 30 seconds. A timeout stops
the connection and does not retry an uncertain operation. Reconnecting is explicit
and does not resume an old backend's in-memory run registry. Preserve its evidence
and reconcile an interrupted run before launching another.

Retained UI jobs are capped at eight, with at most one job per slow operation.
Each has a 45-second overall UI deadline, allowing the backend's bounded worker
deadline and status polling to finish. The backend caps a job result at 16 MiB,
below the workbench response-line bound. No uncertain operation is retried.

Backend stderr and outgoing request bodies are not logged. Serialized outgoing
buffers are overwritten after pipe writing, but Rust strings and backend process
memory are not a certified secret-erasure boundary. Credentials are never stored
in workbench preferences or source files.

Run polling uses `cursor_records`; the UI retains at most 4096 records and shows
loaded/total counts. Timeline rows are rendered on demand. JSON previews stop at
128 KiB with a visible truncation notice. Historical records omit embedded image
bytes; `jarvis.frame.get` retrieves the selected sanitized observation. Decoding
accepts complete PNG captures only, at most 16 MiB base64 and eight million pixels,
with matching declared dimensions and an allocation limit. A file/hash/model
response cannot substitute an independently captured observation. Sanitization
and provenance remain trusted backend responsibilities.

Closing the workbench requests backend EOF, then the supervisor allows 750 ms
before process termination. Process reaping occurs after the native event loop
ends; normal UI frames and reconnect actions do not wait on child exit. A backend
restart does not manufacture successful results for its interrupted calls.

## Verification

```sh
cargo test --workspace
cargo clippy --workspace --all-targets -- -D warnings
cargo fmt --all -- --check
```

The bank's AccessKit SetValue regression failed before the shared field adapter,
then passed for both editable fields, including a native AccessKit Click(Search).
Workbench semantic tests exercise explicit approval and duplicate-click suppression,
credential clearing/redaction, canonical compiler errors, assisted opt-in and
offline operation dispatch. Model/transport tests cover stale source paths and
epochs, typed money, paused capture refresh, timeout behavior, bounded queues,
malformed envelopes and PNG validation. Async regressions cover pending jobs
without automatic approval, wrong job/operation identities, unsuccessful states,
old-run evidence binding, polling intervals, job deadlines and frame unwrapping.
Structured-editor regressions cover canonical-field round trips, unknown-field
refusal and real semantic text editing without dispatch. History tests cover
duplicate/oversized registry responses and stale replies after session selection.
Accessibility comparisons distinguish missing evidence from unchanged fields.
Reconciliation tests keep terminal unknown outcomes immutable.

An additional opt-in test starts the real backend without opening or controlling
any native application:

```sh
JARVIS_TEST_BACKEND=/absolute/Jarvis/build/jarvis \
JARVIS_TEST_ROOT=/absolute/Jarvis \
JARVIS_TEST_VISUAL_ANCHOR=/absolute/Manvi/native/crates/desktop-core/tests/fixtures/visual-anchor.json \
cargo test -p jarvis-workbench real_backend_handshake_catalog_and_compiler_error -- --ignored --nocapture
```

That real-backend test was run successfully locally, including a real asynchronous
unknown-run evidence job which reached failed without launching a broker or verifier,
alongside the headless tests,
Clippy and native linking. Test transport construction exists only under
`cfg(test)` and is absent from the qualification binary. eframe inspection remains
disabled. These checks do not establish physical workbench interaction, live
Gemini recovery, or Windows/Linux execution by themselves.

The extended test also exercised `jarvis.run.list`, an asynchronous refused anchor
request for an unknown run, and the native-owned visual fixture through the Rust
structured editor and real Go compiler. The normalized capability, inline PNG
and template digest survived the round trip exactly. No native input or provider
request was sent by this compatibility test.

Physical macOS QA on 2026-09-09 exercised the rendered catalog, example loading,
typed invocation, diagnostics, exact-path text editing through AccessKit,
offline trace inspection and program-identity refusal during replay. A read-only
balance run (`0dcd7ecfa8a13e528e69cd7a6f8c31a1`) was admitted but failed before
capture with `capture_permission: Screen Recording permission is not granted to
the native helper`. Doctor also reported Accessibility denied and qualification
`not_tested` under the initial development app wrapper. No native success, frame,
or bank write was manufactured. This exposed the need for the stable bundle
above; native workbench completion remains gated on the user's OS permissions.
Live Gemini, Windows and Linux remain separate qualification gates.
