//go:build !windows

package selfupdate

import (
	"os"
	"syscall"
)

// Restart replaces this process with the (new) binary, keeping the PID (so systemd sees the same service).
func Restart(exe string) error {
	return syscall.Exec(exe, os.Args, append(os.Environ(), "LATCHADMIN_RESTARTED=1"))
}
