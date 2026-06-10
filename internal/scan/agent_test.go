package scan

import (
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/template"
	"github.com/open-code-review/open-code-review/internal/llmloop"
	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/session"
	"github.com/open-code-review/open-code-review/internal/tool"
)

func newAgentForTest(t *testing.T, tpl template.Template) *Agent {
	t.Helper()
	return NewAgent(Args{
		Template:         tpl,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session: session.New(t.TempDir(), "main", "test-model", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
}

func makeTemplateWithFullScan() template.Template {
	return template.Template{
		MaxTokens:           1000,
		MaxToolRequestTimes: 5,
		MainTask: template.LlmConversation{
			Messages: []template.ChatMessage{
				{Role: "system", Content: "main"},
				{Role: "user", Content: "main user"},
			},
		},
		FullScanTask: &template.LlmConversation{
			Messages: []template.ChatMessage{
				{Role: "system", Content: "scan system rule={{system_rule}}"},
				{
					Role: "user",
					Content: "path={{current_file_path}}\n" +
						"date={{current_system_date_time}}\n" +
						"siblings=[{{change_files}}]\n" +
						"bg={{requirement_background}}\n" +
						"<content>\n{{file_content}}\n</content>",
				},
			},
		},
	}
}

func TestRenderMessages(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	a := newAgentForTest(t, tpl)
	a.currentDate = "2026-06-09 10:00"
	a.args.Background = "ticket-123"

	it := model.ScanItem{
		Path:    "internal/foo/bar.go",
		Content: "package foo\n\nfunc Bar() {}\n",
	}
	msgs := a.renderMessages(it, "rule-text")

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	sysText := msgs[0].ExtractText()
	if !strings.Contains(sysText, "rule=rule-text") {
		t.Errorf("system missing system_rule: %q", sysText)
	}

	userText := msgs[1].ExtractText()
	checks := map[string]string{
		"path":     "path=internal/foo/bar.go",
		"date":     "date=2026-06-09 10:00",
		"siblings": "siblings=[" + changeFilesScanLiteral + "]",
		"bg":       "bg=ticket-123",
		"content":  "<content>\npackage foo\n\nfunc Bar() {}\n\n</content>",
	}
	for label, want := range checks {
		if !strings.Contains(userText, want) {
			t.Errorf("%s missing %q\nfull: %q", label, want, userText)
		}
	}
	for _, leak := range []string{"{{diff}}", "{{file_content}}", "{{change_files}}", "{{plan_guidance}}"} {
		if strings.Contains(userText, leak) {
			t.Errorf("placeholder %s leaked into prompt", leak)
		}
	}
}

func TestFilterLargeScans(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 40 // threshold = 32
	a := newAgentForTest(t, tpl)

	short := strings.Repeat("a ", 5)
	huge := strings.Repeat("token ", 200)
	in := []model.ScanItem{
		{Path: "a.go", Content: short},
		{Path: "huge.go", Content: huge},
		{Path: "b.go", Content: short},
	}
	out := a.filterLargeScans(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 kept, got %d", len(out))
	}
	for _, it := range out {
		if it.Path == "huge.go" {
			t.Errorf("huge.go should have been filtered")
		}
	}
}

func TestFilterLargeScans_NoLimit(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 0
	a := newAgentForTest(t, tpl)
	in := []model.ScanItem{
		{Path: "a.go", Content: "anything"},
		{Path: "b.go", Content: strings.Repeat("x ", 1000)},
	}
	out := a.filterLargeScans(in)
	if len(out) != 2 {
		t.Errorf("with MaxTokens=0 nothing should be filtered, got %d", len(out))
	}
}

func TestInjectScanContentMap(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	a := newAgentForTest(t, tpl)
	a.args.Tools.Register(tool.NewFileReadDiff(tool.DiffMap{}))

	a.items = []model.ScanItem{
		{Path: "x.go", Content: "package x"},
		{Path: "y.go", Content: "package y"},
	}
	a.injectScanContentMap()

	p, ok := a.args.Tools.Get(tool.FileReadDiff.Name())
	if !ok {
		t.Fatal("file_read_diff not registered")
	}
	frd := p.(*tool.FileReadDiffProvider)
	res, err := frd.Execute(t.Context(), map[string]any{
		"path_array": []any{"x.go", "y.go", "missing.go"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res, "package x") || !strings.Contains(res, "package y") {
		t.Errorf("missing scan content:\n%s", res)
	}
}

func TestNewAgent_SetsSessionMode(t *testing.T) {
	a := NewAgent(Args{Template: makeTemplateWithFullScan()})
	if a.session.ReviewMode != session.ReviewModeFullScan {
		t.Errorf("ReviewMode = %q, want %q", a.session.ReviewMode, session.ReviewModeFullScan)
	}
}

func TestRunner_Warnings_RoundTrip(t *testing.T) {
	a := newAgentForTest(t, makeTemplateWithFullScan())
	a.recordWarning("foo", "x.go", "boom")
	ws := a.Warnings()
	if len(ws) != 1 || ws[0].Type != "foo" || ws[0].File != "x.go" {
		t.Errorf("warnings = %+v", ws)
	}
}

// Ensure llmloop.Runner is the underlying source of token counters so the
// public methods on scan.Agent are not stale (preventing accidental refactor
// regressions).
func TestTokenCountersDelegateToRunner(t *testing.T) {
	a := newAgentForTest(t, makeTemplateWithFullScan())
	if a.TotalInputTokens() != a.runner.TotalInputTokens() ||
		a.TotalOutputTokens() != a.runner.TotalOutputTokens() ||
		a.TotalCacheReadTokens() != a.runner.TotalCacheReadTokens() ||
		a.TotalCacheWriteTokens() != a.runner.TotalCacheWriteTokens() {
		t.Fatal("scan.Agent token getters must mirror runner")
	}
	_ = llmloop.AgentWarning{} // keep llmloop import meaningful
}
