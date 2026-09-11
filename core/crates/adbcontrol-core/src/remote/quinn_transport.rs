use std::net::SocketAddr;

use crate::error::AppError;

use super::{CoreRequestHandler, RemoteControlService};

/// Encrypted remote-control endpoint. Quinn always performs a TLS 1.3 handshake
/// before exposing a connection to this server; plaintext fallback is impossible.
pub struct QuinnRemoteControlServer<H: CoreRequestHandler + 'static> {
    endpoint: quinn::Endpoint,
    service: RemoteControlService<H>,
    max_request_bytes: usize,
}

impl<H: CoreRequestHandler + 'static> QuinnRemoteControlServer<H> {
    pub fn bind(
        listen_addr: SocketAddr,
        server_config: quinn::ServerConfig,
        service: RemoteControlService<H>,
    ) -> Result<Self, AppError> {
        let endpoint = quinn::Endpoint::server(server_config, listen_addr).map_err(|error| {
            AppError::new(
                "REMOTE_QUIC_BIND_FAILED",
                "Core failed to bind the encrypted remote-control QUIC endpoint.",
                "remote.quic",
                true,
            )
            .with_cause(error)
        })?;
        Ok(Self {
            endpoint,
            service,
            max_request_bytes: 1024 * 1024,
        })
    }

    pub fn local_addr(&self) -> Result<SocketAddr, AppError> {
        self.endpoint.local_addr().map_err(|error| {
            AppError::new(
                "REMOTE_QUIC_LOCAL_ADDR_FAILED",
                "Core failed to read the remote-control QUIC endpoint address.",
                "remote.quic",
                true,
            )
            .with_cause(error)
        })
    }

    pub fn with_max_request_bytes(mut self, max_request_bytes: usize) -> Self {
        self.max_request_bytes = max_request_bytes;
        self
    }

    pub async fn run(self) -> Result<(), AppError> {
        while let Some(incoming) = self.endpoint.accept().await {
            let service = self.service.clone();
            let max_request_bytes = self.max_request_bytes;
            tokio::spawn(async move {
                if let Ok(connection) = incoming.await {
                    handle_connection(connection, service, max_request_bytes).await;
                }
            });
        }
        Ok(())
    }
}

async fn handle_connection<H: CoreRequestHandler + 'static>(
    connection: quinn::Connection,
    service: RemoteControlService<H>,
    max_request_bytes: usize,
) {
    loop {
        let (send, recv) = match connection.accept_bi().await {
            Ok(stream) => stream,
            Err(_) => return,
        };
        let service = service.clone();
        tokio::spawn(async move {
            let _ = handle_stream(send, recv, service, max_request_bytes).await;
        });
    }
}

async fn handle_stream<H: CoreRequestHandler + 'static>(
    mut send: quinn::SendStream,
    mut recv: quinn::RecvStream,
    service: RemoteControlService<H>,
    max_request_bytes: usize,
) -> Result<(), AppError> {
    let request = recv.read_to_end(max_request_bytes).await.map_err(|error| {
        AppError::new(
            "REMOTE_QUIC_REQUEST_READ_FAILED",
            "Failed to read the encrypted remote-control request stream.",
            "remote.quic",
            true,
        )
        .with_cause(error)
    })?;
    let response = service.handle_json_bytes(&request);
    send.write_all(&response).await.map_err(|error| {
        AppError::new(
            "REMOTE_QUIC_RESPONSE_WRITE_FAILED",
            "Failed to write the encrypted remote-control response stream.",
            "remote.quic",
            true,
        )
        .with_cause(error)
    })?;
    send.finish().map_err(|error| {
        AppError::new(
            "REMOTE_QUIC_RESPONSE_FINISH_FAILED",
            "Failed to finish the encrypted remote-control response stream.",
            "remote.quic",
            true,
        )
        .with_cause(error)
    })
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use rustls::{pki_types::CertificateDer, RootCertStore};
    use serde_json::json;

    use super::*;
    use crate::{
        companion::CoreQuicIdentity,
        protocol::{IpcRequest, IpcResponse},
        remote::{RemoteAccountManager, RemoteRequest, RemoteResponse},
    };

    struct FakeCore;

    impl CoreRequestHandler for FakeCore {
        fn handle_core_request(&self, request: IpcRequest) -> IpcResponse {
            IpcResponse::success(request.id, json!({}))
        }
    }

    #[tokio::test]
    async fn login_roundtrips_over_certificate_authenticated_quic() {
        let identity = CoreQuicIdentity::generate_self_signed(vec![String::from("localhost")])
            .expect("identity should be generated");
        let path = std::env::temp_dir().join(format!(
            "adbcontrol-remote-quic-test-{}.json",
            rand::random::<u64>()
        ));
        let accounts = Arc::new(RemoteAccountManager::load_or_initialize(&path).unwrap());
        accounts
            .create_user("operator", "operator-password")
            .unwrap();
        let service = RemoteControlService::new(Arc::new(FakeCore), accounts);
        let server = QuinnRemoteControlServer::bind(
            "127.0.0.1:0".parse().unwrap(),
            identity.server_config().unwrap(),
            service,
        )
        .unwrap();
        let server_addr = server.local_addr().unwrap();
        let server_task = tokio::spawn(server.run());

        let mut roots = RootCertStore::empty();
        roots
            .add(CertificateDer::from(identity.cert_der.clone()))
            .expect("remote certificate should be trusted explicitly");
        let client_config = quinn::ClientConfig::with_root_certificates(Arc::new(roots)).unwrap();
        let mut endpoint = quinn::Endpoint::client("127.0.0.1:0".parse().unwrap()).unwrap();
        endpoint.set_default_client_config(client_config);
        let connection = endpoint
            .connect(server_addr, "localhost")
            .unwrap()
            .await
            .expect("TLS-authenticated QUIC handshake should succeed");
        let (mut send, mut recv) = connection.open_bi().await.unwrap();
        let request = serde_json::to_vec(&RemoteRequest {
            id: String::from("login-over-quic"),
            session_token: None,
            method: String::from("auth.login"),
            params: json!({"username": "operator", "password": "operator-password"}),
        })
        .unwrap();
        send.write_all(&request).await.unwrap();
        send.finish().unwrap();
        let response: RemoteResponse =
            serde_json::from_slice(&recv.read_to_end(1024 * 1024).await.unwrap()).unwrap();

        assert!(response.ok);
        assert_eq!(response.result.unwrap()["username"], "operator");
        connection.close(0u32.into(), b"test complete");
        endpoint.close(0u32.into(), b"test complete");
        server_task.abort();
        std::fs::remove_file(path).ok();
    }
}
