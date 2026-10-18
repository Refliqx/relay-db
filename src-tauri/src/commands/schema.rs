//! Step 2: Schema & FK Explorer.

use serde_json::{json, Value};
use tauri::State;

use crate::engine::{protocol, Engine};
use crate::error::AppResult;

/// Fetches the schema snapshot, FK graph, and topological order in a SINGLE
/// round-trip. The UI can draw the relationship diagram immediately instead of
/// waiting on chained calls (which feel slow on a database with hundreds of
/// tables).
#[tauri::command]
pub async fn introspect_schema(
    engine: State<'_, Engine>,
    profile: Value,
    exact_row_count: Option<bool>,
    include_views: Option<bool>,
) -> AppResult<Value> {
    engine
        .call(
            protocol::M_SCHEMA_INTROSPECT,
            json!({
                "profile": profile,
                "exactRowCount": exact_row_count.unwrap_or(false),
                "includeViews": include_views.unwrap_or(false),
            }),
        )
        .await
}

/// Masking strategy catalogue (used by step 3 for the dropdown and for default
/// suggestions derived from the PII heuristic in introspection).
#[tauri::command]
pub async fn mask_catalog(engine: State<'_, Engine>) -> AppResult<Value> {
    engine.call(protocol::M_MASK_CATALOG, json!({})).await
}
