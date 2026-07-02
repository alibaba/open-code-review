package viewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestViewerLoadsCodexOwnedSession(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repo")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "" +
		`{"type":"session_start","sessionId":"run-1","timestamp":"2026-06-30T00:00:00Z","cwd":"/repo","reviewMode":"workspace","controlPlane":"codex-owned","bundleId":"sha256:bundle","tokenUsage":"not_available"}` + "\n" +
		`{"type":"codex_event","sessionId":"run-1","timestamp":"2026-06-30T00:00:01Z","event":"context.search","bundleId":"sha256:bundle","duration_ms":5}` + "\n" +
		`{"type":"session_end","sessionId":"run-1","timestamp":"2026-06-30T00:00:02Z","duration_seconds":2,"files_reviewed":["main.go"],"llm_failures":0,"controlPlane":"codex-owned","bundleId":"sha256:bundle","tokenUsage":"not_available"}` + "\n"
	if err := os.WriteFile(filepath.Join(repository, "run-1.jsonl"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	summaries, err := ListSessions(root, "repo")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].ControlPlane != "codex-owned" ||
		summaries[0].BundleID != "sha256:bundle" ||
		summaries[0].TokenUsageAvailable {
		t.Fatalf("summaries = %+v", summaries)
	}
	session, err := LoadSession(root, "repo", "run-1")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if len(session.CodexEvents) != 1 ||
		session.CodexEvents[0].Event != "context.search" {
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
		`{"type":"agent_event","sessionId":"run-1","timestamp":"2026-06-30T00:00:01Z","event":"context.search","bundleId":"sha256:bundle","duration_ms":5}` + "\n" +
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
	if len(session.CodexEvents) != 1 ||
		session.CodexEvents[0].Event != "context.search" {
		t.Fatalf("session = %+v", session)
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
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("session template missing %q", fragment)
		}
	}
}
