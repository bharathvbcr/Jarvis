# Host background jobs

The stdio host handles requests serially. `jarvis.doctor`, `jarvis.evidence.verify`,
`jarvis.frame.get`, and `jarvis.evidence.export` therefore admit bounded background
jobs, allowing status and control requests to proceed while those operations run.
Their initial response contains `job_id`, the original `operation`, and
`status: "pending"`. Admission does not indicate that a diagnostic or verification
passed, or that a frame or export is available.

Poll `jarvis.job.get` with `{ "id": "<job_id>" }`. Its result keeps the same
`job_id` and `operation`. Status is `pending`, `running`, `succeeded`, `failed`, or
`cancelled`. Only `succeeded` carries `result`, whose shape is the original
operation's result. Failed or cancelled jobs carry an `error` string. The operation
is advertised in the host's `hello` response. The Rust workbench retains the original
request subject and checks the observation ID before displaying a completed frame.
Direct Go calls such as `App.Doctor` and the CLI remain synchronous.

At most 16 jobs are retained. Completed jobs expire after five minutes and can be
evicted earlier to admit new work. Each result is bounded to 16 MiB of JSON, below
the workbench's 20 MiB response-line bound; total retained results are capped at
64 MiB. Returned results own their bytes. A timed-out worker keeps its admission
slot until it physically returns, so repeated timeouts cannot create an unbounded
number of background workers. Unknown or expired IDs return an error.

Doctor, verification, frame loading, and export have respective outer deadlines of
10, 15, 15, and 30 seconds. They inherit application shutdown cancellation. Frame
scans and export copies check cancellation between files and reads. Export stages a
complete ZIP beside the destination and publishes it without replacing an existing
file; a failed copy leaves no partial destination. Cancellation racing publication
can still leave a complete archive at the explicitly requested destination; a failed
job does not assert that a published artifact has been rolled back. Filesystem calls
already blocked in the operating system may outlive their context deadline, with
the worker admission bound still enforced.

`TestSlowDoctorDoesNotBlockHostStatusOrCancellation` drives the actual Manvi stdio
server with a delayed protocol fixture. It verifies hello registration, pending
job admission, and status/cancel delivery before the diagnostic completes. Job
tests cover capacity, deadlines, cancellation, malformed/oversized results, byte
ownership, eviction, and export cleanup. These are host integration tests, not
physical desktop qualification.
