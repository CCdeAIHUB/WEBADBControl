use std::{
    collections::{BTreeMap, BTreeSet, HashMap},
    fs::{self, OpenOptions},
    io::Write,
    path::{Path, PathBuf},
    sync::Mutex,
    time::{Duration, Instant},
};

use argon2::{
    password_hash::{PasswordHash, PasswordHasher, PasswordVerifier, SaltString},
    Argon2,
};
use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use serde::{Deserialize, Serialize};

use crate::error::AppError;

pub const DEFAULT_ADMIN_USERNAME: &str = "admin";
pub const DEFAULT_ADMIN_PASSWORD: &str = "admin";
const DATABASE_VERSION: u32 = 1;
const DEFAULT_SESSION_TTL: Duration = Duration::from_secs(8 * 60 * 60);

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum RemoteRole {
    Admin,
    User,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteAccount {
    pub username: String,
    pub role: RemoteRole,
    pub devices: Vec<String>,
    pub built_in: bool,
    pub password_change_required: bool,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AuthenticatedSession {
    pub token: String,
    pub username: String,
    pub role: RemoteRole,
    pub devices: BTreeSet<String>,
    pub password_change_required: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct StoredAccount {
    username: String,
    password_hash: String,
    role: RemoteRole,
    #[serde(default)]
    devices: BTreeSet<String>,
    #[serde(default)]
    built_in: bool,
    #[serde(default)]
    password_change_required: bool,
}

#[derive(Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AccountDatabase {
    version: u32,
    users: BTreeMap<String, StoredAccount>,
}

#[derive(Debug)]
struct SessionRecord {
    username: String,
    expires_at: Instant,
}

#[derive(Debug)]
struct State {
    database: AccountDatabase,
    sessions: HashMap<String, SessionRecord>,
}

#[derive(Debug)]
pub struct RemoteAccountManager {
    path: PathBuf,
    session_ttl: Duration,
    state: Mutex<State>,
}

impl RemoteAccountManager {
    pub fn load_or_initialize(path: impl Into<PathBuf>) -> Result<Self, AppError> {
        Self::load_or_initialize_with_session_ttl(path, DEFAULT_SESSION_TTL)
    }

    pub fn load_or_initialize_with_session_ttl(
        path: impl Into<PathBuf>,
        session_ttl: Duration,
    ) -> Result<Self, AppError> {
        let path = path.into();
        let database = if path.exists() {
            let bytes =
                fs::read(&path).map_err(|error| storage_error("REMOTE_AUTH_READ_FAILED", error))?;
            let database: AccountDatabase = serde_json::from_slice(&bytes)
                .map_err(|error| storage_error("REMOTE_AUTH_DATABASE_INVALID", error))?;
            validate_database(&database)?;
            database
        } else {
            let mut users = BTreeMap::new();
            let admin = StoredAccount {
                username: DEFAULT_ADMIN_USERNAME.to_string(),
                password_hash: hash_password(DEFAULT_ADMIN_PASSWORD)?,
                role: RemoteRole::Admin,
                devices: BTreeSet::new(),
                built_in: true,
                password_change_required: true,
            };
            users.insert(admin.username.clone(), admin);
            let database = AccountDatabase {
                version: DATABASE_VERSION,
                users,
            };
            persist_database(&path, &database)?;
            database
        };

        Ok(Self {
            path,
            session_ttl,
            state: Mutex::new(State {
                database,
                sessions: HashMap::new(),
            }),
        })
    }

    pub fn login(&self, username: &str, password: &str) -> Result<AuthenticatedSession, AppError> {
        let mut state = self.lock_state()?;
        let user = state
            .database
            .users
            .get(username)
            .ok_or_else(invalid_credentials)?;
        verify_password(password, &user.password_hash)?;

        let token = URL_SAFE_NO_PAD.encode(rand::random::<[u8; 32]>());
        let session = session_from_user(token.clone(), user);
        state.sessions.insert(
            token,
            SessionRecord {
                username: username.to_string(),
                expires_at: Instant::now() + self.session_ttl,
            },
        );
        Ok(session)
    }

    pub fn authenticate(&self, token: &str) -> Result<AuthenticatedSession, AppError> {
        let mut state = self.lock_state()?;
        let now = Instant::now();
        state.sessions.retain(|_, session| session.expires_at > now);
        let record = state.sessions.get(token).ok_or_else(|| {
            AppError::new(
                "REMOTE_AUTH_SESSION_INVALID",
                "Remote session is missing, expired, or revoked.",
                "remote.auth",
                true,
            )
        })?;
        let user = state.database.users.get(&record.username).ok_or_else(|| {
            AppError::new(
                "REMOTE_AUTH_SESSION_INVALID",
                "Remote session account no longer exists.",
                "remote.auth",
                true,
            )
        })?;
        Ok(session_from_user(token.to_string(), user))
    }

    pub fn logout(&self, token: &str) -> Result<(), AppError> {
        self.lock_state()?.sessions.remove(token);
        Ok(())
    }

    pub fn list_accounts(&self) -> Result<Vec<RemoteAccount>, AppError> {
        Ok(self
            .lock_state()?
            .database
            .users
            .values()
            .map(public_account)
            .collect())
    }

    pub fn create_user(&self, username: &str, password: &str) -> Result<RemoteAccount, AppError> {
        validate_username(username)?;
        validate_new_password(password)?;
        let mut state = self.lock_state()?;
        if state.database.users.contains_key(username) {
            return Err(AppError::new(
                "REMOTE_AUTH_USER_EXISTS",
                "A remote account with this username already exists.",
                "remote.auth",
                false,
            ));
        }
        let user = StoredAccount {
            username: username.to_string(),
            password_hash: hash_password(password)?,
            role: RemoteRole::User,
            devices: BTreeSet::new(),
            built_in: false,
            password_change_required: false,
        };
        let public = public_account(&user);
        state.database.users.insert(username.to_string(), user);
        self.persist_locked(&state)?;
        Ok(public)
    }

    pub fn delete_user(&self, username: &str) -> Result<(), AppError> {
        if username == DEFAULT_ADMIN_USERNAME {
            return Err(AppError::new(
                "REMOTE_AUTH_BUILTIN_USER_IMMUTABLE",
                "The built-in administrator account cannot be deleted or renamed.",
                "remote.auth",
                false,
            ));
        }
        let mut state = self.lock_state()?;
        if state.database.users.remove(username).is_none() {
            return Err(user_not_found(username));
        }
        state
            .sessions
            .retain(|_, session| session.username != username);
        self.persist_locked(&state)
    }

    pub fn change_own_password(
        &self,
        username: &str,
        current_password: &str,
        new_password: &str,
    ) -> Result<(), AppError> {
        validate_new_password(new_password)?;
        let mut state = self.lock_state()?;
        let user = state
            .database
            .users
            .get_mut(username)
            .ok_or_else(|| user_not_found(username))?;
        verify_password(current_password, &user.password_hash)?;
        user.password_hash = hash_password(new_password)?;
        user.password_change_required = false;
        state
            .sessions
            .retain(|_, session| session.username != username);
        self.persist_locked(&state)
    }

    pub fn reset_password(&self, username: &str, new_password: &str) -> Result<(), AppError> {
        validate_new_password(new_password)?;
        let mut state = self.lock_state()?;
        let user = state
            .database
            .users
            .get_mut(username)
            .ok_or_else(|| user_not_found(username))?;
        user.password_hash = hash_password(new_password)?;
        user.password_change_required = true;
        state
            .sessions
            .retain(|_, session| session.username != username);
        self.persist_locked(&state)
    }

    pub fn set_device_access(
        &self,
        username: &str,
        device_id: &str,
        assigned: bool,
    ) -> Result<RemoteAccount, AppError> {
        if device_id.trim().is_empty() {
            return Err(AppError::new(
                "REMOTE_AUTH_DEVICE_ID_INVALID",
                "deviceId must not be empty.",
                "remote.auth",
                false,
            ));
        }
        let mut state = self.lock_state()?;
        let user = state
            .database
            .users
            .get_mut(username)
            .ok_or_else(|| user_not_found(username))?;
        if assigned {
            user.devices.insert(device_id.to_string());
        } else {
            user.devices.remove(device_id);
        }
        let public = public_account(user);
        self.persist_locked(&state)?;
        Ok(public)
    }

    fn lock_state(&self) -> Result<std::sync::MutexGuard<'_, State>, AppError> {
        self.state.lock().map_err(|_| {
            AppError::new(
                "REMOTE_AUTH_STATE_UNAVAILABLE",
                "Remote account state lock is unavailable.",
                "remote.auth",
                true,
            )
        })
    }

    fn persist_locked(&self, state: &State) -> Result<(), AppError> {
        persist_database(&self.path, &state.database)
    }
}

fn validate_database(database: &AccountDatabase) -> Result<(), AppError> {
    if database.version != DATABASE_VERSION
        || !database.users.contains_key(DEFAULT_ADMIN_USERNAME)
        || database.users[DEFAULT_ADMIN_USERNAME].role != RemoteRole::Admin
        || !database.users[DEFAULT_ADMIN_USERNAME].built_in
    {
        return Err(AppError::new(
            "REMOTE_AUTH_DATABASE_INVALID",
            "Remote account database is unsupported or missing the built-in administrator.",
            "remote.auth",
            false,
        ));
    }
    Ok(())
}

fn validate_username(username: &str) -> Result<(), AppError> {
    let valid = (3..=64).contains(&username.len())
        && username
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'_' | b'-'));
    if valid {
        Ok(())
    } else {
        Err(AppError::new(
            "REMOTE_AUTH_USERNAME_INVALID",
            "Username must be 3-64 ASCII letters, digits, dots, underscores, or hyphens.",
            "remote.auth",
            false,
        ))
    }
}

fn validate_new_password(password: &str) -> Result<(), AppError> {
    if (8..=1024).contains(&password.len()) {
        Ok(())
    } else {
        Err(AppError::new(
            "REMOTE_AUTH_PASSWORD_INVALID",
            "New passwords must contain 8-1024 bytes.",
            "remote.auth",
            false,
        ))
    }
}

fn hash_password(password: &str) -> Result<String, AppError> {
    let salt = SaltString::encode_b64(&rand::random::<[u8; 16]>()).map_err(|error| {
        AppError::new(
            "REMOTE_AUTH_PASSWORD_HASH_FAILED",
            "Failed to generate a password salt.",
            "remote.auth",
            false,
        )
        .with_cause(error)
    })?;
    Argon2::default()
        .hash_password(password.as_bytes(), &salt)
        .map(|hash| hash.to_string())
        .map_err(|error| {
            AppError::new(
                "REMOTE_AUTH_PASSWORD_HASH_FAILED",
                "Failed to hash the remote account password.",
                "remote.auth",
                false,
            )
            .with_cause(error)
        })
}

fn verify_password(password: &str, encoded_hash: &str) -> Result<(), AppError> {
    let parsed = PasswordHash::new(encoded_hash).map_err(|error| {
        AppError::new(
            "REMOTE_AUTH_DATABASE_INVALID",
            "Stored remote password hash is invalid.",
            "remote.auth",
            false,
        )
        .with_cause(error)
    })?;
    Argon2::default()
        .verify_password(password.as_bytes(), &parsed)
        .map_err(|_| invalid_credentials())
}

fn invalid_credentials() -> AppError {
    AppError::new(
        "REMOTE_AUTH_CREDENTIALS_INVALID",
        "Username or password is incorrect.",
        "remote.auth",
        false,
    )
}

fn user_not_found(username: &str) -> AppError {
    AppError::new(
        "REMOTE_AUTH_USER_NOT_FOUND",
        format!("Remote account {username} does not exist."),
        "remote.auth",
        false,
    )
}

fn public_account(user: &StoredAccount) -> RemoteAccount {
    RemoteAccount {
        username: user.username.clone(),
        role: user.role,
        devices: user.devices.iter().cloned().collect(),
        built_in: user.built_in,
        password_change_required: user.password_change_required,
    }
}

fn session_from_user(token: String, user: &StoredAccount) -> AuthenticatedSession {
    AuthenticatedSession {
        token,
        username: user.username.clone(),
        role: user.role,
        devices: user.devices.clone(),
        password_change_required: user.password_change_required,
    }
}

fn persist_database(path: &Path, database: &AccountDatabase) -> Result<(), AppError> {
    if let Some(parent) = path.parent() {
        if !parent.as_os_str().is_empty() {
            fs::create_dir_all(parent)
                .map_err(|error| storage_error("REMOTE_AUTH_DIRECTORY_CREATE_FAILED", error))?;
        }
    }
    let bytes = serde_json::to_vec_pretty(database)
        .map_err(|error| storage_error("REMOTE_AUTH_SERIALIZE_FAILED", error))?;
    let mut file = OpenOptions::new()
        .create(true)
        .write(true)
        .truncate(true)
        .open(path)
        .map_err(|error| storage_error("REMOTE_AUTH_WRITE_FAILED", error))?;
    file.write_all(&bytes)
        .and_then(|_| file.sync_all())
        .map_err(|error| storage_error("REMOTE_AUTH_WRITE_FAILED", error))?;
    Ok(())
}

fn storage_error(code: &str, error: impl ToString) -> AppError {
    AppError::new(
        code,
        "Failed to access the remote account database.",
        "remote.auth",
        true,
    )
    .with_cause(error)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_path(name: &str) -> PathBuf {
        std::env::temp_dir().join(format!(
            "adbcontrol-remote-auth-{name}-{}.json",
            rand::random::<u64>()
        ))
    }

    #[test]
    fn initializes_builtin_admin_and_hashes_default_password() {
        let path = test_path("default-admin");
        let manager = RemoteAccountManager::load_or_initialize(&path).unwrap();
        let session = manager.login("admin", "admin").unwrap();
        let stored = fs::read_to_string(&path).unwrap();

        assert_eq!(session.role, RemoteRole::Admin);
        assert!(session.password_change_required);
        assert!(!stored.contains("\"passwordHash\": \"admin\""));
        assert!(stored.contains("$argon2"));
        fs::remove_file(path).ok();
    }

    #[test]
    fn builtin_admin_can_change_password_but_cannot_be_deleted() {
        let path = test_path("admin-change");
        let manager = RemoteAccountManager::load_or_initialize(&path).unwrap();
        manager
            .change_own_password("admin", "admin", "new-admin-password")
            .unwrap();

        assert!(manager.login("admin", "admin").is_err());
        assert!(
            !manager
                .login("admin", "new-admin-password")
                .unwrap()
                .password_change_required
        );
        assert_eq!(
            manager.delete_user("admin").unwrap_err().error_code,
            "REMOTE_AUTH_BUILTIN_USER_IMMUTABLE"
        );
        fs::remove_file(path).ok();
    }

    #[test]
    fn device_assignments_are_persisted_and_sessions_are_revoked_on_reset() {
        let path = test_path("assignments");
        let manager = RemoteAccountManager::load_or_initialize(&path).unwrap();
        manager
            .create_user("operator", "operator-password")
            .unwrap();
        manager
            .set_device_access("operator", "device-1", true)
            .unwrap();
        let old_session = manager.login("operator", "operator-password").unwrap();
        manager
            .reset_password("operator", "replacement-password")
            .unwrap();
        assert!(manager.authenticate(&old_session.token).is_err());
        drop(manager);

        let reloaded = RemoteAccountManager::load_or_initialize(&path).unwrap();
        let session = reloaded.login("operator", "replacement-password").unwrap();
        assert!(session.devices.contains("device-1"));
        fs::remove_file(path).ok();
    }
}
