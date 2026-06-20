//go:build unix

package skills

import (
	"os/exec"
	"syscall"
)

// configureGitProcessGroup hardens the bounded git invocation against an
// orphaned grandchild outliving the deadline (PRD #13 M4 carry-over from M3).
//
// git is started in its OWN process group (Setpgid), and the context-cancel
// hook SIGKILLs the WHOLE group (negative pid) rather than just git itself. The
// failure this closes: a wedged transport helper git spawned (e.g. a
// black-holed ssh) can inherit the stderr pipe and keep cmd.Run() blocked on
// that pipe well past the context deadline even after git is gone. Killing the
// group takes the grandchild with it; cmd.WaitDelay (set by the caller) is the
// belt-and-suspenders backstop that force-closes the pipe if the group kill
// still leaves something holding it.
func configureGitProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid targets the entire process group (git + grandchildren).
		// Fall back to killing just the process if the group signal fails.
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
