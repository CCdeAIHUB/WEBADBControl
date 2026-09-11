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

    let mut service = CoreService::new(ProcessAdbRunner, manifest)
        .with_local_admin(LocalAdminService::new(Arc::clone(&remote_accounts)));
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
fn start_remote_control_if_configured(
    core: Arc<CoreService<ProcessAdbRunner>>,
    accounts: Arc<RemoteAccountManager>,
    data_dir: std::path::PathBuf,
) -> Result<(), adbcontrol_core::AppError> {
    use std::{env, net::SocketAddr, thread};

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
    let server = QuinnRemoteControlServer::bind(
        listen_addr,
        identity.server_config()?,
        RemoteControlService::new(core, accounts),
    )?;
    let actual_addr = server.local_addr()?;

    thread::Builder::new()
        .name(String::from("adbcontrol-remote-quic"))
        .spawn(move || {
            let runtime = tokio::runtime::Builder::new_current_thread()
                .enable_all()
                .build();
            match runtime {
                Ok(runtime) => {
                    if let Err(error) = runtime.block_on(server.run()) {
                        eprintln!("{error}");
                    }
                }
                Err(error) => eprintln!("Failed to start remote QUIC runtime: {error}"),
            }
        })
        .map_err(|error| {
            adbcontrol_core::AppError::new(
                "REMOTE_QUIC_THREAD_START_FAILED",
                "Failed to start the remote-control QUIC thread.",
                "remote.startup",
                true,
            )
            .with_cause(error)
        })?;

    eprintln!(
        "Encrypted remote control listening on {actual_addr}; certificate SHA-256: {fingerprint}"
    );
    Ok(())
}
