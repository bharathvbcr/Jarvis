use crate::editor::{Document, PixelPoint, PixelRect, VisualAnchor, rect_ui};
use crate::model::{
    Action, Capability, Control, Entry, Input, Observation, Record, RunList, RunSnapshot,
    revision_diff,
};
use crate::transport::{Backend, Frame, MAX_PENDING, Message, Request, Response};
use eframe::egui;
use jarvis_desktop_ui::editable_text;
use serde::Deserialize;
use serde_json::{Value, json};
use std::collections::{BTreeMap, BTreeSet};
use std::path::PathBuf;
use std::sync::mpsc::TryRecvError;
use std::time::{Duration, Instant};

const MAX_RECORDS: usize = 4096;

#[derive(Clone, Copy, PartialEq, Eq)]
enum Tab {
    Catalog,
    Editor,
    Run,
    Discovery,
    Diagnostics,
    Evidence,
}

struct Pending {
    op: String,
    subject: String,
    sent: Instant,
    source: Option<String>,
    cursor: usize,
    run_binding: Option<String>,
}

struct Job {
    original: Pending,
    next_poll: Instant,
    status: String,
}

#[derive(Deserialize)]
struct JobReply {
    job_id: String,
    operation: String,
    status: String,
    result: Option<Value>,
    error: Option<String>,
}

pub struct Workbench {
    backend: Backend,
    backend_path: PathBuf,
    root: PathBuf,
    ready: bool,
    ops: BTreeSet<String>,
    next: u64,
    pending: BTreeMap<String, Pending>,
    jobs: BTreeMap<String, Job>,
    tab: Tab,
    notice: String,
    error: String,
    entries: Vec<Entry>,
    history: Value,
    selected_id: String,
    source: String,
    editor: Option<Document>,
    editor_error: String,
    baseline: String,
    compiled_source: String,
    compiler_error: String,
    capability: Option<Capability>,
    digest: String,
    capability_path: String,
    compiled_path: String,
    contract_path: String,
    inputs: Vec<Input>,
    tenant: String,
    pid: String,
    assisted: bool,
    run_assisted: bool,
    run_id: String,
    run: Option<RunSnapshot>,
    run_list: RunList,
    run_list_updated: Option<Instant>,
    next_list_poll: Instant,
    snapshot_valid: bool,
    records: Vec<Record>,
    total_records: usize,
    selected_record: Option<usize>,
    next_poll: Instant,
    control_pending: Option<(u64, u64, String)>,
    selected_node: String,
    human_kind: String,
    human_text: String,
    displayed_observation: Option<Observation>,
    texture: Option<(String, egui::TextureHandle)>,
    image_error: String,
    anchor_key: String,
    anchor_crop: PixelRect,
    anchor_search: PixelRect,
    anchor_click: PixelPoint,
    doctor: Value,
    evidence: Value,
    export_path: String,
    trace_path: String,
    trace: Value,
    generated: String,
    package_name: String,
    key: String,
    discovery_task: String,
    discovery_member: String,
    discovery_subaccount: String,
    discovery_id: String,
    discovery: Value,
    discovery_finished: bool,
}

impl Workbench {
    pub fn new(backend: Backend, backend_path: PathBuf, root: PathBuf) -> Self {
        let mut app = Self {
            backend,
            backend_path,
            root,
            ready: false,
            ops: BTreeSet::new(),
            next: 0,
            pending: BTreeMap::new(),
            jobs: BTreeMap::new(),
            tab: Tab::Catalog,
            notice: "Connecting to the local backend…".into(),
            error: String::new(),
            entries: Vec::new(),
            history: Value::Null,
            selected_id: String::new(),
            source: String::new(),
            editor: None,
            editor_error: String::new(),
            baseline: String::new(),
            compiled_source: String::new(),
            compiler_error: String::new(),
            capability: None,
            digest: String::new(),
            capability_path: String::new(),
            compiled_path: String::new(),
            contract_path: String::new(),
            inputs: Vec::new(),
            tenant: "north".into(),
            pid: String::new(),
            assisted: false,
            run_assisted: false,
            run_id: String::new(),
            run: None,
            run_list: RunList::default(),
            run_list_updated: None,
            next_list_poll: Instant::now(),
            snapshot_valid: false,
            records: Vec::new(),
            total_records: 0,
            selected_record: None,
            next_poll: Instant::now(),
            control_pending: None,
            selected_node: String::new(),
            human_kind: "press".into(),
            human_text: String::new(),
            displayed_observation: None,
            texture: None,
            image_error: String::new(),
            anchor_key: "visual_target".into(),
            anchor_crop: PixelRect {
                x: 0,
                y: 0,
                width: 32,
                height: 32,
            },
            anchor_search: PixelRect {
                x: 0,
                y: 0,
                width: 256,
                height: 256,
            },
            anchor_click: PixelPoint { x: 16, y: 16 },
            doctor: Value::Null,
            evidence: Value::Null,
            export_path: String::new(),
            trace_path: String::new(),
            trace: Value::Null,
            generated: String::new(),
            package_name: "capability".into(),
            key: String::new(),
            discovery_task: String::new(),
            discovery_member: "M-1001".into(),
            discovery_subaccount: String::new(),
            discovery_id: String::new(),
            discovery: Value::Null,
            discovery_finished: true,
        };
        app.request("hello", json!({"protocol":1,"host":"jarvis-workbench"}), "");
        app
    }

    fn busy(&self, op: &str) -> bool {
        self.pending.values().any(|p| p.op == op)
            || self.jobs.values().any(|job| job.original.op == op)
    }

    fn request(&mut self, op: &str, params: Value, subject: &str) -> bool {
        if op != "hello" && (!self.ready || !self.ops.contains(op)) {
            self.error = format!("The connected backend does not advertise {op}.");
            return false;
        }
        if self.pending.len() >= MAX_PENDING || self.busy(op) {
            self.error = "A matching request is already pending or the queue is full.".into();
            return false;
        }
        self.next += 1;
        let id = format!("workbench-{}", self.next);
        let source = matches!(op, "jarvis.capability.compile" | "jarvis.anchor.create")
            .then(|| self.source.clone());
        let run_binding = params
            .get("run_id")
            .and_then(Value::as_str)
            .map(str::to_owned);
        match self.backend.send(Request {
            id: id.clone(),
            op: op.into(),
            params,
        }) {
            Ok(()) => {
                self.pending.insert(
                    id,
                    Pending {
                        op: op.into(),
                        subject: subject.into(),
                        sent: Instant::now(),
                        source,
                        cursor: self.records.len(),
                        run_binding,
                    },
                );
                if !matches!(
                    op,
                    "jarvis.run.get"
                        | "jarvis.run.list"
                        | "jarvis.discovery.get"
                        | "jarvis.job.get"
                ) {
                    self.error.clear();
                }
                true
            }
            Err(error) => {
                self.error = error;
                false
            }
        }
    }

    fn poll(&mut self, context: &egui::Context) {
        for _ in 0..MAX_PENDING {
            match self.backend.poll() {
                Ok(Message::Reply {
                    response,
                    frame,
                    image_error,
                }) => self.reply(context, response, frame, image_error),
                Ok(Message::Disconnected(error)) => {
                    self.ready = false;
                    self.pending.clear();
                    self.jobs.clear();
                    self.error = error;
                }
                Err(TryRecvError::Empty) => break,
                Err(TryRecvError::Disconnected) => {
                    if self.ready {
                        self.error = "Backend connection ended. Pending actions have uncertain outcomes; reconnect explicitly.".into();
                    }
                    self.ready = false;
                    self.pending.clear();
                    self.jobs.clear();
                    break;
                }
            }
        }
        if self
            .pending
            .values()
            .any(|p| p.sent.elapsed() > Duration::from_secs(30))
            || self
                .jobs
                .values()
                .any(|job| job.original.sent.elapsed() > Duration::from_secs(45))
        {
            self.backend.stop();
            self.ready = false;
            self.pending.clear();
            self.jobs.clear();
            self.error = "Backend request or job exceeded its deadline. Connection stopped; no operation was retried. Reconcile the run before starting another.".into();
        }
        if self.ready && !self.busy("jarvis.job.get") {
            let due = self
                .jobs
                .iter()
                .filter(|(_, job)| job.next_poll <= Instant::now())
                .min_by_key(|(_, job)| job.next_poll)
                .map(|(id, _)| id.clone());
            if let Some(id) = due {
                if let Some(job) = self.jobs.get_mut(&id) {
                    job.next_poll = Instant::now() + Duration::from_millis(600);
                }
                self.request("jarvis.job.get", json!({"id":id}), &id);
            }
        }
        if self.ready && Instant::now() >= self.next_poll {
            self.next_poll = Instant::now() + Duration::from_millis(600);
            if !self.run_id.is_empty()
                && self
                    .run
                    .as_ref()
                    .is_none_or(|r| !r.finished || self.records.len() < self.total_records)
                && !self.busy("jarvis.run.get")
            {
                let id = self.run_id.clone();
                self.request(
                    "jarvis.run.get",
                    json!({"run_id":id,"cursor_records":self.records.len()}),
                    &id,
                );
            }
            if !self.discovery_id.is_empty()
                && !self.discovery_finished
                && !self.busy("jarvis.discovery.get")
            {
                let id = self.discovery_id.clone();
                self.request("jarvis.discovery.get", json!({"run_id":id}), &id);
            }
        }
        if self.ready
            && self.ops.contains("jarvis.run.list")
            && Instant::now() >= self.next_list_poll
            && !self.busy("jarvis.run.list")
        {
            self.next_list_poll = Instant::now() + Duration::from_secs(1);
            self.request("jarvis.run.list", json!({}), "");
        }
    }

    fn reply(
        &mut self,
        context: &egui::Context,
        response: Response,
        frame: Option<Frame>,
        image_error: Option<String>,
    ) {
        let Some(pending) = self.pending.remove(&response.id) else {
            self.error = "Received an uncorrelated backend response; connection stopped.".into();
            self.backend.stop();
            self.ready = false;
            return;
        };
        if matches!(
            pending.op.as_str(),
            "jarvis.run.get" | "jarvis.run.control" | "jarvis.run.assist"
        ) && !pending.subject.is_empty()
            && pending.subject != self.run_id
        {
            // The user selected another session while this response was in
            // flight. Its state and frame cannot influence the new selection.
            return;
        }
        if !response.ok {
            if pending.op == "jarvis.job.get" {
                self.jobs.remove(&pending.subject);
            }
            if pending.op == "jarvis.run.get" {
                self.snapshot_valid = false;
            }
            if matches!(
                pending.op.as_str(),
                "jarvis.run.control" | "jarvis.run.assist"
            ) {
                self.control_pending = None;
            }
            let message = if pending.op == "jarvis.key.set" {
                "Credential configuration was refused. The credential was cleared from the editor."
                    .into()
            } else {
                response.error.map_or_else(
                    || "Backend failed without a structured reason.".into(),
                    |e| format!("{}: {}", e.code, e.message),
                )
            };
            if pending.op == "jarvis.capability.compile" {
                self.compiler_error = message.clone();
            }
            self.error = message;
            return;
        }
        let Some(value) = response.result else {
            if pending.op == "jarvis.run.get" {
                self.snapshot_valid = false;
            }
            self.error = "Backend success omitted its result.".into();
            return;
        };
        let resolved = self.resolve_job(pending, value);
        let (pending, value) = match resolved {
            Ok(Some(result)) => result,
            Ok(None) => return,
            Err(error) => {
                self.error = error;
                return;
            }
        };
        let result = self.apply_result(&pending, value);
        if let Err(error) = result {
            if pending.op == "jarvis.run.get" {
                self.snapshot_valid = false;
            }
            self.error = error;
            return;
        }
        if let Some(error) = image_error {
            self.image_error = error;
        }
        if let Some(frame) = frame {
            let accept = match pending.op.as_str() {
                "jarvis.frame.get" => self
                    .displayed_observation
                    .as_ref()
                    .is_some_and(|o| o.observation_id == frame.observation_id),
                "jarvis.run.get" => {
                    self.selected_record.is_none()
                        && self
                            .run
                            .as_ref()
                            .and_then(|r| r.observation.as_ref())
                            .is_some_and(|o| o.observation_id == frame.observation_id)
                }
                _ => false,
            };
            if accept {
                let texture = context.load_texture(
                    format!("sanitized-{}", frame.observation_id),
                    frame.image,
                    egui::TextureOptions::LINEAR,
                );
                self.texture = Some((frame.observation_id, texture));
                self.image_error.clear();
            }
        }
    }

    fn resolve_job(
        &mut self,
        pending: Pending,
        value: Value,
    ) -> Result<Option<(Pending, Value)>, String> {
        if pending.op == "jarvis.job.get" {
            let job = self
                .jobs
                .remove(&pending.subject)
                .ok_or("Received an unknown asynchronous job.")?;
            let reply: JobReply = decode(value)?;
            if reply.job_id != pending.subject || reply.operation != job.original.op {
                return Err("Asynchronous job identity or operation mismatch.".into());
            }
            return match reply.status.as_str() {
                "pending" | "running" if reply.result.is_none() && reply.error.is_none() => {
                    self.jobs.insert(
                        reply.job_id,
                        Job {
                            status: reply.status,
                            ..job
                        },
                    );
                    Ok(None)
                }
                "succeeded" if reply.error.is_none() => {
                    let result = reply.result.ok_or("Completed job omitted its result.")?;
                    self.notice = format!("{} completed.", job.original.op);
                    Ok(Some((job.original, result)))
                }
                "failed" | "cancelled" if reply.result.is_none() => Err(format!(
                    "{} {}: {}",
                    job.original.op,
                    reply.status,
                    reply
                        .error
                        .unwrap_or_else(|| "No successful result was produced.".into())
                )),
                _ => Err("Asynchronous job status and result are inconsistent.".into()),
            };
        }
        if matches!(
            pending.op.as_str(),
            "jarvis.doctor"
                | "jarvis.evidence.verify"
                | "jarvis.frame.get"
                | "jarvis.evidence.export"
                | "jarvis.anchor.create"
        ) && value.get("job_id").is_some()
        {
            let id = value
                .get("job_id")
                .and_then(Value::as_str)
                .filter(|id| !id.is_empty() && id.len() <= 128)
                .ok_or("Invalid asynchronous job ID.")?;
            if value.get("status").and_then(Value::as_str) != Some("pending")
                || value
                    .get("operation")
                    .and_then(Value::as_str)
                    .is_some_and(|operation| operation != pending.op)
                || value.get("result").is_some_and(|result| !result.is_null())
                || value.get("error").is_some_and(|error| !error.is_null())
                || !self.ops.contains("jarvis.job.get")
                || self.jobs.len() >= MAX_PENDING
                || self.jobs.contains_key(id)
            {
                return Err(
                    "Asynchronous job receipt is incompatible or exceeds the queue limit.".into(),
                );
            }
            match pending.op.as_str() {
                "jarvis.doctor" => self.doctor = Value::Null,
                "jarvis.evidence.verify" => self.evidence = Value::Null,
                _ => {}
            }
            self.notice = format!("{} job pending.", pending.op);
            self.jobs.insert(
                id.into(),
                Job {
                    original: pending,
                    next_poll: Instant::now() + Duration::from_millis(600),
                    status: "pending".into(),
                },
            );
            Ok(None)
        } else {
            Ok(Some((pending, value)))
        }
    }

    fn apply_result(&mut self, pending: &Pending, value: Value) -> Result<(), String> {
        match pending.op.as_str() {
            "hello" => {
                #[derive(Deserialize)]
                struct Hello {
                    protocol: u32,
                    ops: Vec<String>,
                }
                let hello: Hello = decode(value)?;
                if hello.protocol != 1 {
                    return Err("Backend protocol is incompatible; version 1 is required.".into());
                }
                self.ops = hello.ops.into_iter().collect();
                self.ready = true;
                self.notice =
                    "Connected · native runs require explicit invocation and approval".into();
                self.request("jarvis.catalog.list", json!({}), "");
            }
            "jarvis.catalog.list" => {
                #[derive(Deserialize)]
                struct Catalog {
                    entries: Vec<Entry>,
                }
                let catalog: Catalog = decode(value)?;
                if catalog.entries.len() > 4096 {
                    return Err("Catalog exceeds 4096 entries.".into());
                }
                self.entries = catalog.entries;
            }
            "jarvis.capability.read" | "jarvis.capability.compile" => {
                #[derive(Deserialize)]
                struct Compiled {
                    capability: Capability,
                    digest: String,
                    source: Option<String>,
                    path: Option<String>,
                }
                let compiled: Compiled = decode(value)?;
                if compiled.capability.schema_version != 1 {
                    return Err("Unsupported capability schema.".into());
                }
                if let Some(submitted) = &pending.source {
                    if submitted != &self.source {
                        return Err(
                            "Editor changed while compiling. Compile the current revision again."
                                .into(),
                        );
                    }
                } else {
                    if pending.subject != self.capability_path {
                        return Err(
                            "Capability path changed while loading; the response was discarded."
                                .into(),
                        );
                    }
                    let source = compiled.source.ok_or("Capability read omitted source")?;
                    if source.len() > 1024 * 1024 {
                        return Err("Capability exceeds 1 MiB.".into());
                    }
                    self.source = source.clone();
                    self.baseline = source;
                    self.capability_path = pending.subject.clone();
                }
                self.compiled_source = self.source.clone();
                match Document::from_compiled_source(&self.source) {
                    Ok(document) => {
                        self.editor = Some(document);
                        self.editor_error.clear();
                    }
                    Err(error) => {
                        self.editor = None;
                        self.editor_error =
                            format!("Structured editor cannot preserve this document: {error}");
                    }
                }
                self.compiler_error.clear();
                self.digest = compiled.digest;
                if let Some(path) = compiled.path {
                    self.capability_path = path;
                } else if pending.op == "jarvis.capability.compile" && self.source != self.baseline
                {
                    self.capability_path.clear();
                }
                self.compiled_path = self.capability_path.clone();
                self.selected_id = compiled.capability.id.clone();
                self.inputs = compiled
                    .capability
                    .parameters
                    .iter()
                    .map(|p| {
                        let mut input = self
                            .inputs
                            .iter()
                            .find(|i| i.parameter.name == p.name && i.parameter.kind == p.kind)
                            .cloned()
                            .unwrap_or(Input {
                                parameter: p.clone(),
                                text: String::new(),
                                currency: "USD".into(),
                                boolean: false,
                            });
                        input.parameter = p.clone();
                        input
                    })
                    .collect();
                self.capability = Some(compiled.capability);
                self.notice = format!("Compiled revision · {}", self.digest);
            }
            "jarvis.catalog.history" => self.history = value,
            "jarvis.run.list" => {
                let list: RunList = decode(value)?;
                let unique: BTreeSet<_> = list.runs.iter().map(|r| r.run_id.as_str()).collect();
                if list.runs.len() > 512
                    || unique.len() != list.runs.len()
                    || unique.contains("")
                    || list.total_runs < list.runs.len()
                    || list.truncated != (list.total_runs > list.runs.len())
                {
                    return Err("Session registry has inconsistent identities or counts.".into());
                }
                self.run_list = list;
                self.run_list_updated = Some(Instant::now());
            }
            "jarvis.anchor.create" => {
                #[derive(Deserialize)]
                struct Created {
                    visual: VisualAnchor,
                }
                if pending.run_binding.as_deref() != Some(self.run_id.as_str()) {
                    return Err("Run selection changed while creating the anchor; the template was discarded.".into());
                }
                if pending.source.as_deref() != Some(self.source.as_str()) {
                    return Err("The editor changed while creating the anchor; the returned template was discarded.".into());
                }
                let created: Created = decode(value)?;
                let document = self
                    .editor
                    .as_ref()
                    .ok_or("Compile a capability before inserting an anchor")?;
                let mut next = document.clone();
                next.insert_visual(&pending.subject, created.visual)?;
                let source = next.source()?;
                self.editor = Some(next);
                self.source = source;
                self.compiler_error.clear();
                self.notice = "Anchor added from the coordinator's sanitized capture. Choose a click step with effect change and compile the edited revision.".into();
                self.tab = Tab::Editor;
            }
            "jarvis.catalog.promote" => {
                self.notice = "Promotion recorded by the catalog.".into();
                self.request("jarvis.catalog.list", json!({}), "");
                let id = self.selected_id.clone();
                self.request("jarvis.catalog.history", json!({"id":id}), &id);
            }
            "jarvis.run.start" => {
                let id: String = value
                    .get("run_id")
                    .and_then(Value::as_str)
                    .filter(|s| !s.is_empty())
                    .ok_or("Run start omitted run_id")?
                    .into();
                self.select_run(id);
                self.next_list_poll = Instant::now();
            }
            "jarvis.run.get" => {
                let total = value
                    .get("total_records")
                    .and_then(Value::as_u64)
                    .and_then(|n| usize::try_from(n).ok());
                let mut snapshot: RunSnapshot = decode(value)?;
                if let Some(matching) = &snapshot.matching
                    && (matching.candidates.len() > 200
                        || matching.total_candidates < matching.candidates.len()
                        || matching.truncated
                            != (matching.total_candidates > matching.candidates.len()))
                {
                    return Err(
                        "Candidate diagnostics have inconsistent displayed/total counts.".into(),
                    );
                }
                if snapshot.run_id != pending.subject || snapshot.run_id != self.run_id {
                    return Err("Received a foreign run snapshot.".into());
                }
                if let Some(reconciliation) = &snapshot.reconciliation
                    && (reconciliation.run_id != snapshot.run_id
                        || reconciliation.session_id != snapshot.state.session_id
                        || reconciliation.acceptance != "incomplete"
                        || !matches!(reconciliation.status.as_str(), "observed" | "unavailable")
                        || (reconciliation.status == "observed"
                            && reconciliation.observation_id.is_empty()))
                {
                    return Err(
                        "Reconciliation report has inconsistent identity or acceptance semantics."
                            .into(),
                    );
                }
                if let Some(previous) = &self.run
                    && (snapshot.state.epoch < previous.state.epoch
                        || snapshot.program_generation < previous.program_generation
                        || (snapshot.program_generation == previous.program_generation
                            && snapshot.state.sequence < previous.state.sequence))
                {
                    return Err("Run snapshot regressed; stale controls remain disabled.".into());
                }
                if self
                    .run
                    .as_ref()
                    .is_some_and(|previous| previous.assistance != snapshot.assistance)
                    && snapshot.assistance != "running"
                {
                    self.control_pending = None;
                }
                if pending.cursor != self.records.len() {
                    return Err("Timeline cursor no longer matches local history.".into());
                }
                self.total_records = total.unwrap_or(pending.cursor + snapshot.records.len());
                if self.total_records < pending.cursor + snapshot.records.len() {
                    return Err("Timeline total is smaller than the returned record range.".into());
                }
                if self.total_records > MAX_RECORDS
                    || self.records.len() + snapshot.records.len() > MAX_RECORDS
                {
                    return Err(
                        "Timeline exceeds 4096 records; results were not silently truncated."
                            .into(),
                    );
                }
                for record in &mut snapshot.records {
                    if record.kind == "control_refused" {
                        self.control_pending = None;
                        self.notice = format!("Control refused: {}", record.reason);
                    }
                    if let Some(o) = &mut record.observation {
                        o.screenshot.base64.clear();
                    }
                }
                self.records.append(&mut snapshot.records);
                if snapshot.finished {
                    self.control_pending = None;
                }
                if let Some(o) = &mut snapshot.observation {
                    o.screenshot.base64.clear();
                }
                if self
                    .control_pending
                    .as_ref()
                    .is_some_and(|(epoch, sequence, observation)| {
                        snapshot.state.epoch != *epoch
                            || snapshot.state.sequence > *sequence
                            || snapshot.capture_id() != observation
                    })
                {
                    self.control_pending = None;
                }
                if let Some(report) = &snapshot.report {
                    self.evidence = report.clone();
                }
                self.run_assisted = snapshot.assisted;
                if self.control_pending.is_none()
                    && self.notice
                        == "Control submitted; awaiting the coordinator's resulting state."
                {
                    self.notice = format!("Coordinator state: {}", run_status(&snapshot));
                }
                self.run = Some(snapshot);
                self.snapshot_valid = true;
            }
            "jarvis.run.control" => {
                self.notice = match &self.run {
                    Some(run) if self.control_pending.is_none() => {
                        format!("Coordinator state: {}", run_status(run))
                    }
                    _ => "Control submitted; awaiting the coordinator's resulting state.".into(),
                };
                self.next_poll = Instant::now();
            }
            "jarvis.run.assist" => {
                if value.get("admitted") != Some(&Value::Bool(true)) {
                    return Err("Assisted recovery was not admitted.".into());
                }
                self.notice =
                    "One assisted recovery was admitted. Inspect its result, then resume manually."
                        .into();
                self.next_poll = Instant::now();
            }
            "jarvis.frame.get" => {
                #[derive(Deserialize)]
                struct Captured {
                    observation: Observation,
                }
                if pending.run_binding.as_deref() != Some(self.run_id.as_str()) {
                    return Err(
                        "Run selection changed; the old session's frame was discarded.".into(),
                    );
                }
                let mut captured: Captured = decode(value)?;
                let selected_id = self
                    .selected_record
                    .and_then(|index| self.records.get(index))
                    .and_then(|record| record.observation.as_ref())
                    .map(|observation| observation.observation_id.as_str());
                if selected_id != Some(pending.subject.as_str()) {
                    return Err(
                        "Frame selection changed; stale historical image was discarded.".into(),
                    );
                }
                if captured.observation.observation_id != pending.subject {
                    return Err("Historical frame identity mismatch.".into());
                }
                captured.observation.screenshot.base64.clear();
                self.displayed_observation = Some(captured.observation);
            }
            "jarvis.doctor" => self.doctor = value,
            "jarvis.evidence.verify" => {
                if pending.subject != self.run_id {
                    return Err(
                        "Run selection changed; stale evidence result was discarded.".into(),
                    );
                }
                self.evidence = value;
            }
            "jarvis.evidence.export" => {
                self.notice = format!(
                    "Evidence export for {}: {}",
                    pending.subject,
                    value
                        .get("path")
                        .and_then(Value::as_str)
                        .ok_or("Export response omitted path")?
                );
            }
            "jarvis.trace.load" => self.trace = value,
            "jarvis.trace.replay" => self.trace = value,
            "jarvis.codegen" => {
                self.generated = value
                    .get("source")
                    .and_then(Value::as_str)
                    .ok_or("Codegen omitted source")?
                    .into()
            }
            "jarvis.key.set" => {
                if value.get("configured") != Some(&Value::Bool(true)) {
                    return Err("Credential was not configured.".into());
                }
                self.notice = "Credential configured in backend memory.".into();
            }
            "jarvis.discovery.start" => {
                self.discovery_id = value
                    .get("run_id")
                    .and_then(Value::as_str)
                    .ok_or("Discovery omitted run_id")?
                    .into();
                self.discovery = Value::Null;
                self.discovery_finished = false;
                self.next_poll = Instant::now();
            }
            "jarvis.discovery.get" => {
                let was_finished = self.discovery_finished;
                self.discovery_finished = value.get("finished") == Some(&Value::Bool(true));
                self.discovery = value;
                if self.discovery_finished && !was_finished {
                    self.request("jarvis.catalog.list", json!({}), "");
                }
            }
            _ => return Err("Unhandled backend operation response.".into()),
        }
        Ok(())
    }

    pub fn draw(&mut self, ui: &mut egui::Ui) {
        self.poll(ui.ctx());
        egui::Panel::top("heading").show(ui, |ui| {
            ui.horizontal(|ui| {
                ui.heading("Jarvis");
                ui.label("Native automation workbench");
                ui.separator();
                ui.label(if self.ready {
                    "Connected"
                } else {
                    "Disconnected"
                });
                ui.label(format!(
                    "{} request(s) · {} job(s)",
                    self.pending.len(),
                    self.jobs.len()
                ));
                if !self.ready && ui.button("Reconnect backend").clicked() {
                    match Backend::spawn(&self.backend_path, &self.root) {
                        Ok(backend) => {
                            self.backend = backend;
                            self.pending.clear();
                            self.jobs.clear();
                            self.request(
                                "hello",
                                json!({"protocol":1,"host":"jarvis-workbench"}),
                                "",
                            );
                        }
                        Err(error) => self.error = error,
                    }
                }
            });
            if !self.error.is_empty() {
                ui.colored_label(egui::Color32::from_rgb(180, 40, 35), &self.error);
            } else {
                ui.label(&self.notice);
            }
            for job in self.jobs.values() {
                ui.small(format!("{} · {}", job.original.op, job.status));
            }
        });
        egui::Panel::left("navigation")
            .resizable(false)
            .default_size(170.0)
            .show(ui, |ui| {
                ui.add_space(16.0);
                for (tab, label) in [
                    (Tab::Catalog, "Catalog"),
                    (Tab::Editor, "Editor & invocation"),
                    (Tab::Run, "Run & takeover"),
                    (Tab::Discovery, "Discovery"),
                    (Tab::Diagnostics, "Diagnostics"),
                    (Tab::Evidence, "Evidence & replay"),
                ] {
                    ui.selectable_value(&mut self.tab, tab, label);
                }
                ui.separator();
                ui.label("Workspace");
                ui.small(self.root.display().to_string());
                if let Some(run) = &self.run {
                    ui.separator();
                    ui.label(run_status(run));
                    ui.small(&run.run_id);
                }
            });
        egui::CentralPanel::default().show(ui, |ui| {
            egui::ScrollArea::vertical()
                .id_salt("page-scroll")
                .show(ui, |ui| match self.tab {
                    Tab::Catalog => self.catalog_ui(ui),
                    Tab::Editor => self.editor_ui(ui),
                    Tab::Run => self.run_ui(ui),
                    Tab::Discovery => self.discovery_ui(ui),
                    Tab::Diagnostics => self.diagnostics_ui(ui),
                    Tab::Evidence => self.evidence_ui(ui),
                });
        });
        ui.ctx().request_repaint_after(Duration::from_millis(100));
    }

    fn catalog_ui(&mut self, ui: &mut egui::Ui) {
        ui.heading("Capability catalog");
        if ui.button("Refresh catalog").clicked() {
            self.request("jarvis.catalog.list", json!({}), "");
        }
        if self.entries.is_empty() {
            ui.label("No registered capabilities yet. Load a capability path in the editor or register a compiled revision.");
        }
        ui.horizontal(|ui| {
            if ui.button("Load balance example").clicked() {
                self.load_example(false);
            }
            if ui.button("Load subaccount example").clicked() {
                self.load_example(true);
            }
        });
        let mut selected = None;
        for entry in &self.entries {
            ui.group(|ui| {
                ui.horizontal(|ui| {
                    if ui
                        .button(format!("Open {} / {}", entry.id, entry.revision))
                        .clicked()
                    {
                        selected = Some(entry.clone());
                    }
                    if entry.promoted {
                        ui.label("Promoted");
                    }
                });
                ui.label(&entry.description);
                ui.small(&entry.sha256);
            });
        }
        if let Some(entry) = selected {
            self.selected_id = entry.id.clone();
            self.capability_path = entry.path.clone();
            self.request(
                "jarvis.capability.read",
                json!({"path":entry.path}),
                &entry.path,
            );
            self.request("jarvis.catalog.history", json!({"id":entry.id}), &entry.id);
            self.tab = Tab::Editor;
        }
        ui.separator();
        ui.heading("Promotion history");
        json_view(ui, &self.history);
    }

    fn editor_ui(&mut self, ui: &mut egui::Ui) {
        ui.heading("Capability editor");
        editable_text(
            ui,
            "Capability path",
            "capability-path",
            &mut self.capability_path,
            4096,
            false,
        );
        if ui.button("Load capability").clicked() {
            let path = self.capability_path.clone();
            self.request("jarvis.capability.read", json!({"path":path}), &path);
        }
        if let Some(document) = &mut self.editor {
            if document.ui(ui) {
                self.notice =
                    "Capability source changed. Compile before invoking or promoting.".into();
                match document.source() {
                    Ok(source) => {
                        self.source = source;
                        self.compiler_error.clear();
                        self.editor_error.clear();
                    }
                    Err(error) => {
                        self.editor_error = error;
                    }
                }
            }
        } else {
            ui.label("Load or compile a capability to open its structured forms. Raw edits require canonical compilation before forms can preserve them.");
        }
        if !self.editor_error.is_empty() {
            ui.colored_label(egui::Color32::RED, &self.editor_error);
        }
        let label = ui.label("Capability JSON");
        let changed = ui
            .add(
                egui::TextEdit::multiline(&mut self.source)
                    .code_editor()
                    .desired_rows(16)
                    .desired_width(f32::INFINITY)
                    .char_limit(1024 * 1024),
            )
            .labelled_by(label.id)
            .changed();
        if changed {
            self.compiler_error.clear();
            self.editor = None;
            self.notice = "Capability source changed. Compile before invoking or promoting.".into();
        }
        ui.horizontal(|ui| {
            if ui.button("Compile").clicked() {
                self.request(
                    "jarvis.capability.compile",
                    json!({"source":self.source,"register":false}),
                    "",
                );
            }
            if ui.button("Compile & register revision").clicked() {
                self.request(
                    "jarvis.capability.compile",
                    json!({"source":self.source,"register":true}),
                    "",
                );
            }
            if ui
                .add_enabled(
                    self.current_compilation() && !self.capability_path.is_empty(),
                    egui::Button::new("Promote revision"),
                )
                .clicked()
                && let Some(capability) = &self.capability
            {
                let params = json!({"id":capability.id,"revision":capability.revision});
                self.request("jarvis.catalog.promote", params, "");
            }
        });
        if !self.compiler_error.is_empty() {
            ui.colored_label(egui::Color32::RED, &self.compiler_error);
        }
        if self.current_compilation() {
            ui.small(format!("Compiled SHA-256: {}", self.digest));
        } else {
            ui.label("Compile the current source before invoking or promoting it.");
        }
        egui::CollapsingHeader::new("Revision comparison").show(ui, |ui| {
            ui.label("Equal-position line comparison against the last loaded immutable revision.");
            for line in revision_diff(&self.baseline, &self.source) {
                ui.monospace(line);
            }
        });
        ui.separator();
        ui.heading("Typed invocation");
        self.target_inputs_ui(ui);
        editable_text(
            ui,
            "Acceptance contract path",
            "contract-path",
            &mut self.contract_path,
            4096,
            false,
        );
        self.parameters_ui(ui);
        ui.checkbox(
            &mut self.assisted,
            "Allow assisted recovery for this invocation",
        );
        ui.label(
            "Assisted recovery is requested explicitly while paused and leaves resumption to you.",
        );
        let can_start = self.current_compilation()
            && self.run_list.desktop_owner.is_empty()
            && !self.capability_path.is_empty()
            && !self.contract_path.is_empty()
            && !self.busy("jarvis.run.start")
            && self.discovery_finished
            && self.run.as_ref().is_none_or(|r| r.finished);
        if ui
            .add_enabled(can_start, egui::Button::new("Start native run"))
            .clicked()
        {
            self.start_run();
        }
        ui.separator();
        ui.heading("Generated Go binding");
        editable_text(
            ui,
            "Go package",
            "go-package",
            &mut self.package_name,
            96,
            false,
        );
        if ui
            .add_enabled(
                self.current_compilation() && !self.capability_path.is_empty(),
                egui::Button::new("Generate Go binding"),
            )
            .clicked()
        {
            self.request(
                "jarvis.codegen",
                json!({"capability_path":self.capability_path,"package_name":self.package_name}),
                "",
            );
        }
        if !self.generated.is_empty() {
            if ui.button("Copy generated source").clicked() {
                ui.ctx().copy_text(self.generated.clone());
            }
            egui::CollapsingHeader::new("Generated source").show(ui, |ui| {
                ui.monospace(&self.generated);
            });
        }
    }

    fn load_example(&mut self, subaccount: bool) {
        let path = if subaccount {
            "examples/create-subaccount.json"
        } else {
            "examples/balance.json"
        };
        self.capability_path = path.into();
        self.contract_path = if subaccount {
            "scenarios/subaccount.contract.json"
        } else {
            "scenarios/balance-m1001.contract.json"
        }
        .into();
        self.inputs = vec![Input {
            parameter: crate::model::Parameter {
                name: "member_id".into(),
                kind: "string".into(),
                sensitive: true,
            },
            text: "M-1001".into(),
            currency: "USD".into(),
            boolean: false,
        }];
        if subaccount {
            self.inputs.push(Input {
                parameter: crate::model::Parameter {
                    name: "subaccount_name".into(),
                    kind: "string".into(),
                    sensitive: false,
                },
                text: "Travel".into(),
                currency: "USD".into(),
                boolean: false,
            });
        }
        self.request("jarvis.capability.read", json!({"path":path}), path);
        self.tab = Tab::Editor;
    }

    fn current_compilation(&self) -> bool {
        self.capability.is_some()
            && self.editor_error.is_empty()
            && !self.digest.is_empty()
            && self.source == self.compiled_source
            && self.capability_path == self.compiled_path
    }

    fn target_inputs_ui(&mut self, ui: &mut egui::Ui) {
        ui.horizontal(|ui| {
            ui.label("Tenant");
            ui.selectable_value(&mut self.tenant, "north".into(), "North");
            ui.selectable_value(&mut self.tenant, "south".into(), "South");
        });
        editable_text(
            ui,
            "Target PID (blank launches the demo)",
            "target-pid",
            &mut self.pid,
            10,
            false,
        );
    }

    fn parameters_ui(&mut self, ui: &mut egui::Ui) {
        for input in &mut self.inputs {
            ui.push_id(&input.parameter.name, |ui| {
                ui.small(format!(
                    "{} · {}{}",
                    input.parameter.name,
                    input.parameter.kind,
                    if input.parameter.sensitive {
                        " · sensitive"
                    } else {
                        ""
                    }
                ));
                if input.parameter.kind == "boolean" {
                    ui.checkbox(&mut input.boolean, &input.parameter.name);
                } else {
                    editable_text(
                        ui,
                        &input.parameter.name,
                        "input-value",
                        &mut input.text,
                        65536,
                        input.parameter.sensitive,
                    );
                }
                if input.parameter.kind == "money" {
                    ui.label("Amount is an integer in minor units (cents for USD).");
                    editable_text(ui, "Currency", "currency", &mut input.currency, 3, false);
                }
            });
        }
    }

    fn invocation(&self) -> Result<Value, String> {
        let mut inputs = BTreeMap::new();
        for input in &self.inputs {
            inputs.insert(input.parameter.name.clone(), input.value()?);
        }
        let mut params = json!({"tenant":self.tenant,"inputs":inputs});
        if !self.pid.is_empty() {
            let pid = self
                .pid
                .parse::<u32>()
                .map_err(|_| "Target PID must be a positive integer")?;
            if pid == 0 {
                return Err("Target PID must be positive".into());
            }
            params["pid"] = json!(pid);
        }
        Ok(params)
    }

    fn start_run(&mut self) {
        match self.invocation() {
            Ok(mut params) => {
                params["capability_path"] = json!(self.capability_path);
                params["contract_path"] = json!(self.contract_path);
                params["assisted"] = json!(self.assisted);
                if self.request("jarvis.run.start", params, "") {
                    self.run_assisted = self.assisted;
                }
            }
            Err(error) => self.error = error,
        }
    }

    fn submit_control(&mut self, control: Result<Control, String>) {
        match control {
            Ok(control) => {
                let binding = self
                    .run
                    .as_ref()
                    .map(|r| (r.state.epoch, r.state.sequence, r.capture_id().into()));
                if self.request(
                    "jarvis.run.control",
                    json!({"run_id":self.run_id,"control":control}),
                    &self.run_id.clone(),
                ) {
                    self.control_pending = binding;
                }
            }
            Err(error) => self.error = error,
        }
    }

    fn run_ui(&mut self, ui: &mut egui::Ui) {
        ui.heading("Session & execution");
        self.sessions_ui(ui);
        if self.run.is_none() {
            ui.label(if self.run_id.is_empty() {
                "Start a capability from the editor to create a native session."
            } else {
                "Waiting for the first run snapshot…"
            });
            return;
        }
        let mut control = None;
        let mut frame_id = None;
        let mut human = false;
        let mut assist = false;
        let assist_busy = self.busy("jarvis.run.assist");
        {
            let run = self.run.as_ref().expect("run checked above");
            ui.label(format!("{} · {}", run.run_id, run_status(run)));
            ui.monospace(format!(
                "Session {} · epoch {} · event {} · step {}",
                run.state.session_id, run.state.epoch, run.state.sequence, run.state.step_index
            ));
            ui.small(format!("Capability {}", run.state.capability_sha256));
            if !run.error.is_empty() {
                ui.colored_label(egui::Color32::RED, &run.error);
            }
            if !run.state.reason.is_empty() {
                ui.label(&run.state.reason);
            }
            if !run.assistance.is_empty() {
                ui.label(format!("Assisted recovery: {}", run.assistance));
            }
            if let Some(reconciliation) = &run.reconciliation {
                ui.heading("Observation after uncertain delivery");
                ui.label("The action outcome remains unknown and acceptance remains incomplete. A later UI observation does not prove durable application state. This run cannot resume or repeat the action.");
                if let Ok(value) = serde_json::to_value(reconciliation) {
                    json_view(ui, &value);
                }
                if reconciliation.status == "observed"
                    && ui.button("Show reconciliation capture").clicked()
                {
                    self.selected_record = self.records.iter().position(|r| {
                        r.observation
                            .as_ref()
                            .is_some_and(|o| o.observation_id == reconciliation.observation_id)
                    });
                    self.displayed_observation = None;
                    self.texture = None;
                    frame_id = Some(reconciliation.observation_id.clone());
                }
            }
            let idle = self.ready
                && self.snapshot_valid
                && !self.busy("jarvis.run.control")
                && self.control_pending.is_none();
            ui.horizontal(|ui| {
                for (kind, label) in [
                    ("pause", "Pause / take over"),
                    ("resume", "Resume automation"),
                    ("cancel", "Cancel run"),
                ] {
                    if ui
                        .add_enabled(idle && run.control(kind).is_ok(), egui::Button::new(label))
                        .clicked()
                    {
                        control = Some(run.control(kind));
                    }
                }
            });
            if run.state.phase == "awaiting_approval" {
                ui.group(|ui| {
                    ui.heading("Approval queue");
                    ui.label(
                        "Review the target and captured state before approving this one action.",
                    );
                    ui.monospace(format!(
                        "Action {}\nObservation {}\nEpoch {}",
                        run.pending_action_id, run.state.observation.id, run.state.epoch
                    ));
                    if let Some(observation) = &run.observation {
                        ui.label(format!("Current window: {}", observation.window.title));
                        for node in &observation.nodes {
                            if let Some(value) = &node.value
                                && node.role == "text_field"
                            {
                                ui.label(format!("{}: {}", node.name, value));
                            }
                        }
                    }
                    if let Some(capability) = &run.capability
                        && let Some(step) = capability.steps.get(run.state.step_index)
                    {
                        ui.label(format!(
                            "{} → {} · effect {}",
                            step.kind, step.target, step.effect
                        ));
                    }
                    let visual_frame_ready = run.visual_anchor().is_none() || (self.selected_record.is_none() && self.texture.as_ref().is_some_and(|(id, _)| id == &run.state.observation.id));
                    if let Some(anchor) = run.visual_anchor() {
                        ui.strong("Visual click · human approval required");
                        ui.small(format!("Template SHA-256 {}", anchor.sha256));
                        if let Some(visual) = &run.state.observation.visual {
                            ui.label(format!("Matched crop x={} y={} width={} height={}; click offset {},{}", visual.matched.x, visual.matched.y, visual.matched.width, visual.matched.height, anchor.click.x, anchor.click.y));
                            ui.small(format!("Reviewed sanitized frame SHA-256 {}", visual.frame_sha256));
                            if visual_frame_ready && let Some((_, texture)) = &self.texture {
                                let width = anchor.frame_width.max(1) as f32;
                                let height = anchor.frame_height.max(1) as f32;
                                let uv = egui::Rect::from_min_max(egui::pos2(visual.matched.x as f32 / width, visual.matched.y as f32 / height), egui::pos2(visual.matched.x.saturating_add(visual.matched.width) as f32 / width, visual.matched.y.saturating_add(visual.matched.height) as f32 / height));
                                ui.add(egui::Image::new(texture).uv(uv).fit_to_exact_size(egui::vec2(anchor.width as f32 * 2.0, anchor.height as f32 * 2.0)));
                            }
                        }
                        if !visual_frame_ready { ui.label("Show the latest decoded capture before approving this visual click."); }
                        ui.label("The worker rechecks the exact pixels, geometry and input recipient after approval. Any uncertainty refuses dispatch.");
                    }
                    ui.horizontal(|ui| {
                        if ui
                            .add_enabled(
                                idle && visual_frame_ready && run.approval("approve").is_ok(),
                                egui::Button::new("Approve this action"),
                            )
                            .clicked()
                        {
                            control = Some(run.approval("approve"));
                        }
                        if ui
                            .add_enabled(
                                idle && run.approval("deny").is_ok(),
                                egui::Button::new("Deny this action"),
                            )
                            .clicked()
                        {
                            control = Some(run.approval("deny"));
                        }
                    });
                });
            }
            if run.state.phase == "paused" {
                ui.group(|ui| {
                    ui.heading("Human control of this session");
                    ui.label("Each press sends one broker action, bound to this session and fresh observation.");
                    if ui.add_enabled(idle && self.run_assisted && !assist_busy && run.assistance.is_empty(), egui::Button::new("Request one assisted recovery")).clicked() { assist = true; }
                    ui.label("After assisted recovery, inspect the new capture and resume manually.");
                    ui.horizontal(|ui| {
                        for (kind,label) in [("focus","Focus attached window"),("refresh","Capture current state")] {
                            if ui.add_enabled(idle, egui::Button::new(label)).clicked() { control = Some(run.control(kind)); }
                        }
                    });
                    let observation = run.observation.as_ref().filter(|o| o.complete && o.epoch == run.state.epoch);
                    if let Some(observation) = observation {
                        ui.label(format!("{} · PID {} · window {}", observation.window.title, observation.window.pid, observation.window.window_id));
                        egui::ComboBox::from_label("Human action target").selected_text(if self.selected_node.is_empty() { "Choose a captured control" } else { &self.selected_node }).show_ui(ui, |ui| {
                            for node in &observation.nodes { if node.enabled { ui.selectable_value(&mut self.selected_node, node.id.clone(), format!("{}: {} [{}]", node.role, node.name, node.id)); } }
                        });
                        ui.horizontal(|ui| { ui.selectable_value(&mut self.human_kind, "press".into(), "Press control"); ui.selectable_value(&mut self.human_kind, "set_value".into(), "Set value"); });
                        if self.human_kind == "set_value" { editable_text(ui, "Human replacement text", "human-text", &mut self.human_text, 65536, false); }
                        if ui.add_enabled(idle && !self.selected_node.is_empty(), egui::Button::new("Apply this human action")).clicked() { human = true; }
                    } else { ui.label("Capture a fresh state in this epoch before choosing a target."); }
                });
            }
            if let Some(outputs) = &run.state.outputs {
                egui::CollapsingHeader::new("Typed outputs").show(ui, |ui| {
                    for (name, value) in outputs {
                        ui.label(name);
                        json_view(ui, value);
                    }
                });
            }
            if let Some(matching) = &run.matching {
                egui::CollapsingHeader::new("Selector candidates / refusal inspector").default_open(true).show(ui, |ui| {
                    if matching.selector.get("visual").is_some() {
                        ui.label("Visual locator: exhaustive exact native matching is required. Semantic candidate scores cannot authorize this target.");
                        let mut selector = matching.selector.clone();
                        if let Some(visual) = selector.get_mut("visual").and_then(Value::as_object_mut) { visual.remove("png_base64"); }
                        json_view(ui, &selector);
                        if let Some(visual) = &run.state.observation.visual {
                            ui.label("One native match is bound to this observation.");
                            if let Ok(value) = serde_json::to_value(visual) { json_view(ui, &value); }
                        } else { ui.label("No bound native visual match is available; inspect the recorded refusal."); }
                        return;
                    }
                    ui.label(format!("{} · {} exact matches · {} of {} candidates shown{}", matching.confidence, matching.exact_matches, matching.candidates.len(), matching.total_candidates, if matching.truncated { " · truncated" } else { "" }));
                    ui.label("Score is the fraction of requested semantic fields matched. It is not a probability or an approval.");
                    json_view(ui, &matching.selector);
                    egui::ScrollArea::horizontal().id_salt("candidate-scroll").show(ui, |ui| {
                        egui::Grid::new("candidate-table").striped(true).show(ui, |ui| {
                            for heading in ["Control", "Role / name", "Matched", "Score", "Missing", "Enabled / editable"] { ui.strong(heading); } ui.end_row();
                            for candidate in &matching.candidates {
                                ui.label(&candidate.node_id); ui.label(format!("{} / {}", candidate.role, candidate.name));
                                ui.label(format!("{} / {}", candidate.matched_fields, candidate.required_fields));
                                ui.label(format!("{:.0}%", candidate.score * 100.0)); ui.label(candidate.missing.join(", "));
                                ui.label(format!("{} / {}", candidate.enabled, candidate.editable)); ui.end_row();
                            }
                        });
                    });
                });
            }
        }
        if let Some(control) = control {
            self.submit_control(control);
        }
        if human {
            self.human_action();
        }
        if assist
            && self.request(
                "jarvis.run.assist",
                json!({"run_id":self.run_id}),
                &self.run_id.clone(),
            )
        {
            self.control_pending = self
                .run
                .as_ref()
                .map(|r| (r.state.epoch, r.state.sequence, r.capture_id().into()));
        }
        ui.separator();
        ui.heading("Before · action · after timeline");
        ui.label(format!(
            "{} of {} records loaded",
            self.records.len(),
            self.total_records
        ));
        if ui.button("Show latest captured frame").clicked() {
            self.selected_record = None;
            self.displayed_observation = None;
            self.texture = None;
            self.next_poll = Instant::now();
            let id = self.run_id.clone();
            self.request(
                "jarvis.run.get",
                json!({"run_id":id,"cursor_records":self.records.len()}),
                &id,
            );
        }
        egui::ScrollArea::vertical()
            .id_salt("timeline")
            .max_height(260.0)
            .show_rows(
                ui,
                ui.text_style_height(&egui::TextStyle::Body),
                self.records.len(),
                |ui, range| {
                    for index in range {
                        let record = &self.records[index];
                        let label = format!(
                            "{} · {} {} · {} · {} · epoch {}",
                            index + 1,
                            record.kind,
                            record.stage,
                            record.action_id,
                            record.actor,
                            record.epoch
                        );
                        if ui
                            .selectable_label(self.selected_record == Some(index), label)
                            .clicked()
                        {
                            self.selected_record = Some(index);
                            self.displayed_observation = None;
                            self.texture = None;
                            if let Some(o) = &record.observation {
                                frame_id = Some(o.observation_id.clone());
                            }
                        }
                    }
                },
            );
        if let Some(index) = self.selected_record
            && let Some(record) = self.records.get(index)
        {
            ui.heading("Selected record / refusal inspector");
            if let Ok(value) = serde_json::to_value(record) {
                json_view(ui, &value);
            }
            if let Some(after) = &record.observation {
                if let Some(before) = self.records[..index]
                    .iter()
                    .rev()
                    .find_map(|r| r.observation.as_ref())
                {
                    ui.heading("Accessibility changes since the preceding capture");
                    ui.small(format!(
                        "{} → {}",
                        before.observation_id, after.observation_id
                    ));
                    match crate::changes::accessibility(before, after) {
                        Ok(changes) => {
                            ui.label(format!(
                                "{} of {} changes shown",
                                changes.rows.len(),
                                changes.total
                            ));
                            if changes.total == 0 {
                                ui.label("No changes in the compared accessibility fields.");
                            }
                            for row in changes.rows {
                                ui.label(row);
                            }
                        }
                        Err(error) => {
                            ui.label(error);
                        }
                    }
                } else {
                    ui.label("No preceding captured tree is available for comparison.");
                }
            }
        }
        if let Some(id) = frame_id {
            self.request(
                "jarvis.frame.get",
                json!({"run_id":self.run_id,"observation_id":id}),
                &id,
            );
        }
        self.frame_ui(ui);
        self.anchor_ui(ui);
        egui::CollapsingHeader::new("Refusals and unresolved outcomes").show(ui, |ui| {
            for record in &self.records {
                if !record.reason.is_empty() || record.kind.contains("refus") {
                    ui.label(format!(
                        "{} · {} · {}",
                        record.kind, record.action_id, record.reason
                    ));
                }
            }
        });
    }

    fn select_run(&mut self, id: String) {
        self.run_id = id;
        self.run = None;
        self.snapshot_valid = false;
        self.records.clear();
        self.total_records = 0;
        self.selected_record = None;
        self.texture = None;
        self.displayed_observation = None;
        self.evidence = Value::Null;
        self.control_pending = None;
        self.selected_node.clear();
        self.human_text.clear();
        self.run_assisted = false;
        self.next_poll = Instant::now();
        self.tab = Tab::Run;
    }

    fn sessions_ui(&mut self, ui: &mut egui::Ui) {
        let owner = if self.run_list.desktop_owner.is_empty() {
            "No active owner"
        } else {
            self.run_list.desktop_owner.as_str()
        };
        match self.run_list_updated {
            None => {
                ui.label("Desktop input owner: awaiting registry snapshot");
            }
            Some(updated) => {
                ui.label(format!(
                    "Desktop input owner: {owner}{}",
                    if updated.elapsed() > Duration::from_secs(3) {
                        " (last known; registry snapshot is stale)"
                    } else {
                        ""
                    }
                ));
            }
        }
        let mut selected = None;
        let count = self
            .run_list
            .runs
            .iter()
            .filter(|r| !r.finished && r.state.phase == "awaiting_approval")
            .count();
        ui.label(format!("Approval queue: {count} awaiting review. Select a run to obtain a fresh bound approval request."));
        egui::CollapsingHeader::new("Sessions and run history")
            .default_open(true)
            .show(ui, |ui| {
                ui.label(format!(
                    "{} of {} runs shown",
                    self.run_list.runs.len(),
                    self.run_list.total_runs
                ));
                egui::ScrollArea::vertical()
                    .id_salt("session-history")
                    .max_height(180.0)
                    .show_rows(
                        ui,
                        ui.text_style_height(&egui::TextStyle::Body) + 5.0,
                        self.run_list.runs.len(),
                        |ui, range| {
                            for index in range {
                                let run = &self.run_list.runs[index];
                                let label = format!(
                                    "{} · {}{} · {} · epoch {}",
                                    run.run_id,
                                    run.state.phase,
                                    if run.finished { " (finished)" } else { "" },
                                    run.state.session_id,
                                    run.state.epoch
                                );
                                if ui
                                    .selectable_label(self.run_id == run.run_id, label)
                                    .on_hover_text(format!(
                                        "{}\n{}\n{}",
                                        run.started_at, run.evidence_dir, run.error
                                    ))
                                    .clicked()
                                {
                                    selected = Some(run.run_id.clone());
                                }
                            }
                        },
                    );
            });
        if let Some(id) = selected {
            self.select_run(id);
        }
    }

    fn human_action(&mut self) {
        let result = (|| -> Result<Control, String> {
            let run = self.run.as_ref().ok_or("No active run")?;
            if !run.bound() || run.state.phase != "paused" {
                return Err("Human control requires a paused owned session.".into());
            }
            let observation = run
                .observation
                .as_ref()
                .filter(|o| o.complete && o.epoch == run.state.epoch)
                .ok_or("Fresh paused observation required")?;
            let node = observation
                .nodes
                .iter()
                .find(|n| n.id == self.selected_node && n.enabled)
                .ok_or("Selected target is no longer available")?;
            if !node.actions.contains(&self.human_kind) {
                return Err("The captured control does not advertise this action.".into());
            }
            if self.human_kind == "set_value" && !node.editable {
                return Err("Selected target is read-only.".into());
            }
            Ok(Control {
                kind: "human_act".into(),
                action_id: String::new(),
                observation_id: observation.observation_id.clone(),
                epoch: run.state.epoch,
                action: Some(Action {
                    action_id: format!("{}:human-{}", run.run_id, self.next + 1),
                    observation_id: observation.observation_id.clone(),
                    target_id: node.id.clone(),
                    kind: self.human_kind.clone(),
                    text: (self.human_kind == "set_value").then(|| self.human_text.clone()),
                    effect: "change".into(),
                }),
            })
        })();
        self.submit_control(result);
    }

    fn frame_ui(&mut self, ui: &mut egui::Ui) {
        let observation = self.displayed_observation.as_ref().or_else(|| {
            if self.selected_record.is_none() {
                self.run.as_ref().and_then(|r| r.observation.as_ref())
            } else {
                None
            }
        });
        if let Some(observation) = observation {
            ui.heading("Sanitized native capture");
            ui.label(format!(
                "{} · epoch {} · {}",
                observation.observation_id,
                observation.epoch,
                if observation.complete {
                    "complete"
                } else {
                    "incomplete"
                }
            ));
            if let Some((id, texture)) = &self.texture
                && id == &observation.observation_id
            {
                ui.add(egui::Image::new(texture).fit_to_exact_size(
                    texture.size_vec2() * (ui.available_width() / texture.size_vec2().x).min(1.0),
                ));
            } else {
                ui.label("No decoded screenshot is available for this observation.");
            }
            if !self.image_error.is_empty() {
                ui.label(&self.image_error);
            }
            egui::CollapsingHeader::new("Accessibility tree").show(ui, |ui| {
                for node in &observation.nodes {
                    ui.label(format!(
                        "{} · {} · {} · {}",
                        node.id,
                        node.role,
                        node.name,
                        node.value.as_deref().unwrap_or("")
                    ));
                }
            });
        }
    }

    fn anchor_ui(&mut self, ui: &mut egui::Ui) {
        egui::CollapsingHeader::new("Create a visual target from the latest capture").show(ui, |ui| {
            ui.label("Coordinates are pixels in the captured image. The coordinator rejects sensitive-region overlap, invisible or uniform templates, and oversized searches. Every visual click requires a fresh human approval; no input is sent by this editor.");
            let current = self.run.as_ref().and_then(|run| run.observation.as_ref().filter(|o| {
                !run.finished && o.complete && o.epoch == run.state.epoch && self.snapshot_valid
            }).map(|o| (run.run_id.clone(), o.observation_id.clone(), o.epoch, o.screenshot.width, o.screenshot.height)));
            let Some((run_id, observation_id, epoch, width, height)) = current else {
                ui.label("A complete current capture from an active run is required. Pause and capture the attached session before choosing an anchor.");
                return;
            };
            ui.label(format!("Source {observation_id} · {width}×{height} pixels"));
            editable_text(ui, "New visual target key", "anchor-key", &mut self.anchor_key, 96, false);
            rect_ui(ui, "Template crop (8–128 pixels per side)", &mut self.anchor_crop, width, height);
            rect_ui(ui, "Bounded search region", &mut self.anchor_search, width, height);
            ui.horizontal(|ui| {
                let label = ui.label("Click X inside template");
                ui.add(egui::DragValue::new(&mut self.anchor_click.x).range(0..=127)).labelled_by(label.id);
                let label = ui.label("Click Y inside template");
                ui.add(egui::DragValue::new(&mut self.anchor_click.y).range(0..=127)).labelled_by(label.id);
            });
            if self.editor.is_none() { ui.label("Load or compile a capability to receive this target."); }
            if ui.add_enabled(self.editor.is_some() && !self.busy("jarvis.anchor.create"), egui::Button::new("Create and insert visual anchor")).clicked() {
                let key = self.anchor_key.clone();
                self.request("jarvis.anchor.create", json!({"run_id":run_id,"observation_id":observation_id,"epoch":epoch,"crop":self.anchor_crop,"search":self.anchor_search,"click":self.anchor_click}), &key);
            }
        });
    }

    fn discovery_ui(&mut self, ui: &mut egui::Ui) {
        ui.heading("Discover a reusable workflow");
        ui.label("Discovery uses the configured Gemini provider and the approved native session. Review and compile its proposed capability before promotion.");
        self.target_inputs_ui(ui);
        editable_text(
            ui,
            "Discovery task",
            "discovery-task",
            &mut self.discovery_task,
            65536,
            false,
        );
        editable_text(
            ui,
            "Discovery member ID",
            "discovery-member",
            &mut self.discovery_member,
            64,
            true,
        );
        ui.small("The synthetic example defaults to member M-1001.");
        editable_text(
            ui,
            "Discovery subaccount name (optional)",
            "discovery-subaccount",
            &mut self.discovery_subaccount,
            64,
            false,
        );
        if ui
            .add_enabled(
                self.discovery_finished
                    && self.run.as_ref().is_none_or(|r| r.finished)
                    && !self.discovery_task.trim().is_empty()
                    && !self.busy("jarvis.discovery.start"),
                egui::Button::new("Start discovery"),
            )
            .clicked()
        {
            match self.discovery_inputs() {
                Ok(mut params) => {
                    params["task"] = json!(self.discovery_task);
                    self.request("jarvis.discovery.start", params, "");
                }
                Err(error) => self.error = error,
            }
        }
        if ui
            .add_enabled(
                !self.discovery_finished && !self.discovery_id.is_empty(),
                egui::Button::new("Cancel discovery"),
            )
            .clicked()
        {
            let epoch = self
                .discovery
                .get("state")
                .and_then(|s| s.get("epoch"))
                .and_then(Value::as_u64)
                .unwrap_or(0);
            self.request(
                "jarvis.run.control",
                json!({"run_id":self.discovery_id,"control":{"kind":"cancel","epoch":epoch}}),
                "",
            );
        }
        json_view(ui, &self.discovery);
        if !self.discovery_id.is_empty() && ui.button("Inspect live discovery session").clicked() {
            self.select_run(self.discovery_id.clone());
        }
        let published = self.discovery.get("capability").and_then(|c| {
            let id = c.get("id")?.as_str()?;
            let revision = c.get("revision")?.as_str()?;
            self.entries
                .iter()
                .find(|entry| entry.id == id && entry.revision == revision)
                .cloned()
        });
        if let Some(entry) = published
            && ui.button("Review discovered revision in editor").clicked()
        {
            self.capability_path = entry.path.clone();
            self.request(
                "jarvis.capability.read",
                json!({"path":entry.path}),
                &entry.path,
            );
            self.tab = Tab::Editor;
        }
    }

    fn discovery_inputs(&self) -> Result<Value, String> {
        if self.discovery_member.trim().is_empty() {
            return Err("A member ID is required for bank discovery.".into());
        }
        let mut params = json!({"tenant":self.tenant,"inputs":{"member_id":{"type":"string","text":self.discovery_member}}});
        if !self.discovery_subaccount.is_empty() {
            params["inputs"]["subaccount_name"] =
                json!({"type":"string","text":self.discovery_subaccount});
        }
        if !self.pid.is_empty() {
            let pid = self
                .pid
                .parse::<u32>()
                .map_err(|_| "Target PID must be positive")?;
            if pid == 0 {
                return Err("Target PID must be positive".into());
            }
            params["pid"] = json!(pid);
        }
        Ok(params)
    }

    fn diagnostics_ui(&mut self, ui: &mut egui::Ui) {
        ui.heading("Diagnostics");
        if ui.button("Run doctor").clicked() {
            self.request("jarvis.doctor", json!({}), "");
        }
        json_view(ui, &self.doctor);
        ui.separator();
        ui.heading("Gemini credential");
        ui.label("The backend keeps this credential in memory. Sending it clears this field; request payloads and backend stderr are never logged by this workbench.");
        editable_text(
            ui,
            "Gemini API key",
            "gemini-key",
            &mut self.key,
            16384,
            true,
        );
        if ui
            .add_enabled(
                !self.key.is_empty() && !self.busy("jarvis.key.set"),
                egui::Button::new("Set session credential"),
            )
            .clicked()
        {
            self.send_key();
        }
    }

    fn send_key(&mut self) {
        let key = std::mem::take(&mut self.key);
        self.request("jarvis.key.set", json!({"key":key}), "");
    }

    fn evidence_ui(&mut self, ui: &mut egui::Ui) {
        ui.heading("Acceptance evidence");
        if let Some(run) = &self.run {
            ui.label(format!("Evidence directory: {}", run.evidence_dir));
        }
        if ui
            .add_enabled(
                !self.run_id.is_empty() && self.run.as_ref().is_some_and(|r| r.finished),
                egui::Button::new("Verify recorded evidence"),
            )
            .clicked()
        {
            let id = self.run_id.clone();
            self.request("jarvis.evidence.verify", json!({"run_id":id}), &id);
        }
        json_view(ui, &self.evidence);
        editable_text(
            ui,
            "Export ZIP path",
            "export-path",
            &mut self.export_path,
            4096,
            false,
        );
        if ui
            .add_enabled(
                !self.run_id.is_empty()
                    && self.export_path.ends_with(".zip")
                    && self.run.as_ref().is_some_and(|r| r.finished),
                egui::Button::new("Export evidence bundle"),
            )
            .clicked()
        {
            let id = self.run_id.clone();
            self.request(
                "jarvis.evidence.export",
                json!({"run_id":id,"destination":self.export_path}),
                &id,
            );
        }
        ui.separator();
        ui.heading("Offline trace inspection & replay");
        ui.label("Replay inspects recorded data without issuing native input or requesting new model output.");
        editable_text(
            ui,
            "Trace path",
            "trace-path",
            &mut self.trace_path,
            4096,
            false,
        );
        if ui.button("Load offline trace").clicked() {
            self.request("jarvis.trace.load", json!({"path":self.trace_path}), "");
        }
        if ui
            .add_enabled(
                self.current_compilation()
                    && !self.capability_path.is_empty()
                    && !self.trace_path.is_empty(),
                egui::Button::new("Replay trace & compare final state"),
            )
            .clicked()
        {
            match self.invocation() {
                Ok(params) => {
                    self.request("jarvis.trace.replay", json!({"path":self.trace_path,"capability_path":self.capability_path,"inputs":params["inputs"]}), "");
                }
                Err(error) => self.error = error,
            }
        }
        json_view(ui, &self.trace);
    }
}

fn run_status(run: &RunSnapshot) -> String {
    if run.finished {
        format!(
            "{} · {}",
            if run.error.is_empty() {
                "Finished"
            } else {
                "Finished with error"
            },
            run.state.phase
        )
    } else {
        run.state.phase.clone()
    }
}

fn decode<T: serde::de::DeserializeOwned>(value: Value) -> Result<T, String> {
    serde_json::from_value(value).map_err(|error| format!("Backend schema mismatch: {error}"))
}
fn json_view(ui: &mut egui::Ui, value: &Value) {
    if value.is_null() {
        ui.label("No result loaded.");
        return;
    }
    struct Preview(Vec<u8>);
    impl std::io::Write for Preview {
        fn write(&mut self, bytes: &[u8]) -> std::io::Result<usize> {
            if self.0.len() + bytes.len() > 128 * 1024 {
                return Err(std::io::Error::other("preview limit"));
            }
            self.0.extend_from_slice(bytes);
            Ok(bytes.len())
        }
        fn flush(&mut self) -> std::io::Result<()> {
            Ok(())
        }
    }
    let mut preview = Preview(Vec::new());
    let truncated = serde_json::to_writer_pretty(&mut preview, value).is_err();
    ui.monospace(String::from_utf8_lossy(&preview.0));
    if truncated {
        ui.label("JSON preview stopped at 128 KiB. Export the evidence for the complete data.");
    }
}

impl eframe::App for Workbench {
    fn ui(&mut self, ui: &mut egui::Ui, _frame: &mut eframe::Frame) {
        self.draw(ui);
    }
}

#[cfg(test)]
mod tests {
    use super::{Pending, Tab, Workbench};
    use crate::model::{Capability, Input, Parameter, RunSnapshot};
    use crate::transport::{ApiError, Backend, Message, Response};
    use eframe::egui::{self, accesskit::Role};
    use egui_kittest::{
        Harness,
        kittest::{NodeT, Queryable},
    };
    use serde_json::{Value, json};
    use std::path::PathBuf;
    use std::sync::mpsc::{Receiver, SyncSender};
    use std::time::{Duration, Instant};

    fn app() -> (Workbench, Receiver<Vec<u8>>, SyncSender<Message>) {
        let (backend, input, output) = Backend::test_pair();
        let mut app = Workbench::new(
            backend,
            PathBuf::from("unused-test-backend"),
            PathBuf::from("/test-root"),
        );
        let hello: Value = serde_json::from_slice(&input.try_recv().unwrap()).unwrap();
        assert_eq!(hello["op"], "hello");
        assert_eq!(hello["params"]["protocol"], 1);
        app.pending.clear();
        app.ready = true;
        app.snapshot_valid = true;
        app.ops = [
            "jarvis.run.control",
            "jarvis.run.get",
            "jarvis.capability.compile",
            "jarvis.key.set",
            "jarvis.trace.load",
            "jarvis.run.assist",
            "jarvis.job.get",
        ]
        .into_iter()
        .map(String::from)
        .collect();
        app.next_poll = Instant::now() + Duration::from_secs(600);
        (app, input, output)
    }

    fn snapshot() -> RunSnapshot {
        serde_json::from_value(json!({
            "run_id":"run-1", "state": {"run_id":"run-1","session_id":"desktop-1","epoch":3,"sequence":14,
                "phase":"awaiting_approval","capability_sha256":"abc","step_index":0,
                "observation":{"id":"obs-14","target_id":"node-7","matches":1,"complete":true,"actionable":true}},
            "finished":false,"pending_action_id":"run-1:confirm","records":[]
        })).unwrap()
    }

    fn visual_snapshot() -> RunSnapshot {
        let anchor = json!({"png_base64":"test-only-unrendered","sha256":"a".repeat(64),"width":8,"height":8,"frame_width":32,"frame_height":32,"window_width":32.0,"window_height":32.0,"scale":1.0,"search":{"x":0,"y":0,"width":16,"height":16},"click":{"x":4,"y":4}});
        serde_json::from_value(json!({"run_id":"run-1","state":{"run_id":"run-1","session_id":"desktop-1","epoch":3,"sequence":14,"phase":"awaiting_approval","step_index":0,"observation":{"id":"obs-14","target_id":"visual-1","matches":1,"complete":true,"actionable":true,"visual":{"target_id":"visual-1","anchor_sha256":"a".repeat(64),"frame_sha256":"b".repeat(64),"matched":{"x":4,"y":4,"width":8,"height":8},"screen_bounds":{"x":4.0,"y":4.0,"width":8.0,"height":8.0}}}},"finished":false,"pending_action_id":"run-1:click","records":[],"capability":{"schema_version":1,"id":"visual","revision":"1","description":"","parameters":null,"steps":[{"id":"click","kind":"click","effect":"change","target":"icon"}],"targets":{"icon":{"visual":anchor}}},"observation":{"observation_id":"obs-14","epoch":3,"complete":true,"window":{"pid":1,"window_id":2,"title":"Bank","foreground":true},"nodes":[],"screenshot":{"mime_type":"image/png","base64":"","width":32,"height":32}}})).unwrap()
    }

    fn harness(app: Workbench) -> Harness<'static, Workbench> {
        Harness::builder()
            .with_size(egui::vec2(1280.0, 900.0))
            .build_ui_state(|ui, app| app.draw(ui), app)
    }

    fn pending(op: &str, subject: &str) -> Pending {
        Pending {
            op: op.into(),
            subject: subject.into(),
            sent: Instant::now(),
            source: None,
            cursor: 0,
            run_binding: None,
        }
    }

    #[test]
    fn structured_revision_edit_invalidates_compilation_without_dispatch() {
        let (mut app, input, _output) = app();
        let source = include_str!("../../../../examples/balance.json");
        app.tab = Tab::Editor;
        app.source = source.into();
        app.compiled_source = source.into();
        app.capability_path = "balance.json".into();
        app.compiled_path = app.capability_path.clone();
        app.digest = "old-compiled-digest".into();
        app.notice = "Compiled revision · old-compiled-digest".into();
        app.capability = Some(serde_json::from_str(source).unwrap());
        app.editor = Some(crate::editor::Document::from_compiled_source(source).unwrap());
        assert!(app.current_compilation());
        let mut harness = harness(app);
        harness.run_steps(3);
        assert!(
            harness.state().current_compilation(),
            "merely rendering forms must preserve compiled bytes"
        );
        let (target_node, target_tree) = harness
            .get_by_role_and_label(Role::TextInput, "Revision")
            .accesskit_node()
            .locate();
        harness.event(egui::Event::AccessKitActionRequest(
            egui::accesskit::ActionRequest {
                action: egui::accesskit::Action::SetValue,
                target_node,
                target_tree,
                data: Some(egui::accesskit::ActionData::Value("2".into())),
            },
        ));
        harness.run_steps(3);
        let edited: Value = serde_json::from_str(&harness.state().source).unwrap();
        assert_eq!(edited["revision"], "2");
        assert_eq!(edited["steps"][0]["input"]["key"], "member_id");
        assert!(!harness.state().current_compilation());
        assert_eq!(
            harness.state().notice,
            "Capability source changed. Compile before invoking or promoting."
        );
        assert!(
            input.try_recv().is_err(),
            "form editing cannot compile, invoke or approve implicitly"
        );
        let params = json!({"source":harness.state().source,"register":false});
        harness
            .state_mut()
            .request("jarvis.capability.compile", params, "");
        let request: Value = serde_json::from_slice(&input.try_recv().unwrap()).unwrap();
        assert_eq!(request["op"], "jarvis.capability.compile");
        assert_eq!(
            serde_json::from_str::<Value>(request["params"]["source"].as_str().unwrap()).unwrap()["revision"],
            "2"
        );
    }

    #[test]
    fn anchor_creation_cannot_replace_a_changed_editor() {
        let (mut app, _input, _output) = app();
        app.source = "changed".into();
        let mut request = pending("jarvis.anchor.create", "visual");
        request.source = Some("original".into());
        request.run_binding = Some(app.run_id.clone());
        let error = app
            .apply_result(&request, json!({"visual":{}}))
            .unwrap_err();
        assert!(error.contains("editor changed"));
        assert_eq!(app.source, "changed");
        assert!(app.editor.is_none());
    }

    #[test]
    fn session_switch_clears_approval_and_discards_previous_run_response() {
        let (mut app, input, _output) = app();
        app.run_id = "run-1".into();
        app.run = Some(snapshot());
        app.control_pending = Some((3, 14, "obs-14".into()));
        app.pending
            .insert("old-snapshot".into(), pending("jarvis.run.get", "run-1"));
        app.select_run("run-2".into());
        assert!(!app.snapshot_valid);
        assert!(app.control_pending.is_none());
        assert!(app.run.is_none());
        app.reply(
            &egui::Context::default(),
            Response {
                id: "old-snapshot".into(),
                ok: true,
                result: Some(json!({"run_id":"run-1"})),
                error: None,
            },
            None,
            None,
        );
        assert!(app.run.is_none());
        assert_eq!(app.run_id, "run-2");
        assert!(app.error.is_empty());
        assert!(
            input.try_recv().is_err(),
            "selecting a summary does not approve any action"
        );
    }

    #[test]
    fn historical_frame_keeps_original_run_binding_even_if_observation_ids_repeat() {
        let (mut app, _input, _output) = app();
        app.run_id = "new-run".into();
        let mut request = pending("jarvis.frame.get", "shared-observation-id");
        request.run_binding = Some("old-run".into());
        let error = app
            .apply_result(&request, json!({"observation":{}}))
            .unwrap_err();
        assert!(error.contains("old session's frame"));
        assert!(app.displayed_observation.is_none());
    }

    #[test]
    fn registry_summary_is_bounded_and_cannot_supply_an_approval() {
        let (mut app, input, _output) = app();
        let summary = json!({"run_id":"run-2","started_at":"2026-09-09T00:00:00Z","state":{"session_id":"desktop-2","epoch":3,"phase":"awaiting_approval"},"finished":false,"evidence_dir":"evidence/run-2"});
        app.apply_result(
            &pending("jarvis.run.list", ""),
            json!({"runs":[summary],"total_runs":1,"truncated":false,"desktop_owner":"run-2"}),
        )
        .unwrap();
        assert_eq!(app.run_list.desktop_owner, "run-2");
        assert!(app.run.is_none());
        assert!(input.try_recv().is_err());
        assert!(
            app.apply_result(
                &pending("jarvis.run.list", ""),
                json!({"runs":[summary,summary],"total_runs":2,"truncated":false})
            )
            .is_err()
        );
        assert_eq!(app.run_list.runs.len(), 1);
        let oversized: Vec<_> = (0..513)
            .map(|index| {
                let mut row = summary.clone();
                row["run_id"] = json!(format!("run-{index}"));
                row
            })
            .collect();
        assert!(
            app.apply_result(
                &pending("jarvis.run.list", ""),
                json!({"runs":oversized,"total_runs":513,"truncated":false})
            )
            .is_err()
        );
        assert_eq!(app.run_list.runs.len(), 1);
    }

    #[test]
    fn discovery_program_generation_allows_only_explicit_new_program_sequence_reset() {
        let (mut app, _input, _output) = app();
        app.run_id = "run-1".into();
        let mut previous = snapshot();
        previous.program_generation = 1;
        app.run = Some(previous);
        let same = json!({"run_id":"run-1","program_generation":1,"state":{"run_id":"run-1","session_id":"desktop-1","epoch":3,"sequence":1,"phase":"observing"},"finished":false,"records":[],"total_records":0});
        assert!(
            app.apply_result(&pending("jarvis.run.get", "run-1"), same.clone())
                .is_err()
        );
        let mut next = same;
        next["program_generation"] = json!(2);
        app.apply_result(&pending("jarvis.run.get", "run-1"), next.clone())
            .unwrap();
        assert_eq!(app.run.as_ref().unwrap().state.sequence, 1);
        next["program_generation"] = json!(3);
        next["state"]["epoch"] = json!(2);
        assert!(
            app.apply_result(&pending("jarvis.run.get", "run-1"), next)
                .is_err()
        );
        assert_eq!(app.run.as_ref().unwrap().program_generation, 2);
    }

    #[test]
    fn reconciliation_capture_never_enables_terminal_run_recovery() {
        let (mut app, input, _output) = app();
        app.run_id = "unknown-run".into();
        let value = json!({"run_id":"unknown-run","state":{"run_id":"unknown-run","session_id":"desktop-1","epoch":4,"sequence":15,"phase":"outcome_unknown"},"finished":true,"total_records":0,"records":[],"reconciliation":{"status":"observed","run_id":"unknown-run","session_id":"desktop-1","epoch":4,"action_id":"confirm","observation_id":"final-capture","reason":"Read-only observation after uncertain receipt","observed_facts":{"status":"Subaccount created"},"acceptance":"incomplete"}});
        app.apply_result(&pending("jarvis.run.get", "unknown-run"), value)
            .unwrap();
        let run = app.run.as_ref().unwrap();
        assert_eq!(run.state.phase, "outcome_unknown");
        assert!(run.control("resume").is_err());
        assert!(run.approval("approve").is_err());
        assert_eq!(
            run.reconciliation.as_ref().unwrap().acceptance,
            "incomplete"
        );
        assert!(input.try_recv().is_err());
    }

    #[test]
    fn approval_requires_one_explicit_click_and_exact_current_binding() {
        let (mut app, input, _output) = app();
        app.tab = Tab::Run;
        app.run_id = "run-1".into();
        app.run = Some(snapshot());
        let mut harness = harness(app);
        harness.run_steps(3);
        assert!(
            input.try_recv().is_err(),
            "drawing must not automatically approve"
        );
        harness
            .get_by_role_and_label(Role::Button, "Approve this action")
            .click();
        harness.run_steps(3);
        let sent: Value = serde_json::from_slice(&input.try_recv().unwrap()).unwrap();
        assert_eq!(sent["op"], "jarvis.run.control");
        assert_eq!(sent["params"]["run_id"], "run-1");
        assert_eq!(sent["params"]["control"]["action_id"], "run-1:confirm");
        assert_eq!(sent["params"]["control"]["observation_id"], "obs-14");
        assert_eq!(sent["params"]["control"]["epoch"], 3);
        assert!(
            harness
                .get_by_role_and_label(Role::Button, "Approve this action")
                .accesskit_node()
                .is_disabled()
        );
        harness
            .get_by_role_and_label(Role::Button, "Approve this action")
            .click();
        harness.run_steps(3);
        assert!(
            input.try_recv().is_err(),
            "duplicate click must not submit another grant"
        );
    }

    #[test]
    fn missing_or_ambiguous_approval_evidence_never_enables_approval() {
        for case in 0..5 {
            let mut run = snapshot();
            match case {
                0 => run.pending_action_id.clear(),
                1 => run.state.observation.complete = false,
                2 => run.state.observation.matches = 2,
                3 => run.state.run_id = "foreign".into(),
                _ => run.state.epoch = 0,
            }
            assert!(run.approval("approve").is_err());
        }
    }

    #[test]
    fn visual_approval_requires_template_match_and_bound_full_capture() {
        assert!(visual_snapshot().approval("approve").is_ok());
        for case in 0..7 {
            let mut run = visual_snapshot();
            match case {
                0 => run.state.observation.visual = None,
                1 => run.state.observation.visual.as_mut().unwrap().target_id = "foreign".into(),
                2 => run.state.observation.visual.as_mut().unwrap().anchor_sha256 = "c".repeat(64),
                3 => run
                    .state
                    .observation
                    .visual
                    .as_mut()
                    .unwrap()
                    .frame_sha256
                    .clear(),
                4 => run.state.observation.visual.as_mut().unwrap().matched.x = u32::MAX,
                5 => run.observation.as_mut().unwrap().observation_id = "stale".into(),
                _ => run.observation.as_mut().unwrap().screenshot.width += 1,
            }
            assert!(run.approval("approve").is_err(), "case {case}");
        }
    }

    #[test]
    fn visual_approval_button_stays_disabled_without_latest_decoded_picture() {
        let (mut app, input, _output) = app();
        app.tab = Tab::Run;
        app.run_id = "run-1".into();
        app.run = Some(visual_snapshot());
        let mut harness = harness(app);
        harness.run_steps(3);
        harness
            .get_by_role_and_label(Role::Button, "Approve this action")
            .click();
        harness.run_steps(3);
        assert!(
            input.try_recv().is_err(),
            "a visual approval cannot be reviewed without its decoded native capture"
        );
    }

    #[test]
    fn credential_editor_clears_and_rejection_never_echoes_key() {
        let (mut app, input, output) = app();
        app.tab = Tab::Diagnostics;
        app.key = "sensitive-test-key".into();
        let mut harness = harness(app);
        harness
            .get_by_role_and_label(Role::Button, "Set session credential")
            .click();
        harness.run_steps(3);
        assert!(harness.state().key.is_empty());
        let request: Value = serde_json::from_slice(&input.try_recv().unwrap()).unwrap();
        assert_eq!(request["params"]["key"], "sensitive-test-key");
        output
            .send(Message::Reply {
                response: Response {
                    id: request["id"].as_str().unwrap().into(),
                    ok: false,
                    result: None,
                    error: Some(ApiError {
                        code: "bad".into(),
                        message: "sensitive-test-key rejected".into(),
                    }),
                },
                frame: None,
                image_error: None,
            })
            .unwrap();
        harness.run_steps(3);
        assert!(!harness.state().error.contains("sensitive-test-key"));
        assert!(harness.state().error.contains("cleared"));
    }

    #[test]
    fn canonical_compiler_error_reaches_editor_without_local_substitution() {
        let (mut app, input, output) = app();
        app.tab = Tab::Editor;
        app.source = "{\"invalid\":true}".into();
        let mut harness = harness(app);
        harness
            .get_by_role_and_label(Role::Button, "Compile")
            .click();
        harness.run_steps(3);
        let request: Value = serde_json::from_slice(&input.try_recv().unwrap()).unwrap();
        assert_eq!(request["params"]["source"], "{\"invalid\":true}");
        output
            .send(Message::Reply {
                response: Response {
                    id: request["id"].as_str().unwrap().into(),
                    ok: false,
                    result: None,
                    error: Some(ApiError {
                        code: "E_COMPILE".into(),
                        message: "step save uses output before definition".into(),
                    }),
                },
                frame: None,
                image_error: None,
            })
            .unwrap();
        harness.run_steps(3);
        assert_eq!(
            harness.state().compiler_error,
            "E_COMPILE: step save uses output before definition"
        );
        assert!(!harness.state().current_compilation());
    }

    #[test]
    fn changed_path_invalidates_compiled_identity_and_stale_load_is_discarded() {
        let (mut app, _, _) = app();
        app.capability = Some(serde_json::from_value::<Capability>(json!({"schema_version":1,"id":"test","revision":"1","description":"","parameters":[],"steps":[]})).unwrap());
        app.source = "original".into();
        app.compiled_source = "original".into();
        app.digest = "hash".into();
        app.capability_path = "/a.json".into();
        app.compiled_path = "/a.json".into();
        assert!(app.current_compilation());
        app.capability_path = "/b.json".into();
        assert!(!app.current_compilation());
        let response = json!({"source":"replacement","digest":"other","capability":{"schema_version":1,"id":"test","revision":"2","description":"","parameters":[],"steps":[]}});
        assert!(
            app.apply_result(&pending("jarvis.capability.read", "/a.json"), response)
                .is_err()
        );
        assert_eq!(app.source, "original");
    }

    #[test]
    fn old_or_foreign_run_snapshots_do_not_replace_current_state() {
        let (mut app, _, _) = app();
        app.run_id = "run-1".into();
        app.run = Some(snapshot());
        for (epoch, sequence, run_id) in [(2, 15, "run-1"), (3, 13, "run-1"), (3, 15, "other")] {
            let value = json!({"run_id":run_id,"state":{"run_id":run_id,"epoch":epoch,"sequence":sequence},"finished":false,"records":[]});
            assert!(
                app.apply_result(&pending("jarvis.run.get", "run-1"), value)
                    .is_err()
            );
            assert_eq!(app.run.as_ref().unwrap().state.sequence, 14);
        }
    }

    #[test]
    fn failed_latest_snapshot_disables_stale_approval_until_fresh_snapshot() {
        let (mut app, input, _output) = app();
        app.tab = Tab::Run;
        app.run_id = "run-1".into();
        app.run = Some(snapshot());
        app.pending
            .insert("bad".into(), pending("jarvis.run.get", "run-1"));
        app.reply(
            &egui::Context::default(),
            Response {
                id: "bad".into(),
                ok: true,
                result: Some(json!({"run_id":"run-1","state":"malformed"})),
                error: None,
            },
            None,
            None,
        );
        assert!(!app.snapshot_valid);
        let mut harness = harness(app);
        assert!(
            harness
                .get_by_role_and_label(Role::Button, "Approve this action")
                .accesskit_node()
                .is_disabled()
        );
        harness
            .get_by_role_and_label(Role::Button, "Approve this action")
            .click();
        harness.run_steps(3);
        assert!(
            input.try_recv().is_err(),
            "a stale visible binding cannot authorize input"
        );
        let fresh = json!({
            "run_id":"run-1", "state": {"run_id":"run-1","session_id":"desktop-1","epoch":3,"sequence":14,
                "phase":"awaiting_approval","capability_sha256":"abc","step_index":0,
                "observation":{"id":"obs-14","target_id":"node-7","matches":1,"complete":true,"actionable":true}},
            "finished":false,"pending_action_id":"run-1:confirm","records":[], "total_records":0
        });
        harness
            .state_mut()
            .apply_result(&pending("jarvis.run.get", "run-1"), fresh)
            .unwrap();
        harness.run_steps(3);
        assert!(harness.state().snapshot_valid);
        assert!(
            !harness
                .get_by_role_and_label(Role::Button, "Approve this action")
                .accesskit_node()
                .is_disabled()
        );
        assert!(
            input.try_recv().is_err(),
            "fresh status restores choice without automatic approval"
        );
    }

    #[test]
    fn admission_failure_is_rendered_as_finished_with_error() {
        let (mut app, _, _) = app();
        app.tab = Tab::Run;
        app.run_id = "run-1".into();
        let mut run = snapshot();
        run.finished = true;
        run.state.phase = "admitted".into();
        run.error = "capture_permission: denied".into();
        app.run = Some(run);
        let harness = harness(app);
        assert!(
            harness
                .query_by_label("Finished with error · admitted")
                .is_some()
        );
    }

    #[test]
    fn pending_diagnostics_job_never_applies_a_result_or_blocks_explicit_approval() {
        let (mut app, input, _output) = app();
        app.tab = Tab::Run;
        app.run_id = "run-1".into();
        app.run = Some(snapshot());
        app.doctor = json!({"qualification":"old_result"});
        assert!(
            app.resolve_job(
                pending("jarvis.doctor", ""),
                json!({"job_id":"job-1","status":"pending"})
            )
            .unwrap()
            .is_none()
        );
        assert!(app.doctor.is_null());
        assert!(app.busy("jarvis.doctor"));
        let mut harness = harness(app);
        assert!(
            input.try_recv().is_err(),
            "pending jobs do not approve native actions"
        );
        harness
            .get_by_role_and_label(Role::Button, "Approve this action")
            .click();
        harness.run_steps(3);
        let request: Value = serde_json::from_slice(&input.try_recv().unwrap()).unwrap();
        assert_eq!(request["op"], "jarvis.run.control");
        assert_eq!(request["params"]["control"]["observation_id"], "obs-14");
        assert!(harness.state().doctor.is_null());
        let result = harness.state_mut().resolve_job(pending("jarvis.job.get", "job-1"), json!({"job_id":"job-1","operation":"jarvis.doctor","status":"succeeded","result":{"qualification":"not_tested"}})).unwrap().unwrap();
        harness
            .state_mut()
            .apply_result(&result.0, result.1)
            .unwrap();
        assert_eq!(harness.state().doctor["qualification"], "not_tested");
        assert!(!harness.state().busy("jarvis.doctor"));
    }

    #[test]
    fn asynchronous_jobs_reject_misbound_and_unsuccessful_results() {
        for payload in [
            json!({"job_id":"other","operation":"jarvis.doctor","status":"succeeded","result":{"qualification":"passed"}}),
            json!({"job_id":"job-1","operation":"jarvis.evidence.verify","status":"succeeded","result":{"verdict":"passed"}}),
            json!({"job_id":"job-1","operation":"jarvis.doctor","status":"running","result":{"qualification":"passed"}}),
            json!({"job_id":"job-1","operation":"jarvis.doctor","status":"failed","error":"capture denied"}),
            json!({"job_id":"job-1","operation":"jarvis.doctor","status":"cancelled"}),
        ] {
            let (mut app, _, _) = app();
            app.resolve_job(
                pending("jarvis.doctor", ""),
                json!({"job_id":"job-1","status":"pending"}),
            )
            .unwrap();
            assert!(
                app.resolve_job(pending("jarvis.job.get", "job-1"), payload)
                    .is_err()
            );
            assert!(app.doctor.is_null());
            assert!(app.evidence.is_null());
        }
    }

    #[test]
    fn asynchronous_evidence_keeps_original_run_binding() {
        let (mut app, _, _) = app();
        app.run_id = "run-1".into();
        app.resolve_job(
            pending("jarvis.evidence.verify", "run-1"),
            json!({"job_id":"job-1","status":"pending"}),
        )
        .unwrap();
        app.run_id = "run-2".into();
        let result = app.resolve_job(pending("jarvis.job.get", "job-1"), json!({"job_id":"job-1","operation":"jarvis.evidence.verify","status":"succeeded","result":{"verdict":"passed"}})).unwrap().unwrap();
        assert!(app.apply_result(&result.0, result.1).is_err());
        assert!(app.evidence.is_null());
    }

    #[test]
    fn job_polls_are_spaced_and_do_not_clear_visible_failures() {
        let (mut app, input, _output) = app();
        app.resolve_job(
            pending("jarvis.doctor", ""),
            json!({"job_id":"job-1","status":"pending"}),
        )
        .unwrap();
        app.error = "A prior control was refused.".into();
        app.poll(&egui::Context::default());
        assert!(
            input.try_recv().is_err(),
            "job receipt does not start an immediate polling loop"
        );
        app.jobs.get_mut("job-1").unwrap().next_poll = Instant::now();
        app.poll(&egui::Context::default());
        let request: Value = serde_json::from_slice(&input.try_recv().unwrap()).unwrap();
        assert_eq!(request["op"], "jarvis.job.get");
        assert_eq!(request["params"], json!({"id":"job-1"}));
        assert_eq!(app.error, "A prior control was refused.");
        app.poll(&egui::Context::default());
        assert!(
            input.try_recv().is_err(),
            "one pending poll must not produce duplicate requests"
        );
    }

    #[test]
    fn hung_job_stops_at_deadline_without_reissuing_the_operation() {
        let (mut app, input, _output) = app();
        let mut original = pending("jarvis.evidence.export", "run-1");
        original.sent = Instant::now() - Duration::from_secs(46);
        app.resolve_job(original, json!({"job_id":"job-1","status":"pending"}))
            .unwrap();
        app.poll(&egui::Context::default());
        assert!(!app.ready);
        assert!(app.jobs.is_empty());
        assert!(input.try_recv().is_err());
        assert!(app.error.contains("no operation was retried"));
    }

    #[test]
    fn typed_money_keeps_minor_units_and_boolean_is_not_a_string() {
        let mut input = Input {
            parameter: Parameter {
                name: "amount".into(),
                kind: "money".into(),
                sensitive: false,
            },
            text: "125000".into(),
            currency: "USD".into(),
            boolean: false,
        };
        assert_eq!(
            input.value().unwrap(),
            json!({"type":"money","integer":125000,"currency":"USD"})
        );
        input.text = "1250.00".into();
        assert!(input.value().is_err());
        input.parameter.kind = "boolean".into();
        input.boolean = true;
        assert_eq!(
            input.value().unwrap(),
            json!({"type":"boolean","boolean":true})
        );
    }

    #[test]
    fn offline_trace_action_uses_only_the_offline_operation() {
        let (mut app, input, _output) = app();
        app.tab = Tab::Evidence;
        app.trace_path = "/run/trace.json".into();
        let mut harness = harness(app);
        harness
            .get_by_role_and_label(Role::Button, "Load offline trace")
            .click();
        harness.run_steps(3);
        let request: Value = serde_json::from_slice(&input.try_recv().unwrap()).unwrap();
        assert_eq!(request["op"], "jarvis.trace.load");
        assert!(input.try_recv().is_err());
    }

    #[test]
    fn assisted_recovery_requires_opt_in_and_one_explicit_request() {
        let (mut app, input, _output) = app();
        app.tab = Tab::Run;
        app.run_id = "run-1".into();
        let mut run = snapshot();
        run.state.phase = "paused".into();
        app.run = Some(run);
        let mut harness = harness(app);
        assert!(
            harness
                .get_by_role_and_label(Role::Button, "Request one assisted recovery")
                .accesskit_node()
                .is_disabled()
        );
        assert!(input.try_recv().is_err());
        harness.state_mut().run_assisted = true;
        harness.run_steps(3);
        harness
            .get_by_role_and_label(Role::Button, "Request one assisted recovery")
            .click();
        harness.run_steps(3);
        let request: Value = serde_json::from_slice(&input.try_recv().unwrap()).unwrap();
        assert_eq!(request["op"], "jarvis.run.assist");
        assert_eq!(request["params"], json!({"run_id":"run-1"}));
        assert!(
            input.try_recv().is_err(),
            "assistance must not automatically resume"
        );
    }

    #[test]
    fn paused_capture_releases_ui_control_lock_without_reducing_workflow_sequence() {
        let (mut app, _, _) = app();
        app.run_id = "run-1".into();
        app.run = Some(snapshot());
        app.control_pending = Some((3, 14, "old-capture".into()));
        let value = json!({"run_id":"run-1","state":{"run_id":"run-1","session_id":"desktop-1","epoch":3,"sequence":14,"phase":"paused"},
            "finished":false,"records":[],"total_records":0,
            "observation":{"observation_id":"new-capture","epoch":3,"complete":true,"window":{"pid":1,"window_id":1,"title":"Bank","foreground":true},"nodes":[],
                "screenshot":{"mime_type":"image/png","base64":"","width":1,"height":1}}});
        app.apply_result(&pending("jarvis.run.get", "run-1"), value)
            .unwrap();
        assert!(app.control_pending.is_none());
        assert_eq!(app.run.as_ref().unwrap().state.sequence, 14);
    }

    #[test]
    fn finished_control_notice_does_not_keep_waiting_in_either_response_order() {
        for response_first in [true, false] {
            let (mut app, _, _) = app();
            app.run_id = "run-1".into();
            app.run = Some(snapshot());
            app.control_pending = Some((3, 14, "old-capture".into()));
            let completed = json!({"run_id":"run-1","state":{"run_id":"run-1","session_id":"desktop-1","epoch":4,"sequence":15,"phase":"cancelled"},"finished":true,"records":[],"total_records":0});
            if response_first {
                app.apply_result(&pending("jarvis.run.control", "run-1"), json!({}))
                    .unwrap();
            }
            app.apply_result(&pending("jarvis.run.get", "run-1"), completed)
                .unwrap();
            if !response_first {
                app.apply_result(&pending("jarvis.run.control", "run-1"), json!({}))
                    .unwrap();
            }
            assert_eq!(app.notice, "Coordinator state: Finished · cancelled");
        }
    }

    #[test]
    fn watchdog_stops_connection_without_retrying_uncertain_request() {
        let (mut app, input, _output) = app();
        let mut request = pending("jarvis.run.control", "run-1");
        request.sent = Instant::now() - Duration::from_secs(31);
        app.pending.insert("uncertain".into(), request);
        app.poll(&egui::Context::default());
        assert!(!app.ready);
        assert!(app.pending.is_empty());
        assert!(app.error.contains("no operation was retried"));
        assert!(input.try_recv().is_err());
    }
}
