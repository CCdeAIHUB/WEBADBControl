//! Bounded metadata-only core diagnostics. Stdout remains exclusively IPC.
use serde_json::json;
use std::{
    fs::{self, OpenOptions},
    io::Write,
    path::PathBuf,
    sync::{
        atomic::{AtomicU64, Ordering},
        mpsc::{self, SyncSender},
        OnceLock,
    },
    time::{SystemTime, UNIX_EPOCH},
};

enum Pending {
    Line(String),
    Flush(std::sync::mpsc::Sender<()>),
}
static SENDER: OnceLock<SyncSender<Pending>> = OnceLock::new();
static DROPPED: AtomicU64 = AtomicU64::new(0);
static SESSION: OnceLock<String> = OnceLock::new();

fn token(value: &str) -> &str {
    if !value.is_empty()
        && value.len() <= 128
        && value
            .bytes()
            .all(|c| c.is_ascii_alphanumeric() || b"_.:-".contains(&c))
    {
        value
    } else {
        "unknown"
    }
}

fn root() -> PathBuf {
    if let Some(local) = std::env::var_os("LOCALAPPDATA") {
        return PathBuf::from(local).join("ADBControl/diagnostics");
    }
    let base = std::env::var_os("XDG_STATE_HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|| {
            std::env::var_os("HOME")
                .map(PathBuf::from)
                .unwrap_or_else(std::env::temp_dir)
                .join(".local/state")
        });
    base.join("ADBControl/diagnostics")
}

pub fn initialize() {
    let session = std::env::var("ADBCONTROL_DIAGNOSTIC_SESSION")
        .unwrap_or_else(|_| format!("core-{}", now()));
    let _ = SESSION.set(token(&session).to_owned());
    let (sender, receiver) = mpsc::sync_channel::<Pending>(1024);
    if SENDER.set(sender).is_err() {
        return;
    }
    let directory = root();
    let spawn = std::thread::Builder::new()
        .name("core-diagnostics".into())
        .spawn(move || {
            let mut generation = 0;
            let mut bytes = 0;
            for pending in receiver {
                let line = match pending {
                    Pending::Line(line) => line,
                    Pending::Flush(done) => {
                        let _ = done.send(());
                        continue;
                    }
                };
                if bytes > 2 * 1024 * 1024 {
                    generation = (generation + 1) % 4;
                    bytes = 0;
                }
                let path = directory.join(format!(
                    "events-core-{}-{generation}.jsonl",
                    std::process::id()
                ));
                let result = (|| -> std::io::Result<()> {
                    fs::create_dir_all(&directory)?;
                    let mut file = OpenOptions::new()
                        .create(true)
                        .write(true)
                        .append(bytes != 0)
                        .truncate(bytes == 0)
                        .open(path)?;
                    writeln!(file, "{line}")?;
                    bytes += line.len() + 1;
                    Ok(())
                })();
                if result.is_err() {
                    eprintln!("DIAGNOSTIC_CORE_WRITE_FAILED");
                }
            }
        });
    if spawn.is_err() {
        eprintln!("DIAGNOSTIC_CORE_WORKER_FAILED");
    }
    let previous = std::panic::take_hook();
    std::panic::set_hook(Box::new(move |panic| {
        // A panic cannot rely on a volatile queue being drained before process death.
        let folder = root();
        if fs::create_dir_all(&folder).is_ok() {
            if let Ok(mut file) = OpenOptions::new()
                .create(true)
                .append(true)
                .open(folder.join(format!("emergency-core-{}.jsonl", std::process::id())))
            {
                let _ = writeln!(
                    file,
                    "{}",
                    event("panic", "runtime", "panic", false, 0, Some("RUST_PANIC"))
                );
            }
        }
        previous(panic);
    }));
    record("start", "process", "startup", true, 0, None);
}

fn now() -> u128 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis()
}

fn event(
    stage: &str,
    method: &str,
    trace: &str,
    ok: bool,
    elapsed: u128,
    code: Option<&str>,
) -> String {
    json!({"schemaVersion":1,"timestampUnixMs":now(),"sessionId":SESSION.get(),"processId":std::process::id(),
        "module":"core","event":token(stage),"traceId":token(trace),"level":if ok {"info"} else {"error"},
        "fields":{"operation":token(method),"outcome":if ok {"success"} else {"failed"},"elapsedMs":elapsed,
            "errorCode":code.map(token),"dropped":DROPPED.load(Ordering::Relaxed)}}).to_string()
}

pub fn record(stage: &str, method: &str, trace: &str, ok: bool, elapsed: u128, code: Option<&str>) {
    if let Some(sender) = SENDER.get() {
        if sender
            .try_send(Pending::Line(event(
                stage, method, trace, ok, elapsed, code,
            )))
            .is_err()
        {
            DROPPED.fetch_add(1, Ordering::Relaxed);
        }
    }
}

pub fn flush() {
    if let Some(sender) = SENDER.get() {
        let (done, completed) = mpsc::channel();
        if sender.try_send(Pending::Flush(done)).is_err()
            || completed
                .recv_timeout(std::time::Duration::from_secs(2))
                .is_err()
        {
            eprintln!("DIAGNOSTIC_CORE_FLUSH_FAILED");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn metadata_rejects_payload_text_and_keeps_trace() {
        // Scenario: only operation identifiers, never credentials/arguments, enter diagnostics.
        let line = event("end", "adb.exec", "trace-1", false, 10, Some("BAD_ERROR"));
        assert!(line.contains("trace-1"));
        assert_eq!(token("bearer secret"), "unknown");
        assert!(
            !event("end", "https://private?key=secret", "trace", true, 1, None).contains("secret")
        );
    }
}
