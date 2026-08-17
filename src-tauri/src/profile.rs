use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;
use tauri::{AppHandle, Manager};
use uuid::Uuid;

#[derive(Serialize, Deserialize, Clone, Debug)]
pub struct Profile {
    pub id: Uuid,
    pub name: String,
    pub created_at: String,
}

fn profiles_path(app: &AppHandle) -> Result<PathBuf, String> {
    let dir = app
        .path()
        .app_config_dir()
        .map_err(|e| format!("app config dir: {e}"))?;
    if !dir.exists() {
        fs::create_dir_all(&dir).map_err(|e| format!("create config dir: {e}"))?;
    }
    Ok(dir.join("profiles.json"))
}

pub fn load_profiles(app: &AppHandle) -> Result<Vec<Profile>, String> {
    let path = profiles_path(app)?;
    if !path.exists() {
        fs::write(&path, "[]").map_err(|e| format!("create profiles.json: {e}"))?;
        return Ok(Vec::new());
    }
    let data = fs::read_to_string(&path).map_err(|e| format!("read profiles.json: {e}"))?;
    serde_json::from_str(&data).map_err(|e| format!("parse profiles.json: {e}"))
}

pub fn save_profiles(app: &AppHandle, profiles: &[Profile]) -> Result<(), String> {
    let path = profiles_path(app)?;
    let data = serde_json::to_string_pretty(profiles)
        .map_err(|e| format!("serialize profiles: {e}"))?;
    fs::write(&path, data).map_err(|e| format!("write profiles.json: {e}"))
}

pub fn create_profile(app: &AppHandle, name: String) -> Result<Profile, String> {
    let name = name.trim().to_string();
    if name.is_empty() {
        return Err("profile name cannot be empty".into());
    }

    let profile = Profile {
        id: Uuid::new_v4(),
        name,
        created_at: chrono::Utc::now().to_rfc3339(),
    };

    let mut profiles = load_profiles(app)?;
    profiles.push(profile.clone());
    save_profiles(app, &profiles)?;
    Ok(profile)
}

pub fn delete_profile_record(app: &AppHandle, id: Uuid) -> Result<(), String> {
    let mut profiles = load_profiles(app)?;
    let before = profiles.len();
    profiles.retain(|p| p.id != id);
    if profiles.len() == before {
        return Err(format!("profile {id} not found"));
    }
    save_profiles(app, &profiles)
}

pub fn session_dir(app: &AppHandle, profile_id: Uuid) -> Result<PathBuf, String> {
    let dir = app
        .path()
        .app_data_dir()
        .map_err(|e| format!("app data dir: {e}"))?
        .join("profile-sessions")
        .join(profile_id.to_string());
    Ok(dir)
}
