// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sessionFixture writes a session JSONL under root/repo and returns root.
func sessionFixture(t *testing.T, lines ...string) string {
	t.Helper()
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, "sess1.jsonl"), lines...)
	return root
}

const (
	fixtureStart    = `{"type":"session_start","timestamp":"2025-06-10T08:00:00Z","cwd":"/home/dev/proj","gitBranch":"feat","model":"claude-3","reviewMode":"commit","diffCommit":"ccc"}`
	fixtureRequest  = `{"type":"llm_request","filePath":"main.go","taskType":"main_task","request_no":1,"messages":[{"role":"user","content":"review this"}]}`
	fixtureResponse = `{"type":"llm_response","filePath":"main.go","taskType":"main_task","content":"Code looks good","duration_ms":1500,"model":"claude-3","usage":{"prompt_tokens":100,"completion_tokens":50}}`
	fixtureEnd      = `{"type":"session_end","duration_seconds":120.5,"files_reviewed":["main.go"],"llm_failures":0}`
)

// TestExportSession_SelfContained is the whole point of the feature: the page
// must render offline, so both /static/ assets have to arrive inlined and
// correctly typed. A plain string in <style>/<script> is what html/template
// rejects - it renders the literal ZgotmplZ for CSS and a JSON-escaped string
// literal (\u003c, never a bare <) for JS - so asserting on real asset content
// rather than on the presence of a <style> tag is what makes this test fail
// against the naive implementation.
func TestExportSession_SelfContained(t *testing.T) {
	root := sessionFixture(t, fixtureStart, fixtureRequest, fixtureResponse, fixtureEnd)

	var buf bytes.Buffer
	if err := ExportSession(&buf, root, "repo", "sess1"); err != nil {
		t.Fatalf("ExportSession: %v", err)
	}
	body := buf.String()

	for _, want := range []string{
		"--font: -apple-system",                 // style.css:5, inlined verbatim
		`'<code class="inline-code">$1</code>'`, // session.js:17, inlined verbatim
		`<span class="crumb">proj</span>`,       // the repo breadcrumb, de-linked
		"sess1",                                 // the session itself rendered
	} {
		if !strings.Contains(body, want) {
			t.Errorf("export missing %q", want)
		}
	}

	for _, bad := range []string{
		"ZgotmplZ",                 // untyped CSS: the stylesheet silently vanished
		`\u003c`,                   // untyped JS: the script became a JSON string literal
		`href="/static/style.css"`, // still fetching over http
		`src="/static/session.js"`, // still fetching over http
		`<a href="/"`,              // nav-brand link, dead over file://
		`href="/r/`,                // repo link, dead over file://
	} {
		if strings.Contains(body, bad) {
			t.Errorf("export contains %q", bad)
		}
	}
}

func TestExportSession_Variants(t *testing.T) {
	tests := []struct {
		name    string
		lines   []string
		want    []string
		notWant []string
	}{
		{
			name:  "aborted session has no session_end record",
			lines: []string{fixtureStart, fixtureRequest, fixtureResponse},
			want:  []string{"aborted", "--font: -apple-system"},
		},
		{
			// The heading, not the bare phrase: the inlined stylesheet carries
			// a "Review Comments Section" comment of its own, so a substring
			// assertion on the page body has to pin the template's own form.
			name:    "session without comments omits the findings section",
			lines:   []string{fixtureStart, fixtureRequest, fixtureResponse, fixtureEnd},
			notWant: []string{"<h3>Review Comments ("},
		},
		{
			name:  "session_start only still renders",
			lines: []string{fixtureStart},
			want:  []string{"sess1", "--font: -apple-system"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sessionFixture(t, tt.lines...)
			var buf bytes.Buffer
			if err := ExportSession(&buf, root, "repo", "sess1"); err != nil {
				t.Fatalf("ExportSession: %v", err)
			}
			body := buf.String()
			for _, want := range tt.want {
				if !strings.Contains(body, want) {
					t.Errorf("export missing %q", want)
				}
			}
			for _, bad := range tt.notWant {
				if strings.Contains(body, bad) {
					t.Errorf("export contains %q", bad)
				}
			}
		})
	}
}

// TestExportSession_Errors pins that a failure writes nothing, so a caller
// pointing at a file cannot end up with a half-rendered page.
func TestExportSession_Errors(t *testing.T) {
	t.Run("unknown session id", func(t *testing.T) {
		root := sessionFixture(t, fixtureStart, fixtureEnd)
		var buf bytes.Buffer
		err := ExportSession(&buf, root, "repo", "nope")
		if err == nil {
			t.Fatal("expected an error for an unknown session id")
		}
		if !strings.Contains(err.Error(), "nope") {
			t.Errorf("error %q does not name the session", err)
		}
		if buf.Len() != 0 {
			t.Errorf("wrote %d bytes on failure, want 0", buf.Len())
		}
	})

	// A directory where the JSONL belongs is the cheap route to a non-nil
	// readErr from LoadSession: a truncated file returns nil at EOF.
	t.Run("directory at the session path", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "repo", "sess1.jsonl"), 0755); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := ExportSession(&buf, root, "repo", "sess1"); err == nil {
			t.Fatal("expected an error when the session path is a directory")
		}
		if buf.Len() != 0 {
			t.Errorf("wrote %d bytes on failure, want 0", buf.Len())
		}
	})
}

// TestEmbeddedAssetsHaveNoTerminators is the static half of the inlining
// contract. template.CSS and template.JS pass their contents through
// unescaped, so an asset containing "</style" or "</script" would break out of
// the tag it was inlined into. Both are compile-time embedded, so checking
// them here is cheaper and stricter than a runtime guard.
func TestEmbeddedAssetsHaveNoTerminators(t *testing.T) {
	for _, tt := range []struct{ path, terminator string }{
		{"static/style.css", "</style"},
		{"static/session.js", "</script"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			b, err := assets.ReadFile(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.ToLower(string(b)), tt.terminator) {
				t.Errorf("%s contains %q; inlining it would break out of the tag", tt.path, tt.terminator)
			}
		})
	}
}
