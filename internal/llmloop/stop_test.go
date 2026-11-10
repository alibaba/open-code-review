// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import "testing"

// MainLoopStop must be self-describing in both registers: String() for logs and
// telemetry, Reason() for the manifest reason and scan warning that are the only
// stop diagnostics surviving a --format json run. Neither may collide across
// stops, or a reader cannot tell which exit fired from the artifact alone.
func TestMainLoopStopStringAndReason(t *testing.T) {
	names := make(map[string]MainLoopStop)
	reasons := make(map[string]MainLoopStop)
	for _, tc := range []struct {
		stop     MainLoopStop
		wantName string
	}{
		{StopNone, "none"},
		{StopMaxRounds, "max_rounds"},
		{StopEmptyRounds, "empty_rounds"},
		{StopCompression, "compression"},
		{StopTokenBudget, "token_budget"},
	} {
		t.Run(tc.wantName, func(t *testing.T) {
			name := tc.stop.String()
			if name != tc.wantName {
				t.Errorf("String() = %q, want %q", name, tc.wantName)
			}
			reason := tc.stop.Reason()
			if reason == "" {
				t.Fatal("Reason() is empty; every stop needs a diagnostic sentence")
			}
			if prev, dup := names[name]; dup {
				t.Errorf("String() %q is shared by %v and %v", name, prev, tc.stop)
			}
			if prev, dup := reasons[reason]; dup {
				t.Errorf("Reason() %q is shared by %v and %v; stops must stay distinguishable", reason, prev, tc.stop)
			}
			names[name] = tc.stop
			reasons[reason] = tc.stop
		})
	}
}

// A value outside the enum must name itself in both registers rather than borrow
// the StopNone catch-all. This also guards the enum's growth: adding a constant
// after StopCompression makes this test fail, which is the prompt to give the new
// stop its own String() and Reason() case instead of letting it fall through to
// a message that says nothing.
func TestMainLoopStopUnknownValue(t *testing.T) {
	unknown := StopTokenBudget + 1

	if got, want := unknown.String(), "MainLoopStop(5)"; got != want {
		t.Errorf("String() = %q, want %q; a new constant needs its own case in String() and Reason()", got, want)
	}
	if got, want := unknown.Reason(), "main task stopped for an unrecognized reason (stop=5)"; got != want {
		t.Errorf("Reason() = %q, want %q; a new constant needs its own case in String() and Reason()", got, want)
	}
	if unknown.Reason() == StopNone.Reason() {
		t.Error("an unrecognized stop reuses the StopNone catch-all; the collapsed message is back")
	}
}
