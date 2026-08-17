//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"os/exec"
)

// newKeyCmd builds the OS-specific shell invocation (sh -c on Unix) that runs a
// credential command under ctx, so its timeout and cancellation are honored.
//
// Deliberately no SysProcAttr.Setpgid, even though it would let us SIGKILL the
// whole process group and so reap a grandchild the command backgrounded
// (`sleep 200 & printf tok` does outlive resolution today). Setpgid puts the
// child in a group that is not the terminal's foreground group, so the moment it
// reads the tty it takes SIGTTIN and stops -- measured: a child running
// `read -r x </dev/tty` answers in 7ms as written here, and under Setpgid
// produces nothing and is killed at the timeout instead. That read is exactly
// what pinentry, ssh-askpass and `op`'s fallback prompt do, which is the case
// c.Stdin = os.Stdin exists to support (see resolveKeyCmd). The group has to be
// chosen at Start, so this cannot be narrowed to the timeout path.
//
// Trading a working credential prompt for a process the user's own command
// asked to background -- one that a short-lived CLI outlives anyway, exactly as
// it would if they had typed the same line at a shell -- is the wrong side of
// that deal. Reaping it properly needs tcsetpgrp-style job control, which is far
// more machinery than the leak justifies.
func newKeyCmd(ctx context.Context, cmd string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", cmd)
}
