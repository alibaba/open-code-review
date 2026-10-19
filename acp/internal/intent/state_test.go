// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"context"
	"testing"
)

func TestStateLifecycle(t *testing.T) {
	st := NewState()
	if st.Pending() != nil {
		t.Fatal("new state should have no pending clarification")
	}

	st.setPending(&Pending{Action: "review", From: "main", Paths: []string{"a"}, Extra: []string{"--effort"}, Missing: []string{"to"}})
	got := st.Pending()
	if got.Action != "review" || got.From != "main" || len(got.Paths) != 1 || len(got.Missing) != 1 {
		t.Fatalf("pending = %+v", got)
	}

	// Pending must be a copy: mutating it cannot change stored state.
	got.From = "other"
	got.Paths[0] = "b"
	if again := st.Pending(); again.From != "main" || again.Paths[0] != "a" {
		t.Fatalf("stored pending was mutated through its copy: %+v", again)
	}

	st.Clear()
	if st.Pending() != nil {
		t.Fatal("Clear did not drop the pending clarification")
	}
}

func TestStateReplacesOnNewCommand(t *testing.T) {
	st := NewState()
	st.setPending(&Pending{Action: "review", From: "main", Missing: []string{"to"}})
	st.setPending(&Pending{Action: "scan", Missing: nil})
	if got := st.Pending(); got == nil || got.Action != "scan" {
		t.Fatalf("new command did not replace pending: %+v", got)
	}
}

func TestStateNilReceiver(t *testing.T) {
	var st *State
	if st.Pending() != nil {
		t.Fatal("nil state Pending should be nil")
	}
	st.Clear()
	st.setPending(&Pending{Action: "review"})
	st.setPending(nil)
}

func TestNewCommandReplacesPending(t *testing.T) {
	p := newTestParser(&fakeLLM{}, nil)
	st := NewState()
	if _, err := p.Parse(context.Background(), "/review --from main", st); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if st.Pending() == nil {
		t.Fatal("expected pending after incomplete range")
	}
	// A completed new command replaces and clears the pending clarification.
	r, err := p.Parse(context.Background(), "/scan", st)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	requireIntent(t, r)
	if st.Pending() != nil {
		t.Fatalf("pending not cleared by a new command: %+v", st.Pending())
	}
}

func TestRejectClearsPending(t *testing.T) {
	p := newTestParser(&fakeLLM{}, nil)
	st := NewState()
	if _, err := p.Parse(context.Background(), "/review --from main", st); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r, err := p.Parse(context.Background(), "/foo", st)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	requireReject(t, r)
	if st.Pending() != nil {
		t.Fatalf("reject did not clear pending: %+v", st.Pending())
	}
}
