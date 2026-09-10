use crate::editor::{PixelRect, VisualAnchor};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::BTreeMap;

#[derive(Clone, Deserialize)]
pub struct Entry {
    pub id: String,
    pub revision: String,
    #[serde(alias = "digest")]
    pub sha256: String,
    pub path: String,
    pub promoted: bool,
    #[serde(default)]
    pub description: String,
}

#[derive(Clone, Deserialize)]
pub struct Parameter {
    pub name: String,
    #[serde(rename = "type")]
    pub kind: String,
    pub sensitive: bool,
}

#[derive(Clone, Deserialize)]
pub struct OutputDecl {
    pub name: String,
    #[serde(rename = "type")]
    pub kind: String,
    #[serde(default)]
    pub currency: String,
    #[serde(default)]
    pub description: String,
}

#[derive(Clone, Deserialize)]
pub struct OutcomeDef {
    pub id: String,
    pub kind: String,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub outputs: Vec<String>,
}

#[derive(Clone, Deserialize)]
pub struct Recovery {
    pub id: String,
    pub when: RecoveryWhen,
    pub action: RecoveryAction,
    pub max: u32,
}

#[derive(Clone, Deserialize)]
pub struct RecoveryWhen {
    pub target: String,
    #[serde(default)]
    pub predicate: Value,
}

#[derive(Clone, Deserialize)]
pub struct RecoveryAction {
    pub kind: String,
    pub target: String,
}

#[derive(Clone, Deserialize)]
pub struct Step {
    pub id: String,
    pub kind: String,
    #[serde(default)]
    pub target: String,
    #[serde(default)]
    pub effect: String,
    #[serde(default)]
    pub outcome: String,
}

#[derive(Clone, Deserialize)]
pub struct Capability {
    pub schema_version: u32,
    pub id: String,
    pub revision: String,
    pub description: String,
    #[serde(default, deserialize_with = "null_vec")]
    pub parameters: Vec<Parameter>,
    #[serde(default, deserialize_with = "null_vec")]
    pub outputs: Vec<OutputDecl>,
    #[serde(default, deserialize_with = "null_vec")]
    pub outcomes: Vec<OutcomeDef>,
    #[serde(default, deserialize_with = "null_vec")]
    pub recoveries: Vec<Recovery>,
    #[serde(default, deserialize_with = "null_vec")]
    pub steps: Vec<Step>,
    #[serde(default, deserialize_with = "null_map")]
    pub targets: BTreeMap<String, ApprovalSelector>,
}

#[derive(Clone, Deserialize)]
pub struct ApprovalSelector {
    pub visual: Option<VisualAnchor>,
    #[serde(default)]
    pub role: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub identifier: String,
    #[serde(default)]
    pub ancestor: String,
    #[serde(default)]
    pub strategies: Vec<ApprovalSelector>,
    #[serde(default)]
    pub rationale: String,
    #[serde(default)]
    pub stability: String,
}

impl ApprovalSelector {
    pub fn visual_anchor(&self) -> Option<&VisualAnchor> {
        self.visual.as_ref().or_else(|| {
            self.strategies
                .iter()
                .find_map(ApprovalSelector::visual_anchor)
        })
    }
}

#[derive(Clone, Deserialize, Serialize)]
pub struct VisualMatch {
    pub target_id: String,
    pub anchor_sha256: String,
    #[serde(default)]
    pub frame_sha256: String,
    pub matched: PixelRect,
    pub screen_bounds: Bounds,
}

#[derive(Clone, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct WorkflowObservation {
    pub id: String,
    pub target_id: String,
    pub matches: u32,
    pub complete: bool,
    pub actionable: bool,
    pub reason: String,
    pub visual: Option<VisualMatch>,
}

#[derive(Clone, Default, Deserialize)]
#[serde(default)]
pub struct State {
    pub run_id: String,
    pub session_id: String,
    pub epoch: u64,
    pub capability_sha256: String,
    pub sequence: u64,
    pub phase: String,
    pub step_index: usize,
    pub observation: WorkflowObservation,
    pub outputs: Option<BTreeMap<String, Value>>,
    pub reason: String,
}

#[derive(Clone, Deserialize, Serialize)]
pub struct Node {
    pub id: String,
    pub role: String,
    pub name: String,
    pub value: Option<String>,
    pub enabled: bool,
    pub editable: bool,
    #[serde(default, deserialize_with = "null_vec")]
    pub actions: Vec<String>,
    #[serde(default, deserialize_with = "null_vec")]
    pub native_path: Vec<u32>,
    #[serde(default)]
    pub identifier: Option<String>,
    #[serde(default)]
    pub bounds: Option<Bounds>,
}

#[derive(Clone, Default, Deserialize, Serialize, PartialEq)]
pub struct Bounds {
    pub x: f64,
    pub y: f64,
    pub width: f64,
    pub height: f64,
}

fn null_vec<'de, D, T>(deserializer: D) -> Result<Vec<T>, D::Error>
where
    D: serde::Deserializer<'de>,
    T: Deserialize<'de>,
{
    Ok(Option::<Vec<T>>::deserialize(deserializer)?.unwrap_or_default())
}

fn null_map<'de, D, T>(deserializer: D) -> Result<BTreeMap<String, T>, D::Error>
where
    D: serde::Deserializer<'de>,
    T: Deserialize<'de>,
{
    Ok(Option::<BTreeMap<String, T>>::deserialize(deserializer)?.unwrap_or_default())
}

#[derive(Clone, Deserialize, Serialize)]
pub struct Screenshot {
    pub mime_type: String,
    pub base64: String,
    pub width: u32,
    pub height: u32,
}

#[derive(Clone, Deserialize, Serialize)]
pub struct Window {
    pub pid: u32,
    pub window_id: u64,
    pub title: String,
    pub foreground: bool,
}

#[derive(Clone, Deserialize, Serialize)]
pub struct Observation {
    pub observation_id: String,
    pub epoch: u64,
    pub complete: bool,
    #[serde(default)]
    pub truncated_reason: String,
    pub window: Window,
    pub nodes: Vec<Node>,
    pub screenshot: Screenshot,
}

#[derive(Clone, Deserialize, Serialize)]
pub struct Record {
    pub kind: String,
    #[serde(default)]
    pub stage: String,
    pub action_id: String,
    pub actor: String,
    pub epoch: u64,
    pub observation: Option<Observation>,
    pub receipt: Option<Value>,
    pub event: Option<Value>,
    #[serde(default)]
    pub reason: String,
}

#[derive(Clone, Deserialize)]
pub struct RunSnapshot {
    pub run_id: String,
    #[serde(default)]
    pub program_generation: u64,
    pub state: State,
    #[serde(default)]
    pub evidence_dir: String,
    pub finished: bool,
    #[serde(default)]
    pub error: String,
    pub report: Option<Value>,
    pub observation: Option<Observation>,
    #[serde(default)]
    pub records: Vec<Record>,
    #[serde(default)]
    pub pending_action_id: String,
    pub capability: Option<Capability>,
    pub matching: Option<MatchDiagnostics>,
    #[serde(default)]
    pub assistance: String,
    #[serde(default)]
    pub assisted: bool,
    pub reconciliation: Option<Reconciliation>,
}

#[derive(Clone, Deserialize, Serialize)]
pub struct Reconciliation {
    pub status: String,
    pub run_id: String,
    pub session_id: String,
    pub epoch: u64,
    pub action_id: String,
    #[serde(default)]
    pub observation_id: String,
    pub reason: String,
    pub observed_facts: BTreeMap<String, Value>,
    pub acceptance: String,
}

#[derive(Clone, Default, Deserialize)]
pub struct RunList {
    pub runs: Vec<RunSummary>,
    pub total_runs: usize,
    pub truncated: bool,
    #[serde(default)]
    pub desktop_owner: String,
}

#[derive(Clone, Deserialize)]
pub struct RunSummary {
    pub run_id: String,
    pub started_at: String,
    pub state: State,
    pub finished: bool,
    #[serde(default)]
    pub error: String,
    pub evidence_dir: String,
}

#[derive(Clone, Deserialize)]
pub struct MatchDiagnostics {
    pub selector: Value,
    pub exact_matches: usize,
    pub total_candidates: usize,
    pub truncated: bool,
    pub confidence: String,
    pub candidates: Vec<MatchCandidate>,
}

#[derive(Clone, Deserialize)]
pub struct MatchCandidate {
    pub node_id: String,
    pub role: String,
    pub name: String,
    pub matched_fields: usize,
    pub required_fields: usize,
    pub score: f64,
    pub missing: Vec<String>,
    pub enabled: bool,
    pub editable: bool,
}

#[derive(Clone, Serialize)]
pub struct Action {
    pub action_id: String,
    pub observation_id: String,
    pub target_id: String,
    pub kind: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub text: Option<String>,
    pub effect: String,
}

#[derive(Clone, Serialize)]
pub struct Control {
    pub kind: String,
    pub action_id: String,
    pub observation_id: String,
    pub epoch: u64,
    pub action: Option<Action>,
}

impl RunSnapshot {
    pub fn visual_anchor(&self) -> Option<&VisualAnchor> {
        let capability = self.capability.as_ref()?;
        let step = capability.steps.get(self.state.step_index)?;
        capability.targets.get(&step.target)?.visual_anchor()
    }
    pub fn capture_id(&self) -> &str {
        self.observation
            .as_ref()
            .map_or(self.state.observation.id.as_str(), |o| {
                o.observation_id.as_str()
            })
    }
    pub fn bound(&self) -> bool {
        !self.finished
            && !self.run_id.is_empty()
            && self.state.run_id == self.run_id
            && !self.state.session_id.is_empty()
            && self.state.epoch > 0
    }

    pub fn approval(&self, kind: &str) -> Result<Control, String> {
        let o = &self.state.observation;
        if !self.bound()
            || self.state.phase != "awaiting_approval"
            || self.pending_action_id.is_empty()
            || !o.complete
            || !o.actionable
            || o.matches != 1
            || o.id.is_empty()
            || o.target_id.is_empty()
            || !matches!(kind, "approve" | "deny")
        {
            return Err("The current snapshot does not contain a bound approval request.".into());
        }
        if let Some(anchor) = self.visual_anchor() {
            let visual = o
                .visual
                .as_ref()
                .ok_or("Visual approval requires a bound native match")?;
            let capture = self
                .observation
                .as_ref()
                .filter(|capture| {
                    capture.complete
                        && capture.epoch == self.state.epoch
                        && capture.observation_id == o.id
                })
                .ok_or("Visual approval requires the reviewed complete capture")?;
            let hash = |s: &str| {
                s.len() == 64
                    && s.bytes()
                        .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
            };
            if visual.target_id != o.target_id
                || visual.anchor_sha256 != anchor.sha256
                || !hash(&visual.frame_sha256)
                || capture.screenshot.width != anchor.frame_width
                || capture.screenshot.height != anchor.frame_height
                || visual.matched.width != anchor.width
                || visual.matched.height != anchor.height
                || visual
                    .matched
                    .x
                    .checked_add(visual.matched.width)
                    .is_none_or(|right| right > anchor.frame_width)
                || visual
                    .matched
                    .y
                    .checked_add(visual.matched.height)
                    .is_none_or(|bottom| bottom > anchor.frame_height)
            {
                return Err("Visual match does not bind the template and current frame.".into());
            }
        } else if o.visual.is_some() {
            return Err("Visual match lacks its compiled template.".into());
        }
        Ok(Control {
            kind: kind.into(),
            action_id: self.pending_action_id.clone(),
            observation_id: o.id.clone(),
            epoch: self.state.epoch,
            action: None,
        })
    }

    pub fn control(&self, kind: &str) -> Result<Control, String> {
        if kind == "cancel"
            && !self.finished
            && !self.run_id.is_empty()
            && self.state.phase == "admitted"
        {
            return Ok(Control {
                kind: kind.into(),
                action_id: String::new(),
                observation_id: String::new(),
                epoch: self.state.epoch,
                action: None,
            });
        }
        if !self.bound() || !matches!(kind, "pause" | "resume" | "refresh" | "focus" | "cancel") {
            return Err("A current owned session is required.".into());
        }
        if matches!(kind, "resume" | "refresh" | "focus") && self.state.phase != "paused" {
            return Err("Pause the run before controlling its human session.".into());
        }
        Ok(Control {
            kind: kind.into(),
            action_id: String::new(),
            observation_id: String::new(),
            epoch: self.state.epoch,
            action: None,
        })
    }
}

#[derive(Clone)]
pub struct Input {
    pub parameter: Parameter,
    pub text: String,
    pub currency: String,
    pub boolean: bool,
}

impl Input {
    pub fn value(&self) -> Result<Value, String> {
        let kind = self.parameter.kind.as_str();
        match kind {
            "string" => Ok(serde_json::json!({"type":"string","text":self.text})),
            "boolean" => Ok(serde_json::json!({"type":"boolean","boolean":self.boolean})),
            "integer" | "money" => {
                let number = self.text.parse::<i64>().map_err(|_| {
                    format!(
                        "{} requires a signed integer{}.",
                        self.parameter.name,
                        if kind == "money" {
                            " in minor currency units"
                        } else {
                            ""
                        }
                    )
                })?;
                if kind == "money" {
                    if self.currency.len() != 3
                        || !self.currency.bytes().all(|b| b.is_ascii_uppercase())
                    {
                        return Err(format!(
                            "{} needs a three-letter uppercase currency.",
                            self.parameter.name
                        ));
                    }
                    Ok(
                        serde_json::json!({"type":"money","integer":number,"currency":self.currency}),
                    )
                } else {
                    Ok(serde_json::json!({"type":"integer","integer":number}))
                }
            }
            _ => Err(format!("Unsupported input type: {kind}")),
        }
    }
}

/// A bounded line comparison for review; it deliberately does not claim a
/// minimal edit script. Unchanged equal-position lines are omitted.
pub fn revision_diff(before: &str, after: &str) -> Vec<String> {
    let a: Vec<_> = before.lines().take(4096).collect();
    let b: Vec<_> = after.lines().take(4096).collect();
    let mut out = Vec::new();
    for index in 0..a.len().max(b.len()) {
        if a.get(index) != b.get(index) {
            if let Some(line) = a.get(index) {
                out.push(format!("- {}: {line}", index + 1));
            }
            if let Some(line) = b.get(index) {
                out.push(format!("+ {}: {line}", index + 1));
            }
        }
    }
    if before.lines().count() > 4096 || after.lines().count() > 4096 {
        out.push("Comparison truncated after 4096 lines per revision.".into());
    }
    out
}
