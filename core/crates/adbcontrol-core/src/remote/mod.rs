mod auth;
mod local_admin;
#[cfg(feature = "quinn-transport")]
mod quinn_transport;
mod service;

pub use auth::{
    AuthenticatedSession, RemoteAccount, RemoteAccountManager, RemoteRole, DEFAULT_ADMIN_PASSWORD,
    DEFAULT_ADMIN_USERNAME,
};
pub use local_admin::LocalAdminService;
#[cfg(feature = "quinn-transport")]
pub use quinn_transport::QuinnRemoteControlServer;
pub use service::{
    CoreRequestHandler, RemoteControlService, RemoteRequest, RemoteResponse, REMOTE_PROTOCOL,
    REMOTE_PROTOCOL_VERSION,
};
