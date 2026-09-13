use std::{
    collections::HashMap,
    sync::Mutex,
    time::{Duration, Instant, SystemTime, UNIX_EPOCH},
};

use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use qrcode::{types::Color, QrCode};
use serde::Serialize;

use crate::{adb::AdbRunner, error::AppError};

const QR_PAIRING_TTL: Duration = Duration::from_secs(120);
const QR_QUIET_ZONE_MODULES: usize = 4;
const QR_RANDOM_ALPHABET: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WirelessPairingQr {
    pub session_id: String,
    pub service_name: String,
    pub qr_svg: String,
    pub mime_type: String,
    pub expires_at: u64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WirelessPairingResult {
    pub paired: bool,
    pub service_name: String,
    pub endpoint: String,
}

#[derive(Debug, Clone)]
struct PendingPairing {
    service_name: String,
    password: String,
    expires_at: Instant,
}

#[derive(Debug)]
pub struct WirelessPairingManager {
    pending: Mutex<HashMap<String, PendingPairing>>,
}

impl Default for WirelessPairingManager {
    fn default() -> Self {
        Self {
            pending: Mutex::new(HashMap::new()),
        }
    }
}

impl WirelessPairingManager {
    pub fn create_qr(&self) -> Result<WirelessPairingQr, AppError> {
        let session_id = URL_SAFE_NO_PAD.encode(rand::random::<[u8; 18]>());
        let service_name = format!("studio-{}", random_ascii(10));
        let password = random_ascii(12);
        let qr_svg = render_qr_svg(&format!("WIFI:T:ADB;S:{service_name};P:{password};;"))?;
        let expires_at = unix_now().saturating_add(QR_PAIRING_TTL.as_secs());
        let mut pending = self.lock_pending()?;
        let now = Instant::now();
        pending.retain(|_, pairing| pairing.expires_at > now);
        pending.insert(
            session_id.clone(),
            PendingPairing {
                service_name: service_name.clone(),
                password,
                expires_at: now + QR_PAIRING_TTL,
            },
        );
        Ok(WirelessPairingQr {
            session_id,
            service_name,
            qr_svg,
            mime_type: String::from("image/svg+xml"),
            expires_at,
        })
    }

    pub fn pair<R: AdbRunner>(
        &self,
        session_id: &str,
        runner: &R,
        adb_path: &std::path::Path,
    ) -> Result<WirelessPairingResult, AppError> {
        let pairing = self.get_pending(session_id)?;
        let discovery = runner.run(adb_path, &[String::from("mdns"), String::from("services")])?;
        if discovery.exit_code != 0 {
            return Err(AppError::new(
                "ADB_QR_PAIRING_DISCOVERY_FAILED",
                "ADB could not query mDNS pairing services.",
                "adb.wifi.qr",
                true,
            ));
        }
        let endpoint =
            find_pairing_endpoint(&discovery.stdout, &pairing.service_name).ok_or_else(|| {
                AppError::new(
                    "ADB_QR_PAIRING_NOT_DISCOVERED",
                    "The phone has not advertised the QR pairing service yet.",
                    "adb.wifi.qr",
                    true,
                )
                .with_suggestion("Keep the QR scanner open and retry this request shortly.")
            })?;
        let output = runner.run(
            adb_path,
            &[String::from("pair"), endpoint.clone(), pairing.password],
        )?;
        if output.exit_code != 0
            || !output
                .stdout
                .trim_start()
                .to_ascii_lowercase()
                .starts_with("successfully paired to ")
        {
            return Err(AppError::new(
                "ADB_QR_PAIRING_FAILED",
                "ADB did not confirm QR-code pairing.",
                "adb.wifi.qr",
                true,
            ));
        }
        self.cancel(session_id)?;
        Ok(WirelessPairingResult {
            paired: true,
            service_name: pairing.service_name,
            endpoint,
        })
    }

    pub fn cancel(&self, session_id: &str) -> Result<(), AppError> {
        self.lock_pending()?.remove(session_id);
        Ok(())
    }

    fn get_pending(&self, session_id: &str) -> Result<PendingPairing, AppError> {
        let mut pending = self.lock_pending()?;
        let now = Instant::now();
        pending.retain(|_, pairing| pairing.expires_at > now);
        pending.get(session_id).cloned().ok_or_else(|| {
            AppError::new(
                "ADB_QR_PAIRING_SESSION_INVALID",
                "The QR pairing session is missing, expired, or cancelled.",
                "adb.wifi.qr",
                true,
            )
        })
    }

    fn lock_pending(
        &self,
    ) -> Result<std::sync::MutexGuard<'_, HashMap<String, PendingPairing>>, AppError> {
        self.pending.lock().map_err(|_| {
            AppError::new(
                "ADB_QR_PAIRING_STATE_UNAVAILABLE",
                "The QR pairing state is unavailable.",
                "adb.wifi.qr",
                true,
            )
        })
    }
}

fn random_ascii(length: usize) -> String {
    (0..length)
        .map(|_| char::from(QR_RANDOM_ALPHABET[rand::random_range(0..QR_RANDOM_ALPHABET.len())]))
        .collect()
}

fn unix_now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

fn render_qr_svg(content: &str) -> Result<String, AppError> {
    let code = QrCode::new(content.as_bytes()).map_err(|error| {
        AppError::new(
            "ADB_QR_GENERATION_FAILED",
            "Core failed to encode the wireless ADB pairing QR code.",
            "adb.wifi.qr",
            true,
        )
        .with_cause(error)
    })?;
    let width = code.width();
    let canvas = width + QR_QUIET_ZONE_MODULES * 2;
    let mut path = String::new();
    for (index, color) in code.to_colors().iter().enumerate() {
        if *color == Color::Dark {
            let x = index % width + QR_QUIET_ZONE_MODULES;
            let y = index / width + QR_QUIET_ZONE_MODULES;
            path.push_str(&format!("M{x} {y}h1v1h-1z"));
        }
    }
    Ok(format!("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 {canvas} {canvas}\" shape-rendering=\"crispEdges\"><rect width=\"100%\" height=\"100%\" fill=\"white\"/><path d=\"{path}\" fill=\"black\"/></svg>"))
}

fn find_pairing_endpoint(output: &str, service_name: &str) -> Option<String> {
    output.lines().find_map(|line| {
        let mut fields = line.split_whitespace();
        let name = fields.next()?;
        let service_type = fields.next()?;
        let endpoint = fields.next()?;
        (name == service_name && service_type == "_adb-tls-pairing._tcp")
            .then(|| endpoint.to_string())
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn create_qr_hides_pairing_password_from_serialized_result() {
        let qr = WirelessPairingManager::default()
            .create_qr()
            .expect("QR should generate");
        assert!(qr.qr_svg.starts_with("<svg"));
        assert!(qr.service_name.starts_with("studio-"));
        assert!(!serde_json::to_string(&qr).unwrap().contains("password"));
    }
}
