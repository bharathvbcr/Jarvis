//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// stopProcessGroup signals whatever is left of the command's process group
// once the command is done. Nothing is left in the usual case, which is not an
// error.
func stopProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
}

// ownProcessGroup puts the command in its own process group and makes
// cancelling it signal the whole group. exec's own cancel kills only the direct
// child, so cancelling `go test` or `cargo` would leave the test binaries and
// rustc under it running. The terminal's Ctrl-C reaches `dev` alone, which
// cancels its context and so reaches the group through here.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
