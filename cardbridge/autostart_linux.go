//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Linux (Ubuntu and others): a systemd user service, plus an XDG autostart
// entry for desktops without a user systemd.
const linuxUnit = "starterp-cardbridge.service"

func osName() string { return "Linux" }

func linuxPaths() (unit, desktop string) {
	cfg, _ := os.UserConfigDir()
	return filepath.Join(cfg, "systemd", "user", linuxUnit), filepath.Join(cfg, "autostart", "starterp-cardbridge.desktop")
}

func installAutostart(exe string) error {
	unit, desktop := linuxPaths()
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unit, []byte(linuxUnitText(exe)), 0o644); err != nil {
		return err
	}
	if exec.Command("systemctl", "--user", "daemon-reload").Run() == nil &&
		exec.Command("systemctl", "--user", "enable", linuxUnit).Run() == nil {
		return nil
	}
	_ = os.Remove(unit)
	if err := os.MkdirAll(filepath.Dir(desktop), 0o755); err != nil {
		return err
	}
	return os.WriteFile(desktop, []byte(linuxDesktopText(exe)), 0o644)
}

func linuxUnitText(exe string) string {
	return "[Unit]\nDescription=StartERP Card Bridge\nAfter=network-online.target\n\n[Service]\nExecStart=" + quoteSystemd(exe) +
		" run --no-browser\nRestart=always\nRestartSec=5\n\n[Install]\nWantedBy=default.target\n"
}

func linuxDesktopText(exe string) string {
	return "[Desktop Entry]\nType=Application\nName=StartERP Card Bridge\nExec=" + quoteSystemd(exe) + " run --no-browser\nX-GNOME-Autostart-enabled=true\nNoDisplay=true\n"
}

func quoteSystemd(s string) string { return `"` + s + `"` }

func uninstallAutostart() error {
	unit, desktop := linuxPaths()
	_ = exec.Command("systemctl", "--user", "disable", linuxUnit).Run()
	_ = os.Remove(unit)
	_ = os.Remove(desktop)
	return nil
}

func autostartInstalled() bool {
	unit, desktop := linuxPaths()
	_, e1 := os.Stat(unit)
	_, e2 := os.Stat(desktop)
	return e1 == nil || e2 == nil
}

func openBrowser(u string) { _ = exec.Command("xdg-open", u).Start() }
