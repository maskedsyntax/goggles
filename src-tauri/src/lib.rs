mod profile;
mod webviews;

use tauri::Manager;
use uuid::Uuid;
use webviews::AppState;

#[tauri::command]
fn list_profiles(app: tauri::AppHandle) -> Result<Vec<profile::Profile>, String> {
    profile::load_profiles(&app)
}

#[tauri::command]
fn create_profile(app: tauri::AppHandle, name: String) -> Result<profile::Profile, String> {
    profile::create_profile(&app, name)
}

#[tauri::command]
async fn delete_profile(
    app: tauri::AppHandle,
    state: tauri::State<'_, AppState>,
    id: Uuid,
) -> Result<(), String> {
    webviews::close_profile_webviews(&app, &state, id)?;
    webviews::delete_session_data(&app, id)?;
    profile::delete_profile_record(&app, id)
}

#[tauri::command]
async fn switch_to_tab(
    app: tauri::AppHandle,
    state: tauri::State<'_, AppState>,
    profile_id: Uuid,
    platform: String,
) -> Result<(), String> {
    webviews::switch_to_tab(&app, &state, profile_id, platform)
}

#[tauri::command]
fn resize_content_area(
    app: tauri::AppHandle,
    state: tauri::State<AppState>,
    width: f64,
    height: f64,
) -> Result<(), String> {
    webviews::resize_content_area(&app, &state, width, height)
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .manage(AppState::default())
        .invoke_handler(tauri::generate_handler![
            list_profiles,
            create_profile,
            delete_profile,
            switch_to_tab,
            resize_content_area
        ])
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::ThemeChanged(theme) = event {
                webviews::apply_system_theme(window.app_handle(), *theme);
            }
        })
        .setup(|app| {
            if let Some(window) = app.get_window("main") {
                let _ = window.set_title("Goggles");
                let _ = window.set_theme(None);
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
