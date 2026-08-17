use crate::profile;
use std::sync::Mutex;
use tauri::{
    webview::{Color, WebviewBuilder},
    AppHandle, LogicalPosition, LogicalSize, Manager, Runtime, Theme, WebviewUrl,
};
use uuid::Uuid;

pub const PLATFORMS: &[(&str, &str)] = &[
    ("threads", "https://www.threads.net"),
    ("x", "https://x.com"),
    ("instagram", "https://www.instagram.com"),
];

const FALLBACK_SIDEBAR_WIDTH: f64 = 256.0;

#[cfg(target_os = "macos")]
const USER_AGENT: &str = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15";

#[cfg(not(target_os = "macos"))]
const USER_AGENT: &str = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36";

const COLOR_SCHEME_SCRIPT: &str = r#"
(function () {
  document.documentElement.style.colorScheme = "light dark";
  var existing = document.querySelector('meta[name="color-scheme"]');
  if (!existing) {
    var meta = document.createElement("meta");
    meta.name = "color-scheme";
    meta.content = "light dark";
    (document.head || document.documentElement).appendChild(meta);
  }
})();
"#;

#[derive(Clone, Copy, Debug)]
pub struct ContentBounds {
    pub width: f64,
    pub height: f64,
}

impl Default for ContentBounds {
    fn default() -> Self {
        Self {
            width: 0.0,
            height: 0.0,
        }
    }
}

pub struct AppState {
    pub visible_label: Mutex<Option<String>>,
    pub content_bounds: Mutex<ContentBounds>,
}

impl Default for AppState {
    fn default() -> Self {
        Self {
            visible_label: Mutex::new(None),
            content_bounds: Mutex::new(ContentBounds::default()),
        }
    }
}

fn webview_label(profile_id: Uuid, platform: &str) -> String {
    format!("{profile_id}-{platform}")
}

fn platform_url(platform: &str) -> Result<&'static str, String> {
    PLATFORMS
        .iter()
        .find(|(id, _)| *id == platform)
        .map(|(_, url)| *url)
        .ok_or_else(|| format!("unknown platform: {platform}"))
}

fn main_window(app: &AppHandle) -> Result<tauri::Window, String> {
    if let Some(window) = app.get_window("main") {
        return Ok(window);
    }
    if let Some(webview) = app.get_webview("main") {
        return Ok(webview.window());
    }
    Err("main window not found".into())
}

fn resolved_bounds(
    window: &tauri::Window,
    stored: ContentBounds,
) -> Result<(LogicalPosition<f64>, LogicalSize<f64>), String> {
    let scale = window
        .scale_factor()
        .map_err(|e| format!("scale factor: {e}"))?;
    let win = window
        .inner_size()
        .map_err(|e| format!("window size: {e}"))?
        .to_logical::<f64>(scale);

    let width = if stored.width > 1.0 {
        stored.width
    } else {
        (win.width - FALLBACK_SIDEBAR_WIDTH).max(100.0)
    };
    let height = if stored.height > 1.0 {
        stored.height
    } else {
        win.height
    };
    let x = (win.width - width).max(0.0);
    Ok((LogicalPosition::new(x, 0.0), LogicalSize::new(width, height)))
}

fn apply_isolation<R: Runtime>(
    app: &AppHandle<R>,
    builder: WebviewBuilder<R>,
    profile_id: Uuid,
) -> Result<WebviewBuilder<R>, String> {
    #[cfg(target_os = "macos")]
    {
        let _ = app;
        let identifier = *profile_id.as_bytes();
        Ok(builder.data_store_identifier(identifier))
    }

    #[cfg(not(target_os = "macos"))]
    {
        let dir = profile::session_dir(app, profile_id)?;
        std::fs::create_dir_all(&dir).map_err(|e| format!("create session dir: {e}"))?;
        Ok(builder.data_directory(dir))
    }
}

fn ensure_profile_webviews(
    app: &AppHandle,
    profile_id: Uuid,
    bounds: (LogicalPosition<f64>, LogicalSize<f64>),
) -> Result<(), String> {
    let window = main_window(app)?;
    let (position, size) = bounds;

    for (platform, url) in PLATFORMS {
        let label = webview_label(profile_id, platform);
        if app.get_webview(&label).is_some() {
            continue;
        }

        let parsed = url
            .parse()
            .map_err(|e| format!("parse url {url}: {e}"))?;
        let theme = window.theme().unwrap_or(Theme::Dark);
        let builder = WebviewBuilder::new(&label, WebviewUrl::External(parsed))
            .user_agent(USER_AGENT)
            .initialization_script(COLOR_SCHEME_SCRIPT)
            .background_color(theme_background(theme))
            .focused(false);
        let builder = apply_isolation(app, builder, profile_id)?;

        let webview = window
            .add_child(builder, position, size)
            .map_err(|e| format!("create webview {label}: {e}"))?;
        apply_theme_to_webview(&webview, theme);
        webview
            .hide()
            .map_err(|e| format!("hide webview {label}: {e}"))?;
    }

    Ok(())
}

fn hide_label(app: &AppHandle, label: &str) -> Result<(), String> {
    if let Some(webview) = app.get_webview(label) {
        webview
            .hide()
            .map_err(|e| format!("hide webview {label}: {e}"))?;
    }
    Ok(())
}

pub fn switch_to_tab(
    app: &AppHandle,
    state: &AppState,
    profile_id: Uuid,
    platform: String,
) -> Result<(), String> {
    platform_url(&platform)?;

    let window = main_window(app)?;
    let stored = *state
        .content_bounds
        .lock()
        .map_err(|e| format!("lock bounds: {e}"))?;
    let bounds = resolved_bounds(&window, stored)?;

    ensure_profile_webviews(app, profile_id, bounds)?;

    let target = webview_label(profile_id, &platform);
    let mut visible = state
        .visible_label
        .lock()
        .map_err(|e| format!("lock visible: {e}"))?;

    if let Some(current) = visible.as_ref() {
        if current != &target {
            hide_label(app, current)?;
        }
    }

    let webview = app
        .get_webview(&target)
        .ok_or_else(|| format!("webview {target} missing after create"))?;
    webview
        .set_position(bounds.0)
        .map_err(|e| format!("set position: {e}"))?;
    webview
        .set_size(bounds.1)
        .map_err(|e| format!("set size: {e}"))?;
    webview
        .show()
        .map_err(|e| format!("show webview: {e}"))?;

    *visible = Some(target);
    Ok(())
}

pub fn resize_content_area(app: &AppHandle, state: &AppState, width: f64, height: f64) -> Result<(), String> {
    {
        let mut stored = state
            .content_bounds
            .lock()
            .map_err(|e| format!("lock bounds: {e}"))?;
        stored.width = width;
        stored.height = height;
    }

    let visible = state
        .visible_label
        .lock()
        .map_err(|e| format!("lock visible: {e}"))?
        .clone();

    let Some(label) = visible else {
        return Ok(());
    };

    let window = main_window(app)?;
    let bounds = resolved_bounds(
        &window,
        ContentBounds { width, height },
    )?;

    if let Some(webview) = app.get_webview(&label) {
        webview
            .set_position(bounds.0)
            .map_err(|e| format!("set position: {e}"))?;
        webview
            .set_size(bounds.1)
            .map_err(|e| format!("set size: {e}"))?;
    }

    Ok(())
}

pub fn close_profile_webviews(app: &AppHandle, state: &AppState, profile_id: Uuid) -> Result<(), String> {
    let mut visible = state
        .visible_label
        .lock()
        .map_err(|e| format!("lock visible: {e}"))?;

    for (platform, _) in PLATFORMS {
        let label = webview_label(profile_id, platform);
        if visible.as_deref() == Some(label.as_str()) {
            *visible = None;
        }
        if let Some(webview) = app.get_webview(&label) {
            webview
                .close()
                .map_err(|e| format!("close webview {label}: {e}"))?;
        }
    }

    Ok(())
}

pub fn delete_session_data(app: &AppHandle, profile_id: Uuid) -> Result<(), String> {
    let dir = profile::session_dir(app, profile_id)?;
    if dir.exists() {
        std::fs::remove_dir_all(&dir)
            .map_err(|e| format!("delete session dir {}: {e}", dir.display()))?;
    }
    Ok(())
}

fn theme_background(theme: Theme) -> Color {
    match theme {
        Theme::Light => Color(255, 255, 255, 255),
        _ => Color(20, 20, 20, 255),
    }
}

pub fn apply_system_theme(app: &AppHandle, theme: Theme) {
    for (label, webview) in app.webviews() {
        if label == "main" {
            continue;
        }
        apply_theme_to_webview(&webview, theme);
    }
}

fn apply_theme_to_webview(webview: &tauri::Webview, theme: Theme) {
    #[cfg(target_os = "macos")]
    {
        let _ = webview.with_webview(move |platform| {
            apply_macos_appearance(platform.inner(), theme);
        });
    }

    #[cfg(not(target_os = "macos"))]
    {
        let _ = theme;
        let _ = webview;
    }
}

#[cfg(target_os = "macos")]
fn apply_macos_appearance(view: *mut std::ffi::c_void, _theme: Theme) {
    if view.is_null() {
        return;
    }

    unsafe {
        use objc2::runtime::AnyObject;
        use objc2::{class, msg_send};

        let app: *mut AnyObject = msg_send![class!(NSApplication), sharedApplication];
        if app.is_null() {
            return;
        }
        let appearance: *mut AnyObject = msg_send![app, effectiveAppearance];
        if appearance.is_null() {
            return;
        }
        let view = view as *mut AnyObject;
        let _: () = msg_send![view, setAppearance: appearance];
    }
}
