//! Minimal mirror of `engine/internal/protocol/protocol.go`.
//!
//! ARCHITECTURE DECISION: the Rust layer deliberately does NOT duplicate the
//! whole domain schema (Table, Column, ExportPlan, ...). Rust is the
//! *supervisor & transport*, not the owner of the domain:
//!
//!   - Source of truth for data shapes : Go (`internal/protocol`)
//!   - Strongly typed consumer         : TypeScript (`src/lib/types/protocol.ts`)
//!
//! Copying 40+ structs into Rust would only add a second place that can
//! "drift" without buying any extra safety, because the payloads still have to
//! cross the JSON process boundary. The only things typed here are the envelope
//! (which Rust really does parse) and the status used by the UI.
//!
//! The constant groups below are a CONTRACT, not dead code: their values are
//! used by the frontend (via `src/lib/types/protocol.ts`) and by the docs
//! generator script. Rust itself only uses some of them, so `dead_code` is
//! allowed explicitly rather than deleting the contract.
#![allow(dead_code)]

use serde::{Deserialize, Serialize};
use serde_json::Value;

pub const PROTOCOL_VERSION: &str = "1.0";

// Methods (must match the constants in protocol.go)
pub const M_ENGINE_HANDSHAKE: &str = "engine.handshake";
pub const M_ENGINE_SHUTDOWN: &str = "engine.shutdown";
pub const M_CONNECTION_TEST: &str = "connection.test";
pub const M_SCHEMA_INTROSPECT: &str = "schema.introspect";
pub const M_EXPORT_PLAN: &str = "export.plan";
pub const M_EXPORT_RUN: &str = "export.run";
pub const M_EXPORT_CANCEL: &str = "export.cancel";
pub const M_CONFIG_LOAD: &str = "config.load";
pub const M_CONFIG_VALIDATE: &str = "config.validate";
pub const M_MASK_CATALOG: &str = "mask.catalog";

// Notifications
pub const N_ENGINE_READY: &str = "engine.ready";
pub const N_ENGINE_LOG: &str = "engine.log";
pub const N_EXPORT_PROGRESS: &str = "export.progress";
pub const N_EXPORT_TABLE: &str = "export.table";

// Domain error codes (in sync with Go)
pub const E_ENGINE_NOT_READY: i64 = -32000;
pub const E_KIND_UNSUPPORTED: i64 = -32001;
pub const E_CONNECTION_FAILED: i64 = -32002;
pub const E_AUTH_FAILED: i64 = -32003;
pub const E_INTROSPECT_FAILED: i64 = -32004;
pub const E_EXPORT_FAILED: i64 = -32005;
pub const E_CONFIG_INVALID: i64 = -32006;
pub const E_CANCELLED: i64 = -32010;
pub const E_TIMEOUT: i64 = -32099;

/// Request sent Rust -> Go.
#[derive(Debug, Serialize)]
pub struct RpcRequest<'a> {
    pub jsonrpc: &'a str,
    pub id: u64,
    pub method: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub params: Option<Value>,
}

/// Any message from Go. Its shape is disambiguated by `is_notification`.
#[derive(Debug, Deserialize)]
pub struct RpcMessage {
    #[serde(default)]
    pub id: Option<Value>,
    #[serde(default)]
    pub method: Option<String>,
    #[serde(default)]
    pub params: Option<Value>,
    #[serde(default)]
    pub result: Option<Value>,
    #[serde(default)]
    pub error: Option<RpcErrorBody>,
}

#[derive(Debug, Deserialize, Clone)]
pub struct RpcErrorBody {
    pub code: i64,
    pub message: String,
    #[serde(default)]
    pub data: Option<Value>,
}

impl RpcMessage {
    pub fn is_notification(&self) -> bool {
        self.method.is_some() && self.id.is_none()
    }

    pub fn id_u64(&self) -> Option<u64> {
        match self.id.as_ref()? {
            Value::Number(n) => n.as_u64(),
            Value::String(s) => s.parse().ok(),
            _ => None,
        }
    }
}

/// Engine status published to the frontend.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct EngineStatus {
    /// starting | ready | stopped | crashed
    pub state: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pid: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub engine_version: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub protocol_version: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub platform: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub drivers: Option<Vec<DriverInfo>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_error: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DriverInfo {
    pub kind: String,
    pub label: String,
    pub dsn_example: String,
    #[serde(default)]
    pub capabilities: Vec<String>,
}

impl EngineStatus {
    pub fn starting() -> Self {
        Self::state("starting")
    }

    pub fn state(state: &str) -> Self {
        Self {
            state: state.to_string(),
            pid: None,
            engine_version: None,
            protocol_version: None,
            platform: None,
            drivers: None,
            last_error: None,
        }
    }
}

/// Tauri event names. The `relaydb://` prefix avoids collisions with the
/// internal events of other plugins.
pub mod events {
    pub const ENGINE_STATUS: &str = "relaydb://engine-status";
    pub const ENGINE_LOG: &str = "relaydb://engine-log";
    pub const EXPORT_PROGRESS: &str = "relaydb://export-progress";
    pub const EXPORT_TABLE: &str = "relaydb://export-table";
}
