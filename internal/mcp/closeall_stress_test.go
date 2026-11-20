// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Stress coverage for issue #1141: the acceptance criterion says MCP shutdown
// must stay bounded "regardless of how many stdio servers are configured".
// These tests scale past the realistic fleet size (typical configs mount 1-5
// servers) and exercise mixed-failure profiles, so the concurrency model's
// "total cost = the slowest server" property is demonstrated rather than
// assumed from the 2-server case.

// TestCloseAll_StressManyUnresponsiveServers drives 24 hanging servers —
// an order of magnitude past any realistic configuration — and asserts the
// whole phase still finishes inside the 3s budget: with concurrent closing
// the wall time is the single slowest server (~1.6s), not the sum.
func TestCloseAll_StressManyUnresponsiveServers(t *testing.T) {
	pinProductionTerminateDuration(t)

	var clients []*Client
	for i := 0; i < 24; i++ {
		clients = append(clients, startEnvServer(t, fmt.Sprintf("hang-%02d", i), runAsHangingServerEnv))
	}

	start := time.Now()
	err := CloseAll(context.Background(), clients)
	elapsed := time.Since(start)

	if elapsed >= 3*time.Second {
		t.Errorf("24 unresponsive servers took %s to close, want well inside 3s", elapsed)
	}
	if err == nil {
		t.Fatal("CloseAll: expected errors for the SIGKILLed servers, got nil")
	}
	// Every server is named in the joined error, including the last one.
	for _, name := range []string{"hang-00", "hang-12", "hang-23"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("CloseAll error %q does not mention server %q", err, name)
		}
	}
}

// TestCloseAll_MixedFleetRealisticProfile models a realistic fleet: healthy
// servers, hung servers, and servers that exit non-zero, all closing at once.
// The total stays inside the budget, every failing server is named, and the
// healthy ones are not reported as failures.
func TestCloseAll_MixedFleetRealisticProfile(t *testing.T) {
	if raceDetectorEnabled {
		// Raced children notice stdin EOF ~1s late and, under the load of
		// ten concurrent children, can miss even the widened 2500ms stage —
		// healthy servers then get SIGTERM'd and pollute the error names.
		// The production timing semantics this test asserts are only
		// deterministic in non-race builds; CloseAll logic remains covered
		// under -race by the pinned-budget tests.
		t.Skip("child EOF-notice latency under -race breaks production timing; covered by non-race runs")
	}
	pinProductionTerminateDuration(t)

	var clients []*Client
	for i := 0; i < 4; i++ {
		clients = append(clients, startEnvServer(t, fmt.Sprintf("normal-%d", i), runAsServerEnv))
	}
	for i := 0; i < 4; i++ {
		clients = append(clients, startEnvServer(t, fmt.Sprintf("hang-%d", i), runAsHangingServerEnv))
	}
	for i := 0; i < 2; i++ {
		clients = append(clients, startEnvServer(t, fmt.Sprintf("exit3-%d", i), runAsExit3ServerEnv))
	}

	start := time.Now()
	err := CloseAll(context.Background(), clients)
	elapsed := time.Since(start)

	if elapsed >= 3*time.Second {
		t.Errorf("mixed fleet took %s to close, want well inside 3s", elapsed)
	}
	if err == nil {
		t.Fatal("CloseAll: expected errors from hung and exit-3 servers, got nil")
	}
	msgText := err.Error()
	for i := 0; i < 4; i++ {
		if !strings.Contains(msgText, fmt.Sprintf("hang-%d", i)) {
			t.Errorf("error %q does not mention hung server hang-%d", msgText, i)
		}
	}
	for i := 0; i < 2; i++ {
		if !strings.Contains(msgText, fmt.Sprintf("exit3-%d", i)) {
			t.Errorf("error %q does not mention exit-3 server exit3-%d", msgText, i)
		}
	}
	if strings.Contains(msgText, "normal-") {
		t.Errorf("error %q mentions healthy servers", msgText)
	}
}

// TestCloseAll_SlowCooperativeServerIsWaitedFor covers the distinction the
// escalation exists to make: a server that needs ~1.2s to wind down after
// stdin closes is waited for and exits cleanly (no error, no SIGKILL), while
// still finishing inside the budget. Only genuinely unresponsive servers
// should be killed.
func TestCloseAll_SlowCooperativeServerIsWaitedFor(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Process.Signal(SIGTERM) is unsupported on Windows, so the SDK
		// skips its SIGTERM wait and Kills right after the stdin-close
		// stage: the "waited for, clean wind-down" premise is POSIX-only.
		t.Skip("SIGTERM wind-down wait does not apply on Windows")
	}
	if raceDetectorEnabled {
		// The raced child notices stdin EOF ~1s late, so its 1.2s wind-down
		// self-exit lands after the SIGKILL escalation (1.6s) — production
		// semantics (waited for, clean exit) are only deterministic in
		// non-race builds.
		t.Skip("raced child wind-down misses its self-exit window; covered by non-race runs")
	}
	pinProductionTerminateDuration(t)

	client := startEnvServer(t, "slow-exit", runAsSlowExitServerEnv)

	start := time.Now()
	err := CloseAll(context.Background(), []*Client{client})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("CloseAll of a cooperative slow server: %v (want clean exit)", err)
	}
	if elapsed < time.Second {
		t.Errorf("CloseAll returned after %s; the slow wind-down should have been awaited", elapsed)
	}
	if elapsed >= 3*time.Second {
		t.Errorf("CloseAll took %s, want well inside 3s", elapsed)
	}
}
