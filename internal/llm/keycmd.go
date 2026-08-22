// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// keyCmdTimeout bounds how long an api_key_cmd / auth_token_cmd may run.
// It is a package var (not const) so tests can shrink it.
var keyCmdTimeout = 60 * time.Second

// keyCmdWaitDelay bounds how long Wait keeps waiting on the child's stdout pipe
// after the command's own deadline has passed. Package var (not const) so tests
// can shrink it, same as keyCmdTimeout.
var keyCmdWaitDelay = 5 * time.Second

// keyCmdMaxOutput caps how much of a credential command's stdout we buffer.
const keyCmdMaxOutput = 64 << 10

// errKeyCmdOutputTooLarge aborts the stdout copy once the cap is hit. It never
// reaches the caller: cappedBuffer.overflow is what produces the error message.
var errKeyCmdOutputTooLarge = errors.New("credential command output exceeds cap")

// cappedBuffer collects at most max bytes and records whether more were offered.
// Refusing the write makes os/exec's copier close the pipe, so a runaway command
// (`cat /dev/urandom`) dies of SIGPIPE instead of growing our heap without bound.
type cappedBuffer struct {
	max      int
	buf      bytes.Buffer
	overflow bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len()+len(p) > b.max {
		b.overflow = true
		return 0, errKeyCmdOutputTooLarge
	}
	return b.buf.Write(p)
}

// resolveKeyCmd runs a credential-fetching shell command and returns its
// trimmed, single-line stdout. label names the source (e.g.
// `api_key_cmd for provider "x"`) and is used in error messages.
//
// The child's stderr is wired to the process stderr so interactive prompts
// (pinentry, 1Password, `op`) stay visible, and its stdin to the process stdin
// so those prompts can be answered. Any failure is a hard error, never a silent
// fallback. The resolved credential is used in memory only and is never written
// to config or logged.
func resolveKeyCmd(cmd, label string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), keyCmdTimeout)
	defer cancel()

	c := newKeyCmd(ctx, cmd)
	c.Stderr = os.Stderr
	// With Stdin nil, os/exec hands the child /dev/null, so a helper that needs
	// to prompt for a passphrase gets EOF or refuses to prompt at all because it
	// sees no tty. Safe to hand over os.Stdin because no code path resolves an
	// endpoint while the bubbletea TUI (which also reads os.Stdin) is running:
	// ResolveEndpoint's only callers are the non-TUI review/scan and `ocr llm
	// test` paths. Adding an in-TUI connection test would break that.
	c.Stdin = os.Stdin
	// Buffer stdout through cappedBuffer rather than an *os.File so os/exec does
	// the copying in its own goroutine: that is what lets WaitDelay force the
	// pipe closed. exec.CommandContext SIGKILLs only the shell, so a grandchild
	// (gpg-agent, pinentry, `op`) that inherited the stdout pipe keeps it open
	// and Wait blocks on the read long past the timeout -- reproducible with
	// api_key_cmd = "sleep 200 & printf tok". WaitDelay makes Wait give up
	// shortly after the context dies.
	out := &cappedBuffer{max: keyCmdMaxOutput}
	c.Stdout = out
	c.WaitDelay = keyCmdWaitDelay

	err := c.Run()
	// Checked first so a timeout reports as such instead of as the SIGKILL exit
	// status it produces. (Run has already joined every stdout copier, so the
	// buffer below is safe to read on all paths.)
	if ctx.Err() == context.DeadlineExceeded {
		// Wrap ctx.Err() so callers can errors.Is(err, context.DeadlineExceeded).
		return "", fmt.Errorf("%s timed out after %s: %w", label, keyCmdTimeout, ctx.Err())
	}
	if out.overflow {
		return "", fmt.Errorf("%s produced more than 64KiB of output", label)
	}
	// ErrWaitDelay only means an orphaned grandchild still holds the pipe; the
	// command itself exited fine and its output is already buffered, so use it
	// rather than surfacing an exec-internal error.
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		// Covers non-zero exit and command-not-found (the shell exits non-zero
		// and prints its not-found message on the child's stderr). ExitError.Stderr
		// stays nil because we assigned c.Stderr, so no output can leak here.
		return "", fmt.Errorf("%s failed: %w", label, err)
	}

	// Trim a trailing line break; multi-line output past that is ambiguous and refused.
	// ContainsAny (not Contains "\n") so a lone interior CR is caught too: TrimRight
	// leaves it, TrimSpace below only strips the edges, and a CR inside a credential
	// makes net/http reject the Authorization header with an opaque error.
	trimmed := strings.TrimRight(out.buf.String(), "\r\n")
	if strings.ContainsAny(trimmed, "\n\r") {
		return "", fmt.Errorf("%s produced multi-line output; expected a single credential (pipe through 'head -n1' if your command prints more)", label)
	}
	// Same reason as the line-break check, wider net: httpguts.ValidHeaderFieldValue
	// (what net/http enforces) rejects every byte below 0x20 except SP and TAB, plus
	// DEL. A NUL or VT smuggled in by e.g. `printf 'sk-a\0b'` would otherwise reach
	// net/http as the opaque `invalid header field value for "Authorization"`.
	//
	// Deliberately before the TrimSpace below, so a trailing control byte is an
	// error naming its offset rather than silently stripped: only TAB, SP and the
	// line breaks already handled above are things a credential command can
	// plausibly append by accident. Offsets are therefore into the pre-TrimSpace
	// string, which is what the command actually produced.
	for i := 0; i < len(trimmed); i++ {
		if b := trimmed[i]; (b < 0x20 && b != '\t') || b == 0x7f {
			return "", fmt.Errorf("%s produced a control byte 0x%02X at offset %d; a credential must not contain control characters", label, b, i)
		}
	}

	key := strings.TrimSpace(trimmed)
	if key == "" {
		return "", fmt.Errorf("%s produced empty output", label)
	}
	return key, nil
}
