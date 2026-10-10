//go:build !linux && !darwin && !windows

package main

import "errors"

func osName() string                    { return "unknown" }
func installAutostart(exe string) error { return errors.New("not supported on this system") }
func uninstallAutostart() error         { return nil }
func autostartInstalled() bool          { return false }
func openBrowser(u string)              {}
