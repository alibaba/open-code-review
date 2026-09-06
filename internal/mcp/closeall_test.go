// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// startEnvServer starts this test binary as the given child MCP server mode.
func startEnvServer(t *testing.T, name, env string) *Client {
	t.Helper()
	requireExec(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewClient(context.Background(), name, exe, nil, []string{env + "=1"}, "", "v0.0.1")
	if err != nil {
		t.Fatalf("NewClient(%s): %v", name, err)
	}
	return c
}

// pinProductionTerminateDuration restores the production shutdown budget for
// tests whose timing must not shift with the race detector's widened budget.
func pinProductionTerminateDuration(t *testing.T) {
	t.Helper()
	old := subprocessTerminateDuration
	subprocessTerminateDuration = SubprocessTerminateDuration
	t.Cleanup(func() { subprocessTerminateDuration = old })
}

// TestCloseAll_UnresponsiveServersStayWithinBudget covers the issue #1141
// acceptance criterion: total MCP shutdown stays well inside 3s no matter how
// unresponsive the configured servers are. The hanging children ignore
// SIGTERM, so each one exercises the full stdin-close -> SIGTERM -> SIGKILL
// escalation of the SDK before Close returns.
//
// The test pins the production TerminateDuration: a SIGKILLed child dies
// immediately even under the race detector, so the measured wall time is
// deterministic in both build modes.
func TestCloseAll_UnresponsiveServersStayWithinBudget(t *testing.T) {
	pinProductionTerminateDuration(t)

	clients := []*Client{
		startEnvServer(t, "hang-a", runAsHangingServerEnv),
		startEnvServer(t, "hang-b", runAsHangingServerEnv),
	}

	start := time.Now()
	err := CloseAll(context.Background(), clients)
	elapsed := time.Since(start)

	// POSIX: two escalation waits of 800ms each, then the SIGKILL lands.
	// Windows: Process.Signal(SIGTERM) is unsupported (EWINDOWS), so the SDK
	// skips its wait entirely and Kills right after the first stage — total
	// ~800ms, below the POSIX lower bound.
	if elapsed >= 3*time.Second {
		t.Errorf("CloseAll took %s, want well inside 3s", elapsed)
	}
	if runtime.GOOS != "windows" && elapsed < 1500*time.Millisecond {
		t.Errorf("CloseAll took %s; the SIGTERM stage should elapse before the SIGKILL", elapsed)
	}
	if err == nil {
		t.Fatal("CloseAll: expected errors for the SIGKILLed servers, got nil")
	}
	if runtime.GOOS != "windows" && !strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("CloseAll error = %v, want it to mention the killed subprocess", err)
	}
}

// TestCloseAll_ResponsiveServersCloseCleanly pins the unchanged behavior for
// well-behaved servers: close is quick and error-free.
func TestCloseAll_ResponsiveServersCloseCleanly(t *testing.T) {
	clients := []*Client{
		startEnvServer(t, "normal-a", runAsServerEnv),
		startEnvServer(t, "normal-b", runAsServerEnv),
	}

	start := time.Now()
	if err := CloseAll(context.Background(), clients); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
	// Under the race detector the children need ~1s to notice stdin EOF, so
	// the speed claim is only enforced in production builds; the error-free
	// close is asserted everywhere.
	if elapsed := time.Since(start); !raceDetectorEnabled && elapsed > time.Second {
		t.Errorf("responsive servers took %s to close, want well under 1s", elapsed)
	}
}

// TestCloseAll_JoinsPerServerErrorMessages checks the error aggregation: every
// failing server is named, in a single joined message, and quiet servers do
// not pollute it.
func TestCloseAll_JoinsPerServerErrorMessages(t *testing.T) {
	clients := []*Client{
		startEnvServer(t, "exit3-a", runAsExit3ServerEnv),
		startEnvServer(t, "normal", runAsServerEnv),
		startEnvServer(t, "exit3-b", runAsExit3ServerEnv),
	}

	err := CloseAll(context.Background(), clients)
	if err == nil {
		t.Fatal("CloseAll: expected joined errors from the exit-3 servers, got nil")
	}
	msg := err.Error()
	for _, name := range []string{"exit3-a", "exit3-b"} {
		if !strings.Contains(msg, name) {
			t.Errorf("CloseAll error %q does not mention server %q", msg, name)
		}
	}
	if !strings.Contains(msg, "exit status 3") {
		t.Errorf("CloseAll error %q does not mention the child exit status", msg)
	}
	if strings.Contains(msg, `"normal"`) {
		t.Errorf("CloseAll error %q mentions the responsive server", msg)
	}
}
