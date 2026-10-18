// RelayDB: Tauri host layer.
//
// Responsibility map (see docs/ARCHITECTURE.md):
//
//   Svelte  : UI/UX, presentation state, never touches the DB/FS.
//   Rust    : process lifecycle, OS dialogs, preset I/O, error translation.
//   Go      : DB driver, introspection, FK graph, masking, SQL generator.

mod commands;
mod engine;
mod error;

use tauri::Manager;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .setup(|app| {
            // The engine starts in setup (not in Builder::manage) because the
            // supervisor needs an AppHandle to reach the shell plugin.
            // Spawning is asynchronous: the window is already shown, so UI
            // startup does not wait for the Go process.
            let engine = engine::Engine::spawn(app.handle().clone());
            app.manage(engine);
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            // System & lifecycle
            commands::engine_status,
            commands::engine_handshake,
            commands::engine_restart,
            commands::ping_engine,
            commands::pick_sqlite_file,
            commands::pick_save_file,
            commands::read_text_file,
            commands::write_text_file,
            commands::reveal_in_os,
            // Step 1: connection
            commands::test_connection,
            // Step 2: schema
            commands::introspect_schema,
            commands::mask_catalog,
            // Step 3-4: masking & export
            commands::plan_export,
            commands::run_export,
            commands::cancel_export,
            commands::load_preset,
            commands::validate_preset,
        ])
        .build(tauri::generate_context!())
        .expect("failed to build the RelayDB application")
        .run(|app, event| {
            // Without this, closing the window leaves the Go process behind as
            // a zombie (especially on Windows). Hooking the explicit Exit event
            // keeps the shutdown in one place.
            if matches!(event, tauri::RunEvent::Exit) {
                if let Some(engine) = app.try_state::<engine::Engine>() {
                    engine.shutdown();
                }
            }
        });
}
