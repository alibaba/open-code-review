package viewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestViewerIgnoresUnknownSessionEventTypes(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repo")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "" +
		`{"type":"session_start","sessionId":"run-1","timestamp":"2026-06-30T00:00:00Z","cwd":"/repo","reviewMode":"agent","controlPlane":"agent","bundleId":"sha256:bundle","tokenUsage":"not_available"}` + "\n" +
		`{"type":"legacy_event","sessionId":"run-1","timestamp":"2026-06-30T00:00:01Z","event":"context.search","bundleId":"sha256:bundle","duration_ms":5}` + "\n" +
		`{"type":"session_end","sessionId":"run-1","timestamp":"2026-06-30T00:00:02Z","duration_seconds":2,"files_reviewed":["main.go"],"llm_failures":0,"controlPlane":"agent","bundleId":"sha256:bundle","tokenUsage":"not_available"}` + "\n"
	if err := os.WriteFile(filepath.Join(repository, "run-1.jsonl"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	summaries, err := ListSessions(root, "repo")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
	session, err := LoadSession(root, "repo", "run-1")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if len(session.AgentEvents) != 0 || session.Summary.ControlPlane != "agent" {
		t.Fatalf("session = %+v", session)
	}
}

func TestViewerLoadsAgentSession(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repo")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "" +
		`{"type":"session_start","sessionId":"run-1","timestamp":"2026-06-30T00:00:00Z","cwd":"/repo","reviewMode":"agent","controlPlane":"agent","bundleId":"sha256:bundle","tokenUsage":"not_available"}` + "\n" +
		`{"type":"agent_event","sessionId":"run-1","timestamp":"2026-06-30T00:00:01Z","event":"validate","bundleId":"sha256:bundle","duration_ms":5,"files":3,"findings":2,"warnings":1,"validation_valid":true}` + "\n" +
		`{"type":"session_end","sessionId":"run-1","timestamp":"2026-06-30T00:00:02Z","duration_seconds":2,"files_reviewed":["main.go"],"llm_failures":0,"controlPlane":"agent","bundleId":"sha256:bundle","tokenUsage":"not_available"}` + "\n"
	if err := os.WriteFile(filepath.Join(repository, "run-1.jsonl"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	summaries, err := ListSessions(root, "repo")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].ControlPlane != "agent" ||
		summaries[0].BundleID != "sha256:bundle" ||
		summaries[0].TokenUsageAvailable {
		t.Fatalf("summaries = %+v", summaries)
	}
	session, err := LoadSession(root, "repo", "run-1")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if len(session.AgentEvents) != 1 ||
		session.AgentEvents[0].Event != "validate" ||
		session.AgentEvents[0].Files != 3 ||
		session.AgentEvents[0].Findings != 2 ||
		session.AgentEvents[0].Warnings != 1 ||
		session.AgentEvents[0].ValidationValid == nil ||
		!*session.AgentEvents[0].ValidationValid {
		t.Fatalf("session = %+v", session)
	}
}

func TestViewerSummaryUsesSessionEndBeforeLateAgentEvent(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repo")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "" +
		`{"type":"session_start","sessionId":"run-1","timestamp":"2026-06-30T00:00:00Z","cwd":"/repo","reviewMode":"agent","controlPlane":"agent","bundleId":"sha256:bundle","tokenUsage":"not_available"}` + "\n" +
		`{"type":"session_end","sessionId":"run-1","timestamp":"2026-06-30T00:00:02Z","duration_seconds":2,"files_reviewed":["main.go"],"llm_failures":0,"controlPlane":"agent","bundleId":"sha256:bundle","tokenUsage":"not_available"}` + "\n" +
		`{"type":"agent_event","sessionId":"run-1","timestamp":"2026-06-30T00:00:03Z","event":"context.read","bundleId":"sha256:bundle","context_calls":1}` + "\n"
	if err := os.WriteFile(filepath.Join(repository, "run-1.jsonl"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	summaries, err := ListSessions(root, "repo")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(summaries) != 1 ||
		summaries[0].DurationSec != 2 ||
		summaries[0].FileCount != 1 ||
		summaries[0].FilesReviewed[0] != "main.go" {
		t.Fatalf("summary = %+v, want session_end data despite late event", summaries)
	}
}

func TestAgentEventValidationLabelDistinguishesFalse(t *testing.T) {
	valid := true
	invalid := false
	tests := []struct {
		name  string
		event AgentEvent
		want  string
	}{
		{name: "missing", event: AgentEvent{}, want: "-"},
		{name: "valid", event: AgentEvent{ValidationValid: &valid}, want: "yes"},
		{name: "invalid", event: AgentEvent{ValidationValid: &invalid}, want: "no"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.event.ValidationLabel(); got != tc.want {
				t.Fatalf("ValidationLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestViewerTemplateDistinguishesAgentControlPlaneAndUnavailableTokens(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("templates", "session.html"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, fragment := range []string{
		"Control plane:",
		"Bundle:",
		"Token usage is not available",
		"Agent workflow events",
		"Findings",
		"Context",
		"ValidationLabel",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("session template missing %q", fragment)
		}
	}
}
