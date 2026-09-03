// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestStartServer_SessionsRootError forces os.UserHomeDir to fail by clearing
// HOME so StartServer returns before binding a socket.
func TestStartServer_SessionsRootError(t *testing.T) {
	t.Setenv("HOME", "")
	// On unix os.UserHomeDir errors when HOME is empty.
	if _, err := SessionsRoot(); err == nil {
		t.Skip("home dir resolvable despite empty HOME; platform-specific")
	}
	if err := StartServer("127.0.0.1:0", OpenNever); err == nil {
		t.Fatal("expected StartServer to fail when sessions root cannot resolve")
	}
}

// TestStartServer_AddrInUse runs the full setup path (routes, host guard,
// security headers, server construction) and then fails fast on ListenAndServe
// because the port is already bound — no goroutine leak.
func TestStartServer_AddrInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()

	err = StartServer(ln.Addr().String(), OpenNever)
	if err == nil {
		t.Fatal("expected StartServer to fail binding an in-use address")
	}
}

// TestDisplayURL pins the host to the *requested* address and the port to the
// listener. Deriving the host from the listener instead makes the printed URL
// disagree with the hostGuard allowlist, which is built from the requested one:
// see TestDisplayURL_AgreesWithHostGuard.
func TestDisplayURL(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		listener  string
		want      string
		wantErr   bool
	}{
		{"default", "localhost:5483", "127.0.0.1:5483", "http://localhost:5483", false},
		{"hostname bind keeps the hostname", "box.local:5483", "192.168.1.10:5483", "http://box.local:5483", false},
		{"empty host wildcard", ":3000", "[::]:3000", "http://localhost:3000", false},
		{"ipv4 wildcard", "0.0.0.0:8080", "0.0.0.0:8080", "http://localhost:8080", false},
		{"ipv6 wildcard", "[::]:8080", "[::]:8080", "http://localhost:8080", false},
		{"explicit loopback", "127.0.0.1:5483", "127.0.0.1:5483", "http://127.0.0.1:5483", false},
		{"lan ip", "192.168.1.10:5483", "192.168.1.10:5483", "http://192.168.1.10:5483", false},
		{"port zero reports the assigned port", "localhost:0", "127.0.0.1:53229", "http://localhost:53229", false},
		{"wildcard port zero", ":0", "[::]:53229", "http://localhost:53229", false},
		{"unparseable listener addr", "localhost:5483", "not-an-addr", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := displayURL(tt.requested, tt.listener)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("displayURL(%q, %q) = %q, want error", tt.requested, tt.listener, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("displayURL(%q, %q) error = %v", tt.requested, tt.listener, err)
			}
			if got != tt.want {
				t.Errorf("displayURL(%q, %q) = %q, want %q", tt.requested, tt.listener, got, tt.want)
			}
		})
	}
}

// TestDisplayURL_AgreesWithHostGuard is the regression test for the auto-open
// path: whatever URL we print and hand to the browser must survive the viewer's
// own Host allowlist. Before displayURL existed, `--addr box.local:5483` opened
// the resolved IP and landed on "403 forbidden host".
func TestDisplayURL_AgreesWithHostGuard(t *testing.T) {
	cases := []struct{ requested, listener string }{
		{"localhost:5483", "127.0.0.1:5483"},
		{"box.local:5483", "192.168.1.10:5483"},
		{"127.0.0.1:5483", "127.0.0.1:5483"},
		{"192.168.1.10:5483", "192.168.1.10:5483"},
		{":3000", "[::]:3000"},
		{"0.0.0.0:8080", "0.0.0.0:8080"},
	}
	for _, c := range cases {
		t.Run(c.requested, func(t *testing.T) {
			url, err := displayURL(c.requested, c.listener)
			if err != nil {
				t.Fatalf("displayURL: %v", err)
			}
			host := strings.TrimPrefix(url, "http://")

			allowed := buildAllowedHosts(splitBindHost(c.requested), "")
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.Host = host
			hostGuard(allowed, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("hostGuard rejected the URL we would open: addr=%q url=%q status=%d body=%q",
					c.requested, url, rec.Code, strings.TrimSpace(rec.Body.String()))
			}
		})
	}
}

func TestShouldAutoOpenEnv(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		stdoutTTY  bool
		sshConn    string
		display    string
		wayland    string
		goos       string
		want       bool
		wantReason string
	}{
		{"never", OpenNever, true, "", "", "", "darwin", false, ""},
		{"auto with tty", OpenAuto, true, "", "", "", "darwin", true, ""},
		{"auto non-tty stdout", OpenAuto, false, "", "", "", "darwin", false, "stdout is not a terminal"},
		{"auto linux with display", OpenAuto, true, "", ":0", "", "linux", true, ""},
		{"auto linux with wayland", OpenAuto, true, "", "", "wayland-0", "linux", true, ""},
		{"auto linux headless", OpenAuto, true, "", "", "", "linux", false, "no DISPLAY or WAYLAND_DISPLAY"},
		// SSH is judged together with the display variables, not on its own: being
		// remote only rules out a browser when nothing was forwarded.
		{"auto ssh without forwarding", OpenAuto, true, "10.0.0.1 1234", "", "", "darwin", false, "SSH session with no forwarded display"},
		{"auto ssh with X11 forwarding", OpenAuto, true, "10.0.0.1 1234", "localhost:10.0", "", "linux", true, ""},
		{"auto ssh with wayland forwarding", OpenAuto, true, "10.0.0.1 1234", "", "wayland-0", "linux", true, ""},
		{"auto ssh to linux without forwarding reports the ssh reason", OpenAuto, true, "10.0.0.1 1234", "", "", "linux", false, "SSH session with no forwarded display"},
		// A local macOS session has no DISPLAY at all and must still open.
		{"auto local darwin without display", OpenAuto, true, "", "", "", "darwin", true, ""},
		// always overrides every auto-mode guard.
		{"always over ssh", OpenAlways, true, "10.0.0.1 1234", "", "", "darwin", true, ""},
		{"always on headless linux", OpenAlways, false, "10.0.0.1 1234", "", "", "linux", true, ""},
		// An unvalidated value degrades to auto rather than opening blindly.
		{"unknown mode behaves as auto", "bogus", false, "", "", "", "darwin", false, "stdout is not a terminal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := shouldAutoOpenEnv(tt.mode, tt.stdoutTTY, tt.sshConn, tt.display, tt.wayland, tt.goos)
			if got != tt.want {
				t.Errorf("shouldAutoOpenEnv(%q, ...) = %v, want %v", tt.mode, got, tt.want)
			}
			if reason != tt.wantReason {
				t.Errorf("shouldAutoOpenEnv(%q, ...) reason = %q, want %q", tt.mode, reason, tt.wantReason)
			}
		})
	}
}

// TestParseTemplate_SessionWithComments renders session.html with review
// comments spanning several severities and categories so the template helpers
// (severityCounts, categoryCounts, severityClass, categoryClass,
// groupCommentsByFile, and the normalization helpers) execute.
func TestParseTemplate_SessionWithComments(t *testing.T) {
	tmpl, err := parseTemplate("session.html")
	if err != nil {
		t.Fatalf("parseTemplate: %v", err)
	}

	comments := []*ReviewComment{
		{FilePath: "a.go", Content: "c1", Category: "bug", Severity: "critical", StartLine: 1, EndLine: 2},
		{FilePath: "a.go", Content: "c2", Category: "security", Severity: "high"},
		{FilePath: "b.go", Content: "c3", Category: "performance", Severity: "medium"},
		{FilePath: "b.go", Content: "c4", Category: "docs", Severity: "low"},
	}
	vs := &ViewSession{
		Summary:  SessionSummary{SessionID: "s", CWD: "/p"},
		Comments: comments,
		Files: []*FileGroup{
			{FilePath: "a.go", Tasks: map[TaskType][]*TaskCard{
				MainTask: {{RequestNo: 1, ResponseContent: "ok", DurationMs: 1500, PromptTokens: 1200, CompletionTokens: 2_000_000}},
			}},
		},
	}

	rr := httptest.NewRecorder()
	if err := tmpl.Execute(rr, sessionPageData{EncodedRepo: "r", RepoName: "R", Session: vs}); err != nil {
		t.Fatalf("execute session.html with comments: %v", err)
	}
	if !strings.Contains(rr.Body.String(), "Review Comments") {
		t.Error("rendered page missing Review Comments section")
	}
	body := rr.Body.String()
	for _, want := range []string{
		`<span class="comment-filter-label">Severity:</span>`,
		`<span class="comment-filter-label">Category:</span>`,
		`data-filter-kind="severity" data-filter-value="all"`,
		`data-filter-kind="category" data-filter-value="all"`,
		`data-filter-kind="severity" data-filter-value="critical"`,
		`data-filter-kind="category" data-filter-value="bug"`,
		`data-filter-kind="category" data-filter-value="other"`,
		`data-comment-card data-category="bug" data-severity="critical"`,
		`data-comment-card data-category="other" data-severity="low"`,
		`data-comment-filter-empty`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
}

func TestCategoryCounts_NormalizesUnknownCategories(t *testing.T) {
	counts := categoryCounts([]*ReviewComment{
		{Category: "bug"},
		{Category: "MAINTAINABILITY"},
		{Category: ""},
		{Category: "not-a-category"},
	})
	if counts.Bug != 1 || counts.Maintainability != 1 || counts.Other != 2 {
		t.Fatalf("unexpected category counts: %+v", counts)
	}
}
