//! Structured editing of the canonical document. These forms never certify an
//! artifact: every change invalidates compilation and goes back to Manvi.
use eframe::egui;
use jarvis_desktop_ui::editable_text;
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

#[derive(Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Document {
    schema_version: u32,
    id: String,
    revision: String,
    application: String,
    #[serde(default)]
    description: String,
    #[serde(default)]
    parameters: Option<Vec<Parameter>>,
    targets: BTreeMap<String, Target>,
    steps: Vec<Step>,
    limits: Limits,
}

#[derive(Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
struct Parameter {
    name: String,
    #[serde(rename = "type")]
    kind: String,
    #[serde(default)]
    sensitive: bool,
}

#[derive(Clone, Default, Deserialize, Serialize)]
#[serde(default, deny_unknown_fields)]
struct Target {
    #[serde(skip_serializing_if = "Option::is_none")]
    visual: Option<VisualAnchor>,
    #[serde(skip_serializing_if = "String::is_empty")]
    role: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    name: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    identifier: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    ancestor: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct PixelRect {
    pub x: u32,
    pub y: u32,
    pub width: u32,
    pub height: u32,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct PixelPoint {
    pub x: u32,
    pub y: u32,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct VisualAnchor {
    pub png_base64: String,
    pub sha256: String,
    pub width: u32,
    pub height: u32,
    pub frame_width: u32,
    pub frame_height: u32,
    pub window_width: f64,
    pub window_height: f64,
    pub scale: f64,
    pub search: PixelRect,
    pub click: PixelPoint,
}

#[derive(Clone, Default, Deserialize, Serialize)]
#[serde(default, deny_unknown_fields)]
struct TypedValue {
    #[serde(rename = "type")]
    kind: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    text: String,
    #[serde(skip_serializing_if = "is_zero")]
    integer: i64,
    #[serde(skip_serializing_if = "is_false")]
    boolean: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    currency: String,
}
fn is_zero(value: &i64) -> bool {
    *value == 0
}
fn is_false(value: &bool) -> bool {
    !*value
}

#[derive(Clone, Default, Deserialize, Serialize)]
#[serde(default, deny_unknown_fields)]
struct Reference {
    source: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    key: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    literal: Option<TypedValue>,
}

#[derive(Clone, Default, Deserialize, Serialize)]
#[serde(default, deny_unknown_fields)]
struct Predicate {
    op: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    expected: Option<Reference>,
}

#[derive(Clone, Default, Deserialize, Serialize)]
#[serde(default, deny_unknown_fields)]
struct Step {
    id: String,
    kind: String,
    target: String,
    effect: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    input: Option<Reference>,
    #[serde(skip_serializing_if = "Option::is_none")]
    predicate: Option<Predicate>,
    #[serde(skip_serializing_if = "String::is_empty")]
    output: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    output_type: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    currency: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    next: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    otherwise: String,
}

#[derive(Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
struct Limits {
    max_actions: u32,
    active_seconds: u32,
    observation_attempts: u32,
    intervention_seconds: u32,
}

impl Document {
    pub fn insert_visual(&mut self, key: &str, visual: VisualAnchor) -> Result<(), String> {
        if key.is_empty()
            || key.len() > 96
            || self.targets.contains_key(key)
            || self.targets.len() >= 128
        {
            return Err(
                "Choose a new target key of at most 96 characters; the target limit is 128.".into(),
            );
        }
        self.targets.insert(
            key.into(),
            Target {
                visual: Some(visual),
                ..Default::default()
            },
        );
        Ok(())
    }
    /// Only call for source that the canonical compiler has accepted. This
    /// parser is a lossless form adapter, not another compiler.
    pub fn from_compiled_source(source: &str) -> Result<Self, String> {
        if source.len() > 1024 * 1024 {
            return Err("Document exceeds 1 MiB.".into());
        }
        let document: Self = serde_json::from_str(source).map_err(|e| e.to_string())?;
        if document.steps.len() > 128
            || document.targets.len() > 128
            || document.parameters.as_ref().is_some_and(|p| p.len() > 128)
        {
            return Err("The editor supports at most 128 steps, targets and parameters.".into());
        }
        Ok(document)
    }

    pub fn source(&self) -> Result<String, String> {
        let source = serde_json::to_string_pretty(self).map_err(|e| e.to_string())?;
        if source.len() > 1024 * 1024 {
            return Err("Edited document exceeds 1 MiB.".into());
        }
        Ok(source)
    }

    pub fn ui(&mut self, ui: &mut egui::Ui) -> bool {
        let before = self.source();
        ui.heading("Structured capability editor");
        ui.label("Edit the artifact using forms. Compile to validate types, targets, edges and policy before registration or execution.");
        egui::CollapsingHeader::new("Identity and limits")
            .default_open(true)
            .show(ui, |ui| {
                field(ui, "Capability ID", &mut self.id, 96);
                field(ui, "Revision", &mut self.revision, 512);
                field(ui, "Application", &mut self.application, 512);
                field(ui, "Description", &mut self.description, 4096);
                number(
                    ui,
                    "Maximum actions",
                    &mut self.limits.max_actions,
                    1..=1000,
                );
                number(
                    ui,
                    "Active seconds",
                    &mut self.limits.active_seconds,
                    1..=3600,
                );
                number(
                    ui,
                    "Observation attempts",
                    &mut self.limits.observation_attempts,
                    1..=5,
                );
                number(
                    ui,
                    "Intervention seconds",
                    &mut self.limits.intervention_seconds,
                    1..=3600,
                );
            });
        egui::CollapsingHeader::new("Parameters").show(ui, |ui| {
            let mut parameters = self.parameters.clone().unwrap_or_default();
            let previous = serde_json::to_string(&parameters).ok();
            let mut remove = None;
            for (index, parameter) in parameters.iter_mut().enumerate() {
                ui.push_id(index, |ui| {
                    ui.group(|ui| {
                        field(ui, "Parameter name", &mut parameter.name, 96);
                        choice(
                            ui,
                            "Parameter type",
                            &mut parameter.kind,
                            &["string", "integer", "money", "boolean"],
                        );
                        ui.checkbox(&mut parameter.sensitive, "Sensitive parameter");
                        if ui.button("Remove parameter").clicked() {
                            remove = Some(index);
                        }
                    });
                });
            }
            if let Some(index) = remove {
                parameters.remove(index);
            }
            if ui
                .add_enabled(parameters.len() < 128, egui::Button::new("Add parameter"))
                .clicked()
            {
                parameters.push(Parameter {
                    name: unique("parameter", parameters.iter().map(|p| p.name.as_str())),
                    kind: "string".into(),
                    sensitive: true,
                });
            }
            if previous != serde_json::to_string(&parameters).ok() {
                self.parameters = Some(parameters);
            }
        });
        egui::CollapsingHeader::new("Targets").show(ui, |ui| {
            let mut remove = None;
            let mut rename = None;
            let keys: Vec<_> = self.targets.keys().cloned().collect();
            for key in keys {
                let target = self.targets.get_mut(&key).expect("key from map");
                ui.push_id(&key, |ui| {
                    ui.group(|ui| {
                        let draft_id = ui.id().with("rename-draft");
                        let mut name = ui.data(|data| data.get_temp::<String>(draft_id)).unwrap_or_else(|| key.clone());
                        field(ui, "Target key", &mut name, 96);
                        ui.data_mut(|data| data.insert_temp(draft_id, name.clone()));
                        if name != key && ui.button("Apply target rename and update step references").clicked() { rename = Some((key.clone(), name)); }
                        if let Some(visual) = &mut target.visual {
                            ui.label("Visual anchor · exact pixels · human approval required");
                            ui.small(format!("Template {} · {}×{} pixels; frame {}×{}; scale {}", visual.sha256, visual.width, visual.height, visual.frame_width, visual.frame_height, visual.scale));
                            rect_ui(ui, "Search region", &mut visual.search, visual.frame_width, visual.frame_height);
                            number(ui, "Click X inside template", &mut visual.click.x, 0..=visual.width.saturating_sub(1));
                            number(ui, "Click Y inside template", &mut visual.click.y, 0..=visual.height.saturating_sub(1));
                            ui.label("Only click steps with business effect change can use this target. Text extraction requires an accessibility target.");
                        } else {
                            field(ui, "Role", &mut target.role, 512);
                            field(ui, "Exact accessible name", &mut target.name, 512);
                            field(ui, "Stable identifier", &mut target.identifier, 512);
                            field(ui, "Ancestor name", &mut target.ancestor, 512);
                        }
                        if ui.button("Remove target").clicked() { remove = Some(key.clone()); }
                    });
                });
            }
            if let Some(key) = remove { self.targets.remove(&key); }
            if let Some((old, new)) = rename {
                if new.is_empty() || self.targets.contains_key(&new) {
                    ui.colored_label(egui::Color32::RED, "Target keys must be unique and nonempty.");
                } else if let Some(target) = self.targets.remove(&old) {
                    self.targets.insert(new.clone(), target);
                    for step in &mut self.steps { if step.target == old { step.target = new.clone(); } }
                }
            }
            if ui.add_enabled(self.targets.len() < 128, egui::Button::new("Add semantic target")).clicked() {
                self.targets.insert(unique("target", self.targets.keys().map(String::as_str)), Target { role: "button".into(), name: "Control".into(), ..Default::default() });
            }
        });
        egui::CollapsingHeader::new("Ordered steps")
            .default_open(true)
            .show(ui, |ui| {
                let targets: Vec<_> = self.targets.keys().map(String::as_str).collect();
                let mut remove = None;
                let mut move_step = None;
                let count = self.steps.len();
                for (index, step) in self.steps.iter_mut().enumerate() {
                    ui.push_id(index, |ui| {
                        egui::CollapsingHeader::new(format!(
                            "{}. {} · {}",
                            index + 1,
                            step.id,
                            step.kind
                        ))
                        .id_salt("step")
                        .show(ui, |ui| {
                            field(ui, "Step ID", &mut step.id, 96);
                            let previous_kind = step.kind.clone();
                            choice(
                                ui,
                                "Step kind",
                                &mut step.kind,
                                &[
                                    "press",
                                    "click",
                                    "set_value",
                                    "type_text",
                                    "scroll",
                                    "extract",
                                    "assert",
                                    "wait",
                                    "branch",
                                ],
                            );
                            if step.kind != previous_kind {
                                if !matches!(
                                    step.kind.as_str(),
                                    "set_value" | "type_text" | "scroll"
                                ) {
                                    step.input = None;
                                }
                                if !matches!(step.kind.as_str(), "assert" | "wait" | "branch") {
                                    step.predicate = None;
                                }
                                if step.kind != "extract" {
                                    step.output.clear();
                                    step.output_type.clear();
                                    step.currency.clear();
                                }
                                if step.kind != "branch" {
                                    step.otherwise.clear();
                                }
                                if matches!(
                                    step.kind.as_str(),
                                    "extract" | "assert" | "wait" | "branch"
                                ) {
                                    step.effect = "read".into();
                                }
                            }
                            choice(ui, "Target", &mut step.target, &targets);
                            choice(ui, "Business effect", &mut step.effect, &["read", "change"]);
                            if matches!(step.kind.as_str(), "set_value" | "type_text" | "scroll") {
                                reference_ui(
                                    ui,
                                    "Input",
                                    step.input.get_or_insert_with(|| Reference {
                                        source: "input".into(),
                                        ..Default::default()
                                    }),
                                );
                            }
                            if matches!(step.kind.as_str(), "assert" | "wait" | "branch") {
                                let predicate = step.predicate.get_or_insert_with(|| Predicate {
                                    op: "exists".into(),
                                    ..Default::default()
                                });
                                choice(
                                    ui,
                                    "Predicate",
                                    &mut predicate.op,
                                    &["exists", "equals", "not_equals", "contains"],
                                );
                                if predicate.op != "exists" {
                                    reference_ui(
                                        ui,
                                        "Expected",
                                        predicate.expected.get_or_insert_with(|| Reference {
                                            source: "literal".into(),
                                            ..Default::default()
                                        }),
                                    );
                                }
                            }
                            if step.kind == "extract" {
                                field(ui, "Output name", &mut step.output, 96);
                                let previous_type = step.output_type.clone();
                                choice(
                                    ui,
                                    "Output type",
                                    &mut step.output_type,
                                    &["string", "integer", "money", "boolean"],
                                );
                                if step.output_type != previous_type && step.output_type != "money"
                                {
                                    step.currency.clear();
                                }
                                if step.output_type == "money" {
                                    field(ui, "Output currency", &mut step.currency, 3);
                                }
                            }
                            field(ui, "Next step (empty = sequential)", &mut step.next, 96);
                            if step.kind == "branch" {
                                field(ui, "Otherwise step", &mut step.otherwise, 96);
                            }
                            ui.horizontal(|ui| {
                                if ui
                                    .add_enabled(index > 0, egui::Button::new("Move step up"))
                                    .clicked()
                                {
                                    move_step = Some((index, index - 1));
                                }
                                if ui
                                    .add_enabled(
                                        index + 1 < count,
                                        egui::Button::new("Move step down"),
                                    )
                                    .clicked()
                                {
                                    move_step = Some((index, index + 1));
                                }
                                if ui.button("Remove step").clicked() {
                                    remove = Some(index);
                                }
                            });
                        });
                    });
                }
                if let Some(index) = remove {
                    self.steps.remove(index);
                } else if let Some((a, b)) = move_step {
                    self.steps.swap(a, b);
                }
                if ui
                    .add_enabled(self.steps.len() < 128, egui::Button::new("Add step"))
                    .clicked()
                {
                    self.steps.push(Step {
                        id: unique("step", self.steps.iter().map(|s| s.id.as_str())),
                        kind: "press".into(),
                        target: self.targets.keys().next().cloned().unwrap_or_default(),
                        effect: "read".into(),
                        ..Default::default()
                    });
                }
            });
        before != self.source()
    }
}

fn unique<'a>(prefix: &str, names: impl Iterator<Item = &'a str>) -> String {
    let existing: Vec<_> = names.collect();
    for suffix in 1..=129 {
        let name = format!("{prefix}_{suffix}");
        if !existing.contains(&name.as_str()) {
            return name;
        }
    }
    unreachable!("editor enforces 128 item limit")
}

fn field(ui: &mut egui::Ui, label: &str, value: &mut String, limit: usize) -> bool {
    editable_text(ui, label, label, value, limit, false).changed()
}

fn number(ui: &mut egui::Ui, label: &str, value: &mut u32, range: std::ops::RangeInclusive<u32>) {
    ui.horizontal(|ui| {
        let label = ui.label(label);
        ui.add(egui::DragValue::new(value).range(range))
            .labelled_by(label.id);
    });
}

pub fn rect_ui(ui: &mut egui::Ui, label: &str, rect: &mut PixelRect, width: u32, height: u32) {
    ui.push_id(label, |ui| {
        ui.label(label);
        number(ui, "X", &mut rect.x, 0..=width.saturating_sub(1));
        number(ui, "Y", &mut rect.y, 0..=height.saturating_sub(1));
        number(ui, "Width", &mut rect.width, 1..=width.max(1));
        number(ui, "Height", &mut rect.height, 1..=height.max(1));
    });
}

fn choice(ui: &mut egui::Ui, label: &str, value: &mut String, options: &[&str]) {
    egui::ComboBox::from_label(label)
        .selected_text(value.as_str())
        .show_ui(ui, |ui| {
            for option in options {
                ui.selectable_value(value, (*option).into(), *option);
            }
        });
}

fn reference_ui(ui: &mut egui::Ui, label: &str, value: &mut Reference) {
    ui.push_id(label, |ui| {
        ui.label(label);
        let previous = value.source.clone();
        choice(
            ui,
            "Reference source",
            &mut value.source,
            &["input", "output", "literal"],
        );
        if previous != value.source {
            value.key.clear();
            value.literal = None;
        }
        if value.source != "literal" {
            field(ui, "Reference key", &mut value.key, 96);
        } else {
            let literal = value.literal.get_or_insert_with(|| TypedValue {
                kind: "string".into(),
                ..Default::default()
            });
            let previous = literal.kind.clone();
            choice(
                ui,
                "Literal type",
                &mut literal.kind,
                &["string", "integer", "money", "boolean"],
            );
            if previous != literal.kind {
                *literal = TypedValue {
                    kind: literal.kind.clone(),
                    ..Default::default()
                };
            }
            match literal.kind.as_str() {
                "boolean" => {
                    ui.checkbox(&mut literal.boolean, "Literal value");
                }
                "integer" | "money" => {
                    ui.horizontal(|ui| {
                        let label = ui.label("Literal integer (money uses minor units)");
                        ui.add(egui::DragValue::new(&mut literal.integer))
                            .labelled_by(label.id);
                    });
                    if literal.kind == "money" {
                        field(ui, "Literal currency", &mut literal.currency, 3);
                    }
                }
                _ => {
                    field(ui, "Literal text", &mut literal.text, 65536);
                }
            }
        }
    });
}

#[cfg(test)]
mod tests {
    use super::Document;
    #[test]
    fn canonical_fields_survive_structured_round_trip() {
        for source in [
            include_str!("../../../../examples/balance.json"),
            include_str!("../../../../examples/create-subaccount.json"),
        ] {
            let before: serde_json::Value = serde_json::from_str(source).unwrap();
            let after: serde_json::Value = serde_json::from_str(
                &Document::from_compiled_source(source)
                    .unwrap()
                    .source()
                    .unwrap(),
            )
            .unwrap();
            assert_eq!(
                before, after,
                "forms must not erase fields that are not currently visible"
            );
        }
    }
    #[test]
    fn unsupported_fields_are_refused_instead_of_silently_erased() {
        let source = include_str!("../../../../examples/balance.json")
            .replace("\"limits\":", "\"new_semantics\":true,\"limits\":");
        assert!(
            Document::from_compiled_source(&source)
                .err()
                .unwrap()
                .contains("unknown field")
        );
    }
}
