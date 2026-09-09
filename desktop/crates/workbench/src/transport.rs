use crate::model::Observation;
use base64::{Engine, engine::general_purpose::STANDARD};
use eframe::egui::ColorImage;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::io::{BufRead, BufReader, Cursor, Read, Write};
use std::path::Path;
use std::process::{Command, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::mpsc::{self, Receiver, SyncSender, TryRecvError, TrySendError};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

pub const MAX_PENDING: usize = 8;
pub const MAX_RESPONSE: usize = 20 * 1024 * 1024;
pub const MAX_REQUEST: usize = 4 * 1024 * 1024;
static SUPERVISORS: Mutex<Vec<std::thread::JoinHandle<()>>> = Mutex::new(Vec::new());

/// Called after the native event loop returns, so process reaping never blocks
/// a live UI frame and child processes are not orphaned when main exits.
pub fn finish_shutdown() -> Result<(), String> {
    let handles = std::mem::take(
        &mut *SUPERVISORS
            .lock()
            .map_err(|_| "Backend supervisor lock failed")?,
    );
    for handle in handles {
        handle
            .join()
            .map_err(|_| "Backend supervisor failed during shutdown")?;
    }
    Ok(())
}

#[derive(Serialize)]
pub struct Request {
    pub id: String,
    pub op: String,
    pub params: Value,
}

#[derive(Deserialize)]
pub struct ApiError {
    pub code: String,
    pub message: String,
}

#[derive(Deserialize)]
pub struct Response {
    pub id: String,
    pub ok: bool,
    pub result: Option<Value>,
    pub error: Option<ApiError>,
}

pub struct Frame {
    pub observation_id: String,
    pub image: ColorImage,
}

pub enum Message {
    Reply {
        response: Response,
        frame: Option<Frame>,
        image_error: Option<String>,
    },
    Disconnected(String),
}

/// Bounded queues and dedicated pipe threads keep all process I/O and image
/// decoding off the UI thread. No automatic retry of uncertain operations.
pub struct Backend {
    requests: SyncSender<Vec<u8>>,
    messages: Receiver<Message>,
    stop: Arc<AtomicBool>,
}

impl Backend {
    pub fn spawn(binary: &Path, root: &Path) -> Result<Self, String> {
        let mut child = Command::new(binary)
            .arg("serve")
            .arg("--root")
            .arg(root)
            .current_dir(root)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::null())
            .spawn()
            .map_err(|e| format!("Cannot start backend: {e}"))?;
        let Some(mut input) = child.stdin.take() else {
            return Err("Backend stdin is unavailable".into());
        };
        let Some(output) = child.stdout.take() else {
            return Err("Backend stdout is unavailable".into());
        };
        let (requests, incoming) = mpsc::sync_channel::<Vec<u8>>(MAX_PENDING);
        let (outgoing, messages) = mpsc::sync_channel(MAX_PENDING);
        let stop = Arc::new(AtomicBool::new(false));
        let writer_stop = stop.clone();
        std::thread::spawn(move || {
            while !writer_stop.load(Ordering::Acquire) {
                match incoming.recv_timeout(Duration::from_millis(50)) {
                    Ok(mut bytes) => {
                        let failed = input
                            .write_all(&bytes)
                            .and_then(|()| input.flush())
                            .is_err();
                        bytes.fill(0);
                        if failed {
                            writer_stop.store(true, Ordering::Release);
                            break;
                        }
                    }
                    Err(mpsc::RecvTimeoutError::Timeout) => {}
                    Err(mpsc::RecvTimeoutError::Disconnected) => break,
                }
            }
        });
        let reader_stop = stop.clone();
        std::thread::spawn(move || {
            read_responses(output, &outgoing, &reader_stop);
            reader_stop.store(true, Ordering::Release);
        });
        let supervisor_stop = stop.clone();
        let supervisor = std::thread::spawn(move || {
            loop {
                match child.try_wait() {
                    Ok(Some(_)) => break,
                    Ok(None) if !supervisor_stop.load(Ordering::Acquire) => {
                        std::thread::sleep(Duration::from_millis(50))
                    }
                    _ => {
                        // The writer closes stdin first; allow bounded graceful
                        // backend cancellation before forcing process teardown.
                        let deadline = Instant::now() + Duration::from_millis(750);
                        while Instant::now() < deadline {
                            if matches!(child.try_wait(), Ok(Some(_))) {
                                return;
                            }
                            std::thread::sleep(Duration::from_millis(25));
                        }
                        let _ = child.kill();
                        let _ = child.wait();
                        break;
                    }
                }
            }
            supervisor_stop.store(true, Ordering::Release);
        });
        let mut supervisors = SUPERVISORS
            .lock()
            .map_err(|_| "Backend supervisor lock failed")?;
        let mut active = Vec::new();
        for prior in supervisors.drain(..) {
            if prior.is_finished() {
                prior
                    .join()
                    .map_err(|_| "Previous backend supervisor failed")?;
            } else {
                active.push(prior);
            }
        }
        active.push(supervisor);
        *supervisors = active;
        Ok(Self {
            requests,
            messages,
            stop,
        })
    }

    pub fn send(&self, request: Request) -> Result<(), String> {
        if self.stop.load(Ordering::Acquire) {
            return Err("Backend is disconnected; restart explicitly before continuing.".into());
        }
        let mut bytes = serde_json::to_vec(&request).map_err(|_| "Cannot serialize request")?;
        if bytes.len() >= MAX_REQUEST {
            return Err("Request exceeds 4 MiB".into());
        }
        bytes.push(b'\n');
        self.requests.try_send(bytes).map_err(|e| match e {
            TrySendError::Full(mut bytes) => {
                bytes.fill(0);
                "Backend queue is full; no request was submitted.".into()
            }
            TrySendError::Disconnected(mut bytes) => {
                bytes.fill(0);
                "Backend writer disconnected.".into()
            }
        })
    }

    pub fn poll(&self) -> Result<Message, TryRecvError> {
        self.messages.try_recv()
    }
    pub fn stop(&self) {
        self.stop.store(true, Ordering::Release);
    }

    #[cfg(test)]
    pub fn test_pair() -> (Self, Receiver<Vec<u8>>, SyncSender<Message>) {
        let (requests, incoming) = mpsc::sync_channel(MAX_PENDING);
        let (outgoing, messages) = mpsc::sync_channel(MAX_PENDING);
        (
            Self {
                requests,
                messages,
                stop: Arc::new(AtomicBool::new(false)),
            },
            incoming,
            outgoing,
        )
    }
}

impl Drop for Backend {
    fn drop(&mut self) {
        self.stop();
    }
}

fn bounded_line(reader: &mut impl BufRead) -> Result<Option<Vec<u8>>, String> {
    let mut bytes = Vec::new();
    let read = reader
        .take(MAX_RESPONSE as u64 + 1)
        .read_until(b'\n', &mut bytes)
        .map_err(|_| "Backend read failed")?;
    if read == 0 {
        return Ok(None);
    }
    if bytes.len() > MAX_RESPONSE || bytes.last() != Some(&b'\n') {
        return Err("Backend emitted an oversized or unterminated response.".into());
    }
    Ok(Some(bytes))
}

fn read_responses(output: impl Read, outgoing: &SyncSender<Message>, stop: &AtomicBool) {
    let mut reader = BufReader::new(output);
    let result = (|| -> Result<(), String> {
        while !stop.load(Ordering::Acquire) {
            let Some(bytes) = bounded_line(&mut reader)? else {
                return Err("Backend closed its response stream.".into());
            };
            let response: Response = serde_json::from_slice(&bytes)
                .map_err(|_| "Backend emitted invalid JSON; raw response withheld.")?;
            if response.id.is_empty()
                || (response.ok && (response.result.is_none() || response.error.is_some()))
                || (!response.ok && response.error.is_none())
            {
                return Err("Backend response envelope is inconsistent.".into());
            }
            let mut frame = None;
            let mut image_error = None;
            if response.ok
                && let Some(value) = response.result.as_ref().and_then(frame_observation)
                && !value.is_null()
            {
                match serde_json::from_value::<Observation>(value.clone()) {
                    Ok(observation) if !observation.screenshot.base64.is_empty() => {
                        match decode_frame(&observation) {
                            Ok(decoded) => frame = Some(decoded),
                            Err(error) => image_error = Some(error),
                        }
                    }
                    Ok(_) => {}
                    Err(_) => {
                        image_error =
                            Some("Observation image envelope could not be decoded.".into())
                    }
                }
            }
            if outgoing
                .try_send(Message::Reply {
                    response,
                    frame,
                    image_error,
                })
                .is_err()
            {
                return Err("Backend response queue overflowed; connection stopped.".into());
            }
        }
        Ok(())
    })();
    if let Err(error) = result {
        let _ = outgoing.try_send(Message::Disconnected(error));
    }
}

fn frame_observation(result: &Value) -> Option<&Value> {
    if result.get("job_id").is_some() {
        if result.get("status").and_then(Value::as_str) != Some("succeeded")
            || result.get("operation").and_then(Value::as_str) != Some("jarvis.frame.get")
            || result.get("error").is_some_and(|error| !error.is_null())
        {
            return None;
        }
        result.get("result")?.get("observation")
    } else {
        result.get("observation")
    }
}

pub fn decode_frame(observation: &Observation) -> Result<Frame, String> {
    let shot = &observation.screenshot;
    if !observation.complete
        || observation.observation_id.is_empty()
        || shot.mime_type != "image/png"
        || shot.base64.len() > 16 * 1024 * 1024
        || shot.width == 0
        || shot.height == 0
        || u64::from(shot.width) * u64::from(shot.height) > 8_000_000
    {
        return Err("Incomplete, oversized or unsupported sanitized frame.".into());
    }
    let bytes = STANDARD
        .decode(&shot.base64)
        .map_err(|_| "Frame base64 is invalid")?;
    let mut reader = image::ImageReader::with_format(Cursor::new(&bytes), image::ImageFormat::Png);
    let mut limits = image::Limits::default();
    limits.max_image_width = Some(shot.width);
    limits.max_image_height = Some(shot.height);
    limits.max_alloc = Some(64 * 1024 * 1024);
    reader.limits(limits);
    let decoded = reader
        .decode()
        .map_err(|_| "PNG decode failed or exceeded image bounds")?;
    if decoded.width() != shot.width || decoded.height() != shot.height {
        return Err("Frame dimensions disagree.".into());
    }
    let pixels = decoded.into_rgba8();
    Ok(Frame {
        observation_id: observation.observation_id.clone(),
        image: ColorImage::from_rgba_unmultiplied(
            [shot.width as usize, shot.height as usize],
            pixels.as_raw(),
        ),
    })
}

#[cfg(test)]
mod tests {
    use super::{Backend, MAX_PENDING, Request, bounded_line};
    use std::io::Cursor;

    #[test]
    fn line_framing_refuses_truncation_and_keeps_adjacent_responses_separate() {
        let mut input = Cursor::new(b"{}\n{}\n");
        assert_eq!(bounded_line(&mut input).unwrap().unwrap(), b"{}\n");
        assert_eq!(bounded_line(&mut input).unwrap().unwrap(), b"{}\n");
        assert!(bounded_line(&mut input).unwrap().is_none());
        assert!(bounded_line(&mut Cursor::new(b"{}")).is_err());
    }

    #[test]
    fn image_worker_only_unwraps_successful_frame_jobs() {
        let result = serde_json::json!({"job_id":"job-1","operation":"jarvis.frame.get","status":"succeeded","result":{"observation":{"observation_id":"obs-1"}}});
        assert_eq!(
            super::frame_observation(&result).unwrap()["observation_id"],
            "obs-1"
        );
        for status in ["pending", "running", "failed", "cancelled"] {
            let mut changed = result.clone();
            changed["status"] = status.into();
            assert!(super::frame_observation(&changed).is_none());
        }
        let mut changed = result.clone();
        changed["operation"] = "jarvis.doctor".into();
        assert!(super::frame_observation(&changed).is_none());
        changed = result;
        changed["error"] = "inconsistent success".into();
        assert!(super::frame_observation(&changed).is_none());
    }

    #[test]
    fn full_queue_refuses_without_blocking_or_retrying() {
        let (backend, input, _) = Backend::test_pair();
        for i in 0..MAX_PENDING {
            backend
                .send(Request {
                    id: i.to_string(),
                    op: "operation".into(),
                    params: serde_json::json!({}),
                })
                .unwrap();
        }
        assert!(
            backend
                .send(Request {
                    id: "overflow".into(),
                    op: "operation".into(),
                    params: serde_json::json!({})
                })
                .is_err()
        );
        assert_eq!(input.try_iter().count(), MAX_PENDING);
    }

    #[test]
    fn screenshot_decoder_checks_completeness_dimensions_and_png() {
        use base64::{Engine, engine::general_purpose::STANDARD};
        let mut encoded = Cursor::new(Vec::new());
        image::DynamicImage::new_rgba8(2, 1)
            .write_to(&mut encoded, image::ImageFormat::Png)
            .unwrap();
        let mut observation: crate::model::Observation = serde_json::from_value(serde_json::json!({
            "observation_id":"capture-1","epoch":1,"complete":true,
            "window":{"pid":1,"window_id":1,"title":"test","foreground":true},"nodes":[],
            "screenshot":{"mime_type":"image/png","base64":STANDARD.encode(encoded.into_inner()),"width":2,"height":1}
        })).unwrap();
        assert_eq!(
            super::decode_frame(&observation).unwrap().image.size,
            [2, 1]
        );
        observation.complete = false;
        assert!(super::decode_frame(&observation).is_err());
        observation.complete = true;
        observation.screenshot.width = 3;
        assert!(super::decode_frame(&observation).is_err());
        observation.screenshot.width = u32::MAX;
        assert!(super::decode_frame(&observation).is_err());
        observation.screenshot.width = 2;
        observation.screenshot.base64 = "not PNG".into();
        assert!(super::decode_frame(&observation).is_err());
    }

    #[test]
    fn malformed_stream_withholds_raw_content_and_inconsistent_envelopes() {
        use std::sync::atomic::AtomicBool;
        for raw in [b"secret text\n".as_slice(), b"{\"id\":\"x\",\"ok\":true}\n"] {
            let (out, input) = std::sync::mpsc::sync_channel(8);
            super::read_responses(Cursor::new(raw), &out, &AtomicBool::new(false));
            match input.try_recv().unwrap() {
                super::Message::Disconnected(message) => assert!(!message.contains("secret text")),
                _ => panic!("invalid stream accepted"),
            }
        }
    }

    #[test]
    #[ignore = "requires explicit JARVIS_TEST_BACKEND and JARVIS_TEST_ROOT; read-only real backend integration"]
    fn real_backend_handshake_catalog_and_compiler_error() {
        use std::time::{Duration, Instant};
        let binary = std::env::var("JARVIS_TEST_BACKEND").expect("JARVIS_TEST_BACKEND required");
        let root = std::env::var("JARVIS_TEST_ROOT").expect("JARVIS_TEST_ROOT required");
        let backend =
            Backend::spawn(std::path::Path::new(&binary), std::path::Path::new(&root)).unwrap();
        let call = |id: &str, op: &str, params| {
            backend
                .send(Request {
                    id: id.into(),
                    op: op.into(),
                    params,
                })
                .unwrap();
            let deadline = Instant::now() + Duration::from_secs(5);
            loop {
                assert!(Instant::now() < deadline, "backend reply timed out");
                match backend.poll() {
                    Ok(super::Message::Reply { response, .. }) => {
                        assert_eq!(response.id, id);
                        break response;
                    }
                    Ok(super::Message::Disconnected(error)) => panic!("{error}"),
                    Err(std::sync::mpsc::TryRecvError::Empty) => {
                        std::thread::sleep(Duration::from_millis(10))
                    }
                    Err(error) => panic!("{error}"),
                }
            }
        };
        for (id, op, params, expected_ok) in [
            (
                "hello",
                "hello",
                serde_json::json!({"protocol":1,"host":"workbench-test"}),
                true,
            ),
            (
                "catalog",
                "jarvis.catalog.list",
                serde_json::json!({}),
                true,
            ),
            (
                "compile",
                "jarvis.capability.compile",
                serde_json::json!({"source":"{}","register":false}),
                false,
            ),
        ] {
            assert_eq!(call(id, op, params).ok, expected_ok);
        }
        let registry = call("registry", "jarvis.run.list", serde_json::json!({}));
        assert!(registry.ok);
        let registry: crate::model::RunList =
            serde_json::from_value(registry.result.unwrap()).unwrap();
        assert_eq!(registry.total_runs, 0);
        assert!(registry.runs.is_empty());
        assert!(!registry.truncated);
        let mut source = include_str!("../../../../examples/balance.json").to_owned();
        // The native package owns the canonical fixture. An explicit path lets
        // this opt-in test prove Rust UI -> Go compiler compatibility without
        // copying an independently maintained fixture into Jarvis.
        if let Ok(path) = std::env::var("JARVIS_TEST_VISUAL_ANCHOR") {
            let bytes = std::fs::read(path).unwrap();
            assert!(bytes.len() <= 128 * 1024);
            let anchor: crate::editor::VisualAnchor = serde_json::from_slice(&bytes).unwrap();
            let mut document: serde_json::Value = serde_json::from_str(&source).unwrap();
            document["targets"]["visual_icon"] = serde_json::json!({"visual":anchor});
            document["steps"].as_array_mut().unwrap().insert(0, serde_json::json!({"id":"visual_click","kind":"click","target":"visual_icon","effect":"change"}));
            source = serde_json::to_string_pretty(&document).unwrap();
        }
        let accepted = call(
            "source-compile",
            "jarvis.capability.compile",
            serde_json::json!({"source":source,"register":false}),
        );
        assert!(
            accepted.ok,
            "canonical source compile: {}",
            accepted
                .error
                .as_ref()
                .map_or("missing error", |error| error.message.as_str())
        );
        let document = crate::editor::Document::from_compiled_source(&source).unwrap();
        let edited = document.source().unwrap();
        let round_trip = call(
            "form-compile",
            "jarvis.capability.compile",
            serde_json::json!({"source":edited,"register":false}),
        );
        assert!(
            round_trip.ok,
            "structured form compile: {}",
            round_trip
                .error
                .as_ref()
                .map_or("missing error", |error| error.message.as_str())
        );
        assert_eq!(
            accepted.result.unwrap()["capability"],
            round_trip.result.unwrap()["capability"],
            "structured forms must preserve every compiled field, including exact PNG and digest"
        );
        // An unknown run exercises the real asynchronous protocol without
        // opening a broker, verifier or native application.
        for (operation, params) in [
            (
                "jarvis.evidence.verify",
                serde_json::json!({"run_id":"missing-workbench-test-run"}),
            ),
            (
                "jarvis.anchor.create",
                serde_json::json!({"run_id":"missing-workbench-test-run","observation_id":"missing","epoch":1,"crop":{"x":0,"y":0,"width":8,"height":8},"search":{"x":0,"y":0,"width":64,"height":64},"click":{"x":4,"y":4}}),
            ),
        ] {
            let admitted = call("job-start", operation, params);
            assert!(admitted.ok);
            let admitted = admitted.result.unwrap();
            assert_eq!(admitted["status"], "pending");
            let id = admitted["job_id"].as_str().unwrap();
            let deadline = Instant::now() + Duration::from_secs(5);
            loop {
                assert!(
                    Instant::now() < deadline,
                    "failed job did not reach a terminal state"
                );
                let response = call("job-poll", "jarvis.job.get", serde_json::json!({"id":id}));
                assert!(response.ok);
                let result = response.result.unwrap();
                assert_eq!(result["job_id"], id);
                assert_eq!(result["operation"], operation);
                match result["status"].as_str().unwrap() {
                    "pending" | "running" => std::thread::sleep(Duration::from_millis(250)),
                    "failed" => {
                        assert!(!result["error"].as_str().unwrap().is_empty());
                        assert!(result.get("result").is_none());
                        break;
                    }
                    status => panic!("unexpected job status: {status}"),
                }
            }
        }
        drop(backend);
        super::finish_shutdown().unwrap();
    }
}
