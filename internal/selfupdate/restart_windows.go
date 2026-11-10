//go:build windows

package selfupdate

import (
	"os"
	"os/exec"
)

// Restart starts the (new) binary with the same arguments and exits this process.
func Restart(exe string) error {
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "LATCHADMIN_RESTARTED=1")
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
