//! ADB server 保活。
//!
//! Core 启动后由后台线程周期性执行 `adb start-server`：ADB server 已经存活时该命令
//! 是轻量 no-op，server 被杀掉或异常退出时会被重新拉起，从而保证设备链路不因
//! ADB server 掉线而中断。前端可以通过 IPC `adb.keepalive.status` /
//! `adb.keepalive.configure` 观测和调整保活行为。

use std::{
    sync::{
        mpsc::{self, RecvTimeoutError},
        Arc, Mutex,
    },
    thread::{self, JoinHandle},
    time::{Duration, SystemTime, UNIX_EPOCH},
};

use serde::{Deserialize, Serialize};

use crate::{
    adb::{AdbCommandOutput, AdbRunner},
    assets::{find_adb_asset, resolve_asset_path, AdbManifest},
    error::AppError,
    platform::HostTarget,
};

/// 保活允许的最小巡检间隔（秒）。过小的间隔只会反复空跑 `adb start-server`。
pub const ADB_KEEPALIVE_MIN_INTERVAL_SECS: u32 = 5;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AdbKeepAliveConfig {
    pub enabled: bool,
    pub interval_secs: u32,
}

impl Default for AdbKeepAliveConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            interval_secs: 30,
        }
    }
}

impl AdbKeepAliveConfig {
    pub fn validate(&self) -> Result<(), AppError> {
        if self.interval_secs < ADB_KEEPALIVE_MIN_INTERVAL_SECS {
            return Err(AppError::new(
                "ADB_KEEPALIVE_INTERVAL_INVALID",
                format!(
                    "ADB keep-alive interval must be at least {ADB_KEEPALIVE_MIN_INTERVAL_SECS} seconds."
                ),
                "adb.keepalive",
                false,
            ));
        }

        Ok(())
    }
}

/// 保活 supervisor 的可序列化状态快照，直接作为 IPC `adb.keepalive.status` 的结果。
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AdbKeepAliveStatus {
    pub enabled: bool,
    pub interval_secs: u32,
    pub running: bool,
    pub started_at: Option<u64>,
    pub last_check_at: Option<u64>,
    pub last_ok: Option<bool>,
    pub last_exit_code: Option<i32>,
    pub consecutive_failures: u32,
    pub last_error: Option<AppError>,
}

impl AdbKeepAliveStatus {
    fn initial(config: AdbKeepAliveConfig) -> Self {
        Self {
            enabled: config.enabled,
            interval_secs: config.interval_secs,
            running: true,
            started_at: Some(unix_now()),
            last_check_at: None,
            last_ok: None,
            last_exit_code: None,
            consecutive_failures: 0,
            last_error: None,
        }
    }
}

enum KeepAliveControl {
    Configure(AdbKeepAliveConfig),
    Stop,
}

/// 后台保活线程的句柄：查询状态、动态重配置、停止线程。
pub struct AdbKeepAliveHandle {
    shared: Arc<Mutex<AdbKeepAliveStatus>>,
    control: mpsc::Sender<KeepAliveControl>,
    join: Option<JoinHandle<()>>,
}

impl AdbKeepAliveHandle {
    pub fn status(&self) -> AdbKeepAliveStatus {
        self.shared
            .lock()
            .expect("keepalive state lock should not be poisoned")
            .clone()
    }

    pub fn configure(&self, config: AdbKeepAliveConfig) -> Result<(), AppError> {
        config.validate()?;
        apply_configure(&self.shared, config.clone());

        self.control
            .send(KeepAliveControl::Configure(config))
            .map_err(|_| {
                AppError::new(
                    "ADB_KEEPALIVE_THREAD_STOPPED",
                    "ADB keep-alive thread has already stopped.",
                    "adb.keepalive",
                    false,
                )
            })
    }

    pub fn stop(&self) {
        let _ = self.control.send(KeepAliveControl::Stop);
    }

    pub fn join(&mut self) {
        if let Some(join) = self.join.take() {
            let _ = join.join();
        }
    }
}

/// 启动 ADB 保活后台线程。线程启动后立即执行一次 `adb start-server`，
/// 之后按 `config.interval_secs` 周期巡检。
pub fn spawn_adb_keepalive<R>(
    runner: R,
    manifest: AdbManifest,
    config: AdbKeepAliveConfig,
) -> Result<AdbKeepAliveHandle, AppError>
where
    R: AdbRunner + Send + 'static,
{
    config.validate()?;

    let shared = Arc::new(Mutex::new(AdbKeepAliveStatus::initial(config)));
    let (control_tx, control_rx) = mpsc::channel();
    let thread_shared = Arc::clone(&shared);

    let join = thread::Builder::new()
        .name("adb-keepalive".to_string())
        .spawn(move || keepalive_loop(runner, manifest, thread_shared, control_rx))
        .map_err(|error| {
            AppError::new(
                "ADB_KEEPALIVE_THREAD_SPAWN_FAILED",
                "Failed to spawn ADB keep-alive thread.",
                "adb.keepalive",
                true,
            )
            .with_cause(error)
        })?;

    Ok(AdbKeepAliveHandle {
        shared,
        control: control_tx,
        join: Some(join),
    })
}

/// 执行一次保活巡检：解析当前平台 ADB 资产并运行 `adb start-server`。
pub fn keepalive_check<R: AdbRunner>(
    runner: &R,
    manifest: &AdbManifest,
) -> Result<AdbCommandOutput, AppError> {
    let target = HostTarget::current()?;
    let asset = find_adb_asset(manifest, &target)?;
    let adb_path = resolve_asset_path(&asset)?;
    let args = vec![String::from("start-server")];

    runner.run(&adb_path, &args)
}

fn keepalive_loop<R: AdbRunner>(
    runner: R,
    manifest: AdbManifest,
    shared: Arc<Mutex<AdbKeepAliveStatus>>,
    control: mpsc::Receiver<KeepAliveControl>,
) {
    apply_cycle(&shared, keepalive_check(&runner, &manifest));

    loop {
        let (enabled, interval_secs) = {
            let status = shared
                .lock()
                .expect("keepalive state lock should not be poisoned");
            (status.enabled, status.interval_secs)
        };

        if !enabled {
            // 关闭保活时不需要周期性唤醒，阻塞等待下一条控制指令即可。
            match control.recv() {
                Ok(KeepAliveControl::Configure(config)) => {
                    apply_configure(&shared, config);
                }
                Ok(KeepAliveControl::Stop) | Err(_) => break,
            }
            continue;
        }

        match control.recv_timeout(Duration::from_secs(interval_secs as u64)) {
            Ok(KeepAliveControl::Configure(config)) => {
                apply_configure(&shared, config);
            }
            Ok(KeepAliveControl::Stop) | Err(RecvTimeoutError::Disconnected) => break,
            Err(RecvTimeoutError::Timeout) => {
                apply_cycle(&shared, keepalive_check(&runner, &manifest));
            }
        }
    }

    if let Ok(mut status) = shared.lock() {
        status.running = false;
    }
}

fn apply_cycle(shared: &Mutex<AdbKeepAliveStatus>, result: Result<AdbCommandOutput, AppError>) {
    let mut status = shared
        .lock()
        .expect("keepalive state lock should not be poisoned");

    status.last_check_at = Some(unix_now());

    match result {
        Ok(output) => {
            status.last_exit_code = Some(output.exit_code);

            if output.exit_code == 0 {
                status.last_ok = Some(true);
                status.consecutive_failures = 0;
                status.last_error = None;
            } else {
                status.last_ok = Some(false);
                status.consecutive_failures += 1;
                status.last_error = Some(
                    AppError::new(
                        "ADB_KEEPALIVE_START_SERVER_FAILED",
                        format!("adb start-server exited with code {}.", output.exit_code),
                        "adb.keepalive",
                        true,
                    )
                    .with_cause(format!("stderr: {}", output.stderr.trim())),
                );
            }
        }
        Err(error) => {
            status.last_ok = Some(false);
            status.last_exit_code = None;
            status.consecutive_failures += 1;
            status.last_error = Some(error);
        }
    }
}

fn apply_configure(shared: &Mutex<AdbKeepAliveStatus>, config: AdbKeepAliveConfig) {
    let mut status = shared
        .lock()
        .expect("keepalive state lock should not be poisoned");

    status.enabled = config.enabled;
    status.interval_secs = config.interval_secs;
}

fn unix_now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_secs())
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use std::path::Path;

    use super::*;
    use crate::{adb::AdbCommandOutput, assets::load_embedded_manifest, error::AppError};

    struct RecordingRunner {
        calls: std::sync::Mutex<Vec<Vec<String>>>,
        exit_code: i32,
    }

    impl AdbRunner for RecordingRunner {
        fn run(&self, _adb_binary: &Path, args: &[String]) -> Result<AdbCommandOutput, AppError> {
            self.calls
                .lock()
                .expect("lock should not be poisoned")
                .push(args.to_vec());

            Ok(AdbCommandOutput {
                exit_code: self.exit_code,
                stdout: String::new(),
                stderr: String::new(),
            })
        }
    }

    struct FailingRunner;

    impl AdbRunner for FailingRunner {
        fn run(&self, _adb_binary: &Path, _args: &[String]) -> Result<AdbCommandOutput, AppError> {
            Err(AppError::new(
                "ADB_PROCESS_SPAWN_FAILED",
                "runner failed",
                "adb.runner",
                true,
            ))
        }
    }

    #[test]
    fn rejects_interval_below_minimum() {
        // 场景：过小的保活间隔只会空转，配置层必须显式拒绝。
        let config = AdbKeepAliveConfig {
            enabled: true,
            interval_secs: ADB_KEEPALIVE_MIN_INTERVAL_SECS - 1,
        };

        let error = config
            .validate()
            .expect_err("small interval must be rejected");

        assert_eq!(error.error_code, "ADB_KEEPALIVE_INTERVAL_INVALID");
        assert_eq!(error.module, "adb.keepalive");
    }

    #[test]
    fn accepts_minimum_interval() {
        let config = AdbKeepAliveConfig {
            enabled: true,
            interval_secs: ADB_KEEPALIVE_MIN_INTERVAL_SECS,
        };

        config.validate().expect("minimum interval should be valid");
    }

    #[test]
    fn keepalive_check_runs_start_server_with_current_asset() {
        // 场景：保活巡检必须复用 manifest 资产解析，并且只执行 `adb start-server`。
        let runner = RecordingRunner {
            calls: std::sync::Mutex::new(Vec::new()),
            exit_code: 0,
        };
        let manifest = load_embedded_manifest().expect("embedded manifest must be valid");

        let output = keepalive_check(&runner, &manifest).expect("check should succeed");

        assert_eq!(output.exit_code, 0);
        assert_eq!(
            runner.calls.lock().expect("lock should not be poisoned")[0],
            vec!["start-server".to_string()]
        );
    }

    #[test]
    fn spawn_runs_first_cycle_and_stop_ends_thread() {
        // 场景：保活线程启动后立即执行第一次巡检，stop 后必须干净退出。
        let runner = RecordingRunner {
            calls: std::sync::Mutex::new(Vec::new()),
            exit_code: 0,
        };
        let manifest = load_embedded_manifest().expect("embedded manifest must be valid");

        let mut handle = spawn_adb_keepalive(
            runner,
            manifest,
            AdbKeepAliveConfig {
                enabled: true,
                interval_secs: ADB_KEEPALIVE_MIN_INTERVAL_SECS,
            },
        )
        .expect("keepalive thread should spawn");

        let deadline = std::time::Instant::now() + Duration::from_secs(5);
        loop {
            let status = handle.status();
            if status.last_check_at.is_some() {
                assert_eq!(status.last_ok, Some(true));
                assert!(status.running);
                break;
            }
            assert!(std::time::Instant::now() < deadline, "first cycle must run");
            thread::sleep(Duration::from_millis(20));
        }

        handle.stop();
        handle.join();

        let status = handle.status();
        assert!(!status.running);
    }

    #[test]
    fn failed_cycles_accumulate_consecutive_failures() {
        // 场景：ADB server 拉起失败时状态必须累积失败计数并保留错误，供前端告警。
        let shared = Mutex::new(AdbKeepAliveStatus::initial(AdbKeepAliveConfig::default()));

        apply_cycle(
            &shared,
            Err(FailingRunner.run(Path::new("adb"), &[]).unwrap_err()),
        );
        apply_cycle(
            &shared,
            Err(FailingRunner.run(Path::new("adb"), &[]).unwrap_err()),
        );

        let status = shared.lock().expect("lock should not be poisoned");
        assert_eq!(status.last_ok, Some(false));
        assert_eq!(status.consecutive_failures, 2);
        assert!(status.last_error.is_some());
        assert!(status.last_check_at.is_some());
    }

    #[test]
    fn successful_cycle_resets_failures() {
        // 场景：恢复成功后失败计数必须清零，不能永久保留历史告警。
        let shared = Mutex::new(AdbKeepAliveStatus::initial(AdbKeepAliveConfig::default()));

        apply_cycle(
            &shared,
            Err(FailingRunner.run(Path::new("adb"), &[]).unwrap_err()),
        );
        apply_cycle(
            &shared,
            Ok(AdbCommandOutput {
                exit_code: 0,
                stdout: String::new(),
                stderr: String::new(),
            }),
        );

        let status = shared.lock().expect("lock should not be poisoned");
        assert_eq!(status.last_ok, Some(true));
        assert_eq!(status.consecutive_failures, 0);
        assert_eq!(status.last_error, None);
    }

    #[test]
    fn non_zero_exit_code_is_recorded_as_failure() {
        // 场景：adb start-server 返回非零时视为巡检失败，stderr 进入 cause。
        let shared = Mutex::new(AdbKeepAliveStatus::initial(AdbKeepAliveConfig::default()));

        apply_cycle(
            &shared,
            Ok(AdbCommandOutput {
                exit_code: 1,
                stdout: String::new(),
                stderr: "device offline\n".to_string(),
            }),
        );

        let status = shared.lock().expect("lock should not be poisoned");
        assert_eq!(status.last_ok, Some(false));
        assert_eq!(status.last_exit_code, Some(1));
        assert_eq!(status.consecutive_failures, 1);
        let error = status.last_error.as_ref().expect("error is required");
        assert_eq!(error.error_code, "ADB_KEEPALIVE_START_SERVER_FAILED");
    }

    #[test]
    fn configure_updates_shared_config() {
        let shared = Mutex::new(AdbKeepAliveStatus::initial(AdbKeepAliveConfig::default()));

        apply_configure(
            &shared,
            AdbKeepAliveConfig {
                enabled: false,
                interval_secs: 60,
            },
        );

        let status = shared.lock().expect("lock should not be poisoned");
        assert!(!status.enabled);
        assert_eq!(status.interval_secs, 60);
    }
}
