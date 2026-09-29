//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"fmt"
	"syscall"
	"testing"
)

// A console-less agent cannot deliver CTRL_BREAK at all. That is the expected
// outcome of the cooperative interrupt, not a cleanup failure, so it must be
// classified as undeliverable while other Windows errors still surface.
func TestCtrlBreakUndeliverable(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"invalid handle", syscall.Errno(6), true},
		{"invalid parameter", syscall.Errno(87), true},
		{"wrapped invalid handle", fmt.Errorf("GenerateConsoleCtrlEvent: %w", syscall.Errno(6)), true},
		{"access denied", syscall.Errno(5), false},
	} {
		if got := ctrlBreakUndeliverable(tc.err); got != tc.want {
			t.Errorf("%s: ctrlBreakUndeliverable = %v, want %v", tc.name, got, tc.want)
		}
	}
}
