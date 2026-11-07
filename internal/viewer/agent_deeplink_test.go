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

// The menu is absolutely positioned inside the card, and the card's group
// clips its overflow to round its corners, so an open menu was cut off at the
// group's bottom edge. The clip has to be lifted while a menu is open.
func TestAgentMenu_IsNotClippedByItsGroup(t *testing.T) {
	css, err := assets.ReadFile("static/style.css")
	if err != nil {
		t.Fatal(err)
	}
	style := string(css)
	if !strings.Contains(style, ".comment-file-group:has(.agent-menu:not([hidden]))") {
		t.Error("the comment group still clips the agent menu it contains")
	}
	if !strings.Contains(style, "overflow: visible") {
		t.Error("the agent menu's group does not lift its clip while a menu is open")
	}
}

// The prompt is truncated to fit the vendors' limits, and the vendors measure
// the URL-encoded value. Truncating on the raw character count would let a
// prompt of mostly non-ASCII text encode to several times the limit.
func TestAgentScript_TruncatesOnTheEncodedLength(t *testing.T) {
	src, err := assets.ReadFile("static/agent.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)
	if !strings.Contains(js, "encodedLength") {
		t.Error("agent.js does not measure the encoded prompt length")
	}
	if strings.Contains(js, "const MAX_PROMPT = 4000") {
		t.Error("agent.js still truncates the prompt on its raw character count")
	}
}

// The finding's text is read from the rendered card instead of being shipped
// again in a data attribute, which roughly doubled the size of a session page.
// The toggle's click used to stop propagating, which kept the page-level
// dismiss handler from seeing it: opening a second menu then left the first
// one open, so two dropdowns were on screen at once.
func TestAgentScript_ToggleDoesNotStopPropagation(t *testing.T) {
	src, err := assets.ReadFile("static/agent.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)
	if strings.Contains(js, ".stopPropagation(") {
		t.Error("agent.js stops the toggle's click from propagating, so the dismiss handler never sees it")
	}
}

// The workspace parameter is a file URI, and building it by concatenating
// "file://" with the path does not produce a valid URI for a Windows path
// (backslashes, no third slash) nor for a path containing "#" or "?", which
// would start a fragment or a query.
func TestAgentScript_BuildsTheWorkspaceURIWithTheURLParser(t *testing.T) {
	src, err := assets.ReadFile("static/agent.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)
	if strings.Contains(js, `"file://" + dir`) {
		t.Error("agent.js builds the workspace URI by concatenation, which is not a valid URI for every path")
	}
	if !strings.Contains(js, "new URL(") {
		t.Error("agent.js does not build the workspace URI with the URL parser")
	}
}

// Two copies of the dismissal can drift apart, and they already had: one
// closed the menu the page-level handler was looking at, the other the menu
// whose button was pressed.
func TestAgentScript_HasOneDismissal(t *testing.T) {
	src, err := assets.ReadFile("static/agent.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)
	if !strings.Contains(js, "closeMenu(root)") {
		t.Error("agent.js does not route the per-finding dismissal through closeMenu")
	}
	if strings.Contains(js, `menu.hidden = true;
            toggle.setAttribute("aria-expanded", "false");`) {
		t.Error("agent.js has a second copy of the dismissal logic")
	}
}

func TestSessionPage_DoesNotDuplicateTheFindingText(t *testing.T) {
	body := renderSessionPage(t,
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj"}`,
		`{"type":"review_item_done","filePath":"a.go","comments":[{"content":"a unique finding body","category":"bug","severity":"high"}]}`,
	)
	for _, unwanted := range []string{"data-content=", "data-existing=", "data-suggestion="} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the session page ships the finding text twice, in %s", unwanted)
		}
	}
	// The text still has to reach the page, or the handoff has nothing to send.
	if !strings.Contains(body, "a unique finding body") {
		t.Error("the finding body is not on the page for the handoff to read")
	}
}

// The collapse-all control's label carries its state, so a toggle attribute
// would announce the opposite state to the one the label describes.
func TestCollapseAll_LabelCarriesTheState(t *testing.T) {
	body := renderSessionPage(t,
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj"}`,
		`{"type":"review_item_done","filePath":"a.go","comments":[{"content":"c","category":"bug","severity":"high"}]}`,
	)
	if !strings.Contains(body, ">Collapse all</button>") {
		t.Error("the collapse-all control does not name the action it performs")
	}
	if strings.Contains(body, `data-collapse-all aria-pressed`) {
		t.Error("the collapse-all control has a toggle state that contradicts its label")
	}
}
