//! Go sidecar process supervisor + JSON-RPC multiplexer.
//!
//! RESPONSIBILITIES OF THIS LAYER (and nothing else):
//!   1. Process lifecycle: spawn, restart, shut down on app exit.
//!   2. Multiplexing: many concurrent UI requests must be correlated by
//!      `id` -> oneshot channel. Without this, the export progress stream
//!      would "steal" the Test Connection response.
//!   3. Forwarding engine notifications (progress/log) as Tauri events.
//!   4. Translating JSON-RPC errors into an `AppError` the UI can understand.
//!
//! What this layer does NOT do: touch the database, masking, or SQL. All of
//! that belongs to Go. Rust deliberately stays "thin" so there is never a
//! second copy of the domain logic that could behave differently.
//!
//! MEMORY STRUCTURE: all state is shared through `Arc<Shared>` so the reader
//! task (which lives as long as the sidecar process) and the command handlers
//! (which live briefly) use the same state without `unsafe` and without Rc
//! cycles.

pub mod protocol;

pub use protocol::{events, EngineStatus};

use std::collections::HashMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use serde_json::{json, Value};
use tauri::{AppHandle, Emitter};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;
use tokio::sync::oneshot;

use crate::error::{AppError, AppResult};
use protocol as proto;

/// Default timeout per method. `export.run` is very generous because its
/// duration is dictated by the database size; it is cancelled via `export.cancel`.
fn default_timeout(method: &str) -> Duration {
    match method {
        proto::M_CONNECTION_TEST => Duration::from_secs(30),
        proto::M_SCHEMA_INTROSPECT => Duration::from_secs(180),
        proto::M_EXPORT_PLAN => Duration::from_secs(180),
        proto::M_EXPORT_RUN => Duration::from_secs(60 * 60 * 6),
        proto::M_ENGINE_HANDSHAKE => Duration::from_secs(15),
        _ => Duration::from_secs(60),
    }
}

/// State shared between the command handlers and the reader task.
pub struct Shared {
    app: AppHandle,
    pending: Mutex<HashMap<u64, oneshot::Sender<Result<Value, AppError>>>>,
    next_id: AtomicU64,
    child: Mutex<Option<CommandChild>>,
    status: Mutex<EngineStatus>,
    handshake: Mutex<Option<Value>>,
    /// Held for the duration of the spawn so that two concurrent first
    /// requests do not start two sidecar processes.
    spawn_lock: tokio::sync::Mutex<()>,
}

impl Shared {
    fn publish_status(&self, status: EngineStatus) {
        *self.status.lock().unwrap() = status.clone();
        let _ = self.app.emit(events::ENGINE_STATUS, status);
    }

    fn fail_all_pending(&self, reason: AppError) {
        let mut guard = self.pending.lock().unwrap();
        for (_, tx) in guard.drain() {
            let _ = tx.send(Err(reason.clone()));
        }
    }

    /// Reads one NDJSON line from the engine and routes it to the right
    /// destination: a response (has an `id`) or a notification (has a `method`).
    fn handle_line(&self, raw: &[u8]) {
        let text = String::from_utf8_lossy(raw);
        let text = text.trim();
        if text.is_empty() {
            return;
        }
        let msg: proto::RpcMessage = match serde_json::from_str(text) {
            Ok(m) => m,
            Err(err) => {
                // Not JSON = a framing bug in the engine. Logged, but NOT fatal
                // to the app: one corrupt line must not bring down the UI.
                eprintln!("[relaydb] non-JSON line from engine: {err}: {text}");
                return;
            }
        };

        if let Some(id) = msg.id_u64() {
            if let Some(tx) = self.pending.lock().unwrap().remove(&id) {
                let _ = tx.send(dispatch_response(msg));
            }
            return;
        }

        if msg.is_notification() {
            let method = msg.method.clone().unwrap_or_default();
            let params = msg.params.clone().unwrap_or(Value::Null);
            match method.as_str() {
                proto::N_ENGINE_READY => {
                    let mut status = self.status.lock().unwrap().clone();
                    status.state = "ready".into();
                    status.protocol_version =
                        params.get("protocolVersion").and_then(|v| v.as_str()).map(String::from);
                    status.engine_version =
                        params.get("engineVersion").and_then(|v| v.as_str()).map(String::from);
                    self.publish_status(status);
                }
                _ => {
                    let event = match method.as_str() {
                        proto::N_EXPORT_PROGRESS => events::EXPORT_PROGRESS,
                        proto::N_EXPORT_TABLE => events::EXPORT_TABLE,
                        proto::N_ENGINE_LOG => events::ENGINE_LOG,
                        _ => return,
                    };
                    let _ = self.app.emit(event, params);
                }
            }
        }
    }

    fn status(&self) -> EngineStatus {
        self.status.lock().unwrap().clone()
    }
}

/// Engine is the handle managed by Tauri (`app.state::<Engine>()`).
/// Created inside `setup()` because it needs an `AppHandle` to reach the shell
/// plugin.
#[derive(Clone)]
pub struct Engine {
    shared: Arc<Shared>,
}

impl Engine {
    /// Creates the engine AND starts the sidecar.
    pub fn spawn(app: AppHandle) -> Self {
        let engine = Self {
            shared: Arc::new(Shared {
                app,
                pending: Mutex::new(HashMap::new()),
                next_id: AtomicU64::new(1),
                child: Mutex::new(None),
                status: Mutex::new(EngineStatus::starting()),
                handshake: Mutex::new(None),
                spawn_lock: tokio::sync::Mutex::new(()),
            }),
        };
        let cloned = engine.shared.clone();
        tauri::async_runtime::spawn(async move {
            if let Err(err) = ensure_alive(&cloned).await {
                cloned.publish_status(EngineStatus {
                    last_error: Some(err.message.clone()),
                    ..EngineStatus::state("crashed")
                });
                eprintln!("[relaydb] failed to start engine: {err}");
            }
        });
        engine
    }

    pub fn status(&self) -> EngineStatus {
        self.shared.status()
    }

    /// Calls an engine method. `params` is passed through as-is (see the note in
    /// protocol.rs about where type responsibility lies).
    pub async fn call(&self, method: &str, params: Value) -> AppResult<Value> {
        self.call_with_timeout(method, params, default_timeout(method)).await
    }

    pub async fn call_with_timeout(
        &self,
        method: &str,
        params: Value,
        timeout: Duration,
    ) -> AppResult<Value> {
        ensure_alive(&self.shared).await?;

        let id = self.shared.next_id.fetch_add(1, Ordering::SeqCst);
        let (tx, rx) = oneshot::channel();
        self.shared.pending.lock().unwrap().insert(id, tx);

        let req = proto::RpcRequest {
            jsonrpc: "2.0",
            id,
            method,
            params: if params.is_null() { None } else { Some(params) },
        };
        let mut line = serde_json::to_vec(&req)
            .map_err(|e| AppError::invalid(format!("failed to encode request: {e}")))?;
        line.push(b'\n'); // NDJSON framing

        {
            let mut guard = self.shared.child.lock().unwrap();
            let child = guard
                .as_mut()
                .ok_or_else(|| AppError::engine_down("process unavailable"))?;
            child
                .write(&line)
                .map_err(|e| AppError::engine_down(format!("failed to write to engine: {e}")))?;
        }

        match tokio::time::timeout(timeout, rx).await {
            Ok(Ok(result)) => result,
            Ok(Err(_)) => Err(AppError::engine_down("response channel closed (engine dead?)")),
            Err(_) => {
                self.shared.pending.lock().unwrap().remove(&id);
                Err(AppError::timeout(method, timeout.as_millis() as u64))
            }
        }
    }

    /// Idempotent handshake; the result is cached and used by `engine_status`.
    pub async fn handshake(&self) -> AppResult<Value> {
        if let Some(cached) = self.shared.handshake.lock().unwrap().clone() {
            return Ok(cached);
        }
        let result = self.call(proto::M_ENGINE_HANDSHAKE, json!({})).await?;

        let mut status = self.shared.status.lock().unwrap().clone();
        status.state = "ready".into();
        status.protocol_version =
            result.get("protocolVersion").and_then(|v| v.as_str()).map(String::from);
        status.engine_version =
            result.get("engineVersion").and_then(|v| v.as_str()).map(String::from);
        status.platform = result.get("platform").and_then(|v| v.as_str()).map(String::from);
        status.pid = result.get("pid").and_then(|v| v.as_u64()).map(|v| v as u32);
        if let Some(list) = result.get("drivers").and_then(|v| v.as_array()) {
            status.drivers = serde_json::from_value(Value::Array(list.clone())).ok();
        }
        *self.shared.handshake.lock().unwrap() = Some(result.clone());
        self.shared.publish_status(status);
        Ok(result)
    }

    /// Forced restart (used by the UI after an engine crash).
    pub async fn restart(&self) -> AppResult<Value> {
        self.kill();
        *self.shared.handshake.lock().unwrap() = None;
        self.shared.publish_status(EngineStatus::starting());
        ensure_alive(&self.shared).await?;
        self.handshake().await
    }

    pub fn kill(&self) {
        if let Some(child) = self.shared.child.lock().unwrap().take() {
            let _ = child.kill();
        }
        self.shared.fail_all_pending(AppError::engine_down("engine stopped"));
        self.shared.publish_status(EngineStatus::state("stopped"));
    }

    /// Called from `RunEvent::Exit` so it does not leave a zombie process behind.
    pub fn shutdown(&self) {
        if let Some(child) = self.shared.child.lock().unwrap().take() {
            let _ = child.kill();
        }
    }
}

async fn ensure_alive(shared: &Arc<Shared>) -> AppResult<()> {
    if shared.child.lock().unwrap().is_some() {
        return Ok(());
    }
    let _guard = shared.spawn_lock.lock().await;
    if shared.child.lock().unwrap().is_some() {
        return Ok(());
    }
    start_process(shared)?;
    for _ in 0..50 {
        if shared.child.lock().unwrap().is_some() {
            return Ok(());
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    Err(AppError::engine_down("engine failed to start within 5 seconds"))
}

fn start_process(shared: &Arc<Shared>) -> AppResult<()> {
    shared.publish_status(EngineStatus::starting());

    let command = shared.app.shell().sidecar("relaydb-engine").map_err(|e| {
        AppError::engine_down(format!(
            "sidecar `relaydb-engine` not found ({e}). Run `bun run engine:build` first."
        ))
    })?;

    let (mut rx, child) = command
        .spawn()
        .map_err(|e| AppError::engine_down(format!("failed to spawn sidecar: {e}")))?;

    *shared.child.lock().unwrap() = Some(child);

    // The reader task stays attached to the Receiver until the process is
    // Terminated. It holds an Arc<Shared> (not a reference to Engine), so
    // there is no lifetime trouble and no cross-reference.
    let state = shared.clone();
    tauri::async_runtime::spawn(async move {
        while let Some(event) = rx.recv().await {
            match event {
                CommandEvent::Stdout(bytes) => state.handle_line(&bytes),
                CommandEvent::Stderr(bytes) => {
                    let text = String::from_utf8_lossy(&bytes).trim().to_string();
                    if !text.is_empty() {
                        eprintln!("[engine] {text}");
                        let _ = state
                            .app
                            .emit(events::ENGINE_LOG, json!({"level":"debug","message":text}));
                    }
                }
                CommandEvent::Error(err) => {
                    let _ = state
                        .app
                        .emit(events::ENGINE_LOG, json!({"level":"error","message":err}));
                }
                CommandEvent::Terminated(payload) => {
                    state.child.lock().unwrap().take();
                    let last_error = payload
                        .code
                        .filter(|code| *code != 0)
                        .map(|code| format!("engine exited with code {code}"));
                    state.fail_all_pending(AppError::engine_down(
                        last_error.clone().unwrap_or_else(|| "engine stopped".into()),
                    ));
                    state.publish_status(EngineStatus {
                        last_error,
                        ..EngineStatus::state("stopped")
                    });
                    break;
                }
                _ => {}
            }
        }
    });
    Ok(())
}

fn dispatch_response(msg: proto::RpcMessage) -> Result<Value, AppError> {
    if let Some(err) = msg.error {
        let mut e = AppError::new(err.code, err.message).stage("engine");
        if let Some(data) = err.data {
            e = e.detail(data.to_string());
        }
        return Err(e);
    }
    Ok(msg.result.unwrap_or(Value::Null))
}
