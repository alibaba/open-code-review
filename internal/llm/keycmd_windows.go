//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"os/exec"
	"syscall"
)

// newKeyCmd builds the OS-specific shell invocation (cmd.exe /C on Windows) that runs a
// credential command under ctx, so its timeout and cancellation are honored.
// Spelled with the extension so a file named `cmd` on PATH cannot shadow the shell.
//
// The command line is handed over through SysProcAttr.CmdLine instead of Args
// because os/exec quotes Args with syscall.EscapeArg, which targets
// CommandLineToArgvW; cmd.exe is a documented exception with different unquoting
// rules (see the exec.Command doc comment), and its escaping mangles any command
// containing a double quote -- `op read "op://Private/My Vault/api-key"` would
// arrive as a single literal filename. /S makes cmd.exe strip exactly the outer
// pair of quotes we add and pass the rest through verbatim.
//
// Not escaping the interpolated cmd is deliberate rather than an injection hole:
// api_key_cmd is a command line its author asked us to run, so they already have
// arbitrary execution by design (`api_key_cmd = "whoami"` is a supported config,
// and the Unix arm hands the same string to `sh -c`), and it is read only from
// the user-level ~/.opencodereview/config.json -- never from the repository
// under review. Escaping the inner quotes would defeat the single case CmdLine
// exists for. See keycmd_windows_test.go for which quote shapes /S does and does
// not keep as one command.
//
// Note that a command string is not portable between the two arms: %VAR% and ^
// are cmd.exe metacharacters and $VAR expansion / \ escaping do not apply, so an
// sh-authored api_key_cmd generally needs a Windows-specific rewrite.
func newKeyCmd(ctx context.Context, cmd string) *exec.Cmd {
	// Still built by CommandContext so ctx cancellation and WaitDelay behave
	// exactly as on Unix; only the command-line construction differs.
	c := exec.CommandContext(ctx, "cmd.exe")
	// CmdLine is the whole command line including argv[0]; the executable itself
	// still comes from c.Path. Args stays at Command's default ([]string{"cmd.exe"})
	// rather than nil: syscall.StartProcess uses SysProcAttr.CmdLine verbatim when
	// non-empty and never looks at argv, so the doc's "leaving Args empty" is not
	// load-bearing here -- and a one-element Args keeps Cmd.String() from panicking
	// on Args[1:].
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /S /C "` + cmd + `"`}
	return c
}
