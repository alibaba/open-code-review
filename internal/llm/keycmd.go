// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// keyCmdTimeout bounds how long an api_key_cmd / auth_token_cmd may run.
// It is a package var (not const) so tests can shrink it.
var keyCmdTimeout = 60 * time.Second

// resolveKeyCmd runs a credential-fetching shell command and returns its
// trimmed, single-line stdout. label names the source (e.g.
// `api_key_cmd for provider "x"`) and is used in error messages.
//
// The child's stderr is wired to the process stderr so interactive prompts
// (pinentry, 1Password, `op`) stay visible. Any failure is a hard error, never
// a silent fallback. The resolved credential is used in memory only and is
// never written to config or logged.
func resolveKeyCmd(cmd, label string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), keyCmdTimeout)
	defer cancel()

	c := newKeyCmd(ctx, cmd)
	c.Stderr = os.Stderr

	out, err := c.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%s timed out after %s", label, keyCmdTimeout)
	}
	if err != nil {
		// Covers non-zero exit and command-not-found (the shell exits non-zero
		// and prints its not-found message on the child's stderr).
		return "", fmt.Errorf("%s failed: %w", label, err)
	}

	// Trim a trailing line break; multi-line output past that is ambiguous and refused.
	trimmed := strings.TrimRight(string(out), "\r\n")
	if strings.Contains(trimmed, "\n") {
		return "", fmt.Errorf("%s produced multi-line output; expected a single credential", label)
	}
	key := strings.TrimSpace(trimmed)
	if key == "" {
		return "", fmt.Errorf("%s produced empty output", label)
	}
	return key, nil
}
