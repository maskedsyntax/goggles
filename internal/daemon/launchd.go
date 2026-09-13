package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

const launchLabel = "com.maskedsyntax.goggles"

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchLabel+".plist"), nil
}

func InstallLaunchAgent(exe, logDir string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", apperr.New(apperr.DaemonUnavailable, "launchd install is only supported on macOS")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", err
	}
	path, err := plistPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>daemon</string>
    <string>run</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, launchLabel, exe, filepath.Join(logDir, "daemon.out.log"), filepath.Join(logDir, "daemon.err.log"))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := exec.Command("launchctl", "load", "-w", path).Run(); err != nil {
		uid := os.Getuid()
		if err2 := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", uid), path).Run(); err2 != nil {
			return path, apperr.Wrap(apperr.DaemonUnavailable, "installed plist but failed to load LaunchAgent", err)
		}
	}
	return path, nil
}

func UninstallLaunchAgent() error {
	if runtime.GOOS != "darwin" {
		return apperr.New(apperr.DaemonUnavailable, "launchd uninstall is only supported on macOS")
	}
	path, err := plistPath()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", path).Run()
	uid := os.Getuid()
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", uid, launchLabel)).Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func LaunchAgentStatus() (map[string]any, error) {
	path, err := plistPath()
	if err != nil {
		return nil, err
	}
	_, statErr := os.Stat(path)
	installed := statErr == nil
	loaded := false
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("launchctl", "list", launchLabel).CombinedOutput()
		loaded = err == nil && strings.Contains(string(out), launchLabel)
	}
	return map[string]any{
		"label":     launchLabel,
		"plist":     path,
		"installed": installed,
		"loaded":    loaded,
	}, nil
}

func ReadLogs(logDir string, lines int) (string, error) {
	if lines <= 0 {
		lines = 50
	}
	path := filepath.Join(logDir, "daemon.err.log")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	all := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n"), nil
}
