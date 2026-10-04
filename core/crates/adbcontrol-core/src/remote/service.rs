use std::sync::Arc;

use serde::{de::DeserializeOwned, Deserialize, Serialize};
use serde_json::{json, Value};

use crate::{
    error::AppError,
    protocol::{CoreService, IpcRequest, IpcResponse},
    AdbRunner, CompanionCommandRouter,
};

use super::{AuthenticatedSession, RemoteAccountManager, RemoteRole, DEFAULT_ADMIN_USERNAME};

pub const REMOTE_PROTOCOL: &str = "adbcontrol-core-remote-quic";
pub const REMOTE_PROTOCOL_VERSION: u32 = 1;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteRequest {
    pub id: String,
    #[serde(default)]
    pub session_token: Option<String>,
    pub method: String,
    #[serde(default)]
    pub params: Value,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteResponse {
    pub id: Option<String>,
    pub ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub result: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<AppError>,
}

impl RemoteResponse {
    fn success(id: impl Into<String>, result: Value) -> Self {
        Self {
            id: Some(id.into()),
            ok: true,
            result: Some(result),
            error: None,
        }
    }

    fn failure(id: Option<String>, error: AppError) -> Self {
        Self {
            id,
            ok: false,
            result: None,
            error: Some(error),
        }
    }
}

pub trait CoreRequestHandler: Send + Sync {
    fn handle_core_request(&self, request: IpcRequest) -> IpcResponse;
}

impl<R, Q> CoreRequestHandler for CoreService<R, Q>
where
    R: AdbRunner,
    Q: CompanionCommandRouter,
{
    fn handle_core_request(&self, request: IpcRequest) -> IpcResponse {
        self.handle_request(request)
    }
}

pub struct RemoteControlService<H: CoreRequestHandler> {
    core: Arc<H>,
    accounts: Arc<RemoteAccountManager>,
}

impl<H: CoreRequestHandler> Clone for RemoteControlService<H> {
    fn clone(&self) -> Self {
        Self {
            core: Arc::clone(&self.core),
            accounts: Arc::clone(&self.accounts),
        }
    }
}

impl<H: CoreRequestHandler> RemoteControlService<H> {
    pub fn new(core: Arc<H>, accounts: Arc<RemoteAccountManager>) -> Self {
        Self { core, accounts }
    }

    pub fn handle_json_bytes(&self, bytes: &[u8]) -> Vec<u8> {
        let response = match serde_json::from_slice::<RemoteRequest>(bytes) {
            Ok(request) => self.handle_request(request),
            Err(error) => RemoteResponse::failure(
                None,
                AppError::new(
                    "REMOTE_PROTOCOL_INVALID_JSON",
                    "Remote request must be a valid JSON object.",
                    "remote.protocol",
                    false,
                )
                .with_cause(error),
            ),
        };
        serde_json::to_vec(&response).unwrap_or_else(|_| {
            br#"{"id":null,"ok":false,"error":{"errorCode":"REMOTE_PROTOCOL_ENCODE_FAILED","message":"Failed to encode remote response.","module":"remote.protocol","recoverable":false}}"#.to_vec()
        })
    }

    pub fn handle_request(&self, request: RemoteRequest) -> RemoteResponse {
        if request.method == "auth.login" {
            return self.login(request);
        }

        let token = match request.session_token.as_deref() {
            Some(token) if !token.is_empty() => token,
            _ => {
                return RemoteResponse::failure(
                    Some(request.id),
                    AppError::new(
                        "REMOTE_AUTH_REQUIRED",
                        "A valid remote session token is required.",
                        "remote.auth",
                        true,
                    ),
                )
            }
        };
        let session = match self.accounts.authenticate(token) {
            Ok(session) => session,
            Err(error) => return RemoteResponse::failure(Some(request.id), error),
        };

        if session.password_change_required
            && !matches!(
                request.method.as_str(),
                "auth.me" | "auth.changePassword" | "auth.logout"
            )
        {
            return RemoteResponse::failure(
                Some(request.id),
                AppError::new(
                    "REMOTE_AUTH_PASSWORD_CHANGE_REQUIRED",
                    "The account password must be changed before remote control is allowed.",
                    "remote.auth",
                    false,
                ),
            );
        }

        match request.method.as_str() {
            "auth.me" => RemoteResponse::success(
                request.id,
                json!({
                    "username": session.username,
                    "role": session.role,
                    "devices": session.devices,
                    "passwordChangeRequired": session.password_change_required,
                }),
            ),
            "auth.logout" => match self.accounts.logout(token) {
                Ok(()) => RemoteResponse::success(request.id, json!({"loggedOut": true})),
                Err(error) => RemoteResponse::failure(Some(request.id), error),
            },
            "auth.changePassword" => self.change_password(request, &session),
            "diagnostics.report" => self.report_diagnostics(request, &session),
            "admin.users.list" => self.admin_list_users(request, &session),
            "admin.users.create" => self.admin_create_user(request, &session),
            "admin.users.delete" => self.admin_delete_user(request, &session),
            "admin.users.resetPassword" => self.admin_reset_password(request, &session),
            "admin.devices.assign" => self.admin_assign_device(request, &session),
            _ => self.forward_core_request(request, &session),
        }
    }

    fn report_diagnostics(
        &self,
        request: RemoteRequest,
        session: &AuthenticatedSession,
    ) -> RemoteResponse {
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct ClientEvent {
            #[serde(default)]
            level: String,
            #[serde(default)]
            phase: String,
            #[serde(default)]
            module: String,
            #[serde(default)]
            ok: bool,
            #[serde(default)]
            elapsed_ms: u64,
            #[serde(default)]
            error_code: String,
            #[serde(default)]
            detail: String,
        }
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct Params {
            #[serde(default)]
            session_id: String,
            events: Vec<ClientEvent>,
        }
        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return RemoteResponse::failure(Some(request.id), error),
        };
        if params.events.is_empty() || params.events.len() > 50 {
            return RemoteResponse::failure(
                Some(request.id),
                AppError::new(
                    "REMOTE_DIAGNOSTIC_BATCH_INVALID",
                    "Diagnostic batches must contain between 1 and 50 events.",
                    "remote.diagnostics",
                    true,
                ),
            );
        }
        fn safe(value: &str, max: usize) -> String {
            let value = value.trim();
            if value.is_empty()
                || value.len() > max
                || !value
                    .bytes()
                    .all(|byte| byte.is_ascii_alphanumeric() || b"_.:+-/=@ ".contains(&byte))
            {
                return "unknown".into();
            }
            value.to_owned()
        }
        let session_id = safe(&params.session_id, 64);
        for event in &params.events {
            let phase = safe(&event.phase, 96);
            let module = safe(&event.module, 96);
            let code = safe(&event.error_code, 96);
            let detail = safe(&event.detail, 200);
            let trace = format!("mobile-{session_id}");
            crate::diagnostics::record(
                "mobile.client",
                &phase,
                &trace,
                event.ok,
                event.elapsed_ms as u128,
                (!event.ok).then_some(code.as_str()),
            );
            eprintln!(
                "{}",
                json!({
                    "event": "mobile_client_diagnostic",
                    "actor": session.username,
                    "sessionId": session_id,
                    "level": safe(&event.level, 16),
                    "phase": phase,
                    "module": module,
                    "ok": event.ok,
                    "elapsedMs": event.elapsed_ms,
                    "errorCode": if event.ok { "" } else { &code },
                    "detail": detail,
                })
            );
        }
        RemoteResponse::success(request.id, json!({"accepted": params.events.len()}))
    }

    fn login(&self, request: RemoteRequest) -> RemoteResponse {
        #[derive(Deserialize)]
        struct Params {
            username: String,
            password: String,
        }
        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return RemoteResponse::failure(Some(request.id), error),
        };
        // The built-in administrator is a local-console identity. Exposing it
        // through the remote transport would collapse the management and
        // remote-user trust boundaries.
        if params.username == DEFAULT_ADMIN_USERNAME {
            return RemoteResponse::failure(
                Some(request.id),
                AppError::new(
                    "REMOTE_AUTH_ADMIN_LOCAL_ONLY",
                    "The built-in administrator can only sign in through the local management frontend.",
                    "remote.auth",
                    false,
                ),
            );
        }
        match self.accounts.login(&params.username, &params.password) {
            Ok(session) => RemoteResponse::success(
                request.id,
                json!({
                    "protocol": REMOTE_PROTOCOL,
                    "protocolVersion": REMOTE_PROTOCOL_VERSION,
                    "sessionToken": session.token,
                    "username": session.username,
                    "role": session.role,
                    "devices": session.devices,
                    "passwordChangeRequired": session.password_change_required,
                }),
            ),
            Err(error) => RemoteResponse::failure(Some(request.id), error),
        }
    }

    fn change_password(
        &self,
        request: RemoteRequest,
        session: &AuthenticatedSession,
    ) -> RemoteResponse {
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct Params {
            current_password: String,
            new_password: String,
        }
        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return RemoteResponse::failure(Some(request.id), error),
        };
        match self.accounts.change_own_password(
            &session.username,
            &params.current_password,
            &params.new_password,
        ) {
            Ok(()) => RemoteResponse::success(
                request.id,
                json!({"passwordChanged": true, "loginRequired": true}),
            ),
            Err(error) => RemoteResponse::failure(Some(request.id), error),
        }
    }

    fn admin_list_users(
        &self,
        request: RemoteRequest,
        session: &AuthenticatedSession,
    ) -> RemoteResponse {
        if let Err(error) = require_admin(session) {
            return RemoteResponse::failure(Some(request.id), error);
        }
        match self.accounts.list_accounts() {
            Ok(accounts) => serialize_success(request.id, &accounts),
            Err(error) => RemoteResponse::failure(Some(request.id), error),
        }
    }

    fn admin_create_user(
        &self,
        request: RemoteRequest,
        session: &AuthenticatedSession,
    ) -> RemoteResponse {
        #[derive(Deserialize)]
        struct Params {
            username: String,
            password: String,
        }
        if let Err(error) = require_admin(session) {
            return RemoteResponse::failure(Some(request.id), error);
        }
        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return RemoteResponse::failure(Some(request.id), error),
        };
        match self
            .accounts
            .create_user(&params.username, &params.password)
        {
            Ok(account) => serialize_success(request.id, &account),
            Err(error) => RemoteResponse::failure(Some(request.id), error),
        }
    }

    fn admin_delete_user(
        &self,
        request: RemoteRequest,
        session: &AuthenticatedSession,
    ) -> RemoteResponse {
        #[derive(Deserialize)]
        struct Params {
            username: String,
        }
        if let Err(error) = require_admin(session) {
            return RemoteResponse::failure(Some(request.id), error);
        }
        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return RemoteResponse::failure(Some(request.id), error),
        };
        match self.accounts.delete_user(&params.username) {
            Ok(()) => RemoteResponse::success(request.id, json!({"deleted": true})),
            Err(error) => RemoteResponse::failure(Some(request.id), error),
        }
    }

    fn admin_reset_password(
        &self,
        request: RemoteRequest,
        session: &AuthenticatedSession,
    ) -> RemoteResponse {
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct Params {
            username: String,
            new_password: String,
        }
        if let Err(error) = require_admin(session) {
            return RemoteResponse::failure(Some(request.id), error);
        }
        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return RemoteResponse::failure(Some(request.id), error),
        };
        match self
            .accounts
            .reset_password(&params.username, &params.new_password)
        {
            Ok(()) => RemoteResponse::success(
                request.id,
                json!({"passwordReset": true, "loginRequired": true}),
            ),
            Err(error) => RemoteResponse::failure(Some(request.id), error),
        }
    }

    fn admin_assign_device(
        &self,
        request: RemoteRequest,
        session: &AuthenticatedSession,
    ) -> RemoteResponse {
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct Params {
            username: String,
            device_id: String,
            #[serde(default = "default_true")]
            assigned: bool,
        }
        if let Err(error) = require_admin(session) {
            return RemoteResponse::failure(Some(request.id), error);
        }
        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return RemoteResponse::failure(Some(request.id), error),
        };
        if params.assigned && !self.core_has_device(&params.device_id) {
            return RemoteResponse::failure(
                Some(request.id),
                AppError::new(
                    "REMOTE_AUTH_DEVICE_NOT_CONNECTED",
                    "Only a device currently known to the core can be assigned.",
                    "remote.auth",
                    true,
                ),
            );
        }
        match self
            .accounts
            .set_device_access(&params.username, &params.device_id, params.assigned)
        {
            Ok(account) => serialize_success(request.id, &account),
            Err(error) => RemoteResponse::failure(Some(request.id), error),
        }
    }

    fn forward_core_request(
        &self,
        mut request: RemoteRequest,
        session: &AuthenticatedSession,
    ) -> RemoteResponse {
        if session.role != RemoteRole::Admin {
            self.resolve_request_device_alias(&mut request, session);
            if let Err(error) = authorize_user_core_request(&request, session) {
                return RemoteResponse::failure(Some(request.id), error);
            }
        }
        let is_device_list = request.method == "device.list";
        let id = request.id.clone();
        let mut response = self.core.handle_core_request(IpcRequest {
            id,
            method: request.method,
            params: request.params,
        });
        if is_device_list && session.role != RemoteRole::Admin && response.ok {
            if let Some(Value::Array(devices)) = response.result.as_mut() {
                reconcile_remote_device_list(devices, session, self.current_adb_device_ids());
            }
        }
        RemoteResponse {
            id: response.id,
            ok: response.ok,
            result: response.result,
            error: response.error,
        }
    }

    fn core_has_device(&self, device_id: &str) -> bool {
        let response = self.core.handle_core_request(IpcRequest {
            id: String::from("remote-assignment-device-check"),
            method: String::from("device.list"),
            params: json!({}),
        });
        response
            .result
            .and_then(|value| value.as_array().cloned())
            .is_some_and(|devices| {
                devices
                    .iter()
                    .any(|device| device.get("deviceId").and_then(Value::as_str) == Some(device_id))
            })
    }

    fn resolve_request_device_alias(
        &self,
        request: &mut RemoteRequest,
        session: &AuthenticatedSession,
    ) {
        let requested = match request.method.as_str() {
            "adb.exec" => request
                .params
                .get("args")
                .and_then(Value::as_array)
                .and_then(|args| args.get(1))
                .and_then(Value::as_str),
            "device.getCapabilities" | "device.getPermissionState" | "device.invoke" => {
                request.params.get("deviceId").and_then(Value::as_str)
            }
            _ => None,
        };
        let Some(requested) = requested else { return };
        if !session_can_access_device(session, requested) {
            return;
        }
        let Some(current) = self
            .current_adb_device_ids()
            .into_iter()
            .find(|candidate| same_remote_device_identity(requested, candidate))
        else {
            return;
        };
        match request.method.as_str() {
            "adb.exec" => {
                if let Some(args) = request.params.get_mut("args").and_then(Value::as_array_mut) {
                    args[1] = Value::String(current);
                }
            }
            _ => request.params["deviceId"] = Value::String(current),
        }
    }

    fn current_adb_device_ids(&self) -> Vec<String> {
        let response = self.core.handle_core_request(IpcRequest {
            id: String::from("remote-current-adb-devices"),
            method: String::from("adb.exec"),
            params: json!({"args": ["devices", "-l"]}),
        });
        let Some(stdout) = response
            .result
            .as_ref()
            .and_then(|value| value.get("stdout"))
            .and_then(Value::as_str)
        else {
            return Vec::new();
        };
        stdout.lines().filter_map(parse_adb_device_id).collect()
    }
}

fn authorize_user_core_request(
    request: &RemoteRequest,
    session: &AuthenticatedSession,
) -> Result<(), AppError> {
    match request.method.as_str() {
        "core.getHostTarget"
        | "adb.asset.current"
        | "companion.protocol.info"
        | "capability.list"
        | "device.list" => Ok(()),
        "device.getCapabilities" | "device.getPermissionState" | "device.invoke" => {
            authorize_device_id(request.params.get("deviceId"), session)
        }
        "adb.exec" => {
            let args = request
                .params
                .get("args")
                .and_then(Value::as_array)
                .ok_or_else(permission_denied)?;
            // Only accept ADB's global `-s <serial>` position. Searching the
            // entire argument list would mistake a shell-command argument for
            // a device selector and could authorize the wrong default device.
            let device_id = match args.as_slice() {
                [selector, device_id, ..] if selector.as_str() == Some("-s") => device_id.as_str(),
                _ => None,
            };
            let device_command = args.get(2).and_then(Value::as_str);
            match (device_id, device_command) {
                (Some(device_id), Some(command))
                    if session_can_access_device(session, device_id)
                        && is_device_scoped_adb_command(command) =>
                {
                    Ok(())
                }
                _ => Err(permission_denied()),
            }
        }
        _ => Err(permission_denied()),
    }
}

fn authorize_device_id(
    device_id: Option<&Value>,
    session: &AuthenticatedSession,
) -> Result<(), AppError> {
    let device_id = device_id.and_then(Value::as_str).unwrap_or_default();
    if !device_id.is_empty() && session_can_access_device(session, device_id) {
        Ok(())
    } else {
        Err(permission_denied())
    }
}

fn session_can_access_device(session: &AuthenticatedSession, device_id: &str) -> bool {
    session
        .devices
        .iter()
        .any(|assigned| same_remote_device_identity(assigned, device_id))
}

fn reconcile_remote_device_list(
    devices: &mut Vec<Value>,
    session: &AuthenticatedSession,
    current_adb_ids: Vec<String>,
) {
    let registered = std::mem::take(devices);
    let mut reconciled: Vec<Value> = Vec::new();
    for assigned_id in &session.devices {
        let current_id = current_adb_ids
            .iter()
            .find(|current| same_remote_device_identity(assigned_id, current));
        let registered_entry = registered
            .iter()
            .filter(|device| {
                device
                    .get("deviceId")
                    .and_then(Value::as_str)
                    .is_some_and(|known| same_remote_device_identity(known, assigned_id))
            })
            .max_by_key(|device| {
                matches!(
                    device.get("connectionState").and_then(Value::as_str),
                    Some("ready")
                )
            })
            .cloned();
        let remembered_id = registered_entry
            .as_ref()
            .and_then(|device| device.get("deviceId"))
            .and_then(Value::as_str);
        let display_id = current_id
            .map(String::as_str)
            .or(remembered_id)
            .unwrap_or(assigned_id)
            .to_owned();
        if reconciled.iter().any(|device| {
            device
                .get("deviceId")
                .and_then(Value::as_str)
                .is_some_and(|known| same_remote_device_identity(known, &display_id))
        }) {
            continue;
        }

        // Account assignments are the persistent device directory for remote
        // users. Companion and ADB state enrich that directory; they do not
        // remove a remembered card merely because the transport is offline.
        let mut entry = registered_entry.unwrap_or_else(|| json!({}));
        let previous_id = entry
            .get("deviceId")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_owned();
        let replace_name = entry
            .get("name")
            .and_then(Value::as_str)
            .is_none_or(|name| name.is_empty() || name == previous_id);
        entry["deviceId"] = Value::String(display_id.clone());
        entry["adbState"] = Value::String(if current_id.is_some() {
            String::from("device")
        } else {
            String::from("offline")
        });
        if entry.get("connectionState").is_none()
            || matches!(
                entry.get("connectionState").and_then(Value::as_str),
                Some("adb" | "disconnected" | "offline")
            )
        {
            entry["connectionState"] = Value::String(if current_id.is_some() {
                String::from("adb")
            } else {
                String::from("offline")
            });
        }
        if replace_name {
            entry["name"] = Value::String(display_id);
        }
        reconciled.push(entry);
    }
    *devices = reconciled;
}

fn same_remote_device_identity(left: &str, right: &str) -> bool {
    left == right
        || normalized_mdns_service_identity(left)
            .zip(normalized_mdns_service_identity(right))
            .is_some_and(|(left, right)| left == right)
}

fn normalized_mdns_service_identity(value: &str) -> Option<&str> {
    const SUFFIX: &str = "._adb-tls-connect._tcp";
    let service = value.strip_suffix(SUFFIX)?;
    if let Some(opening) = service.rfind(" (") {
        let instance = service.strip_suffix(')')?.get(opening + 2..)?;
        if !instance.is_empty() && instance.bytes().all(|byte| byte.is_ascii_digit()) {
            return service.get(..opening);
        }
    }
    (!service.is_empty()).then_some(service)
}

fn parse_adb_device_id(line: &str) -> Option<String> {
    let fields: Vec<&str> = line.split_whitespace().collect();
    let state_index = fields
        .iter()
        .position(|field| matches!(*field, "device" | "offline" | "unauthorized"))?;
    (state_index > 0 && fields[state_index] == "device").then(|| fields[..state_index].join(" "))
}

fn is_device_scoped_adb_command(command: &str) -> bool {
    matches!(
        command,
        "shell"
            | "exec-out"
            | "push"
            | "pull"
            | "install"
            | "install-multiple"
            | "install-multi-package"
            | "uninstall"
            | "forward"
            | "reverse"
            | "reboot"
            | "root"
            | "unroot"
            | "remount"
            | "disable-verity"
            | "enable-verity"
            | "bugreport"
            | "logcat"
            | "jdwp"
            | "get-state"
            | "get-serialno"
            | "get-devpath"
            | "features"
    )
}

fn require_admin(session: &AuthenticatedSession) -> Result<(), AppError> {
    if session.role == RemoteRole::Admin {
        Ok(())
    } else {
        Err(permission_denied())
    }
}

fn permission_denied() -> AppError {
    AppError::new(
        "REMOTE_AUTH_FORBIDDEN",
        "The remote account is not authorized for this operation or device.",
        "remote.auth",
        false,
    )
}

fn parse_params<T: DeserializeOwned>(params: Value) -> Result<T, AppError> {
    serde_json::from_value(params).map_err(|error| {
        AppError::new(
            "REMOTE_PROTOCOL_PARAMS_INVALID",
            "Remote request parameters are invalid.",
            "remote.protocol",
            false,
        )
        .with_cause(error)
    })
}

fn serialize_success(id: String, value: &impl Serialize) -> RemoteResponse {
    match serde_json::to_value(value) {
        Ok(value) => RemoteResponse::success(id, value),
        Err(error) => RemoteResponse::failure(
            Some(id),
            AppError::new(
                "REMOTE_PROTOCOL_ENCODE_FAILED",
                "Failed to encode remote response.",
                "remote.protocol",
                false,
            )
            .with_cause(error),
        ),
    }
}

fn default_true() -> bool {
    true
}

#[cfg(test)]
mod tests {
    use std::{path::PathBuf, sync::Mutex};

    use super::*;

    #[derive(Default)]
    struct FakeCore {
        calls: Mutex<Vec<IpcRequest>>,
    }

    impl CoreRequestHandler for FakeCore {
        fn handle_core_request(&self, request: IpcRequest) -> IpcResponse {
            self.calls.lock().unwrap().push(request.clone());
            if request.method == "device.list" {
                IpcResponse::success(
                    request.id,
                    json!([
                        {"deviceId": "device-1", "name": "One"},
                        {"deviceId": "device-2", "name": "Two"}
                    ]),
                )
            } else if request.method == "adb.exec"
                && request.params["args"].get(0).and_then(Value::as_str) == Some("devices")
            {
                IpcResponse::success(
                    request.id,
                    json!({
                        "exitCode": 0,
                        "stdout": "List of devices attached\ndevice-1 device\ndevice-2 device\n",
                        "stderr": ""
                    }),
                )
            } else {
                IpcResponse::success(request.id, json!({"forwarded": true}))
            }
        }
    }

    fn setup() -> (
        RemoteControlService<FakeCore>,
        Arc<RemoteAccountManager>,
        PathBuf,
    ) {
        let path = std::env::temp_dir().join(format!(
            "adbcontrol-remote-service-{}.json",
            rand::random::<u64>()
        ));
        let accounts = Arc::new(RemoteAccountManager::load_or_initialize(&path).unwrap());
        let service =
            RemoteControlService::new(Arc::new(FakeCore::default()), Arc::clone(&accounts));
        (service, accounts, path)
    }

    fn request(id: &str, token: Option<&str>, method: &str, params: Value) -> RemoteRequest {
        RemoteRequest {
            id: id.to_string(),
            session_token: token.map(str::to_string),
            method: method.to_string(),
            params,
        }
    }

    fn login(service: &RemoteControlService<FakeCore>, username: &str, password: &str) -> String {
        service
            .handle_request(request(
                "login",
                None,
                "auth.login",
                json!({"username": username, "password": password}),
            ))
            .result
            .unwrap()["sessionToken"]
            .as_str()
            .unwrap()
            .to_string()
    }

    #[test]
    fn login_is_public_but_other_requests_require_a_session() {
        let (service, accounts, path) = setup();
        accounts
            .create_user("operator", "operator-password")
            .unwrap();
        assert!(service
            .handle_request(request("1", None, "device.list", json!({})))
            .error
            .unwrap()
            .error_code
            .contains("AUTH_REQUIRED"));
        assert!(!login(&service, "operator", "operator-password").is_empty());
        std::fs::remove_file(path).ok();
    }

    #[test]
    fn builtin_admin_cannot_sign_in_through_remote_transport() {
        let (service, _accounts, path) = setup();
        let denied = service.handle_request(request(
            "login-admin",
            None,
            "auth.login",
            json!({"username": "admin", "password": "admin"}),
        ));
        assert_eq!(
            denied.error.unwrap().error_code,
            "REMOTE_AUTH_ADMIN_LOCAL_ONLY"
        );
        std::fs::remove_file(path).ok();
    }

    #[test]
    fn authenticated_mobile_client_can_report_bounded_diagnostics() {
        let (service, accounts, path) = setup();
        accounts
            .create_user("operator", "operator-password")
            .unwrap();
        let operator = login(&service, "operator", "operator-password");
        let accepted = service.handle_request(request(
            "diagnostics",
            Some(&operator),
            "diagnostics.report",
            json!({
                "sessionId": "mobile-test",
                "events": [{
                    "level": "error",
                    "phase": "quic.request",
                    "module": "remote.transport",
                    "ok": false,
                    "elapsedMs": 1200,
                    "errorCode": "REMOTE_QUIC_REQUEST_FAILED",
                    "detail": "timeout"
                }]
            }),
        ));
        assert!(accepted.ok);
        assert_eq!(accepted.result.unwrap()["accepted"], 1);

        let rejected = service.handle_request(request(
            "empty-diagnostics",
            Some(&operator),
            "diagnostics.report",
            json!({"sessionId": "mobile-test", "events": []}),
        ));
        assert_eq!(
            rejected.error.unwrap().error_code,
            "REMOTE_DIAGNOSTIC_BATCH_INVALID"
        );
        std::fs::remove_file(path).ok();
    }

    #[test]
    fn assigned_user_sees_and_controls_only_assigned_devices() {
        let (service, accounts, path) = setup();
        accounts
            .create_user("operator", "operator-password")
            .unwrap();
        accounts
            .set_device_access("operator", "device-1", true)
            .unwrap();

        let operator = login(&service, "operator", "operator-password");
        let list =
            service.handle_request(request("list", Some(&operator), "device.list", json!({})));
        assert_eq!(list.result.unwrap().as_array().unwrap().len(), 1);
        assert!(
            service
                .handle_request(request(
                    "allowed",
                    Some(&operator),
                    "device.invoke",
                    json!({"deviceId": "device-1"}),
                ))
                .ok
        );
        assert_eq!(
            service
                .handle_request(request(
                    "denied",
                    Some(&operator),
                    "device.invoke",
                    json!({"deviceId": "device-2"}),
                ))
                .error
                .unwrap()
                .error_code,
            "REMOTE_AUTH_FORBIDDEN"
        );
        std::fs::remove_file(path).ok();
    }

    #[test]
    fn user_adb_exec_requires_global_assigned_serial_selector() {
        let (service, accounts, path) = setup();
        accounts
            .create_user("operator", "operator-password")
            .unwrap();
        accounts
            .set_device_access("operator", "device-1", true)
            .unwrap();
        let operator = login(&service, "operator", "operator-password");

        assert!(
            service
                .handle_request(request(
                    "allowed",
                    Some(&operator),
                    "adb.exec",
                    json!({"args": ["-s", "device-1", "shell", "id"]}),
                ))
                .ok
        );
        let hidden_selector = service.handle_request(request(
            "denied",
            Some(&operator),
            "adb.exec",
            json!({"args": ["shell", "echo", "-s", "device-1"]}),
        ));
        assert_eq!(
            hidden_selector.error.unwrap().error_code,
            "REMOTE_AUTH_FORBIDDEN"
        );
        let global_command = service.handle_request(request(
            "global-denied",
            Some(&operator),
            "adb.exec",
            json!({"args": ["-s", "device-1", "kill-server"]}),
        ));
        assert_eq!(
            global_command.error.unwrap().error_code,
            "REMOTE_AUTH_FORBIDDEN"
        );
        std::fs::remove_file(path).ok();
    }

    #[test]
    fn mdns_collision_suffix_does_not_break_assigned_device_identity() {
        let assigned = "adb-R3CR70SJHHR-2VyQ5v (2)._adb-tls-connect._tcp";
        let current = "adb-R3CR70SJHHR-2VyQ5v._adb-tls-connect._tcp";
        assert!(same_remote_device_identity(assigned, current));
        assert!(!same_remote_device_identity(
            assigned,
            "adb-other._adb-tls-connect._tcp"
        ));
        assert!(!same_remote_device_identity(assigned, "192.168.3.168:5555"));
    }

    #[test]
    fn adb_identity_refresh_excludes_offline_and_unauthorized_rows() {
        assert_eq!(
            parse_adb_device_id("device-1 device product:x"),
            Some(String::from("device-1"))
        );
        assert_eq!(parse_adb_device_id("device-1 offline product:x"), None);
        assert_eq!(parse_adb_device_id("device-1 unauthorized product:x"), None);
    }

    #[test]
    fn current_adb_identity_replaces_stale_companion_entry_without_duplicate() {
        // 场景：伴侣注册表保留带“ (2)”的旧 ID，但 ADB 已用无后缀 ID 在线；
        // 手机端只能收到一个使用当前 ID 的设备条目；ADB 离线后该记忆条目仍保留。
        let session = AuthenticatedSession {
            token: String::from("test-token"),
            username: String::from("operator"),
            role: RemoteRole::User,
            devices: [String::from(
                "adb-R3CR70SJHHR-2VyQ5v (2)._adb-tls-connect._tcp",
            )]
            .into_iter()
            .collect(),
            password_change_required: false,
        };
        let current = "adb-R3CR70SJHHR-2VyQ5v._adb-tls-connect._tcp";
        let mut devices = vec![json!({
            "deviceId": "adb-R3CR70SJHHR-2VyQ5v (2)._adb-tls-connect._tcp",
            "name": "adb-R3CR70SJHHR-2VyQ5v (2)._adb-tls-connect._tcp",
            "connectionState": "disconnected"
        })];

        reconcile_remote_device_list(&mut devices, &session, vec![String::from(current)]);

        assert_eq!(devices.len(), 1);
        assert_eq!(devices[0]["deviceId"], current);
        assert_eq!(devices[0]["name"], current);
        assert_eq!(devices[0]["adbState"], "device");

        reconcile_remote_device_list(&mut devices, &session, Vec::new());
        assert_eq!(devices.len(), 1);
        assert_eq!(devices[0]["deviceId"], current);
        assert_eq!(devices[0]["adbState"], "offline");
    }
}
