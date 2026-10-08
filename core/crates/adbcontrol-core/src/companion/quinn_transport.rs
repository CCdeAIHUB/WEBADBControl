use std::{
    collections::HashMap,
    net::SocketAddr,
    sync::{mpsc, Arc, Mutex},
    time::Duration,
};

use serde_json::{json, Value};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    sync::mpsc as tokio_mpsc,
};

use crate::error::AppError;

use super::{
    protocol::{
        validate_quic_envelope, QuicChannel, QuicEnvelope, QuicMessageKind, COMPANION_PROTOCOL,
        COMPANION_PROTOCOL_VERSION,
    },
    CompanionCommandTransport, CompanionIngress, CompanionRegistry, CompanionSessionManager,
};

const WIRE_MAGIC: &[u8; 4] = b"ACQ1";
const STREAM_CONTROL: u8 = 1;
#[cfg(test)]
const STREAM_VIDEO: u8 = 2;
const MAX_CONTROL_MESSAGE_BYTES: usize = 4 * 1024 * 1024;
const COMMAND_TIMEOUT: Duration = Duration::from_secs(15);

#[derive(Clone)]
pub struct LiveCompanionTransport {
    state: Arc<LiveState>,
}

struct LiveState {
    registry: CompanionRegistry,
    ingress: Mutex<CompanionIngress>,
    sessions: Mutex<HashMap<String, SessionHandle>>,
    pending: Mutex<HashMap<String, mpsc::SyncSender<QuicEnvelope>>>,
}

#[derive(Clone)]
struct SessionHandle {
    connection_id: u64,
    outgoing: tokio_mpsc::UnboundedSender<Vec<u8>>,
}

impl LiveCompanionTransport {
    pub fn new(registry: CompanionRegistry) -> Self {
        Self {
            state: Arc::new(LiveState {
                registry,
                ingress: Mutex::new(CompanionIngress::new(CompanionSessionManager::default())),
                sessions: Mutex::new(HashMap::new()),
                pending: Mutex::new(HashMap::new()),
            }),
        }
    }

    pub fn registry(&self) -> CompanionRegistry {
        self.state.registry.clone()
    }

    fn accept_envelope(
        &self,
        connection_id: u64,
        outgoing: &tokio_mpsc::UnboundedSender<Vec<u8>>,
        envelope: QuicEnvelope,
    ) -> Result<Option<String>, AppError> {
        validate_quic_envelope(&envelope)?;
        if matches!(
            envelope.kind,
            QuicMessageKind::CommandResponse | QuicMessageKind::Error
        ) {
            let request_id = envelope
                .payload
                .get("requestId")
                .and_then(Value::as_str)
                .or(envelope.trace_id.as_deref())
                .unwrap_or_default()
                .to_owned();
            if let Some(waiter) = self
                .state
                .pending
                .lock()
                .expect("companion pending lock should not be poisoned")
                .remove(&request_id)
            {
                let _ = waiter.send(envelope);
            }
            return Ok(None);
        }

        let is_hello = envelope.kind == QuicMessageKind::Hello;
        let is_heartbeat = envelope.kind == QuicMessageKind::Heartbeat;
        let envelope_device_id = envelope.device_id.clone();
        let (response, devices) = {
            let mut ingress = self
                .state
                .ingress
                .lock()
                .expect("companion ingress lock should not be poisoned");
            let response = ingress.handle_envelope(envelope)?;
            let devices = ingress.session_manager().devices();
            (response, devices)
        };
        for device in devices {
            self.state.registry.upsert(device);
        }
        if is_hello {
            if let Some(device_id) = envelope_device_id.as_ref() {
                self.state
                    .sessions
                    .lock()
                    .expect("companion sessions lock should not be poisoned")
                    .insert(
                        device_id.clone(),
                        SessionHandle {
                            connection_id,
                            outgoing: outgoing.clone(),
                        },
                    );
            }
        }
        // Android replies to Core heartbeats. Sending the session manager's
        // heartbeat response back would create an unbounded ping-pong loop.
        if is_heartbeat {
            return Ok(envelope_device_id);
        }
        let encoded = serde_json::to_vec(&response).map_err(|error| {
            AppError::new(
                "COMPANION_QUIC_RESPONSE_SERIALIZE_FAILED",
                "Core failed to serialize a Companion QUIC response.",
                "companion.quinn",
                true,
            )
            .with_cause(error)
        })?;
        outgoing.send(encoded).map_err(|_| {
            AppError::new(
                "COMPANION_QUIC_CONTROL_CLOSED",
                "Companion QUIC control stream closed before Core could reply.",
                "companion.quinn",
                true,
            )
        })?;
        Ok(envelope_device_id)
    }

    fn disconnect(&self, connection_id: u64, device_id: Option<&str>) {
        let Some(device_id) = device_id else { return };
        let mut sessions = self
            .state
            .sessions
            .lock()
            .expect("companion sessions lock should not be poisoned");
        let belongs_to_connection = sessions
            .get(device_id)
            .is_some_and(|session| session.connection_id == connection_id);
        if belongs_to_connection {
            sessions.remove(device_id);
            self.state.registry.remove(device_id);
        }
    }

    fn disconnect_connection(&self, connection_id: u64) {
        let device_ids = self
            .state
            .sessions
            .lock()
            .expect("companion sessions lock should not be poisoned")
            .iter()
            .filter(|(_, session)| session.connection_id == connection_id)
            .map(|(device_id, _)| device_id.clone())
            .collect::<Vec<_>>();
        for device_id in device_ids {
            self.disconnect(connection_id, Some(&device_id));
        }
    }
}

impl CompanionCommandTransport for LiveCompanionTransport {
    fn send_command_envelope(
        &self,
        device_id: &str,
        envelope: &QuicEnvelope,
    ) -> Result<Value, AppError> {
        let session = self
            .state
            .sessions
            .lock()
            .expect("companion sessions lock should not be poisoned")
            .get(device_id)
            .cloned()
            .ok_or_else(|| {
                AppError::new(
                    "COMPANION_SESSION_NOT_CONNECTED",
                    format!(
                        "Android companion QUIC session is not connected for device {device_id}."
                    ),
                    "companion.quinn",
                    true,
                )
            })?;
        let request_id = envelope
            .payload
            .get("requestId")
            .and_then(Value::as_str)
            .unwrap_or(&envelope.message_id)
            .to_owned();
        let bytes = serde_json::to_vec(envelope).map_err(|error| {
            AppError::new(
                "COMPANION_COMMAND_SERIALIZE_FAILED",
                "Core failed to serialize the Companion command envelope.",
                "companion.quinn",
                false,
            )
            .with_cause(error)
        })?;
        let (response_sender, response_receiver) = mpsc::sync_channel(1);
        self.state
            .pending
            .lock()
            .expect("companion pending lock should not be poisoned")
            .insert(request_id.clone(), response_sender);
        if session.outgoing.send(bytes).is_err() {
            self.state
                .pending
                .lock()
                .expect("companion pending lock should not be poisoned")
                .remove(&request_id);
            return Err(AppError::new(
                "COMPANION_COMMAND_SEND_FAILED",
                "Companion QUIC control stream closed before the command was sent.",
                "companion.quinn",
                true,
            ));
        }
        let response = response_receiver
            .recv_timeout(COMMAND_TIMEOUT)
            .map_err(|error| {
                self.state
                    .pending
                    .lock()
                    .expect("companion pending lock should not be poisoned")
                    .remove(&request_id);
                AppError::new(
                    "COMPANION_COMMAND_TIMEOUT",
                    "Companion did not return a QUIC command response within 15 seconds.",
                    "companion.quinn",
                    true,
                )
                .with_cause(error)
            })?;
        if response.kind == QuicMessageKind::Error
            || response.payload.get("ok").and_then(Value::as_bool) == Some(false)
        {
            let detail = response.payload.get("error").unwrap_or(&response.payload);
            return Err(AppError::new(
                detail
                    .get("errorCode")
                    .and_then(Value::as_str)
                    .unwrap_or("COMPANION_COMMAND_FAILED"),
                detail
                    .get("message")
                    .and_then(Value::as_str)
                    .unwrap_or("Android companion command failed."),
                detail
                    .get("module")
                    .and_then(Value::as_str)
                    .unwrap_or("companion.command"),
                detail
                    .get("recoverable")
                    .and_then(Value::as_bool)
                    .unwrap_or(true),
            ));
        }
        Ok(response.payload)
    }
}

pub struct QuinnCompanionServer {
    endpoint: quinn::Endpoint,
    transport: LiveCompanionTransport,
}

impl QuinnCompanionServer {
    pub fn spawn(
        listen_addr: SocketAddr,
        server_config: quinn::ServerConfig,
        transport: LiveCompanionTransport,
    ) -> Result<SocketAddr, AppError> {
        let (startup_sender, startup_receiver) = mpsc::sync_channel(1);
        std::thread::Builder::new()
            .name(String::from("adbcontrol-companion-quic"))
            .spawn(move || {
                let runtime = match tokio::runtime::Builder::new_current_thread()
                    .enable_all()
                    .build()
                {
                    Ok(runtime) => runtime,
                    Err(error) => {
                        let _ = startup_sender.send(Err(AppError::new(
                            "COMPANION_QUIC_RUNTIME_FAILED",
                            "Core failed to create the Companion QUIC runtime.",
                            "companion.quinn",
                            false,
                        )
                        .with_cause(error)));
                        return;
                    }
                };
                runtime.block_on(async move {
                    let server = match Self::bind(listen_addr, server_config, transport) {
                        Ok(server) => server,
                        Err(error) => {
                            let _ = startup_sender.send(Err(error));
                            return;
                        }
                    };
                    let address = server.local_addr();
                    if startup_sender.send(address).is_err() {
                        return;
                    }
                    if let Err(error) = server.run().await {
                        eprintln!("Companion QUIC server stopped: {error}");
                    }
                });
            })
            .map_err(|error| {
                AppError::new(
                    "COMPANION_QUIC_THREAD_START_FAILED",
                    "Core failed to start the Companion QUIC thread.",
                    "companion.quinn",
                    false,
                )
                .with_cause(error)
            })?;
        startup_receiver.recv().map_err(|error| {
            AppError::new(
                "COMPANION_QUIC_STARTUP_CHANNEL_FAILED",
                "Core did not report Companion QUIC startup status.",
                "companion.quinn",
                false,
            )
            .with_cause(error)
        })?
    }

    pub fn bind(
        listen_addr: SocketAddr,
        server_config: quinn::ServerConfig,
        transport: LiveCompanionTransport,
    ) -> Result<Self, AppError> {
        let endpoint = quinn::Endpoint::server(server_config, listen_addr).map_err(|error| {
            AppError::new(
                "COMPANION_QUIC_BIND_FAILED",
                "Core failed to bind the Companion QUIC listener.",
                "companion.quinn",
                true,
            )
            .with_cause(error)
        })?;
        Ok(Self {
            endpoint,
            transport,
        })
    }

    pub fn local_addr(&self) -> Result<SocketAddr, AppError> {
        self.endpoint.local_addr().map_err(|error| {
            AppError::new(
                "COMPANION_QUIC_LOCAL_ADDR_FAILED",
                "Core failed to read the Companion QUIC listener address.",
                "companion.quinn",
                true,
            )
            .with_cause(error)
        })
    }

    pub async fn run(self) -> Result<(), AppError> {
        let mut next_connection_id = 1_u64;
        while let Some(incoming) = self.endpoint.accept().await {
            let transport = self.transport.clone();
            let connection_id = next_connection_id;
            next_connection_id = next_connection_id.wrapping_add(1);
            tokio::spawn(async move {
                if let Err(error) = handle_connection(incoming, connection_id, transport).await {
                    eprintln!("Companion QUIC connection failed: {error}");
                }
            });
        }
        Ok(())
    }
}

async fn handle_connection(
    incoming: quinn::Incoming,
    connection_id: u64,
    transport: LiveCompanionTransport,
) -> Result<(), AppError> {
    let connection = incoming.await.map_err(|error| {
        AppError::new(
            "COMPANION_QUIC_HANDSHAKE_FAILED",
            "Core failed to complete the Companion QUIC handshake.",
            "companion.quinn",
            true,
        )
        .with_cause(error)
    })?;
    let (send, recv) = connection.accept_bi().await.map_err(|error| {
        AppError::new(
            "COMPANION_QUIC_CONTROL_ACCEPT_FAILED",
            "Core failed to accept the Companion control stream.",
            "companion.quinn",
            true,
        )
        .with_cause(error)
    })?;
    let result = handle_control_stream(send, recv, connection_id, transport.clone()).await;
    // Every exit path, including malformed frames and socket errors, must remove
    // the live registry entry. Otherwise Web/remote clients would see a stale
    // online Companion after Android has already started reconnecting.
    transport.disconnect_connection(connection_id);
    result
}

async fn handle_control_stream(
    mut send: quinn::SendStream,
    mut recv: quinn::RecvStream,
    connection_id: u64,
    transport: LiveCompanionTransport,
) -> Result<(), AppError> {
    let mut preface = [0_u8; 5];
    recv.read_exact(&mut preface).await.map_err(|error| {
        AppError::new(
            "COMPANION_QUIC_PREFACE_READ_FAILED",
            "Core could not read the Companion QUIC stream preface.",
            "companion.quinn",
            true,
        )
        .with_cause(error)
    })?;
    if &preface[..4] != WIRE_MAGIC || preface[4] != STREAM_CONTROL {
        return Err(AppError::new(
            "COMPANION_QUIC_PREFACE_INVALID",
            "Companion QUIC control stream preface is invalid.",
            "companion.quinn",
            false,
        ));
    }
    let (outgoing, mut outgoing_receiver) = tokio_mpsc::unbounded_channel::<Vec<u8>>();
    let writer = tokio::spawn(async move {
        while let Some(bytes) = outgoing_receiver.recv().await {
            write_control_frame(&mut send, &bytes).await?;
        }
        Ok::<(), AppError>(())
    });
    let mut device_id: Option<String> = None;
    let start = tokio::time::Instant::now() + Duration::from_secs(10);
    let mut heartbeat = tokio::time::interval_at(start, Duration::from_secs(10));
    loop {
        let bytes = tokio::select! {
            result = read_control_frame(&mut recv) => match result {
                Ok(bytes) => bytes,
                Err(error) if error.error_code == "COMPANION_QUIC_CONTROL_CLOSED" => break,
                Err(error) => return Err(error),
            },
            _ = heartbeat.tick(), if device_id.is_some() => {
                let target = device_id.as_ref().expect("heartbeat guard requires device id");
                let timestamp = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .unwrap_or_default()
                    .as_millis();
                let envelope = QuicEnvelope {
                    protocol: String::from(COMPANION_PROTOCOL),
                    version: COMPANION_PROTOCOL_VERSION,
                    message_id: format!("core-heartbeat-{timestamp}"),
                    trace_id: Some(format!("heartbeat-{connection_id}")),
                    device_id: Some(target.clone()),
                    channel: QuicChannel::Control,
                    kind: QuicMessageKind::Heartbeat,
                    payload: json!({"timestampUnixMs": timestamp}),
                };
                let encoded = serde_json::to_vec(&envelope).map_err(|error| {
                    AppError::new(
                        "COMPANION_QUIC_HEARTBEAT_SERIALIZE_FAILED",
                        "Core failed to serialize a Companion heartbeat.",
                        "companion.quinn",
                        true,
                    ).with_cause(error)
                })?;
                outgoing.send(encoded).map_err(|_| {
                    AppError::new(
                        "COMPANION_QUIC_CONTROL_CLOSED",
                        "Companion QUIC control stream closed before heartbeat delivery.",
                        "companion.quinn",
                        true,
                    )
                })?;
                continue;
            }
        };
        let envelope: QuicEnvelope = serde_json::from_slice(&bytes).map_err(|error| {
            AppError::new(
                "COMPANION_QUIC_ENVELOPE_INVALID",
                "Companion QUIC control frame is not a valid envelope.",
                "companion.quinn",
                false,
            )
            .with_cause(error)
        })?;
        if let Some(accepted_device_id) =
            transport.accept_envelope(connection_id, &outgoing, envelope)?
        {
            device_id = Some(accepted_device_id);
        }
    }
    transport.disconnect(connection_id, device_id.as_deref());
    drop(outgoing);
    let _ = writer.await;
    Ok(())
}

async fn read_control_frame(recv: &mut quinn::RecvStream) -> Result<Vec<u8>, AppError> {
    let length = recv.read_u32().await.map_err(|error| {
        AppError::new(
            "COMPANION_QUIC_CONTROL_CLOSED",
            "Companion QUIC control stream was closed.",
            "companion.quinn",
            true,
        )
        .with_cause(error)
    })? as usize;
    if length == 0 || length > MAX_CONTROL_MESSAGE_BYTES {
        return Err(AppError::new(
            "COMPANION_QUIC_FRAME_LENGTH_INVALID",
            "Companion QUIC control frame length is outside the allowed range.",
            "companion.quinn",
            false,
        ));
    }
    let mut bytes = vec![0_u8; length];
    recv.read_exact(&mut bytes).await.map_err(|error| {
        AppError::new(
            "COMPANION_QUIC_FRAME_READ_FAILED",
            "Core failed to read a complete Companion QUIC control frame.",
            "companion.quinn",
            true,
        )
        .with_cause(error)
    })?;
    Ok(bytes)
}

async fn write_control_frame(send: &mut quinn::SendStream, bytes: &[u8]) -> Result<(), AppError> {
    if bytes.is_empty() || bytes.len() > MAX_CONTROL_MESSAGE_BYTES {
        return Err(AppError::new(
            "COMPANION_QUIC_FRAME_LENGTH_INVALID",
            "Core refused to write an invalid Companion QUIC frame length.",
            "companion.quinn",
            false,
        ));
    }
    send.write_u32(bytes.len() as u32).await.map_err(|error| {
        AppError::new(
            "COMPANION_QUIC_FRAME_WRITE_FAILED",
            "Core failed to write the Companion QUIC frame length.",
            "companion.quinn",
            true,
        )
        .with_cause(error)
    })?;
    send.write_all(bytes).await.map_err(|error| {
        AppError::new(
            "COMPANION_QUIC_FRAME_WRITE_FAILED",
            "Core failed to write the Companion QUIC frame payload.",
            "companion.quinn",
            true,
        )
        .with_cause(error)
    })?;
    send.flush().await.map_err(|error| {
        AppError::new(
            "COMPANION_QUIC_FRAME_FLUSH_FAILED",
            "Core failed to flush the Companion QUIC control frame.",
            "companion.quinn",
            true,
        )
        .with_cause(error)
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::companion::{CompanionHello, CoreQuicIdentity, COMPANION_ALPN};
    use quinn::crypto::rustls::QuicClientConfig;
    use rustls::{pki_types::CertificateDer, RootCertStore};

    #[test]
    fn wire_constants_match_android_companion() {
        // 场景：Core 与 Android 必须共享 ACQ1 长连接分帧前缀，不能回退到读 EOF 的单请求流。
        assert_eq!(WIRE_MAGIC, b"ACQ1");
        assert_eq!(STREAM_CONTROL, 1);
        assert_eq!(STREAM_VIDEO, 2);
        assert_eq!(MAX_CONTROL_MESSAGE_BYTES, 4 * 1024 * 1024);
    }

    #[test]
    fn live_transport_registry_is_shared_with_ipc() {
        // 场景：网络 transport 和 IPC 必须持有同一个可变 registry，而不是启动时快照。
        let registry = CompanionRegistry::default();
        let transport = LiveCompanionTransport::new(registry.clone());
        assert_eq!(transport.registry().list_devices(), registry.list_devices());
    }

    #[test]
    fn android_heartbeat_reply_does_not_create_ping_pong() {
        // 场景：Android 回复 Core 心跳后，Core 只更新存活状态，不能再回一个 heartbeat 触发无限互答。
        let transport = LiveCompanionTransport::new(CompanionRegistry::default());
        let (outgoing, mut receiver) = tokio_mpsc::unbounded_channel();
        let hello = QuicEnvelope {
            protocol: String::from(COMPANION_PROTOCOL),
            version: COMPANION_PROTOCOL_VERSION,
            message_id: String::from("hello-device-1"),
            trace_id: None,
            device_id: Some(String::from("device-1")),
            channel: QuicChannel::Control,
            kind: QuicMessageKind::Hello,
            payload: serde_json::to_value(CompanionHello {
                app_version: String::from("0.13.0"),
                device_id: String::from("device-1"),
                device_name: String::from("Test phone"),
                android_sdk: 35,
                supported_protocol_versions: vec![COMPANION_PROTOCOL_VERSION],
            })
            .unwrap(),
        };
        transport.accept_envelope(1, &outgoing, hello).unwrap();
        receiver.try_recv().expect("helloAck should be sent");

        let heartbeat = QuicEnvelope {
            protocol: String::from(COMPANION_PROTOCOL),
            version: COMPANION_PROTOCOL_VERSION,
            message_id: String::from("heartbeat-device-1"),
            trace_id: None,
            device_id: Some(String::from("device-1")),
            channel: QuicChannel::Control,
            kind: QuicMessageKind::Heartbeat,
            payload: json!({"timestampUnixMs": 1}),
        };
        transport.accept_envelope(1, &outgoing, heartbeat).unwrap();

        assert!(matches!(
            receiver.try_recv(),
            Err(tokio_mpsc::error::TryRecvError::Empty)
        ));
    }

    #[tokio::test]
    async fn android_wire_hello_roundtrips_over_pinned_quic() {
        // 场景：Android 使用已发布的 ALPN + ACQ1 + 长度帧连接时，Core 必须在同一长连接返回 helloAck 并实时注册设备。
        let identity =
            CoreQuicIdentity::generate_self_signed(vec![String::from("adbcontrol-core")])
                .expect("identity should be generated");
        let registry = CompanionRegistry::default();
        let transport = LiveCompanionTransport::new(registry.clone());
        let server = QuinnCompanionServer::bind(
            "127.0.0.1:0".parse().unwrap(),
            identity.companion_server_config().unwrap(),
            transport,
        )
        .unwrap();
        let server_addr = server.local_addr().unwrap();
        let server_task = tokio::spawn(server.run());

        let mut roots = RootCertStore::empty();
        roots
            .add(CertificateDer::from(identity.cert_der.clone()))
            .unwrap();
        let mut tls = rustls::ClientConfig::builder()
            .with_root_certificates(roots)
            .with_no_client_auth();
        tls.alpn_protocols = vec![COMPANION_ALPN.to_vec()];
        let crypto = QuicClientConfig::try_from(tls).unwrap();
        let mut endpoint = quinn::Endpoint::client("127.0.0.1:0".parse().unwrap()).unwrap();
        endpoint.set_default_client_config(quinn::ClientConfig::new(Arc::new(crypto)));
        let connection = endpoint
            .connect(server_addr, "adbcontrol-core")
            .unwrap()
            .await
            .expect("pinned Companion QUIC handshake should succeed");
        let (mut send, mut recv) = connection.open_bi().await.unwrap();
        send.write_all(WIRE_MAGIC).await.unwrap();
        send.write_u8(STREAM_CONTROL).await.unwrap();
        let hello = QuicEnvelope {
            protocol: String::from(COMPANION_PROTOCOL),
            version: COMPANION_PROTOCOL_VERSION,
            message_id: String::from("hello-device-1"),
            trace_id: Some(String::from("wire-test")),
            device_id: Some(String::from("device-1")),
            channel: QuicChannel::Control,
            kind: QuicMessageKind::Hello,
            payload: serde_json::to_value(CompanionHello {
                app_version: String::from("0.13.0"),
                device_id: String::from("device-1"),
                device_name: String::from("Test phone"),
                android_sdk: 35,
                supported_protocol_versions: vec![COMPANION_PROTOCOL_VERSION],
            })
            .unwrap(),
        };
        write_control_frame(&mut send, &serde_json::to_vec(&hello).unwrap())
            .await
            .unwrap();
        let response: QuicEnvelope = serde_json::from_slice(
            &tokio::time::timeout(Duration::from_secs(2), read_control_frame(&mut recv))
                .await
                .expect("helloAck should arrive")
                .unwrap(),
        )
        .unwrap();

        assert_eq!(response.kind, QuicMessageKind::HelloAck);
        assert!(registry.get_device("device-1").is_ok());
        connection.close(0_u32.into(), b"test complete");
        endpoint.close(0_u32.into(), b"test complete");
        server_task.abort();
    }
}
