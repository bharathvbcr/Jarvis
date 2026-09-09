use jarvis_bank::state::{BankState, BankStore, Creation, Tenant, validate_subaccount_name};
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};

struct Fixture(PathBuf);
impl Fixture {
    fn new() -> Self {
        static NEXT: AtomicU64 = AtomicU64::new(0);
        let root = std::env::temp_dir().canonicalize().unwrap().join(format!(
            "jarvis-bank-state-{}-{}",
            std::process::id(),
            NEXT.fetch_add(1, Ordering::Relaxed)
        ));
        std::fs::create_dir(&root).unwrap();
        Self(root)
    }
    fn path(&self) -> PathBuf {
        self.0.join("state.json")
    }
}
impl Drop for Fixture {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.0);
    }
}

#[test]
fn seeded_balances_have_exact_minor_units() {
    for tenant in [Tenant::North, Tenant::South] {
        let state = BankState::seed(tenant);
        assert_eq!(state.member("M-1001").unwrap().balance_cents, 125000);
        assert_eq!(
            state.member("M-1002").unwrap().balance_display(),
            "840.50 USD"
        );
        state.validate().unwrap();
    }
}

#[test]
fn creation_is_persisted_without_changing_existing_balance() {
    let fixture = Fixture::new();
    let mut store = BankStore::open(&fixture.path(), Tenant::North).unwrap();
    assert!(matches!(
        store.create_savings("M-1001", "Rainy day").unwrap(),
        Creation::Created(_)
    ));
    assert_eq!(store.state().revision, 1);
    assert_eq!(
        store.state().member("M-1001").unwrap().balance_cents,
        125000
    );
    let disk: BankState = serde_json::from_slice(&std::fs::read(fixture.path()).unwrap()).unwrap();
    assert_eq!(&disk, store.state());
    drop(store);
    let reopened = BankStore::open(&fixture.path(), Tenant::North).unwrap();
    assert_eq!(
        reopened.state().member("M-1001").unwrap().subaccounts[0].name,
        "Rainy day"
    );
}

#[test]
fn retried_same_name_is_idempotent_and_other_member_remains_independent() {
    let fixture = Fixture::new();
    let mut store = BankStore::open(&fixture.path(), Tenant::South).unwrap();
    store.create_savings("M-1001", "Savings").unwrap();
    let bytes = std::fs::read(fixture.path()).unwrap();
    assert!(matches!(
        store.create_savings("M-1001", "SAVINGS").unwrap(),
        Creation::AlreadyExists(_)
    ));
    assert_eq!(std::fs::read(fixture.path()).unwrap(), bytes);
    assert_eq!(store.state().revision, 1);
    assert!(matches!(
        store.create_savings("M-1002", "Savings").unwrap(),
        Creation::Created(_)
    ));
    assert_eq!(store.state().member("M-1002").unwrap().balance_cents, 84050);
}

#[test]
fn unknown_member_and_invalid_name_never_write() {
    let fixture = Fixture::new();
    let mut store = BankStore::open(&fixture.path(), Tenant::North).unwrap();
    let original = std::fs::read(fixture.path()).unwrap();
    assert!(store.create_savings("not-a-member", "Savings").is_err());
    for name in ["", " ", " Savings", "Savings ", "bad\nname"] {
        assert!(store.create_savings("M-1001", name).is_err());
    }
    assert!(validate_subaccount_name(&"x".repeat(65)).is_err());
    assert_eq!(std::fs::read(fixture.path()).unwrap(), original);
}

#[test]
fn exclusive_store_lock_releases_when_instance_closes() {
    let fixture = Fixture::new();
    let store = BankStore::open(&fixture.path(), Tenant::North).unwrap();
    assert!(BankStore::open(&fixture.path(), Tenant::North).is_err());
    drop(store);
    assert!(BankStore::open(&fixture.path(), Tenant::North).is_ok());
}

#[test]
fn malformed_wrong_tenant_or_duplicate_state_is_refused() {
    let fixture = Fixture::new();
    std::fs::write(fixture.path(), b"{}").unwrap();
    assert!(BankStore::open(&fixture.path(), Tenant::North).is_err());
    let mut state = BankState::seed(Tenant::South);
    std::fs::write(fixture.path(), serde_json::to_vec(&state).unwrap()).unwrap();
    assert!(BankStore::open(&fixture.path(), Tenant::North).is_err());
    state.members.push(state.members[0].clone());
    assert!(state.validate().is_err());
}

#[cfg(unix)]
#[test]
fn symlink_state_is_not_followed() {
    let fixture = Fixture::new();
    let target = fixture.0.join("external.json");
    std::fs::write(
        &target,
        serde_json::to_vec(&BankState::seed(Tenant::North)).unwrap(),
    )
    .unwrap();
    std::os::unix::fs::symlink(&target, fixture.path()).unwrap();
    assert!(BankStore::open(&fixture.path(), Tenant::North).is_err());
}
