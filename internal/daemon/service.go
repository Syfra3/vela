package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ---------------------------------------------------------------------------
// System service installer (spec §6.6)
// ---------------------------------------------------------------------------

// InstallService installs the Vela watch daemon as a system service.
// On Linux it creates a systemd user unit; on macOS a launchd plist.
func InstallService(pidFile, logFile string) error {
	switch runtime.GOOS {
	case "linux":
		return installSystemd(pidFile, logFile)
	case "darwin":
		return installLaunchd(pidFile, logFile)
	default:
		return fmt.Errorf("service installation not supported on %s", runtime.GOOS)
	}
}

// UninstallService removes the installed system service.
func UninstallService() error {
	switch runtime.GOOS {
	case "linux":
		return uninstallSystemd()
	case "darwin":
		return uninstallLaunchd()
	default:
		return fmt.Errorf("service removal not supported on %s", runtime.GOOS)
	}
}

// ServiceInstalled reports whether the user-level service definition exists on
// the current platform.
func ServiceInstalled() bool {
	switch runtime.GOOS {
	case "linux":
		_, err := os.Stat(systemdUnitPath())
		return err == nil
	case "darwin":
		_, err := os.Stat(launchdPlistPath())
		return err == nil
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// systemd (Linux)
// ---------------------------------------------------------------------------

const systemdUnit = `[Unit]
Description=Vela Watch Daemon — real-time knowledge graph updates
After=network.target

[Service]
Type=simple
ExecStart=%s watch start --foreground
Restart=on-failure
RestartSec=5s
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=default.target
`

func systemdUnitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "vela-watch.service")
}

func installSystemd(pidFile, logFile string) error {
	velaPath, err := exec.LookPath("vela")
	if err != nil {
		return fmt.Errorf("vela not found in PATH: %w", err)
	}

	unitContent := fmt.Sprintf(systemdUnit, velaPath, logFile, logFile)
	unitPath := systemdUnitPath()

	if err := os.MkdirAll(filepath.Dir(unitPath), 0755); err != nil {
		return fmt.Errorf("creating systemd dir: %w", err)
	}
	if err := os.WriteFile(unitPath, []byte(unitContent), 0644); err != nil {
		return fmt.Errorf("writing unit file: %w", err)
	}

	// Reload systemd and enable the unit.
	for _, args := range [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", "vela-watch.service"},
	} {
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %s: %w\n%s", strings.Join(args, " "), err, out)
		}
	}

	fmt.Printf("Installed systemd service at %s\n", unitPath)
	fmt.Println("Start it with: systemctl --user start vela-watch")
	return nil
}

func uninstallSystemd() error {
	unitPath := systemdUnitPath()
	for _, args := range [][]string{
		{"--user", "stop", "vela-watch.service"},
		{"--user", "disable", "vela-watch.service"},
	} {
		// Ignore errors — the service may already be stopped.
		_, _ = exec.Command("systemctl", args...).CombinedOutput()
	}
	_ = os.Remove(unitPath)
	_, _ = exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput()
	fmt.Println("Removed vela-watch systemd service")
	return nil
}

// ---------------------------------------------------------------------------
// launchd (macOS)
// ---------------------------------------------------------------------------

const launchdPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.syfra3.vela-watch</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>watch</string>
    <string>start</string>
    <string>--foreground</string>
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
`

func launchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.syfra3.vela-watch.plist")
}

func installLaunchd(pidFile, logFile string) error {
	velaPath, err := exec.LookPath("vela")
	if err != nil {
		return fmt.Errorf("vela not found in PATH: %w", err)
	}

	plistContent := fmt.Sprintf(launchdPlist, velaPath, logFile, logFile)
	plistPath := launchdPlistPath()

	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return fmt.Errorf("creating LaunchAgents dir: %w", err)
	}
	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("writing plist: %w", err)
	}

	if out, err := exec.Command("launchctl", "load", plistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load: %w\n%s", err, out)
	}

	fmt.Printf("Installed launchd agent at %s\n", plistPath)
	return nil
}

func uninstallLaunchd() error {
	plistPath := launchdPlistPath()
	if _, err := os.Stat(plistPath); err == nil {
		_, _ = exec.Command("launchctl", "unload", plistPath).CombinedOutput()
	}
	_ = os.Remove(plistPath)
	fmt.Println("Removed vela-watch launchd agent")
	return nil
}
