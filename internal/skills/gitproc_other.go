//go:build !unix

package skills

import "os/exec"

// configureGitProcessGroup is a no-op on platforms without POSIX process
// groups. The default exec.CommandContext cancel (kill the single git process)
// plus the caller's cmd.WaitDelay still bound the run and force-close the
// stderr pipe, so the deadline is honored even without group signaling.
func configureGitProcessGroup(cmd *exec.Cmd) {}
