//go:build !windows

package main

import "os"

// setupTerminal reports whether stdout is an interactive terminal that understands colors. Under systemd the
// output goes to the journal, where escape codes would be noise.
func setupTerminal() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0 && os.Getenv("TERM") != "dumb"
}

// openBrowser does nothing on Linux: the panel usually runs on a server without a desktop.
func openBrowser(string) {}
