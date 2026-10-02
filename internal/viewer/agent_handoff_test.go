// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// renderSessionPage writes a session file and renders its detail page.
func renderSessionPage(t *testing.T, lines ...string) string {
	t.Helper()
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	writeSession(t, repoDir, "s1", lines...)

	req := httptest.NewRequest("GET", "/r/repo/s1", nil)
	rr := httptest.NewRecorder()
	handleSession(rr, req, root, "repo", "s1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	return rr.Body.String()
}

// The agent handoff has to carry the finding's full context, or the agent it
// opens cannot act on the finding without a round trip back to the page.
func TestSessionPage_AgentActionsCarryContext(t *testing.T) {
	body := renderSessionPage(t,
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj","gitBranch":"feat/x"}`,
		`{"type":"review_item_done","filePath":"src/app.js","comments":[{"content":"Null deref on empty input.","category":"bug","severity":"high","start_line":10,"end_line":20,"suggestion_code":"const a = x?.y;"}]}`,
	)

	for _, want := range []string{
		`data-agent-actions`,
		`data-file="src/app.js"`,
		`data-start="10"`,
		`data-end="20"`,
		`data-category="bug"`,
		`data-severity="high"`,
		`data-branch="feat/x"`,
		`data-repo="/my/proj"`,
		`data-content="Null deref on empty input."`,
		`data-suggestion="const a = x?.y;"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("agent handoff missing %s", want)
		}
	}
}

// Targets are ordered app -> editor extension per vendor, because the app is
// what a user reaches for first and the extension is the last resort.
func TestSessionPage_AgentMenuOrdersAppBeforeExtension(t *testing.T) {
	body := renderSessionPage(t,
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj"}`,
		`{"type":"review_item_done","filePath":"a.go","comments":[{"content":"c","category":"bug","severity":"high"}]}`,
	)

	appAt := strings.Index(body, `data-agent-open="claude-app"`)
	extAt := strings.Index(body, `data-agent-open="claude-vscode"`)
	if appAt < 0 || extAt < 0 {
		t.Fatal("agent menu is missing a Claude Code app or extension target")
	}
	if appAt > extAt {
		t.Error("app target should precede the editor extension target")
	}

	// The CLI has no prefilled-prompt scheme, so the copy action is the path
	// and must be present rather than implied.
	if !strings.Contains(body, `data-agent-copy`) {
		t.Error("agent menu has no copy-prompt action")
	}
	if !strings.Contains(body, `data-agent-open="codex"`) || !strings.Contains(body, `data-agent-open="cursor"`) {
		t.Error("agent menu is missing the Codex or Cursor target")
	}
}

func TestSessionPage_CollapseAllControlPresent(t *testing.T) {
	body := renderSessionPage(t,
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj"}`,
		`{"type":"review_item_done","filePath":"a.go","comments":[{"content":"c","category":"bug","severity":"high"}]}`,
	)
	if !strings.Contains(body, `data-collapse-all`) {
		t.Error("session page has no collapse-all control")
	}
	// aria-pressed is what session.js toggles, and what a screen reader reads.
	if !strings.Contains(body, `aria-pressed="false"`) {
		t.Error("collapse-all control is missing its initial aria-pressed state")
	}
}

// The control belongs to the comments section, so a session with no findings
// must not render a button that would do nothing.
func TestSessionPage_NoCollapseAllWithoutComments(t *testing.T) {
	body := renderSessionPage(t,
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj"}`,
	)
	if strings.Contains(body, `data-collapse-all`) {
		t.Error("collapse-all rendered on a session with no findings")
	}
}

// An exported page has to keep the handoff working offline, which is the whole
// point of the export: no server, but the same actions.
func TestExportSession_InlinesAgentAndLiveScripts(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	writeSession(t, repoDir, "exp1",
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj"}`,
		`{"type":"review_item_done","filePath":"a.go","comments":[{"content":"c","category":"bug","severity":"high"}]}`,
	)

	var buf strings.Builder
	if err := ExportSession(&buf, root, "repo", "exp1"); err != nil {
		t.Fatal(err)
	}
	body := buf.String()

	for _, want := range []string{
		"claude-cli://open",
		"codex://new",
		"cursor://anysphere.cursor-deeplink/prompt",
		"vscode://anthropic.claude-code/open",
		`data-agent-actions`,
		`data-live-duration`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("exported page missing %s", want)
		}
	}
	// The exported copy must not try to fetch /static/ assets it no longer has.
	if strings.Contains(body, `src="/static/agent.js"`) {
		t.Error("exported page still links agent.js instead of inlining it")
	}
}
