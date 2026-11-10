//go:build linux

package supervisor

import (
	"os/exec"
	"syscall"
)

// The server leads its own process group (so Kill reaches crashpad too) and is killed if the panel dies.
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
