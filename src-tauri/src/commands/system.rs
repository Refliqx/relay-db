//! System commands: engine status, file dialogs, and preset I/O.
//!
//! Separation of responsibilities: Rust owns filesystem I/O and dialogs, Go owns
//! the database domain. The frontend never touches the FS directly, so the web
//! view needs no broad `fs:allow-*` permission.

use std::path::{Path, PathBuf};

use serde_json::{json, Value};
use tauri::{AppHandle, State};
use tauri_plugin_dialog::DialogExt;

use crate::engine::{protocol, Engine, EngineStatus};
use crate::error::{AppError, AppResult};

#[tauri::command]
pub fn engine_status(engine: State<'_, Engine>) -> EngineStatus {
    engine.status()
}

/// Explicit handshake; the frontend calls it at startup and displays the
/// available drivers and versions.
#[tauri::command]
pub async fn engine_handshake(engine: State<'_, Engine>) -> AppResult<Value> {
    engine.handshake().await
}

/// For the engine-crash case: the user can recover without restarting the app.
#[tauri::command]
pub async fn engine_restart(engine: State<'_, Engine>) -> AppResult<Value> {
    engine.restart().await
}

/// Dialog for choosing a local SQLite file (step 1).
#[tauri::command]
pub async fn pick_sqlite_file(app: AppHandle) -> AppResult<Option<String>> {
    let picked = tauri::async_runtime::spawn_blocking(move || {
        app.dialog()
            .file()
            .set_title("Choose SQLite file")
            .add_filter("SQLite database", &["db", "sqlite", "sqlite3"])
            .add_filter("All files", &["*"])
            .blocking_pick_file()
    })
    .await
    .map_err(|e| AppError::new(-32603, format!("dialog failed: {e}")))?;

    Ok(picked.and_then(|p| p.into_path().ok()).map(path_to_string))
}

/// Dialog for saving a .sql file (step 4).
#[tauri::command]
pub async fn pick_save_file(app: AppHandle, default_name: Option<String>) -> AppResult<Option<String>> {
    let name = default_name.unwrap_or_else(|| "seed.sql".to_string());
    let picked = tauri::async_runtime::spawn_blocking(move || {
        app.dialog()
            .file()
            .set_title("Save seed SQL")
            .set_file_name(name)
            .add_filter("SQL", &["sql"])
            .blocking_save_file()
    })
    .await
    .map_err(|e| AppError::new(-32603, format!("dialog failed: {e}")))?;

    Ok(picked.and_then(|p| p.into_path().ok()).map(path_to_string))
}

/// Reads a text file (YAML preset) for display in the UI editor.
#[tauri::command]
pub async fn read_text_file(path: String) -> AppResult<String> {
    tokio::fs::read_to_string(&path)
        .await
        .map_err(|e| AppError::new(-32006, format!("failed to read {path}: {e}")).stage("io"))
}

/// Writes the preset ATOMICALLY (tmp + rename) so an editor watching the file
/// never reads a half-written file.
#[tauri::command]
pub async fn write_text_file(path: String, content: String) -> AppResult<String> {
    let target = PathBuf::from(&path);
    let dir = target
        .parent()
        .map(Path::to_path_buf)
        .ok_or_else(|| AppError::new(-32006, "invalid preset path").stage("io"))?;
    tokio::fs::create_dir_all(&dir)
        .await
        .map_err(|e| AppError::new(-32006, format!("failed to create folder {}: {e}", dir.display())).stage("io"))?;

    let tmp = dir.join(format!(
        ".{}.tmp",
        target.file_name().and_then(|s| s.to_str()).unwrap_or("relaydb")
    ));
    tokio::fs::write(&tmp, content.as_bytes())
        .await
        .map_err(|e| AppError::new(-32006, format!("failed to write {}: {e}", tmp.display())).stage("io"))?;
    tokio::fs::rename(&tmp, &target)
        .await
        .map_err(|e| AppError::new(-32006, format!("failed to finish writing: {e}")).stage("io"))?;

    Ok(path_to_string(target))
}

/// Opens the output folder in the OS file manager (step 4, "Reveal" button).
#[tauri::command]
pub async fn reveal_in_os(app: AppHandle, path: String) -> AppResult<()> {
    use tauri_plugin_opener::OpenerExt;
    // Open the parent folder, not the file: this is what the user expects once
    // the export has finished.
    let target = PathBuf::from(&path);
    let folder = target.parent().map(Path::to_path_buf).unwrap_or(target);
    app.opener()
        .open_path(folder.to_string_lossy().to_string(), None::<&str>)
        .map_err(|e| AppError::new(-32006, format!("failed to open folder: {e}")).stage("io"))
}

fn path_to_string(p: PathBuf) -> String {
    p.to_string_lossy().to_string()
}

/// Lightweight health check the UI uses to monitor the engine.
#[tauri::command]
pub async fn ping_engine(engine: State<'_, Engine>) -> AppResult<Value> {
    engine.call(protocol::M_ENGINE_HANDSHAKE, json!({})).await.map(|_| json!({"ok": true}))
}
