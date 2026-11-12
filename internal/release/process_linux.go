//go:build linux

package release

import (
	"os/exec"
	"syscall"
)

// Cancel the whole Chrome process group before removing its profile: helpers
// can outlive the main process and recreate a removed directory.
func browserCommand(cmd *exec.Cmd) func() {
	lowerPriority(cmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	return func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}
}

// lowerPriority wraps only the child; exec preserves its process group.
func lowerPriority(cmd *exec.Cmd) {
	for _, wrapper := range []struct {
		name string
		args []string
	}{
		{"ionice", []string{"-c", "2", "-n", "7"}},
		{"nice", []string{"-n", "10"}},
	} {
		if path, err := exec.LookPath(wrapper.name); err == nil {
			cmd.Args = append(append([]string{path}, wrapper.args...), append([]string{cmd.Path}, cmd.Args[1:]...)...)
			cmd.Path = path
		}
	}
}

func prioritizeCommand(cmd *exec.Cmd) { lowerPriority(cmd) }
