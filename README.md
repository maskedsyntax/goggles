# Goggles

A Tauri 2 desktop app for browsing Threads, X, and Instagram across isolated profiles.

Each profile keeps its own cookies and login state. Tabs inside a profile share that session.

## Requirements

- Node.js 18+
- Rust (stable)
- macOS 14+ (Sonoma) for session isolation via `WKWebsiteDataStore`

## Develop

```sh
npm install
npm run tauri dev
```

## Build

```sh
npm run tauri build
```
