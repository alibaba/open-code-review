//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package process

import (
	"runtime"
	"syscall"
	"testing"
)

func TestKillProcessGroupRetriesOnlyDarwinPermissionRace(t *testing.T) {
	for _, final := range []error{nil, syscall.ESRCH, syscall.EINVAL} {
		calls := 0
		got := killUnixProcessGroup(123, func(pid int, signal syscall.Signal) error {
			if pid != -123 || signal != syscall.SIGKILL {
				t.Fatalf("kill(%d, %v)", pid, signal)
			}
			calls++
			if calls == 1 {
				return syscall.EPERM
			}
			return final
		})
		if runtime.GOOS != "darwin" {
			if got != syscall.EPERM || calls != 1 {
				t.Fatalf("non-Darwin result=%v calls=%d", got, calls)
			}
			continue
		}
		want := final
		if final == syscall.ESRCH {
			want = nil
		}
		if got != want || calls != 2 {
			t.Fatalf("result=%v want=%v calls=%d", got, want, calls)
		}
	}
}

func TestKillProcessGroupPreservesPersistentErrors(t *testing.T) {
	for _, failure := range []error{syscall.EPERM, syscall.EINVAL} {
		calls := 0
		got := killUnixProcessGroup(123, func(int, syscall.Signal) error { calls++; return failure })
		wantCalls := 1
		if runtime.GOOS == "darwin" && failure == syscall.EPERM {
			wantCalls = 11
		}
		if got != failure || calls != wantCalls {
			t.Fatalf("result=%v calls=%d want=%v calls=%d", got, calls, failure, wantCalls)
		}
	}
}
