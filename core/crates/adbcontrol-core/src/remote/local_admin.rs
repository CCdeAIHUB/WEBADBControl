use std::sync::Arc;

use serde::{de::DeserializeOwned, Deserialize, Serialize};
use serde_json::{json, Value};

use crate::{
    error::AppError,
    protocol::{IpcRequest, IpcResponse},
};

use super::{RemoteAccountManager, RemoteRole, DEFAULT_ADMIN_USERNAME};

/// LocalAdminService is exposed only through the Core stdio IPC owned by the
/// desktop/Web host process. Remote QUIC clients never receive this boundary.
#[derive(Clone)]
pub struct LocalAdminService {
    accounts: Arc<RemoteAccountManager>,
}

impl LocalAdminService {
    pub fn new(accounts: Arc<RemoteAccountManager>) -> Self {
        Self { accounts }
    }

    pub fn handle_request(&self, request: IpcRequest) -> IpcResponse {
        if request.method == "remote.admin.login" {
            return self.login(request);
        }

        let session = match self.authenticate_admin(&request.params) {
            Ok(session) => session,
            Err(error) => return IpcResponse::failure(Some(request.id), error),
        };
        if session.password_change_required
            && !matches!(
                request.method.as_str(),
                "remote.admin.me" | "remote.admin.logout" | "remote.admin.changePassword"
            )
        {
            return IpcResponse::failure(
                Some(request.id),
                AppError::new(
                    "REMOTE_AUTH_PASSWORD_CHANGE_REQUIRED",
                    "The administrator password must be changed before management is allowed.",
                    "remote.auth",
                    false,
                ),
            );
        }

        match request.method.as_str() {
            "remote.admin.me" => IpcResponse::success(
                request.id,
                json!({
                    "username": session.username,
                    "role": session.role,
                    "passwordChangeRequired": session.password_change_required,
                }),
            ),
            "remote.admin.logout" => match self.accounts.logout(&session.token) {
                Ok(()) => IpcResponse::success(request.id, json!({"loggedOut": true})),
                Err(error) => IpcResponse::failure(Some(request.id), error),
            },
            "remote.admin.changePassword" => self.change_password(request, &session.token),
            "remote.admin.users.list" => self.list_users(request.id),
            "remote.admin.users.create" => self.create_user(request),
            "remote.admin.users.delete" => self.delete_user(request),
            "remote.admin.users.resetPassword" => self.reset_password(request),
            "remote.admin.devices.assign" => self.assign_device(request),
            _ => IpcResponse::failure(
                Some(request.id),
                AppError::new(
                    "IPC_METHOD_NOT_FOUND",
                    "Unknown local remote-administration method.",
                    "ipc.protocol",
                    false,
                ),
            ),
        }
    }

    fn login(&self, request: IpcRequest) -> IpcResponse {
        #[derive(Deserialize)]
        struct Params {
            password: String,
        }

        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return IpcResponse::failure(Some(request.id), error),
        };
        match self
            .accounts
            .login(DEFAULT_ADMIN_USERNAME, &params.password)
        {
            Ok(session) => IpcResponse::success(
                request.id,
                json!({
                    "sessionToken": session.token,
                    "username": session.username,
                    "role": session.role,
                    "passwordChangeRequired": session.password_change_required,
                }),
            ),
            Err(error) => IpcResponse::failure(Some(request.id), error),
        }
    }

    fn authenticate_admin(&self, params: &Value) -> Result<super::AuthenticatedSession, AppError> {
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct Params {
            session_token: String,
        }

        let params: Params = parse_params(params.clone())?;
        let session = self.accounts.authenticate(&params.session_token)?;
        if session.role != RemoteRole::Admin || session.username != DEFAULT_ADMIN_USERNAME {
            return Err(AppError::new(
                "REMOTE_AUTH_FORBIDDEN",
                "Only the built-in administrator can use the local management interface.",
                "remote.auth",
                false,
            ));
        }
        Ok(session)
    }

    fn change_password(&self, request: IpcRequest, session_token: &str) -> IpcResponse {
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct Params {
            current_password: String,
            new_password: String,
        }

        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return IpcResponse::failure(Some(request.id), error),
        };
        let session = match self.accounts.authenticate(session_token) {
            Ok(session) => session,
            Err(error) => return IpcResponse::failure(Some(request.id), error),
        };
        match self.accounts.change_own_password(
            &session.username,
            &params.current_password,
            &params.new_password,
        ) {
            Ok(()) => IpcResponse::success(
                request.id,
                json!({"passwordChanged": true, "loginRequired": true}),
            ),
            Err(error) => IpcResponse::failure(Some(request.id), error),
        }
    }

    fn list_users(&self, id: String) -> IpcResponse {
        match self.accounts.list_accounts() {
            Ok(mut accounts) => {
                // The administrator is a frontend identity, not a remotely
                // manageable user, so it must not appear in this collection.
                accounts.retain(|account| account.role == RemoteRole::User);
                serialize_success(id, &accounts)
            }
            Err(error) => IpcResponse::failure(Some(id), error),
        }
    }

    fn create_user(&self, request: IpcRequest) -> IpcResponse {
        #[derive(Deserialize)]
        struct Params {
            username: String,
            password: String,
        }

        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return IpcResponse::failure(Some(request.id), error),
        };
        match self
            .accounts
            .create_user(&params.username, &params.password)
        {
            Ok(account) => serialize_success(request.id, &account),
            Err(error) => IpcResponse::failure(Some(request.id), error),
        }
    }

    fn delete_user(&self, request: IpcRequest) -> IpcResponse {
        #[derive(Deserialize)]
        struct Params {
            username: String,
        }

        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return IpcResponse::failure(Some(request.id), error),
        };
        if let Err(error) = require_remote_user(&params.username) {
            return IpcResponse::failure(Some(request.id), error);
        }
        match self.accounts.delete_user(&params.username) {
            Ok(()) => IpcResponse::success(request.id, json!({"deleted": true})),
            Err(error) => IpcResponse::failure(Some(request.id), error),
        }
    }

    fn reset_password(&self, request: IpcRequest) -> IpcResponse {
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct Params {
            username: String,
            new_password: String,
        }

        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return IpcResponse::failure(Some(request.id), error),
        };
        if let Err(error) = require_remote_user(&params.username) {
            return IpcResponse::failure(Some(request.id), error);
        }
        match self
            .accounts
            .reset_password(&params.username, &params.new_password)
        {
            Ok(()) => IpcResponse::success(
                request.id,
                json!({"passwordReset": true, "loginRequired": true}),
            ),
            Err(error) => IpcResponse::failure(Some(request.id), error),
        }
    }

    fn assign_device(&self, request: IpcRequest) -> IpcResponse {
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct Params {
            username: String,
            device_id: String,
            #[serde(default = "default_true")]
            assigned: bool,
        }

        let params: Params = match parse_params(request.params) {
            Ok(params) => params,
            Err(error) => return IpcResponse::failure(Some(request.id), error),
        };
        if let Err(error) = require_remote_user(&params.username) {
            return IpcResponse::failure(Some(request.id), error);
        }
        // The local Web adapter validates that a new assignment is online.
        // Removing a stale assignment remains possible after a device leaves.
        match self
            .accounts
            .set_device_access(&params.username, &params.device_id, params.assigned)
        {
            Ok(account) => serialize_success(request.id, &account),
            Err(error) => IpcResponse::failure(Some(request.id), error),
        }
    }
}

fn require_remote_user(username: &str) -> Result<(), AppError> {
    if username == DEFAULT_ADMIN_USERNAME {
        Err(AppError::new(
            "REMOTE_AUTH_BUILTIN_USER_IMMUTABLE",
            "The built-in administrator is managed only through the password settings flow.",
            "remote.auth",
            false,
        ))
    } else {
        Ok(())
    }
}

fn parse_params<T: DeserializeOwned>(params: Value) -> Result<T, AppError> {
    serde_json::from_value(params).map_err(|error| {
        AppError::new(
            "REMOTE_PROTOCOL_PARAMS_INVALID",
            "Local remote-administration parameters are invalid.",
            "remote.protocol",
            false,
        )
        .with_cause(error)
    })
}

fn serialize_success(id: String, value: &impl Serialize) -> IpcResponse {
    match serde_json::to_value(value) {
        Ok(value) => IpcResponse::success(id, value),
        Err(error) => IpcResponse::failure(
            Some(id),
            AppError::new(
                "REMOTE_PROTOCOL_ENCODE_FAILED",
                "Failed to encode local remote-administration response.",
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
    use super::*;

    fn setup() -> (LocalAdminService, std::path::PathBuf) {
        let path = std::env::temp_dir().join(format!(
            "adbcontrol-local-admin-{}.json",
            rand::random::<u64>()
        ));
        let accounts = Arc::new(RemoteAccountManager::load_or_initialize(&path).unwrap());
        (LocalAdminService::new(accounts), path)
    }

    fn request(id: &str, method: &str, params: Value) -> IpcRequest {
        IpcRequest {
            id: id.to_string(),
            method: method.to_string(),
            params,
        }
    }

    fn login(service: &LocalAdminService, password: &str) -> (String, bool) {
        let response = service.handle_request(request(
            "login",
            "remote.admin.login",
            json!({"password": password}),
        ));
        let result = response.result.unwrap();
        (
            result["sessionToken"].as_str().unwrap().to_string(),
            result["passwordChangeRequired"].as_bool().unwrap(),
        )
    }

    #[test]
    fn local_admin_password_is_the_core_builtin_admin_password() {
        // Scenario: the frontend login must authenticate the Core administrator,
        // not a second password database owned by the Web service.
        let (service, path) = setup();
        let (token, required) = login(&service, "admin");
        assert!(!token.is_empty());
        assert!(required);

        let changed = service.handle_request(request(
            "change",
            "remote.admin.changePassword",
            json!({
                "sessionToken": token,
                "currentPassword": "admin",
                "newPassword": "secure-admin-password"
            }),
        ));
        assert!(changed.ok);
        assert!(service
            .handle_request(request(
                "old-login",
                "remote.admin.login",
                json!({"password": "admin"}),
            ))
            .error
            .is_some());
        assert!(!login(&service, "secure-admin-password").1);
        std::fs::remove_file(path).ok();
    }

    #[test]
    fn local_admin_manages_only_remote_users() {
        // Scenario: the settings page can create and assign remote users, while
        // the built-in administrator never appears as a remote user.
        let (service, path) = setup();
        let (token, _) = login(&service, "admin");
        service.handle_request(request(
            "change",
            "remote.admin.changePassword",
            json!({
                "sessionToken": token,
                "currentPassword": "admin",
                "newPassword": "secure-admin-password"
            }),
        ));
        let (token, _) = login(&service, "secure-admin-password");

        let created = service.handle_request(request(
            "create",
            "remote.admin.users.create",
            json!({
                "sessionToken": token,
                "username": "operator",
                "password": "operator-password"
            }),
        ));
        assert!(created.ok);
        let listed = service.handle_request(request(
            "list",
            "remote.admin.users.list",
            json!({"sessionToken": token}),
        ));
        let users = listed.result.unwrap().as_array().unwrap().clone();
        assert_eq!(users.len(), 1);
        assert_eq!(users[0]["username"], "operator");

        let denied = service.handle_request(request(
            "reset-admin",
            "remote.admin.users.resetPassword",
            json!({
                "sessionToken": token,
                "username": "admin",
                "newPassword": "replacement-password"
            }),
        ));
        assert_eq!(
            denied.error.unwrap().error_code,
            "REMOTE_AUTH_BUILTIN_USER_IMMUTABLE"
        );
        std::fs::remove_file(path).ok();
    }
}
