use crate::state::{BankStore, Creation, Tenant, validate_subaccount_name};
use eframe::egui;
use jarvis_desktop_ui::{editable_text, readonly_text as readonly};
use std::time::{Duration, Instant};

/// Branch passcode for `--fault session-expired`. Not shown in the UI; only a
/// human operator (or a test that embeds the same constant) can supply it.
pub const SESSION_PASSCODE: &str = "BRANCH-7741";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Fault {
    None,
    Overlay,
    Delay,
    MissingControl,
    DuplicateControl,
    CommitNoop,
    FalseAck,
    CrashAfterCommit,
    PermissionDenied,
    SessionExpired,
}

impl Fault {
    pub fn parse(value: &str) -> Result<Self, String> {
        match value {
            "none" => Ok(Self::None),
            "overlay" => Ok(Self::Overlay),
            "delay" => Ok(Self::Delay),
            "missing-control" => Ok(Self::MissingControl),
            "duplicate-control" => Ok(Self::DuplicateControl),
            "commit-noop" => Ok(Self::CommitNoop),
            "false-ack" => Ok(Self::FalseAck),
            "crash-after-commit" => Ok(Self::CrashAfterCommit),
            "permission-denied" => Ok(Self::PermissionDenied),
            "session-expired" => Ok(Self::SessionExpired),
            _ => Err(
                "Unknown fault (expected none, overlay, delay, missing-control, duplicate-control, commit-noop, false-ack, crash-after-commit, permission-denied, session-expired)".into(),
            ),
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Variant {
    #[default]
    None,
    RenamedControls,
}

impl Variant {
    pub fn parse(value: &str) -> Result<Self, String> {
        match value {
            "none" => Ok(Self::None),
            "renamed-controls" => Ok(Self::RenamedControls),
            _ => Err("Unknown variant (expected none, renamed-controls)".into()),
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Page {
    Lookup,
    Member,
    Create,
    Review,
    Complete,
}

pub struct BankApp {
    store: BankStore,
    fault: Fault,
    variant: Variant,
    page: Page,
    member_query: String,
    selected_member: Option<String>,
    subaccount_name: String,
    status: String,
    overlay_open: bool,
    session_expired_open: bool,
    session_passcode: String,
    pending_lookup: Option<Instant>,
    crash_requested: bool,
}

impl BankApp {
    pub fn new(store: BankStore, fault: Fault) -> Self {
        Self::with_variant(store, fault, Variant::None)
    }

    pub fn with_variant(store: BankStore, fault: Fault, variant: Variant) -> Self {
        Self {
            store,
            fault,
            variant,
            page: Page::Lookup,
            member_query: String::new(),
            selected_member: None,
            subaccount_name: String::new(),
            status: "Ready".into(),
            overlay_open: fault == Fault::Overlay,
            session_expired_open: fault == Fault::SessionExpired,
            session_passcode: String::new(),
            pending_lookup: None,
            crash_requested: false,
        }
    }

    pub fn page(&self) -> Page {
        self.page
    }
    pub fn status(&self) -> &str {
        &self.status
    }
    pub fn state(&self) -> &crate::state::BankState {
        self.store.state()
    }
    pub fn crash_requested(&self) -> bool {
        self.crash_requested
    }
    pub fn search_label(&self) -> &'static str {
        match self.variant {
            Variant::RenamedControls => "Find member",
            Variant::None => "Search",
        }
    }
    pub fn blocked(&self) -> bool {
        self.overlay_open || self.session_expired_open || self.pending_lookup.is_some()
    }

    pub fn tick(&mut self, now: Instant) {
        if self.pending_lookup.is_some_and(|deadline| now >= deadline) {
            self.pending_lookup = None;
            self.finish_lookup();
        }
    }

    fn search(&mut self) {
        self.selected_member = None;
        self.page = Page::Lookup;
        if self.fault == Fault::Delay {
            self.status = "Searching".into();
            self.pending_lookup = Some(Instant::now() + Duration::from_millis(1500));
        } else {
            self.finish_lookup();
        }
    }

    fn finish_lookup(&mut self) {
        if self.fault == Fault::PermissionDenied {
            self.selected_member = None;
            self.page = Page::Lookup;
            self.status = "Permission denied".into();
            return;
        }
        let id = self.member_query.trim();
        if let Some(member) = self.store.state().member(id) {
            self.selected_member = Some(member.id.clone());
            self.page = Page::Member;
            self.status = "Member found".into();
        } else {
            self.status = "Member not found".into();
        }
    }

    fn review(&mut self) {
        self.subaccount_name = self.subaccount_name.trim().into();
        match validate_subaccount_name(&self.subaccount_name) {
            Ok(()) if self.selected_member.is_some() => {
                self.page = Page::Review;
                self.status = "Ready for confirmation".into();
            }
            Ok(()) => self.status = "Select a member first".into(),
            Err(error) => self.status = error,
        }
    }

    fn confirm(&mut self) {
        if self.page != Page::Review {
            return;
        }
        let Some(member_id) = self.selected_member.as_deref() else {
            self.status = "Select a member first".into();
            return;
        };
        if self.fault == Fault::CommitNoop {
            self.status = "Creation did not complete".into();
            return;
        }
        if self.fault == Fault::FalseAck {
            // Deliberate fault: exercise a normal success acknowledgment with
            // no store mutation so the independent saved-state oracle must disagree.
            self.acknowledge_created();
            return;
        }
        match self.store.create_savings(member_id, &self.subaccount_name) {
            Ok(Creation::Created(_)) => {
                self.acknowledge_created();
                self.crash_requested = self.fault == Fault::CrashAfterCommit;
            }
            Ok(Creation::AlreadyExists(_)) => {
                self.page = Page::Complete;
                self.status = "Subaccount already exists".into();
            }
            Err(error) => self.status = error,
        }
    }

    fn acknowledge_created(&mut self) {
        self.page = Page::Complete;
        self.status = "Subaccount created".into();
    }

    fn try_unlock_session(&mut self) {
        if self.session_passcode == SESSION_PASSCODE {
            self.session_expired_open = false;
            self.session_passcode.clear();
            self.status = "Ready".into();
        } else {
            self.status = "Incorrect passcode".into();
        }
    }

    pub fn draw(&mut self, ui: &mut egui::Ui) {
        self.tick(Instant::now());
        let tenant = self.store.state().tenant;
        ui.ctx()
            .accesskit_node_builder(egui::accesskit_root_id(), |node| {
                node.set_label(format!("Jarvis Bank — {}", tenant.title()));
            });
        ui.spacing_mut().item_spacing = egui::vec2(14.0, 14.0);
        egui::CentralPanel::default().show(ui, |ui| {
            ui.add_space(12.0);
            ui.heading(tenant.title());
            ui.label("Member services · Synthetic demonstration environment");
            ui.separator();
            ui.add_enabled_ui(!self.blocked(), |ui| {
                match tenant {
                    Tenant::North => {
                        ui.columns(2, |columns| {
                            columns[0].heading("Find a member");
                            self.lookup_controls(&mut columns[0]);
                            columns[1].heading("Account details");
                            self.member_controls(&mut columns[1]);
                        });
                    }
                    Tenant::South => {
                        ui.horizontal(|ui| {
                            ui.heading("Member workspace");
                            ui.label("South branch");
                        });
                        self.lookup_controls(ui);
                        ui.separator();
                        self.member_controls(ui);
                    }
                }
                ui.separator();
                self.workflow_controls(ui);
            });
            ui.separator();
            readonly(ui, "Status", &self.status);
        });
        if self.overlay_open {
            egui::Modal::new(egui::Id::new("session-notice")).show(ui.ctx(), |ui| {
                ui.heading("Session notice");
                ui.label("Acknowledge this notice to continue with member services.");
                if ui.button("Dismiss").clicked() {
                    self.overlay_open = false;
                }
            });
        }
        if self.session_expired_open {
            egui::Modal::new(egui::Id::new("session-expired")).show(ui.ctx(), |ui| {
                ui.heading("Session expired");
                ui.label("Enter the branch passcode to continue. Automation does not know this value.");
                editable_text(
                    ui,
                    "Passcode",
                    "session-passcode",
                    &mut self.session_passcode,
                    64,
                    false,
                );
                if ui.button("Continue").clicked() {
                    self.try_unlock_session();
                }
            });
        }
        if self.pending_lookup.is_some() {
            ui.ctx().request_repaint_after(Duration::from_millis(20));
        }
    }

    fn lookup_controls(&mut self, ui: &mut egui::Ui) {
        let response = editable_text(
            ui,
            "Member ID",
            "member-id",
            &mut self.member_query,
            64,
            false,
        );
        if response.changed() {
            self.selected_member = None;
            self.pending_lookup = None;
            self.page = Page::Lookup;
            self.status = "Ready".into();
        }
        let label = self.search_label();
        if ui.button(label).clicked() {
            self.search();
        }
    }

    fn member_controls(&self, ui: &mut egui::Ui) {
        let member = self
            .selected_member
            .as_deref()
            .and_then(|id| self.store.state().member(id));
        if let Some(member) = member {
            ui.label(&member.name);
            readonly(ui, "Balance", &member.balance_display());
            ui.label(format!(
                "{} savings subaccount(s)",
                member.subaccounts.len()
            ));
            for account in &member.subaccounts {
                ui.label(format!("{} · Savings", account.name));
            }
        } else {
            readonly(ui, "Balance", "—");
        }
    }

    fn workflow_controls(&mut self, ui: &mut egui::Ui) {
        match self.page {
            Page::Lookup => {
                ui.label("Enter a member ID to view balances and manage savings subaccounts.");
            }
            Page::Member | Page::Complete => {
                if ui.button("New subaccount").clicked() {
                    self.page = Page::Create;
                    self.subaccount_name.clear();
                    self.status = "Enter subaccount name".into();
                }
            }
            Page::Create => {
                ui.heading("Create a savings subaccount");
                editable_text(
                    ui,
                    "Subaccount name",
                    "subaccount-name",
                    &mut self.subaccount_name,
                    64,
                    false,
                );
                ui.horizontal(|ui| {
                    if ui.button("Back").clicked() {
                        self.page = Page::Member;
                        self.status = "Member found".into();
                    }
                    if self.fault != Fault::MissingControl && ui.button("Review").clicked() {
                        self.review();
                    }
                });
            }
            Page::Review => {
                ui.heading("Review savings subaccount");
                readonly(ui, "Subaccount name", &self.subaccount_name);
                ui.label(
                    "The subaccount starts with a zero balance. Existing funds are unchanged.",
                );
                ui.horizontal(|ui| {
                    if ui.button("Back").clicked() {
                        self.page = Page::Create;
                        self.status = "Enter subaccount name".into();
                    }
                    if ui.button("Confirm creation").clicked() {
                        self.confirm();
                    }
                    if self.fault == Fault::DuplicateControl
                        && ui.button("Confirm creation").clicked()
                    {
                        self.confirm();
                    }
                });
            }
        }
    }
}

impl eframe::App for BankApp {
    fn ui(&mut self, ui: &mut egui::Ui, _frame: &mut eframe::Frame) {
        self.draw(ui);
        // Fault injection exits only after create_savings persisted and synced.
        // This intentionally bypasses shutdown so recovery tests observe the
        // ambiguous client outcome independently from committed bank state.
        if self.crash_requested {
            std::process::exit(86);
        }
    }
}
