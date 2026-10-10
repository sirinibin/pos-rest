//go:build darwin

package main

import (
	"html"
	"os"
	"os/exec"
	"path/filepath"
)

// macOS: a LaunchAgent that starts the bridge when the user signs in and
// restarts it if it stops.
const macLabel = "com.starterp.cardbridge"

func osName() string { return "macOS" }

func macPlist() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", macLabel+".plist")
}

func macPlistText(exe string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>` + macLabel + `</string>
<key>ProgramArguments</key><array><string>` + html.EscapeString(exe) + `</string><string>run</string><string>--no-browser</string></array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
</dict></plist>
`
}

func installAutostart(exe string) error {
	p := macPlist()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(macPlistText(exe)), 0o644); err != nil {
		return err
	}
	return nil // loads at the next sign-in; this run keeps working now
}

func uninstallAutostart() error {
	p := macPlist()
	_ = exec.Command("launchctl", "unload", p).Run()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func autostartInstalled() bool {
	_, err := os.Stat(macPlist())
	return err == nil
}

func openBrowser(u string) { _ = exec.Command("open", u).Start() }
