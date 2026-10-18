//! Step 3 & 4: Visual masking/pruning, plan preview, and export execution.

use serde_json::{json, Value};
use tauri::State;

use crate::engine::{protocol, Engine};
use crate::error::{AppError, AppResult};

/// Plan preview: which tables, in what order, which columns get masked.
/// Called whenever the user changes a rule in the UI (debounced) and is purely
/// computed over the snapshot; it writes no files.
#[tauri::command]
pub async fn plan_export(engine: State<'_, Engine>, params: Value) -> AppResult<Value> {
    engine.call(protocol::M_EXPORT_PLAN, params).await
}

/// Runs the export. The frontend generates `jobId` so that:
///   1. progress notifications can be matched to the right job card,
///   2. the Cancel button knows which job to cancel.
#[tauri::command]
pub async fn run_export(engine: State<'_, Engine>, params: Value, job_id: String) -> AppResult<Value> {
    let mut payload = match params {
        Value::Object(map) => map,
        _ => return Err(AppError::invalid("export params must be an object")),
    };
    payload.insert("jobId".into(), Value::String(job_id));
    engine.call(protocol::M_EXPORT_RUN, Value::Object(payload)).await
}

/// Cancels the running export. The engine closes the writer, removes temporary
/// files, and returns error `-32010` from `run_export`.
#[tauri::command]
pub async fn cancel_export(engine: State<'_, Engine>, job_id: String) -> AppResult<Value> {
    engine
        .call(protocol::M_EXPORT_CANCEL, json!({ "jobId": job_id }))
        .await
}

/// Loads a preset from disk. The parser belongs to Go (single source of truth),
/// so the same file can be used headless in CI.
#[tauri::command]
pub async fn load_preset(engine: State<'_, Engine>, path: String, profile_id: Option<String>) -> AppResult<Value> {
    engine
        .call(
            protocol::M_CONFIG_LOAD,
            json!({ "path": path, "profileId": profile_id.unwrap_or_default() }),
        )
        .await
}

/// Validates a preset still in memory (not yet saved); gives immediate
/// feedback in the form, without writing a file.
#[tauri::command]
pub async fn validate_preset(engine: State<'_, Engine>, preset: Value) -> AppResult<Value> {
    engine.call(protocol::M_CONFIG_VALIDATE, preset).await
}
