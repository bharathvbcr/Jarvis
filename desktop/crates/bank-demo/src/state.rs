use serde::{Deserialize, Serialize};
use std::collections::BTreeSet;
use std::fs::{File, OpenOptions};
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};

const MAX_STATE_BYTES: usize = 1024 * 1024;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Tenant {
    North,
    South,
}

impl Tenant {
    pub fn parse(value: &str) -> Result<Self, String> {
        match value {
            "north" => Ok(Self::North),
            "south" => Ok(Self::South),
            _ => Err("tenant must be north or south".into()),
        }
    }
    pub fn title(self) -> &'static str {
        match self {
            Self::North => "North Cooperative",
            Self::South => "South Mutual",
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Subaccount {
    pub id: String,
    pub name: String,
    pub kind: AccountKind,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum AccountKind {
    Savings,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Member {
    pub id: String,
    pub name: String,
    pub balance_cents: u64,
    pub currency: String,
    pub subaccounts: Vec<Subaccount>,
}

impl Member {
    pub fn balance_display(&self) -> String {
        format!(
            "{}.{:02} {}",
            self.balance_cents / 100,
            self.balance_cents % 100,
            self.currency
        )
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct BankState {
    pub schema_version: u32,
    pub tenant: Tenant,
    pub revision: u64,
    pub members: Vec<Member>,
}

impl BankState {
    pub fn seed(tenant: Tenant) -> Self {
        Self {
            schema_version: 1,
            tenant,
            revision: 0,
            members: vec![
                Member {
                    id: "M-1001".into(),
                    name: "Alex Morgan".into(),
                    balance_cents: 125_000,
                    currency: "USD".into(),
                    subaccounts: Vec::new(),
                },
                Member {
                    id: "M-1002".into(),
                    name: "Jordan Lee".into(),
                    balance_cents: 84_050,
                    currency: "USD".into(),
                    subaccounts: Vec::new(),
                },
            ],
        }
    }

    pub fn validate(&self) -> Result<(), String> {
        if self.schema_version != 1 || self.members.is_empty() || self.members.len() > 128 {
            return Err("unsupported or invalid bank state".into());
        }
        let mut member_ids = BTreeSet::new();
        let mut subaccount_ids = BTreeSet::new();
        for member in &self.members {
            if !valid_identifier(&member.id)
                || !member_ids.insert(&member.id)
                || member.currency != "USD"
                || member.balance_cents > 9_007_199_254_740_991
                || member.subaccounts.len() > 64
                || member.name.is_empty()
                || member.name.len() > 128
            {
                return Err("invalid member record".into());
            }
            let mut names = BTreeSet::new();
            for account in &member.subaccounts {
                validate_subaccount_name(&account.name)?;
                if !valid_identifier(&account.id)
                    || !subaccount_ids.insert(&account.id)
                    || !names.insert(account.name.to_lowercase())
                {
                    return Err("duplicate or invalid subaccount".into());
                }
            }
        }
        Ok(())
    }

    pub fn member(&self, id: &str) -> Option<&Member> {
        self.members.iter().find(|m| m.id == id)
    }
}

fn valid_identifier(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 64
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-')
}

pub fn validate_subaccount_name(name: &str) -> Result<(), String> {
    if name.trim().is_empty()
        || name.trim() != name
        || name.chars().count() > 64
        || name.chars().any(char::is_control)
    {
        return Err("Use a subaccount name of 1–64 characters without surrounding whitespace or control characters.".into());
    }
    Ok(())
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Creation {
    Created(String),
    AlreadyExists(String),
}

/// One native bank instance owns this state file. The OS lock survives path
/// renames and is released even after the injected process crash.
pub struct BankStore {
    state: BankState,
    path: PathBuf,
    _lock: File,
    writes_unavailable: bool,
}

impl BankStore {
    pub fn open(path: &Path, tenant: Tenant) -> Result<Self, String> {
        let parent = path
            .parent()
            .filter(|p| !p.as_os_str().is_empty())
            .unwrap_or(Path::new("."));
        std::fs::create_dir_all(parent)
            .map_err(|e| format!("Cannot create bank state directory: {e}"))?;
        let parent = parent
            .canonicalize()
            .map_err(|e| format!("Cannot resolve bank state directory: {e}"))?;
        let filename = path.file_name().ok_or("Bank state needs a filename")?;
        let path = parent.join(filename);
        reject_nonregular_existing(&path)?;
        let lock_path = path.with_extension("lock");
        if lock_path == path {
            return Err("Bank state filename must not use .lock extension".into());
        }
        reject_nonregular_existing(&lock_path)?;
        let lock = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .truncate(false)
            .open(&lock_path)
            .map_err(|e| format!("Cannot open bank lock: {e}"))?;
        lock.try_lock()
            .map_err(|e| format!("Bank state is already in use or cannot be locked: {e}"))?;
        let state = if path.exists() {
            let mut bytes = Vec::new();
            File::open(&path)
                .map_err(|e| format!("Cannot read bank state: {e}"))?
                .take(MAX_STATE_BYTES as u64 + 1)
                .read_to_end(&mut bytes)
                .map_err(|e| format!("Cannot read bank state: {e}"))?;
            if bytes.len() > MAX_STATE_BYTES {
                return Err("Bank state exceeds 1 MiB".into());
            }
            serde_json::from_slice::<BankState>(&bytes)
                .map_err(|e| format!("Invalid bank state: {e}"))?
        } else {
            BankState::seed(tenant)
        };
        state.validate()?;
        if state.tenant != tenant {
            return Err("Bank state belongs to another tenant".into());
        }
        let store = Self {
            state,
            path,
            _lock: lock,
            writes_unavailable: false,
        };
        if !store.path.exists() {
            store.persist(&store.state)?;
        }
        Ok(store)
    }

    pub fn state(&self) -> &BankState {
        &self.state
    }

    pub fn create_savings(&mut self, member_id: &str, name: &str) -> Result<Creation, String> {
        if self.writes_unavailable {
            return Err("A bank transaction had an uncertain result. Reopen the bank to reconcile persisted state before trying again.".into());
        }
        validate_subaccount_name(name)?;
        let member = self
            .state
            .member(member_id)
            .ok_or("Member no longer exists")?;
        if let Some(existing) = member
            .subaccounts
            .iter()
            .find(|a| a.name.to_lowercase() == name.to_lowercase())
        {
            return Ok(Creation::AlreadyExists(existing.id.clone()));
        }
        if member.subaccounts.len() >= 64 {
            return Err("Member has reached the subaccount limit".into());
        }
        let mut next = self.state.clone();
        let member = next
            .members
            .iter_mut()
            .find(|m| m.id == member_id)
            .ok_or("Member no longer exists")?;
        let id = format!("S-{}-{}", member.id, member.subaccounts.len() + 1);
        member.subaccounts.push(Subaccount {
            id: id.clone(),
            name: name.into(),
            kind: AccountKind::Savings,
        });
        next.revision = next
            .revision
            .checked_add(1)
            .ok_or("Bank revision limit reached")?;
        next.validate()?;
        if let Err(error) = self.persist(&next) {
            self.writes_unavailable = true;
            return Err(error);
        }
        self.state = next;
        Ok(Creation::Created(id))
    }

    fn persist(&self, next: &BankState) -> Result<(), String> {
        static NEXT: AtomicU64 = AtomicU64::new(0);
        let bytes = serde_json::to_vec_pretty(next)
            .map_err(|e| format!("Cannot serialize bank state: {e}"))?;
        if bytes.len() > MAX_STATE_BYTES {
            return Err("Bank state exceeds 1 MiB".into());
        }
        let parent = self.path.parent().ok_or("Bank state has no parent")?;
        let temporary = parent.join(format!(
            ".jarvis-bank-{}-{}.tmp",
            std::process::id(),
            NEXT.fetch_add(1, Ordering::Relaxed)
        ));
        let result = (|| {
            let mut file = File::create_new(&temporary)
                .map_err(|e| format!("Cannot stage bank transaction: {e}"))?;
            file.write_all(&bytes)
                .map_err(|e| format!("Cannot write bank transaction: {e}"))?;
            file.sync_all()
                .map_err(|e| format!("Cannot sync bank transaction: {e}"))?;
            drop(file);
            reject_nonregular_existing(&self.path)?;
            std::fs::rename(&temporary, &self.path)
                .map_err(|e| format!("Cannot commit bank transaction: {e}"))?;
            #[cfg(unix)]
            File::open(parent).and_then(|f| f.sync_all()).map_err(|e| {
                format!("Bank transaction committed but directory sync failed: {e}")
            })?;
            Ok(())
        })();
        if result.is_err()
            && let Err(error) = std::fs::remove_file(&temporary)
            && error.kind() != std::io::ErrorKind::NotFound
        {
            return Err(format!(
                "{result:?}; cannot remove transaction staging file: {error}"
            ));
        }
        result
    }
}

fn reject_nonregular_existing(path: &Path) -> Result<(), String> {
    match std::fs::symlink_metadata(path) {
        Ok(meta) if !meta.file_type().is_file() => {
            Err("Bank state and lock must be regular files, not symlinks or devices".into())
        }
        Ok(_) => Ok(()),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(error) => Err(format!("Cannot inspect bank state: {error}")),
    }
}
