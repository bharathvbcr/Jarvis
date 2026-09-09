//! Accessible text fields shared by the native bank and workbench.
use eframe::egui;

/// egui 0.36 TextEdit exposes text ranges but does not consume SetValue.
/// Honor the native accessibility request only for this enabled editable ID,
/// before rendering, so the emitted tree contains the actual updated value.
pub fn editable_text(
    ui: &mut egui::Ui,
    label: &str,
    salt: &str,
    value: &mut String,
    max_chars: usize,
    password: bool,
) -> egui::Response {
    let id = ui.make_persistent_id(salt);
    let mut changed = false;
    if ui.is_enabled() {
        ui.input(|input| {
            for request in input.accesskit_action_requests(id, egui::accesskit::Action::SetValue) {
                if let Some(egui::accesskit::ActionData::Value(next)) = &request.data
                    && next.chars().count() <= max_chars
                    && !next.chars().any(char::is_control)
                    && next.as_ref() != value
                {
                    *value = next.to_string();
                    changed = true;
                }
            }
        });
    }
    let label = ui.label(label);
    let mut response = ui
        .add(
            egui::TextEdit::singleline(value)
                .id(id)
                .char_limit(max_chars)
                .password(password)
                .desired_width(320.0),
        )
        .labelled_by(label.id);
    if changed {
        response.mark_changed();
    }
    if ui.is_enabled() {
        ui.ctx().accesskit_node_builder(id, |node| {
            node.add_action(egui::accesskit::Action::SetValue)
        });
    }
    response
}

pub fn readonly_text(ui: &mut egui::Ui, label: &str, value: &str) {
    let label_response = ui.label(label);
    let mut immutable = value;
    let response = ui
        .add(
            egui::TextEdit::singleline(&mut immutable)
                .id_salt(label)
                .desired_width(320.0),
        )
        .labelled_by(label_response.id);
    ui.ctx()
        .accesskit_node_builder(response.id, |node| node.set_read_only());
}
