use eframe::egui::accesskit::Role;
use egui_kittest::kittest::NodeT;
use egui_kittest::{Harness, kittest::Queryable};
use jarvis_bank::{
    BankApp, Fault, Page,
    state::{BankStore, Tenant},
};
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{Duration, Instant};

struct Fixture(PathBuf);
impl Fixture {
    fn new() -> Self {
        static NEXT: AtomicU64 = AtomicU64::new(0);
        let path = std::env::temp_dir().canonicalize().unwrap().join(format!(
            "jarvis-bank-ui-{}-{}",
            std::process::id(),
            NEXT.fetch_add(1, Ordering::Relaxed)
        ));
        std::fs::create_dir(&path).unwrap();
        Self(path)
    }
}
impl Drop for Fixture {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.0);
    }
}

fn harness(tenant: Tenant, fault: Fault, fixture: &Fixture) -> Harness<'static, BankApp> {
    let app = BankApp::new(
        BankStore::open(&fixture.0.join("state.json"), tenant).unwrap(),
        fault,
    );
    Harness::builder()
        .with_size(eframe::egui::vec2(880.0, 700.0))
        .build_ui_state(|ui, app| app.draw(ui), app)
}

fn search(harness: &mut Harness<'_, BankApp>) {
    harness
        .get_by_role_and_label(Role::TextInput, "Member ID")
        .focus();
    harness.run();
    harness
        .get_by_role_and_label(Role::TextInput, "Member ID")
        .type_text("M-1001");
    harness.run();
    harness
        .get_by_role_and_label(Role::Button, "Search")
        .click();
    if harness.state().status() == "Searching" {
        harness.run_steps(1);
    } else {
        harness.run();
    }
}

#[test]
fn root_window_has_the_native_title_for_each_tenant() {
    for tenant in [Tenant::North, Tenant::South] {
        let fixture = Fixture::new();
        let harness = harness(tenant, Fault::None, &fixture);
        assert_eq!(
            harness.root().accesskit_node().label(),
            Some(format!("Jarvis Bank — {}", tenant.title()))
        );
    }
}

fn prepare(harness: &mut Harness<'_, BankApp>) {
    search(harness);
    harness
        .get_by_role_and_label(Role::Button, "New subaccount")
        .click();
    harness.run();
    harness
        .get_by_role_and_label(Role::TextInput, "Subaccount name")
        .focus();
    harness.run();
    harness
        .get_by_role_and_label(Role::TextInput, "Subaccount name")
        .type_text("Rainy day");
    harness.run();
    harness
        .get_by_role_and_label(Role::Button, "Review")
        .click();
    harness.run();
    assert_eq!(harness.state().page(), Page::Review);
}

#[test]
fn both_tenant_layouts_expose_named_roles_and_confirm_real_state() {
    for tenant in [Tenant::North, Tenant::South] {
        let fixture = Fixture::new();
        let mut harness = harness(tenant, Fault::None, &fixture);
        prepare(&mut harness);
        assert_eq!(
            harness
                .get_by_role_and_label(Role::TextInput, "Balance")
                .value()
                .as_deref(),
            Some("1250.00 USD")
        );
        assert_eq!(
            harness
                .get_by_role_and_label(Role::TextInput, "Subaccount name")
                .value()
                .as_deref(),
            Some("Rainy day")
        );
        assert_eq!(harness.state().state().members[0].subaccounts.len(), 0);
        harness
            .get_by_role_and_label(Role::Button, "Confirm creation")
            .click();
        harness.run();
        assert_eq!(harness.state().status(), "Subaccount created");
        assert_eq!(harness.state().state().members[0].subaccounts.len(), 1);
        let disk: jarvis_bank::state::BankState =
            serde_json::from_slice(&std::fs::read(fixture.0.join("state.json")).unwrap()).unwrap();
        assert_eq!(disk.members[0].subaccounts[0].name, "Rainy day");
    }
}

#[test]
fn read_only_balance_has_a_text_value_and_cannot_be_edited() {
    let fixture = Fixture::new();
    let mut harness = harness(Tenant::North, Fault::None, &fixture);
    search(&mut harness);
    harness
        .get_by_role_and_label(Role::TextInput, "Balance")
        .focus();
    harness.run();
    harness
        .get_by_role_and_label(Role::TextInput, "Balance")
        .type_text("999999");
    harness.run();
    assert_eq!(
        harness
            .get_by_role_and_label(Role::TextInput, "Balance")
            .value()
            .as_deref(),
        Some("1250.00 USD")
    );
    assert_eq!(harness.state().state().members[0].balance_cents, 125000);
}

#[test]
fn preparing_and_back_never_commit() {
    let fixture = Fixture::new();
    let mut harness = harness(Tenant::South, Fault::None, &fixture);
    prepare(&mut harness);
    harness.get_by_role_and_label(Role::Button, "Back").click();
    harness.run();
    assert_eq!(harness.state().page(), Page::Create);
    assert_eq!(harness.state().state().revision, 0);
}

#[test]
fn changing_member_input_invalidates_old_selection() {
    let fixture = Fixture::new();
    let mut harness = harness(Tenant::North, Fault::None, &fixture);
    prepare(&mut harness);
    harness
        .get_by_role_and_label(Role::TextInput, "Member ID")
        .focus();
    harness.run();
    harness
        .get_by_role_and_label(Role::TextInput, "Member ID")
        .type_text("invalid");
    harness.run();
    assert_eq!(harness.state().page(), Page::Lookup);
    assert!(
        harness
            .query_by_role_and_label(Role::Button, "Confirm creation")
            .is_none()
    );
    assert_eq!(harness.state().state().revision, 0);
}

#[test]
fn overlay_blocks_controls_until_dismissed() {
    let fixture = Fixture::new();
    let mut harness = harness(Tenant::North, Fault::Overlay, &fixture);
    let member = harness.get_by_role_and_label(Role::TextInput, "Member ID");
    assert!(member.accesskit_node().is_disabled());
    let (target_node, target_tree) = member.accesskit_node().locate();
    harness.event(eframe::egui::Event::AccessKitActionRequest(
        eframe::egui::accesskit::ActionRequest {
            action: eframe::egui::accesskit::Action::SetValue,
            target_node,
            target_tree,
            data: Some(eframe::egui::accesskit::ActionData::Value("M-1001".into())),
        },
    ));
    harness.run();
    assert_eq!(
        harness
            .get_by_role_and_label(Role::TextInput, "Member ID")
            .value()
            .as_deref(),
        Some("")
    );
    let search_button = harness.get_by_role_and_label(Role::Button, "Search");
    assert!(search_button.accesskit_node().is_disabled());
    let (target_node, target_tree) = search_button.accesskit_node().locate();
    harness.event(eframe::egui::Event::AccessKitActionRequest(
        eframe::egui::accesskit::ActionRequest {
            action: eframe::egui::accesskit::Action::Click,
            target_node,
            target_tree,
            data: None,
        },
    ));
    harness.run();
    assert_eq!(harness.state().status(), "Ready");
    assert_eq!(harness.state().state().revision, 0);
    harness
        .get_by_role_and_label(Role::Button, "Dismiss")
        .click();
    harness.run();
    assert!(
        harness
            .query_by_role_and_label(Role::Button, "Dismiss")
            .is_none()
    );
    search(&mut harness);
    assert_eq!(harness.state().page(), Page::Member);
}

#[test]
fn missing_and_duplicate_controls_are_real_accessibility_shapes() {
    let fixture = Fixture::new();
    let mut missing = harness(Tenant::North, Fault::MissingControl, &fixture);
    search(&mut missing);
    missing
        .get_by_role_and_label(Role::Button, "New subaccount")
        .click();
    missing.run();
    assert!(
        missing
            .query_by_role_and_label(Role::Button, "Review")
            .is_none()
    );
    let fixture = Fixture::new();
    let mut duplicate = harness(Tenant::South, Fault::DuplicateControl, &fixture);
    prepare(&mut duplicate);
    assert_eq!(
        duplicate
            .get_all_by_role_and_label(Role::Button, "Confirm creation")
            .count(),
        2
    );
    assert_eq!(duplicate.state().state().revision, 0);
}

#[test]
fn commit_noop_reports_no_success_and_does_not_write() {
    let fixture = Fixture::new();
    let mut harness = harness(Tenant::North, Fault::CommitNoop, &fixture);
    prepare(&mut harness);
    harness
        .get_by_role_and_label(Role::Button, "Confirm creation")
        .click();
    harness.run();
    assert_eq!(harness.state().status(), "Creation did not complete");
    assert_eq!(harness.state().state().revision, 0);
}

#[test]
fn false_ack_shows_real_success_ui_while_saved_seed_remains_byte_identical() {
    for tenant in [Tenant::North, Tenant::South] {
        let fixture = Fixture::new();
        let fault = Fault::parse("false-ack").expect("explicit false-ack fault");
        let mut harness = harness(tenant, fault, &fixture);
        let before = std::fs::read(fixture.0.join("state.json")).unwrap();
        prepare(&mut harness);
        assert_eq!(harness.state().status(), "Ready for confirmation");
        harness
            .get_by_role_and_label(Role::Button, "Confirm creation")
            .click();
        harness.run();
        assert_eq!(harness.state().page(), Page::Complete);
        assert_eq!(
            harness
                .get_by_role_and_label(Role::TextInput, "Status")
                .value()
                .as_deref(),
            Some("Subaccount created")
        );
        assert!(
            harness
                .query_by_role_and_label(Role::Button, "Confirm creation")
                .is_none()
        );
        assert_eq!(harness.state().state().revision, 0);
        assert!(
            harness
                .state()
                .state()
                .members
                .iter()
                .all(|member| member.subaccounts.is_empty())
        );
        assert_eq!(std::fs::read(fixture.0.join("state.json")).unwrap(), before);
    }
}

#[test]
fn crash_requested_only_after_commit_exists_on_disk() {
    let fixture = Fixture::new();
    let mut harness = harness(Tenant::North, Fault::CrashAfterCommit, &fixture);
    prepare(&mut harness);
    assert!(!harness.state().crash_requested());
    harness
        .get_by_role_and_label(Role::Button, "Confirm creation")
        .click();
    harness.run();
    assert!(harness.state().crash_requested());
    let disk: jarvis_bank::state::BankState =
        serde_json::from_slice(&std::fs::read(fixture.0.join("state.json")).unwrap()).unwrap();
    assert_eq!(disk.members[0].subaccounts.len(), 1);
}

#[test]
fn delayed_lookup_does_not_publish_member_early() {
    let fixture = Fixture::new();
    let mut harness = harness(Tenant::North, Fault::Delay, &fixture);
    harness
        .get_by_role_and_label(Role::TextInput, "Member ID")
        .focus();
    harness.run();
    harness
        .get_by_role_and_label(Role::TextInput, "Member ID")
        .type_text("M-1001");
    harness.run();
    harness
        .get_by_role_and_label(Role::Button, "Search")
        .click();
    harness.run_steps(1);
    assert_eq!(harness.state().page(), Page::Lookup);
    harness
        .state_mut()
        .tick(Instant::now() + Duration::from_secs(2));
    harness.run();
    assert_eq!(harness.state().page(), Page::Member);
}

#[test]
fn native_accesskit_set_value_updates_editable_field_before_search() {
    let fixture = Fixture::new();
    let mut harness = harness(Tenant::North, Fault::None, &fixture);
    let (target_node, target_tree) = harness
        .get_by_role_and_label(Role::TextInput, "Member ID")
        .accesskit_node()
        .locate();
    harness.event(eframe::egui::Event::AccessKitActionRequest(
        eframe::egui::accesskit::ActionRequest {
            action: eframe::egui::accesskit::Action::SetValue,
            target_node,
            target_tree,
            data: Some(eframe::egui::accesskit::ActionData::Value("M-1002".into())),
        },
    ));
    harness.run();
    assert_eq!(
        harness
            .get_by_role_and_label(Role::TextInput, "Member ID")
            .value()
            .as_deref(),
        Some("M-1002")
    );
    harness
        .get_by_role_and_label(Role::Button, "Search")
        .click_accesskit();
    harness.run();
    assert_eq!(
        harness
            .get_by_role_and_label(Role::TextInput, "Balance")
            .value()
            .as_deref(),
        Some("840.50 USD")
    );
    assert!(
        harness
            .get_by_role_and_label(Role::TextInput, "Balance")
            .accesskit_node()
            .is_read_only()
    );
    assert!(
        harness
            .get_by_role_and_label(Role::TextInput, "Status")
            .accesskit_node()
            .is_read_only()
    );
    harness
        .get_by_role_and_label(Role::Button, "New subaccount")
        .click();
    harness.run();
    let (target_node, target_tree) = harness
        .get_by_role_and_label(Role::TextInput, "Subaccount name")
        .accesskit_node()
        .locate();
    harness.event(eframe::egui::Event::AccessKitActionRequest(
        eframe::egui::accesskit::ActionRequest {
            action: eframe::egui::accesskit::Action::SetValue,
            target_node,
            target_tree,
            data: Some(eframe::egui::accesskit::ActionData::Value(
                "Native savings".into(),
            )),
        },
    ));
    harness.run();
    assert_eq!(
        harness
            .get_by_role_and_label(Role::TextInput, "Subaccount name")
            .value()
            .as_deref(),
        Some("Native savings")
    );
}
