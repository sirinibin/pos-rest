//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Windows: a small launcher in the user's Startup folder (no administrator
// rights needed). The bridge then starts hidden when the user signs in.
func osName() string { return "Windows" }

func winStartup() string {
	return filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "StartERP Card Bridge.vbs")
}

func winLauncherText(exe string) string {
	// VBScript runs the bridge without a console window
	return "Set s = CreateObject(\"WScript.Shell\")\r\ns.Run \"\"\"" + exe + "\"\" run --no-browser\", 0, False\r\n"
}

func installAutostart(exe string) error {
	p := winStartup()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(winLauncherText(exe)), 0o644)
}

func uninstallAutostart() error {
	if err := os.Remove(winStartup()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func autostartInstalled() bool {
	_, err := os.Stat(winStartup())
	return err == nil
}

func openBrowser(u string) { _ = exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start() }
