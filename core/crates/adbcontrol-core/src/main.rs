#![allow(clippy::result_large_err)]

use std::{
    io::{self, BufReader},
    sync::Arc,
};

use adbcontrol_core::{
    ipc::stdio::serve_json_lines, keepalive::spawn_adb_keepalive, keepalive::AdbKeepAliveConfig,
    load_embedded_manifest, CoreService, LocalAdminService, ProcessAdbRunner, RemoteAccountManager,
};

fn main() {
    adbcontrol_core::diagnostics::initialize();
    let manifest = match load_embedded_manifest() {
        Ok(manifest) => manifest,
        Err(error) => {
            eprintln!("{error}");
            adbcontrol_core::diagnostics::record(
                "startup.failed",
                "manifest",
                "startup",
                false,
                0,
                Some(&error.error_code),
            );
            adbcontrol_core::diagnostics::flush();
            std::process::exit(1);
        }
    };

    // ADB 保活：后台线程周期执行 adb start-server，防止 ADB server 掉线后无人恢复。
    let keepalive = match spawn_adb_keepalive(
        ProcessAdbRunner,
        manifest.clone(),
        AdbKeepAliveConfig::default(),
    ) {
        Ok(handle) => Some(handle),
        Err(error) => {
            eprintln!("ADB keep-alive supervisor failed to start: {error}");
            None
        }
    };

    let remote_data_dir = remote_data_dir();
    let remote_accounts =
        match RemoteAccountManager::load_or_initialize(remote_data_dir.join("accounts.json")) {
            Ok(accounts) => Arc::new(accounts),
            Err(error) => {
                eprintln!("{error}");
                std::process::exit(1);
            }
        };

    #[cfg(feature = "quinn-transport")]
    let (companion_registry, companion_router, companion_connection_info) =
        match start_companion_control_if_configured(&remote_data_dir) {
            Ok(runtime) => runtime,
            Err(error) => {
                eprintln!("{error}");
                std::process::exit(1);
            }
        };

    #[cfg(feature = "quinn-transport")]
    let mut service = CoreService::new_with_companion_registry_and_router(
        ProcessAdbRunner,
        manifest,
        companion_registry,
        companion_router,
    )
    .with_local_admin(LocalAdminService::new(Arc::clone(&remote_accounts)));

    #[cfg(not(feature = "quinn-transport"))]
    let mut service = CoreService::new(ProcessAdbRunner, manifest)
        .with_local_admin(LocalAdminService::new(Arc::clone(&remote_accounts)));

    #[cfg(feature = "quinn-transport")]
    if let Some(info) = companion_connection_info {
        service = service.with_companion_connection_info(info);
    }
    if let Some(handle) = keepalive {
        service = service.with_keepalive(handle);
    }

    let service = Arc::new(service);

    #[cfg(feature = "quinn-transport")]
    if let Err(error) = start_remote_control_if_configured(
        Arc::clone(&service),
        Arc::clone(&remote_accounts),
        remote_data_dir,
    ) {
        eprintln!("{error}");
        std::process::exit(1);
    }

    let stdin = io::stdin();
    let stdout = io::stdout();

    if let Err(error) = serve_json_lines(
        service.as_ref(),
        BufReader::new(stdin.lock()),
        stdout.lock(),
    ) {
        eprintln!("{error}");
        adbcontrol_core::diagnostics::record(
            "exit",
            "process",
            "shutdown",
            false,
            0,
            Some(&error.error_code),
        );
        adbcontrol_core::diagnostics::flush();
        std::process::exit(1);
    }
    adbcontrol_core::diagnostics::record("exit", "process", "shutdown", true, 0, None);
    adbcontrol_core::diagnostics::flush();
}

fn remote_data_dir() -> std::path::PathBuf {
    std::env::var_os("ADBCONTROL_REMOTE_DATA_DIR")
        .map(std::path::PathBuf::from)
        .or_else(|| {
            std::env::var_os("LOCALAPPDATA").map(|root| {
                std::path::PathBuf::from(root)
                    .join("ADBControl")
                    .join("remote")
            })
        })
        .unwrap_or_else(|| std::path::PathBuf::from(".adbcontrol").join("remote"))
}

#[cfg(feature = "quinn-transport")]
fn start_companion_control_if_configured(
    data_dir: &std::path::Path,
) -> Result<
    (
        adbcontrol_core::CompanionRegistry,
        adbcontrol_core::TransportCompanionCommandRouter<adbcontrol_core::LiveCompanionTransport>,
        Option<adbcontrol_core::CompanionConnectionInfo>,
    ),
    adbcontrol_core::AppError,
> {
    use std::{env, net::SocketAddr};

    use adbcontrol_core::{
        companion::CoreQuicIdentity, CompanionConnectionInfo, CompanionRegistry,
        LiveCompanionTransport, QuinnCompanionServer, TransportCompanionCommandRouter,
    };
    use base64::Engine;

    let registry = CompanionRegistry::default();
    let transport = LiveCompanionTransport::new(registry.clone());
    let router = TransportCompanionCommandRouter::new(transport.clone());
    let listen_value = match env::var("ADBCONTROL_COMPANION_LISTEN") {
        Ok(value) if !value.trim().is_empty() => value,
        _ => return Ok((registry, router, None)),
    };
    let listen_addr: SocketAddr = listen_value.parse().map_err(|error| {
        adbcontrol_core::AppError::new(
            "COMPANION_QUIC_LISTEN_ADDRESS_INVALID",
            "ADBCONTROL_COMPANION_LISTEN must be an IP socket address such as 0.0.0.0:45922.",
            "companion.startup",
            false,
        )
        .with_cause(error)
    })?;
    let server_name = String::from("adbcontrol-core");
    let identity = CoreQuicIdentity::load_or_generate(
        data_dir.join("companion.cert.der"),
        data_dir.join("companion.key.der"),
        vec![server_name.clone(), String::from("localhost")],
    )?;
    let actual_addr =
        QuinnCompanionServer::spawn(listen_addr, identity.companion_server_config()?, transport)?;
    let info = CompanionConnectionInfo {
        listen_address: actual_addr.to_string(),
        server_name,
        certificate_der_base64: base64::engine::general_purpose::STANDARD
            .encode(&identity.cert_der),
        certificate_fingerprint_sha256: identity.certificate_fingerprint_sha256.clone(),
    };
    eprintln!(
        "Companion QUIC listening on {actual_addr}; certificate SHA-256: {}",
        identity.certificate_fingerprint_sha256
    );
    Ok((registry, router, Some(info)))
}

#[cfg(feature = "quinn-transport")]
fn start_remote_control_if_configured<Q>(
    core: Arc<CoreService<ProcessAdbRunner, Q>>,
    accounts: Arc<RemoteAccountManager>,
    data_dir: std::path::PathBuf,
) -> Result<(), adbcontrol_core::AppError>
where
    Q: adbcontrol_core::CompanionCommandRouter + 'static,
{
    use std::{env, net::SocketAddr};

    use adbcontrol_core::{
        companion::CoreQuicIdentity, QuinnRemoteControlServer, RemoteControlService,
    };

    let listen_value = match env::var("ADBCONTROL_REMOTE_LISTEN") {
        Ok(value) if !value.trim().is_empty() => value,
        _ => return Ok(()),
    };
    let listen_addr: SocketAddr = listen_value.parse().map_err(|error| {
        adbcontrol_core::AppError::new(
            "REMOTE_QUIC_LISTEN_ADDRESS_INVALID",
            "ADBCONTROL_REMOTE_LISTEN must be an IP socket address such as 0.0.0.0:45921.",
            "remote.startup",
            false,
        )
        .with_cause(error)
    })?;

    let identity = CoreQuicIdentity::load_or_generate(
        data_dir.join("remote.cert.der"),
        data_dir.join("remote.key.der"),
        vec![String::from("localhost"), listen_addr.ip().to_string()],
    )?;
    let fingerprint = identity.certificate_fingerprint_sha256.clone();
    let actual_addr = QuinnRemoteControlServer::spawn(
        listen_addr,
        identity.server_config()?,
        RemoteControlService::new(core, accounts),
    )?;

    eprintln!(
        "Encrypted remote control listening on {actual_addr}; certificate SHA-256: {fingerprint}"
    );
    Ok(())
}
