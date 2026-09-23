use std::{
    collections::HashMap,
    io::{BufRead, BufReader},
    path::Path,
    process::{Child, Command, Stdio},
    sync::Mutex,
};

use serde::{Deserialize, Serialize};

use crate::{assets::AdbAsset, error::AppError};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct AdbCommandOutput {
    #[serde(rename = "exitCode")]
    pub exit_code: i32,
    pub stdout: String,
    pub stderr: String,
}

pub trait AdbRunner: Send + Sync {
    fn run(&self, adb_binary: &Path, args: &[String]) -> Result<AdbCommandOutput, AppError>;
}

#[derive(Debug, Default, Clone, Copy)]
pub struct ProcessAdbRunner;

impl AdbRunner for ProcessAdbRunner {
    fn run(&self, adb_binary: &Path, args: &[String]) -> Result<AdbCommandOutput, AppError> {
        validate_adb_args(args)?;

        // 安全边界：核心只能启动 manifest 解析出的 adb 二进制，并且只通过 args 传参。
        let output = Command::new(adb_binary)
            .args(args)
            .output()
            .map_err(|error| {
                AppError::new(
                    "ADB_PROCESS_SPAWN_FAILED",
                    format!("Failed to start adb binary at {}", adb_binary.display()),
                    "adb.runner",
                    true,
                )
                .with_cause(error)
                .with_suggestion(
                    "Verify that the ADB asset exists, is executable, and matches the host target.",
                )
            })?;

        Ok(AdbCommandOutput {
            exit_code: output.status.code().unwrap_or(-1),
            stdout: String::from_utf8_lossy(&output.stdout).into_owned(),
            stderr: String::from_utf8_lossy(&output.stderr).into_owned(),
        })
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ScrcpyServerConfig {
    pub session_id: String,
    pub device_id: String,
    pub apk_path: String,
    pub scid: u32,
    pub max_size: u16,
    pub video_bit_rate: u32,
    pub max_fps: u8,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ScrcpyServerProcess {
    pub session_id: String,
    pub process_id: u32,
}

/// Owns the long-running `adb shell app_process` processes used by scrcpy.
///
/// A regular `adb.exec` call waits for the child and therefore cannot keep the
/// Android server alive while Go connects to the forwarded sockets. Keeping
/// these children inside Core preserves the single ADB ownership boundary and
/// gives every WebSocket session deterministic start/stop semantics.
#[derive(Debug, Default)]
pub struct AdbProcessManager {
    children: Mutex<HashMap<String, Child>>,
}

impl AdbProcessManager {
    pub fn start_scrcpy(
        &self,
        adb_binary: &Path,
        config: &ScrcpyServerConfig,
    ) -> Result<ScrcpyServerProcess, AppError> {
        let args = build_scrcpy_server_args(config)?;
        let mut child = Command::new(adb_binary)
            .args(&args)
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .map_err(|error| {
                AppError::new(
                    "SCRCPY_PROCESS_SPAWN_FAILED",
                    "Failed to start the scrcpy ADB process.",
                    "adb.scrcpy",
                    true,
                )
                .with_cause(error)
                .with_suggestion("Verify the selected device and installed Companion APK.")
            })?;

        if let Some(stdout) = child.stdout.take() {
            log_scrcpy_output(config.session_id.clone(), "stdout", stdout);
        }
        if let Some(stderr) = child.stderr.take() {
            log_scrcpy_output(config.session_id.clone(), "stderr", stderr);
        }

        let process = ScrcpyServerProcess {
            session_id: config.session_id.clone(),
            process_id: child.id(),
        };
        let mut children = self.children.lock().map_err(|error| {
            AppError::new(
                "SCRCPY_PROCESS_REGISTRY_FAILED",
                "The scrcpy process registry is unavailable.",
                "adb.scrcpy",
                true,
            )
            .with_cause(error)
        })?;
        if let Some(mut previous) = children.insert(config.session_id.clone(), child) {
            previous.kill().ok();
            previous.wait().ok();
        }
        Ok(process)
    }

    pub fn stop_scrcpy(&self, session_id: &str) -> Result<bool, AppError> {
        let mut children = self.children.lock().map_err(|error| {
            AppError::new(
                "SCRCPY_PROCESS_REGISTRY_FAILED",
                "The scrcpy process registry is unavailable.",
                "adb.scrcpy",
                true,
            )
            .with_cause(error)
        })?;
        let Some(mut child) = children.remove(session_id) else {
            return Ok(false);
        };
        if child.try_wait().ok().flatten().is_none() {
            child.kill().map_err(|error| {
                AppError::new(
                    "SCRCPY_PROCESS_STOP_FAILED",
                    "Failed to stop the scrcpy ADB process.",
                    "adb.scrcpy",
                    true,
                )
                .with_cause(error)
            })?;
        }
        child.wait().ok();
        Ok(true)
    }
}

impl Drop for AdbProcessManager {
    fn drop(&mut self) {
        if let Ok(children) = self.children.get_mut() {
            for child in children.values_mut() {
                child.kill().ok();
                child.wait().ok();
            }
        }
    }
}

fn log_scrcpy_output<R>(session_id: String, stream: &'static str, reader: R)
where
    R: std::io::Read + Send + 'static,
{
    std::thread::spawn(move || {
        for line in BufReader::new(reader).lines().map_while(Result::ok) {
            eprintln!("scrcpy session={session_id} stream={stream} {line}");
        }
    });
}

pub fn build_scrcpy_server_args(config: &ScrcpyServerConfig) -> Result<Vec<String>, AppError> {
    validate_scrcpy_config(config)?;
    let remote_args = [
        format!("CLASSPATH={}", config.apk_path),
        "app_process".to_string(),
        "/".to_string(),
        "com.genymobile.scrcpy.Server".to_string(),
        "4.0".to_string(),
        format!("scid={:08x}", config.scid),
        "log_level=info".to_string(),
        "audio=false".to_string(),
        "video=true".to_string(),
        "control=true".to_string(),
        "video_codec=h264".to_string(),
        // The LAN HTTP browser path uses TinyH264 when WebCodecs is unavailable.
        // Keep scrcpy within that decoder's published AVC Baseline Level 4 ceiling.
        "video_codec_options=profile=1,level=2048".to_string(),
        format!("max_size={}", config.max_size),
        format!("video_bit_rate={}", config.video_bit_rate),
        format!("max_fps={}", config.max_fps),
        "tunnel_forward=true".to_string(),
        "send_device_meta=false".to_string(),
        "send_dummy_byte=true".to_string(),
        "send_stream_meta=true".to_string(),
        "send_frame_meta=true".to_string(),
    ];
    let mut args = vec![
        "-s".to_string(),
        config.device_id.clone(),
        "shell".to_string(),
    ];
    args.extend(remote_args);
    validate_adb_args(&args)?;
    Ok(args)
}

fn validate_scrcpy_config(config: &ScrcpyServerConfig) -> Result<(), AppError> {
    let valid_session = !config.session_id.is_empty()
        && config.session_id.len() <= 64
        && config
            .session_id
            .chars()
            .all(|character| character.is_ascii_alphanumeric() || character == '-');
    let valid_apk_path = config.apk_path.starts_with("/data/app/")
        && config.apk_path.ends_with(".apk")
        && config.apk_path.chars().all(|character| {
            character.is_ascii_alphanumeric()
                || matches!(character, '/' | '.' | '_' | '-' | '+' | '=' | '~')
        });
    if !valid_session
        || config.device_id.trim().is_empty()
        || config.device_id.len() > 512
        || !valid_apk_path
        || !(0x1000_0000..=0x7fff_ffff).contains(&config.scid)
        || !(128..=4096).contains(&config.max_size)
        || !(500_000..=32_000_000).contains(&config.video_bit_rate)
        || !(1..=60).contains(&config.max_fps)
    {
        return Err(AppError::new(
            "SCRCPY_CONFIG_INVALID",
            "The scrcpy server configuration is invalid.",
            "adb.scrcpy",
            false,
        ));
    }
    Ok(())
}

pub fn validate_adb_args(args: &[String]) -> Result<(), AppError> {
    for arg in args {
        if arg.as_bytes().contains(&0) {
            return Err(AppError::new(
                "ADB_ARGS_CONTAIN_NUL",
                "ADB arguments must not contain NUL bytes.",
                "adb.runner",
                false,
            ));
        }
    }

    Ok(())
}

pub fn adb_asset_display_name(asset: &AdbAsset) -> String {
    format!("{} ({:?})", asset.path, asset.source)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_nul_bytes_in_adb_args() {
        // 场景：前端输入不能通过 NUL 字节污染底层进程参数。
        let mut bad_arg = String::from("echo");
        bad_arg.push(char::from(0));
        let args = vec!["shell".to_string(), bad_arg];

        let error = validate_adb_args(&args).expect_err("NUL byte must be rejected");

        assert_eq!(error.error_code, "ADB_ARGS_CONTAIN_NUL");
        assert_eq!(error.module, "adb.runner");
    }

    #[test]
    fn accepts_normal_adb_args() {
        let args = vec!["devices".to_string(), "-l".to_string()];

        validate_adb_args(&args).expect("normal adb args should be accepted");
    }

    #[test]
    fn builds_long_running_scrcpy_command_without_shell_backgrounding() {
        let config = ScrcpyServerConfig {
            session_id: "web-1234abcd".to_string(),
            device_id: "device-1".to_string(),
            apk_path: "/data/app/~~abc==/com.adbcontrol.companion/base.apk".to_string(),
            scid: 0x1234_abcd,
            max_size: 1280,
            video_bit_rate: 4_000_000,
            max_fps: 30,
        };

        let args = build_scrcpy_server_args(&config).expect("configuration should be valid");

        assert_eq!(&args[..3], ["-s", "device-1", "shell"]);
        assert!(args.contains(&"scid=1234abcd".to_string()));
        // Browser HTTP fallback uses TinyH264, whose documented ceiling is AVC Baseline Level 4.
        assert!(args.contains(&"video_codec_options=profile=1,level=2048".to_string()));
        assert!(!args.iter().any(|argument| argument.contains('&')));
    }

    #[test]
    fn rejects_untrusted_scrcpy_classpath() {
        let config = ScrcpyServerConfig {
            session_id: "web-1234abcd".to_string(),
            device_id: "device-1".to_string(),
            apk_path: "/data/app/base.apk;reboot".to_string(),
            scid: 1,
            max_size: 1280,
            video_bit_rate: 4_000_000,
            max_fps: 30,
        };

        let error = build_scrcpy_server_args(&config).expect_err("shell metacharacters must fail");

        assert_eq!(error.error_code, "SCRCPY_CONFIG_INVALID");
    }

    #[test]
    fn rejects_scrcpy_id_above_java_signed_integer_range() {
        let config = ScrcpyServerConfig {
            session_id: "web-f0000000".to_string(),
            device_id: "device-1".to_string(),
            apk_path: "/data/app/~~abc==/com.adbcontrol.companion/base.apk".to_string(),
            scid: 0xf000_0000,
            max_size: 1280,
            video_bit_rate: 4_000_000,
            max_fps: 30,
        };

        let error = build_scrcpy_server_args(&config).expect_err("signed overflow must fail");

        assert_eq!(error.error_code, "SCRCPY_CONFIG_INVALID");
    }
}
