// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"strings"
	"testing"
)

// A deep link is handed to the desktop only if the browser navigates to it
// inside a real user gesture. Building the href in the menu's own click handler
// and re-dispatching the click afterwards loses that gesture, and every current
// browser then silently refuses the external protocol - the click appears to do
// nothing, with no error to explain it. So the href must be built while the
// menu opens, and the link's own click must be left to the browser.
//
// Confirmed in headless Chromium against this exact regression: zero
// navigations requested before the fix, one after.
func TestAgentScript_DoesNotRedispatchClicks(t *testing.T) {
	src, err := assets.ReadFile("static/agent.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)

	if strings.Contains(js, ".click()") {
		t.Error("agent.js re-dispatches a click; the deep link will lose its user gesture and be dropped")
	}
	// The copy control is a <button> and does cancel its own default, which is
	// correct. The link handler is the one that must not.
	if strings.Contains(js, `item.addEventListener("click", (e)`) {
		t.Error("a deep-link handler still takes an event; it should close the menu and let the browser navigate")
	}
	// The href has to be assigned from the open path, not the click path.
	if !strings.Contains(js, "buildLinks") {
		t.Error("agent.js no longer builds link hrefs; the open path must set them")
	}
}

// A document-level listener per finding would mean a session with hundreds of
// findings registers hundreds of them, all of which run on every click anywhere
// in the viewer. The dismiss handler must be installed once for the whole page.
func TestAgentScript_DismissListenerIsNotPerFinding(t *testing.T) {
	src, err := assets.ReadFile("static/agent.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)

	if strings.Count(js, `document.addEventListener("click"`) > 1 {
		t.Errorf("agent.js registers %d document click listeners; it must register exactly one",
			strings.Count(js, `document.addEventListener("click"`))
	}
	if !strings.Contains(js, "bindDismiss") {
		t.Error("agent.js no longer installs the dismiss listener once per page")
	}
}

// A link whose shipped href is "#" would scroll the page if the script ever
// failed to replace it, and would look like a working link while doing nothing.
func TestAgentMenu_PlaceholderHrefIsInert(t *testing.T) {
	body := renderSessionPage(t,
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj"}`,
		`{"type":"review_item_done","filePath":"a.go","comments":[{"content":"c","category":"bug","severity":"high"}]}`,
	)
	if strings.Contains(body, `data-agent-open="codex" href="#"`) {
		t.Error("agent menu ships a '#' placeholder href, which scrolls the page when followed")
	}
	// Every deep-link anchor must ship an href attribute at all, or it is not
	// focusable and cannot be activated by keyboard.
	if !strings.Contains(body, `data-agent-open="codex" href=`) {
		t.Error("deep-link anchors ship without an href and are not keyboard reachable")
	}
}
