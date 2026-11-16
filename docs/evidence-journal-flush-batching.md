# Evidence journal: flush batching (design only)

Status: proposal from the 2026-11-09 performance audit. Nothing here is
implemented. No sync in the journal has changed; evidence durability is a safety
property and the change needs the owner's sign-off first.

## What is flushed today

Every flush goes through `desktopJournal.sync` (`internal/app/journal.go`), which
is `(*os.File).Sync`: `F_FULLFSYNC` on darwin. Measured on this Mac at about
8-13 ms each (n=5, load average 22-57), so a read step costs roughly 100 ms of
flushing.

Per record, in `desktopJournal.append`:

| Site | What it flushes | When |
|---|---|---|
| `journal.go:53` via `files.go:54` | `frame-NNNNNN.png` (temp file, then rename) | observation records |
| `journal.go:62` via `files.go:54` | `frame-NNNNNN.json`, the tree | observation records |
| `journal.go:82` | `events.jsonl`, after the line is written | every record |

A read-effect step is two observation records and four other records, so
2 x 3 + 4 x 1 = 10 flushes. `TestJournalFlushesPerReadStep` pins that count and
`BenchmarkJournalAppendReadStep` reports it as `flushes/step`.

Once per run, outside the step loop: `evidence.go:75` and `:79` (contract,
capability), `app.go:372`, `:377`, `:382`, `:388` (binding, policy, policy digest,
overlay), and at `Finish` `evidence.go:314`, `:317`, `:320` (trace, bundle,
perfetto) and `app.go:453` (verification). `reconciliation.go:95` adds one when a
run ends unknown.

`writeAtomicSync` never flushes the directory after the rename. A frame's file
name is therefore not durable when the journal line naming it is: after a crash
the line can exist while `frame-NNNNNN.png` is absent. Today the line does not
carry the path or digest (the bundle does, and it is written only at `Finish`), so
the gap is invisible, but any design that defers file flushes widens it and must
close it with a directory flush before the bundle is written.

## What must stay synchronous

An action must not be dispatched before its `action_started` line is on disk.
Manvi emits that record before it starts the goroutine that performs the work
(`manvi/computer/runner.go:458` for act and read-observe steps, `:802` for human
actions). `events.jsonl` is append-only, so one flush of it covers every earlier
line too.

Decision for the owner: whether the approval and human revalidation observations
(`runner.go:1049`, `:812`), recorded between admission and input dispatch, must
also be durable before the input is sent. This design treats them as barriers
only if the owner says so.

Everything else (frames, trees, `observed` and `receipt` lines) can be flushed
later without weakening the property. A crash after dispatch and before the
receipt line leaves the action recorded with disposition `unknown`, which is what
the recorder already assumes.

## Design

One writer goroutine owned by `desktopJournal`, with a bounded queue.

- `append` keeps its contract. It validates the limits, decodes the frame,
  assigns artifact ids and digests in memory and returns them, so the recorder's
  bundle does not change. It then queues the frame file, the tree file and the
  line, in order.
- A barrier record (`action_started`, plus any record the owner adds above)
  waits until everything queued before it and its own line are written and the
  journal file is flushed once. That is the only flush on the dispatch path.
- Frame and tree files are written without a per-file flush. At the barrier at
  `Finish`, and before `bundle.json` is written, the writer flushes every file
  still pending and then the directory. The bundle never references a frame that
  is not durable when `verify` runs.
- The queue is bounded in count and bytes (the existing 256 MiB image and 64 MiB
  journal caps are the ceiling). A full queue blocks `append`; nothing is dropped.
- A write or flush error is sticky: the next `append`, and every waiting barrier,
  returns it. A failed run is never reported as recorded.
- Cancellation and the error paths that close the journal today
  (`app.go` calls to `recorder.file.Close()`) must drain or abandon the queue
  explicitly, and say which in the run's error.

Expected result: about one flush per dispatch plus one drain at the end, instead
of ten per step. Unverified: whether a single `F_FULLFSYNC` on macOS makes other
files' earlier writes durable. If it does not, the drain flushes each pending
file; the saving is then the removal of the flushes from the dispatch path, not
their number. Measure before relying on either.

## Crash-point test

Extend the injected `sync` seam into a small file layer (create temp file, write,
sync, rename, sync directory) backed by memory, in which a write is volatile until
it is synced and a rename is volatile until the directory is synced.

1. Script a multi-step run. After each `action_started` record returns from
   `append`, call a fake dispatch that records the action id and the number of
   operations performed so far.
2. For every operation index N, copy only the durable state as of N (a crash
   after operation N).
3. Assert, for each N: every action whose fake dispatch happened before N has an
   `action_started` line in the durable `events.jsonl`.
4. Assert that a durable line may name a frame that is not durable, and that
   verifying such a run reports it as incomplete rather than passing.
5. Assert that after `Finish` returns, every artifact in `bundle.json` exists in
   the durable state with its recorded digest.

`TestActionStartedIsFlushedBeforeAppendReturns` already pins step 3 for the
current single-path implementation, and `TestJournalFlushesPerReadStep` pins the
flush counts; both change deliberately with the batching work.
