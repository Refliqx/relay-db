use serde::Serialize;

/// AppError is the only error shape that crosses over to the frontend.
///
/// Contract: the frontend ALWAYS receives `{ code, message, detail?, stage? }`.
/// This matters because the UI must distinguish "wrong password" (show the
/// credential form) from "host unreachable" (show network advice) without
/// parsing brittle error strings.
#[derive(Debug, Serialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct AppError {
    /// Domain code, aligned with the engine's JSON-RPC codes (e.g. -32003 auth).
    pub code: i64,
    pub message: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stage: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub detail: Option<String>,
}

impl AppError {
    pub fn new(code: i64, message: impl Into<String>) -> Self {
        Self { code, message: message.into(), stage: None, detail: None }
    }

    pub fn stage(mut self, stage: impl Into<String>) -> Self {
        self.stage = Some(stage.into());
        self
    }

    pub fn detail(mut self, detail: impl Into<String>) -> Self {
        self.detail = Some(detail.into());
        self
    }

    /// The engine is not running / the pipe broke. The frontend shows a
    /// "Restart engine" button instead of the raw message.
    pub fn engine_down(detail: impl Into<String>) -> Self {
        Self::new(-32000, "RelayDB engine is not running")
            .stage("engine")
            .detail(detail)
    }

    pub fn timeout(method: &str, ms: u64) -> Self {
        Self::new(-32099, format!("Operation {method} exceeded {ms} ms"))
            .stage("timeout")
    }

    pub fn invalid(message: impl Into<String>) -> Self {
        Self::new(-32602, message).stage("params")
    }
}

impl std::fmt::Display for AppError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "[{}] {}", self.code, self.message)
    }
}

impl std::error::Error for AppError {}

pub type AppResult<T> = Result<T, AppError>;
