package main

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

// setupTerminal turns on UTF-8 output and ANSI colors in a Windows console. It reports false when the output
// isn't a console (redirected to a file, or running as a service).
func setupTerminal() bool {
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return false
	}
	_ = windows.SetConsoleOutputCP(65001) // UTF-8
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}

// openBrowser opens a link in the default browser (first start only, so setup is one click away).
func openBrowser(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
