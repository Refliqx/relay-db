//! Step 1: Connection Manager.

use serde_json::{json, Value};
use tauri::State;

use crate::engine::{protocol, Engine};
use crate::error::AppResult;

/// Test connection: opens a connection, measures latency, fetches the server
/// version, then closes it again. This operation does NOT store any DSN or
/// password; persistence only happens when the user explicitly saves a preset.
#[tauri::command]
pub async fn test_connection(engine: State<'_, Engine>, profile: Value) -> AppResult<Value> {
    engine
        .call(protocol::M_CONNECTION_TEST, json!({ "profile": profile }))
        .await
}
