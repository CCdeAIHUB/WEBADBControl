#![allow(clippy::result_large_err)]

pub mod adb;
pub mod assets;
pub mod capability;
pub mod companion;
pub mod diagnostics;
pub mod error;
pub mod ipc;
pub mod keepalive;
pub mod platform;
pub mod protocol;
pub mod remote;
pub mod wireless;

pub use adb::{
    AdbCommandOutput, AdbProcessManager, AdbRunner, ProcessAdbRunner, ScrcpyServerConfig,
    ScrcpyServerProcess,
};
pub use assets::{find_adb_asset, load_embedded_manifest, AdbAsset, AdbManifest};
pub use capability::{
    android_companion_capability_catalog, Capability, CapabilityPermissionRequirement,
    CapabilityPermissionState, CapabilityProvider, CapabilitySensitivity, CapabilityTransport,
};
pub use companion::trust::{CompanionTrustStore, PairingChallenge, PairingRequest, TrustedDevice};
#[cfg(feature = "quinn-transport")]
pub use companion::QuinnCompanionServer;
pub use companion::{
    build_command_request_envelope, quic_protocol_descriptor, sample_android_companion_device,
    validate_quic_envelope, CompanionCommandDispatch, CompanionCommandResponse,
    CompanionCommandRouter, CompanionCommandStatus, CompanionCommandTransport, CompanionDevice,
    CompanionIngress, CompanionQuicListener, CompanionRegistry, CompanionSession,
    CompanionSessionManager, ConnectionState, DisconnectedCompanionCommandRouter,
    InMemoryCompanionCommandRouter, InMemoryCompanionSession, IngressBackedQuicListener,
    QuicEnvelope, QuicMessageKind, TransportCompanionCommandRouter, COMPANION_PROTOCOL,
    COMPANION_PROTOCOL_VERSION,
};
pub use error::AppError;
pub use keepalive::{
    keepalive_check, spawn_adb_keepalive, AdbKeepAliveConfig, AdbKeepAliveHandle,
    AdbKeepAliveStatus, ADB_KEEPALIVE_MIN_INTERVAL_SECS,
};
pub use platform::HostTarget;
pub use protocol::{CoreService, IpcRequest, IpcResponse};
#[cfg(feature = "quinn-transport")]
pub use remote::QuinnRemoteControlServer;
pub use remote::{
    LocalAdminService, RemoteAccount, RemoteAccountManager, RemoteControlService, RemoteRequest,
    RemoteResponse, RemoteRole, DEFAULT_ADMIN_PASSWORD, DEFAULT_ADMIN_USERNAME, REMOTE_PROTOCOL,
    REMOTE_PROTOCOL_VERSION,
};
pub use wireless::{WirelessPairingManager, WirelessPairingQr, WirelessPairingResult};
