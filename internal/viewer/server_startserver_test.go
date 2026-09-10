// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
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

// TestNumberedCodeLines pins the numbering contract for the Existing Code
// gutter. The interesting half of the table is the cases where the reported
// range and the snippet disagree: internal/diff/resolver.go drops blank lines
// while matching, so the span can be wider or narrower than the snippet, and
// numbering it anyway would print wrong file line numbers.
func TestNumberedCodeLines(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		start int
		end   int
		want  []codeLine
	}{
		{"single line", "foo()", 10, 10, []codeLine{{10, "foo()"}}},
		{"multi line contiguous", "a\nb\nc", 10, 12, []codeLine{{10, "a"}, {11, "b"}, {12, "c"}}},
		{"trailing newline is not a line", "a\nb\n", 10, 11, []codeLine{{10, "a"}, {11, "b"}}},
		{"end line zero means single line", "foo()", 7, 0, []codeLine{{7, "foo()"}}},
		{"inverted range yields no numbers", "foo()", 12, 10, []codeLine{{0, "foo()"}}},
		{"unresolved range yields no numbers", "a\nb", 0, 0, []codeLine{{0, "a"}, {0, "b"}}},
		{"negative start yields no numbers", "a", -3, -1, []codeLine{{0, "a"}}},
		{"span wider than snippet yields no numbers", "a\nb", 10, 14, []codeLine{{0, "a"}, {0, "b"}}},
		{"span narrower than snippet yields no numbers", "a\nb\nc\nd", 10, 11, []codeLine{{0, "a"}, {0, "b"}, {0, "c"}, {0, "d"}}},
		{"internal blank line counted", "a\n\nb", 10, 12, []codeLine{{10, "a"}, {11, ""}, {12, "b"}}},
		{"leading blank line breaks consistency", "\na\nb", 10, 11, []codeLine{{0, ""}, {0, "a"}, {0, "b"}}},
		{"empty code has no lines", "", 10, 10, nil},
		{"crlf line endings", "a\r\nb", 10, 11, []codeLine{{10, "a"}, {11, "b"}}},
		{"html metacharacters are returned verbatim", "if a < b && c > d {", 5, 5, []codeLine{{5, "if a < b && c > d {"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := numberedCodeLines(tt.code, tt.start, tt.end)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("numberedCodeLines(%q, %d, %d) = %#v, want %#v", tt.code, tt.start, tt.end, got, tt.want)
			}
		})
	}
}

// TestParseTemplate_ExistingCodeLineNumbers asserts the rendered session page,
// not which helper ran: a trustworthy range gets a per-line gutter, anything
// else keeps the plain block it has today.
func TestParseTemplate_ExistingCodeLineNumbers(t *testing.T) {
	tmpl, err := parseTemplate("session.html")
	if err != nil {
		t.Fatalf("parseTemplate: %v", err)
	}

	tests := []struct {
		name       string
		comment    *ReviewComment
		wantHas    []string
		wantHasNot []string
	}{
		{
			name:    "resolved range is numbered",
			comment: &ReviewComment{FilePath: "a.go", Content: "c", ExistingCode: "a\nb\nc", StartLine: 10, EndLine: 12},
			wantHas: []string{
				`<span class="line-no" aria-hidden="true">10</span><span class="line-text">a</span>`,
				`<span class="line-no" aria-hidden="true">11</span><span class="line-text">b</span>`,
				`<span class="line-no" aria-hidden="true">12</span><span class="line-text">c</span>`,
				`<span class="comment-lines sr-only">L10-L12</span>`,
			},
		},
		{
			name:       "unresolved range is not numbered",
			comment:    &ReviewComment{FilePath: "a.go", Content: "c", ExistingCode: "a\nb"},
			wantHas:    []string{"<pre><code>a\nb</code></pre>"},
			wantHasNot: []string{`class="line-no"`},
		},
		{
			name:       "inconsistent span is not numbered",
			comment:    &ReviewComment{FilePath: "a.go", Content: "c", ExistingCode: "a\nb", StartLine: 10, EndLine: 14},
			wantHas:    []string{"<pre><code>a\nb</code></pre>", `<span class="comment-lines">L10-L14</span>`},
			wantHasNot: []string{`class="line-no"`},
		},
		{
			name:       "suggestion block is never numbered",
			comment:    &ReviewComment{FilePath: "a.go", Content: "c", SuggestionCode: "x\ny", StartLine: 10, EndLine: 11},
			wantHas:    []string{`<div class="code-panel-label">Suggested Change</div>`, "<pre><code>x\ny</code></pre>"},
			wantHasNot: []string{`class="line-no"`},
		},
		{
			name:       "html in numbered code is escaped",
			comment:    &ReviewComment{FilePath: "a.go", Content: "c", ExistingCode: "<script>alert(1)</script>", StartLine: 3, EndLine: 3},
			wantHas:    []string{`<span class="line-text">&lt;script&gt;alert(1)&lt;/script&gt;</span>`},
			wantHasNot: []string{"<script>alert(1)</script>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs := &ViewSession{
				Summary:  SessionSummary{SessionID: "s", CWD: "/p"},
				Comments: []*ReviewComment{tt.comment},
			}
			rr := httptest.NewRecorder()
			if err := tmpl.Execute(rr, sessionPageData{EncodedRepo: "r", RepoName: "R", Session: vs}); err != nil {
				t.Fatalf("execute session.html: %v", err)
			}
			body := rr.Body.String()
			for _, want := range tt.wantHas {
				if !strings.Contains(body, want) {
					t.Errorf("rendered page missing %q", want)
				}
			}
			for _, unwanted := range tt.wantHasNot {
				if strings.Contains(body, unwanted) {
					t.Errorf("rendered page unexpectedly contains %q", unwanted)
				}
			}
		})
	}
}
