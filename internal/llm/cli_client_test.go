//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	llmdriver "github.com/lao/llm-driver"
)

// --- fixtures and helpers ---

// writeFakeCLI writes an executable /bin/sh stub that records its argv to
// $FAKE_ARGS_FILE (one arg per line) and its stdin to $FAKE_STDIN_FILE, then
// prints the canned response. The returned path is used as ClientConfig.CLIPath.
func writeFakeCLI(t *testing.T, name, response string) string {
	t.Helper()
	dir := t.TempDir()
	respFile := filepath.Join(dir, "response")
	if err := os.WriteFile(respFile, []byte(response), 0o644); err != nil {
		t.Fatalf("write response fixture: %v", err)
	}
	script := filepath.Join(dir, name)
	body := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$FAKE_ARGS_FILE\"\n" +
		"cat > \"$FAKE_STDIN_FILE\"\n" +
		"cat '" + respFile + "'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake CLI: %v", err)
	}
	return script
}

// captureFiles points FAKE_ARGS_FILE / FAKE_STDIN_FILE at fresh temp paths and
// returns readers for the recorded argv and stdin.
func captureFiles(t *testing.T) (args func() []string, stdin func() string) {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	stdinFile := filepath.Join(dir, "stdin")
	t.Setenv("FAKE_ARGS_FILE", argsFile)
	t.Setenv("FAKE_STDIN_FILE", stdinFile)
	args = func() []string {
		data, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatalf("read args file: %v", err)
		}
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
	stdin = func() string {
		data, err := os.ReadFile(stdinFile)
		if err != nil {
			t.Fatalf("read stdin file: %v", err)
		}
		return string(data)
	}
	return args, stdin
}

func claudeSuccess(result string) string {
	m := map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"result":     result,
		"session_id": "s1",
		"usage": map[string]any{
			"input_tokens":                11,
			"output_tokens":               7,
			"cache_read_input_tokens":     5,
			"cache_creation_input_tokens": 3,
		},
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func codexSuccess(agentText string) string {
	item := map[string]any{
		"type": "item.completed",
		"item": map[string]any{"id": "item_0", "type": "agent_message", "text": agentText},
	}
	itemLine, _ := json.Marshal(item)
	turn := map[string]any{
		"type": "turn.completed",
		"usage": map[string]any{
			"input_tokens":             18,
			"cached_input_tokens":      4,
			"cache_write_input_tokens": 2,
			"output_tokens":            9,
			"reasoning_output_tokens":  3,
		},
	}
	turnLine, _ := json.Marshal(turn)
	return `{"type":"thread.started","thread_id":"t1"}` + "\n" + string(itemLine) + "\n" + string(turnLine) + "\n"
}

// envelope builds the JSON envelope the model is asked to return. Each call is
// {"name": ..., "arguments": ...} with arguments already a JSON-object string.
func envelope(text string, calls ...map[string]string) string {
	tc := make([]map[string]string, 0, len(calls))
	tc = append(tc, calls...)
	m := map[string]any{"text": text, "tool_calls": tc}
	b, _ := json.Marshal(m)
	return string(b)
}

func sampleTools() []ToolDef {
	return []ToolDef{
		{Type: "function", Function: FunctionDef{
			Name:        "file_read",
			Description: "Read a file",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string"}},
				"required":   []any{"path"},
			},
		}},
		{Type: "function", Function: FunctionDef{
			Name:        "search",
			Description: "Search the repo",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		}},
	}
}

func contains(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}

// containsSeq reports whether sub appears as a contiguous run in args.
func containsSeq(args, sub []string) bool {
	for i := 0; i+len(sub) <= len(args); i++ {
		if slicesEqual(args[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// argValue returns the value following the last occurrence of flag in args.
func argValue(args []string, flag string) string {
	for i := len(args) - 1; i >= 0; i-- {
		if args[i] == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// indexOf returns the first index of s in args, or -1 if absent.
func indexOf(args []string, s string) int {
	for i, a := range args {
		if a == s {
			return i
		}
	}
	return -1
}

// --- tests ---

// Test 1: tool-less single user message -> raw stdin, Claude flags present, no
// --json-schema, no --append-system-prompt, security flags after cfg.CLIArgs.
func TestCLIClient_ClaudeToolless(t *testing.T) {
	args, stdin := captureFiles(t)
	script := writeFakeCLI(t, "claude", claudeSuccess("hello there"))
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5", CLIArgs: []string{"--extra", "z"}})

	resp, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{
			NewTextMessage("system", "Be terse."),
			NewTextMessage("user", "Say hi."),
		},
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	if got := stdin(); got != "Say hi." {
		t.Errorf("stdin = %q, want raw %q", got, "Say hi.")
	}
	a := args()
	if !containsSeq(a, []string{"--tools", ""}) {
		t.Errorf("argv %q missing `--tools \"\"`", a)
	}
	for _, want := range []string{"--bare", "--strict-mcp-config"} {
		if !contains(a, want) {
			t.Errorf("argv %q missing %q", a, want)
		}
	}
	if !containsSeq(a, []string{"--system-prompt", "Be terse."}) {
		t.Errorf("argv %q missing `--system-prompt \"Be terse.\"`", a)
	}
	if contains(a, "--append-system-prompt") {
		t.Errorf("argv %q must not contain --append-system-prompt", a)
	}
	if contains(a, "--json-schema") {
		t.Errorf("argv %q must not contain --json-schema for a tool-less request", a)
	}
	// Security flags must follow user-supplied CLIArgs so they cannot be overridden.
	extraIdx := indexOf(a, "--extra")
	toolsIdx := indexOf(a, "--tools")
	strictIdx := indexOf(a, "--strict-mcp-config")
	if extraIdx < 0 || toolsIdx < 0 || strictIdx < 0 {
		t.Fatalf("argv %q missing expected flags", a)
	}
	if toolsIdx < extraIdx {
		t.Errorf("security flag --tools (idx %d) must come after user arg --extra (idx %d); argv = %q", toolsIdx, extraIdx, a)
	}
	if strictIdx < extraIdx {
		t.Errorf("security flag --strict-mcp-config (idx %d) must come after user arg --extra (idx %d); argv = %q", strictIdx, extraIdx, a)
	}
	if resp.Content() != "hello there" {
		t.Errorf("Content() = %q, want %q", resp.Content(), "hello there")
	}
}

// Test 2: Codex argv carries the defaults, the environment-scrubbing config,
// developer_instructions and --output-schema when tools are present — and no -C,
// so the model is not handed a repo working directory it does not need.
func TestCLIClient_CodexArgs(t *testing.T) {
	args, _ := captureFiles(t)
	script := writeFakeCLI(t, "codex", codexSuccess(envelope("ok")))
	c := NewCodexCLIClient(ClientConfig{CLIPath: script, Model: "gpt-5.5"})

	_, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{
			NewTextMessage("system", "You review code."),
			NewTextMessage("user", "Review foo.go"),
		},
		Tools: sampleTools(),
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	a := args()
	for _, want := range []string{"--ephemeral", "--ignore-user-config"} {
		if !contains(a, want) {
			t.Errorf("argv %q missing %q", a, want)
		}
	}
	// The env-inheritance guard keeps a prompt-injected model's shell commands
	// from reading OCR's inherited secrets out of the process environment.
	if !containsSeq(a, []string{"-c", `shell_environment_policy.inherit="none"`}) {
		t.Errorf("argv %q missing `-c shell_environment_policy.inherit=\"none\"`", a)
	}
	if contains(a, "-C") {
		t.Errorf("argv %q must not point Codex at a repo dir with -C", a)
	}
	if !contains(a, `developer_instructions="You review code."`) {
		t.Errorf("argv %q missing developer_instructions config", a)
	}
	if argValue(a, "--output-schema") == "" {
		t.Errorf("argv %q missing --output-schema <file>", a)
	}
}

// TestCLIClient_CodexSecurityFlagsAfterCLIArgs verifies that Codex's security-
// critical flags (--ignore-user-config, shell_environment_policy) come after any
// user-supplied cfg.CLIArgs so they cannot be overridden.
func TestCLIClient_CodexSecurityFlagsAfterCLIArgs(t *testing.T) {
	args, _ := captureFiles(t)
	script := writeFakeCLI(t, "codex", codexSuccess(envelope("ok")))
	c := NewCodexCLIClient(ClientConfig{
		CLIPath: script,
		Model:   "gpt-5.5",
		CLIArgs: []string{"--custom", "val"},
	})

	_, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{NewTextMessage("user", "Review bar.go")},
		Tools:    sampleTools(),
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	a := args()
	customIdx := indexOf(a, "--custom")
	ignoreIdx := indexOf(a, "--ignore-user-config")
	envIdx := indexOf(a, "-c")
	if customIdx < 0 || ignoreIdx < 0 || envIdx < 0 {
		t.Fatalf("argv %q missing expected flags", a)
	}
	if ignoreIdx < customIdx {
		t.Errorf("security flag --ignore-user-config (idx %d) must come after user arg --custom (idx %d); argv = %q", ignoreIdx, customIdx, a)
	}
	if envIdx < customIdx {
		t.Errorf("security flag -c (idx %d) must come after user arg --custom (idx %d); argv = %q", envIdx, customIdx, a)
	}
}

// Test 3: tools present -> --json-schema whose name enum equals the tool names;
// stdin carries the tool catalog and the response-format section.
func TestCLIClient_ClaudeToolSchema(t *testing.T) {
	args, stdin := captureFiles(t)
	script := writeFakeCLI(t, "claude", claudeSuccess(envelope("thinking")))
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})

	_, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{NewTextMessage("user", "Review foo.go")},
		Tools:    sampleTools(),
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	schema := argValue(args(), "--json-schema")
	if schema == "" {
		t.Fatal("argv missing --json-schema")
	}
	enum := enumFromSchema(t, schema)
	want := []string{"file_read", "search"}
	if !slicesEqual(enum, want) {
		t.Errorf("schema name enum = %q, want %q", enum, want)
	}
	in := stdin()
	for _, want := range []string{"## Tools", "file_read", "search", `"path"`, "## Response format"} {
		if !strings.Contains(in, want) {
			t.Errorf("stdin missing %q\n---\n%s", want, in)
		}
	}
}

// enumFromSchema pulls properties.tool_calls.items.properties.name.enum.
func enumFromSchema(t *testing.T, schema string) []string {
	t.Helper()
	var s map[string]any
	if err := json.Unmarshal([]byte(schema), &s); err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
	dig := func(m map[string]any, key string) map[string]any {
		v, _ := m[key].(map[string]any)
		return v
	}
	props := dig(s, "properties")
	items := dig(dig(props, "tool_calls"), "items")
	name := dig(dig(items, "properties"), "name")
	rawEnum, _ := name["enum"].([]any)
	out := make([]string, 0, len(rawEnum))
	for _, e := range rawEnum {
		out = append(out, e.(string))
	}
	return out
}

// Test 4: multi-round flattening renders both tool calls and both results in
// order under a single [assistant] block; system text is absent from Claude
// stdin (it goes to --system-prompt) and present as developer_instructions for
// Codex.
func TestCLIClient_MultiRoundFlattening(t *testing.T) {
	msgs := []Message{
		NewTextMessage("system", "SYSTEM-TEXT"),
		NewTextMessage("user", "first"),
		NewToolCallMessage("looking", []ToolCall{
			{ID: "orig-a", Type: "function", Function: FunctionCall{Name: "file_read", Arguments: `{"path":"a.go"}`}},
			{ID: "orig-b", Type: "function", Function: FunctionCall{Name: "search", Arguments: `{"q":"x"}`}},
		}, NativeTurn{}, ""),
		NewToolResultMessage("orig-a", "contents-a"),
		NewToolResultMessage("orig-b", "hits-b"),
		NewTextMessage("user", "second"),
	}

	t.Run("claude", func(t *testing.T) {
		_, stdin := captureFiles(t)
		script := writeFakeCLI(t, "claude", claudeSuccess(envelope("done")))
		c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})
		if _, err := c.CompletionsWithCtx(context.Background(), ChatRequest{Messages: msgs, Tools: sampleTools()}); err != nil {
			t.Fatalf("CompletionsWithCtx: %v", err)
		}
		in := stdin()
		for _, want := range []string{
			"[assistant]",
			`"id":"call_1","name":"file_read","arguments":{"path":"a.go"}`,
			`"id":"call_2","name":"search","arguments":{"q":"x"}`,
			"[tool_result id=call_1 name=file_read]",
			"contents-a",
			"[tool_result id=call_2 name=search]",
			"hits-b",
		} {
			if !strings.Contains(in, want) {
				t.Errorf("stdin missing %q\n---\n%s", want, in)
			}
		}
		if strings.Contains(in, "SYSTEM-TEXT") {
			t.Errorf("Claude stdin must not carry system text; got:\n%s", in)
		}
		// call_1 must precede call_2 in the flattened order.
		if strings.Index(in, "id=call_1") > strings.Index(in, "id=call_2") {
			t.Errorf("tool_result order wrong:\n%s", in)
		}
	})

	t.Run("codex", func(t *testing.T) {
		args, stdin := captureFiles(t)
		script := writeFakeCLI(t, "codex", codexSuccess(envelope("done")))
		c := NewCodexCLIClient(ClientConfig{CLIPath: script, Model: "gpt-5.5"})
		if _, err := c.CompletionsWithCtx(context.Background(), ChatRequest{Messages: msgs, Tools: sampleTools()}); err != nil {
			t.Fatalf("CompletionsWithCtx: %v", err)
		}
		if strings.Contains(stdin(), "SYSTEM-TEXT") {
			t.Errorf("Codex system text belongs in developer_instructions, not stdin:\n%s", stdin())
		}
		if !contains(args(), `developer_instructions="SYSTEM-TEXT"`) {
			t.Errorf("Codex argv missing developer_instructions=SYSTEM-TEXT: %q", args())
		}
	})
}

// Test 5: an envelope with two calls maps to two ToolCalls with sequential ids,
// compact JSON arguments, finish reason tool_calls, and text as content.
func TestCLIClient_EnvelopeTwoCalls(t *testing.T) {
	captureFiles(t)
	env := envelope("here are two",
		map[string]string{"name": "file_read", "arguments": `{"path":"a.go"}`},
		map[string]string{"name": "search", "arguments": `{"q":"y"}`},
	)
	script := writeFakeCLI(t, "claude", claudeSuccess(env))
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})

	resp, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{NewTextMessage("user", "go")},
		Tools:    sampleTools(),
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	calls := resp.ToolCalls()
	if len(calls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(calls))
	}
	if calls[0].ID != "call_1" || calls[1].ID != "call_2" {
		t.Errorf("ids = %q,%q want call_1,call_2", calls[0].ID, calls[1].ID)
	}
	if calls[0].Function.Arguments != `{"path":"a.go"}` {
		t.Errorf("arg0 = %q", calls[0].Function.Arguments)
	}
	if calls[0].Type != "function" {
		t.Errorf("type = %q, want function", calls[0].Type)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want tool_calls", resp.Choices[0].FinishReason)
	}
	if resp.Content() != "here are two" {
		t.Errorf("Content() = %q, want %q", resp.Content(), "here are two")
	}
}

// Test 6: unknown tool names are dropped; an empty tool_calls array yields no
// calls and finish reason stop.
func TestCLIClient_UnknownAndEmpty(t *testing.T) {
	t.Run("unknown dropped", func(t *testing.T) {
		captureFiles(t)
		env := envelope("mixed",
			map[string]string{"name": "file_read", "arguments": `{"path":"a.go"}`},
			map[string]string{"name": "not_a_tool", "arguments": `{}`},
		)
		script := writeFakeCLI(t, "claude", claudeSuccess(env))
		c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})
		resp, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
			Messages: []Message{NewTextMessage("user", "go")},
			Tools:    sampleTools(),
		})
		if err != nil {
			t.Fatalf("CompletionsWithCtx: %v", err)
		}
		calls := resp.ToolCalls()
		if len(calls) != 1 || calls[0].Function.Name != "file_read" {
			t.Fatalf("calls = %+v, want only file_read", calls)
		}
	})

	t.Run("empty calls", func(t *testing.T) {
		captureFiles(t)
		script := writeFakeCLI(t, "claude", claudeSuccess(envelope("nothing to do")))
		c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})
		resp, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
			Messages: []Message{NewTextMessage("user", "go")},
			Tools:    sampleTools(),
		})
		if err != nil {
			t.Fatalf("CompletionsWithCtx: %v", err)
		}
		if len(resp.ToolCalls()) != 0 {
			t.Errorf("want no tool calls, got %d", len(resp.ToolCalls()))
		}
		if resp.Choices[0].FinishReason != "stop" {
			t.Errorf("FinishReason = %q, want stop", resp.Choices[0].FinishReason)
		}
	})
}

// Test 7: a non-JSON result with a schema requested makes the library return
// invalid_structured_output; the adapter converts it to an empty turn with no
// tool calls (no error) so the agent loop retries the round.
func TestCLIClient_InvalidStructuredOutput(t *testing.T) {
	captureFiles(t)
	script := writeFakeCLI(t, "claude", claudeSuccess("this is not JSON at all"))
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})

	resp, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{NewTextMessage("user", "go")},
		Tools:    sampleTools(),
	})
	if err != nil {
		t.Fatalf("want no error for invalid structured output, got %v", err)
	}
	if len(resp.ToolCalls()) != 0 {
		t.Errorf("want no tool calls, got %d", len(resp.ToolCalls()))
	}
	if resp.VisibleContent() != "" {
		t.Errorf("want empty content, got %q", resp.VisibleContent())
	}
}

// Test 8: usage counters and total are mapped from the CLI reply.
func TestCLIClient_Usage(t *testing.T) {
	captureFiles(t)
	script := writeFakeCLI(t, "claude", claudeSuccess("hi"))
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})
	resp, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{NewTextMessage("user", "go")},
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	u := resp.Usage
	if u == nil {
		t.Fatal("Usage is nil")
	}
	if u.PromptTokens != 11 || u.CompletionTokens != 7 || u.CacheReadTokens != 5 || u.CacheWriteTokens != 3 {
		t.Errorf("usage = %+v, want prompt 11 completion 7 read 5 write 3", u)
	}
	if u.TotalTokens != 18 {
		t.Errorf("TotalTokens = %d, want 18 (input+output)", u.TotalTokens)
	}
}

// Test 9: a cli_path pointing at a missing file surfaces install/login guidance.
func TestCLIClient_ExecutableNotFound(t *testing.T) {
	captureFiles(t)
	c := NewClaudeCLIClient(ClientConfig{CLIPath: "/nonexistent/claude-xyz", Model: "claude-sonnet-5"})
	_, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{NewTextMessage("user", "go")},
	})
	if err == nil {
		t.Fatal("want error for missing executable")
	}
	for _, want := range []string{"not found", "login"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

// Test 10: a non-zero exit with a stderr message keeps the stderr excerpt so a
// logged-out CLI is diagnosable.
func TestCLIClient_NonZeroExitKeepsStderr(t *testing.T) {
	captureFiles(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "claude")
	body := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$FAKE_ARGS_FILE\"\n" +
		"cat > \"$FAKE_STDIN_FILE\"\n" +
		"echo 'Error: not logged in, run claude login' 1>&2\n" +
		"exit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})
	_, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{NewTextMessage("user", "go")},
	})
	if err == nil {
		t.Fatal("want error for non-zero exit")
	}
	if !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("error %q missing stderr excerpt", err.Error())
	}
}

// Test 11: cancelling the context returns promptly with context.Canceled.
func TestCLIClient_ContextCancel(t *testing.T) {
	captureFiles(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "claude")
	body := "#!/bin/sh\ncat > /dev/null\nsleep 5\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := c.CompletionsWithCtx(ctx, ChatRequest{Messages: []Message{NewTextMessage("user", "go")}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Errorf("returned after %v, want prompt cancellation", elapsed)
	}
}

// Test 13: a per-request timeout (cfg.Timeout), distinct from an outer-context
// cancel, surfaces as the context.DeadlineExceeded sentinel the agent loop keys
// off — not a wrapped *llmdriver.Error — and returns promptly.
func TestCLIClient_TimeoutReturnsDeadlineExceeded(t *testing.T) {
	captureFiles(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "claude")
	body := "#!/bin/sh\ncat > /dev/null\nsleep 5\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5", Timeout: 150 * time.Millisecond})

	start := time.Now()
	_, err := c.CompletionsWithCtx(context.Background(), ChatRequest{Messages: []Message{NewTextMessage("user", "go")}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Errorf("returned after %v, want prompt timeout near 150ms", elapsed)
	}
}

// Test 14: a very long CLI stderr is truncated to the excerpt cap so a runaway
// stack trace cannot flood the error, while the head of the message survives.
func TestCLIClient_StderrExcerptCapped(t *testing.T) {
	captureFiles(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "claude")
	// HEAD (survives) + >500 bytes of filler + TAILMARK (dropped past the cap),
	// all on stderr in one redirect.
	body := "#!/bin/sh\ncat > /dev/null\n" +
		"{ printf 'HEAD'; i=0; while [ $i -lt 600 ]; do printf x; i=$((i+1)); done; printf 'TAILMARK'; } 1>&2\n" +
		"exit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})
	_, err := c.CompletionsWithCtx(context.Background(), ChatRequest{Messages: []Message{NewTextMessage("user", "go")}})
	if err == nil {
		t.Fatal("want error for non-zero exit")
	}
	if !strings.Contains(err.Error(), "HEAD") {
		t.Errorf("error %q dropped the head of the stderr", err.Error())
	}
	if strings.Contains(err.Error(), "TAILMARK") {
		t.Errorf("error %q kept the tail past the %d-byte cap", err.Error(), cliErrMessageCap)
	}
}

// Test 15: a tool call whose arguments string is empty maps to "{}" rather than
// an empty string, so the agent loop always sees valid JSON arguments.
func TestCLIClient_EmptyToolArgumentsDefaultToObject(t *testing.T) {
	captureFiles(t)
	env := envelope("go", map[string]string{"name": "file_read", "arguments": ""})
	script := writeFakeCLI(t, "claude", claudeSuccess(env))
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})

	resp, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{NewTextMessage("user", "go")},
		Tools:    sampleTools(),
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	calls := resp.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(calls))
	}
	if calls[0].Function.Arguments != "{}" {
		t.Errorf("Arguments = %q, want %q for an empty arguments string", calls[0].Function.Arguments, "{}")
	}
}

// Test 16: a tool-less request with more than one message still flattens to the
// conversation body, but omits the tool catalog, response-format section, and
// --json-schema; the reply is returned as plain content.
func TestCLIClient_ToollessMultiMessagePrompt(t *testing.T) {
	args, stdin := captureFiles(t)
	script := writeFakeCLI(t, "claude", claudeSuccess("plain reply"))
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})

	resp, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{
			NewTextMessage("user", "first ask"),
			NewTextMessage("assistant", "some reply"),
			NewTextMessage("user", "second ask"),
		},
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	in := stdin()
	if !strings.Contains(in, "## Conversation") {
		t.Errorf("stdin missing conversation body:\n%s", in)
	}
	for _, want := range []string{"[user]\nfirst ask", "[assistant]\nsome reply", "[user]\nsecond ask"} {
		if !strings.Contains(in, want) {
			t.Errorf("stdin missing %q\n---\n%s", want, in)
		}
	}
	for _, unwanted := range []string{"## Tools", "## Response format"} {
		if strings.Contains(in, unwanted) {
			t.Errorf("tool-less stdin must not contain %q\n---\n%s", unwanted, in)
		}
	}
	if contains(args(), "--json-schema") {
		t.Errorf("tool-less argv %q must not contain --json-schema", args())
	}
	if resp.Content() != "plain reply" {
		t.Errorf("Content() = %q, want %q", resp.Content(), "plain reply")
	}
}

// Test 17: malformed history renders defensively — an assistant tool call with an
// empty arguments string emits {} in the flattened call, and a tool result whose
// id matches no prior call keeps its raw id rather than being dropped.
func TestCLIClient_RenderDefensiveHistory(t *testing.T) {
	_, stdin := captureFiles(t)
	script := writeFakeCLI(t, "claude", claudeSuccess(envelope("ok")))
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})

	msgs := []Message{
		NewTextMessage("user", "go"),
		NewToolCallMessage("", []ToolCall{
			{ID: "real", Type: "function", Function: FunctionCall{Name: "file_read", Arguments: ""}},
		}, NativeTurn{}, ""),
		NewToolResultMessage("ghost", "orphaned result"),
	}
	if _, err := c.CompletionsWithCtx(context.Background(), ChatRequest{Messages: msgs, Tools: sampleTools()}); err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	in := stdin()
	if !strings.Contains(in, `"id":"call_1","name":"file_read","arguments":{}`) {
		t.Errorf("empty assistant tool-call args not rendered as {}:\n%s", in)
	}
	if !strings.Contains(in, "[tool_result id=ghost name=]") {
		t.Errorf("orphan tool result should keep its raw id:\n%s", in)
	}
}

// Test 18: a tool whose Parameters map is nil renders as {} in the catalog rather
// than the literal "null", so the prompt stays valid JSON per tool.
func TestCLIClient_NilToolParametersRenderEmptyObject(t *testing.T) {
	_, stdin := captureFiles(t)
	script := writeFakeCLI(t, "claude", claudeSuccess(envelope("ok")))
	c := NewClaudeCLIClient(ClientConfig{CLIPath: script, Model: "claude-sonnet-5"})

	tools := []ToolDef{{Type: "function", Function: FunctionDef{
		Name:        "noargs",
		Description: "Takes nothing",
		Parameters:  nil,
	}}}
	if _, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{NewTextMessage("user", "go")},
		Tools:    tools,
	}); err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	if !strings.Contains(stdin(), "Parameters (JSON Schema): {}") {
		t.Errorf("nil tool parameters not rendered as {}:\n%s", stdin())
	}
}

// Test 19: parseEnvelope returns the text and the tool calls from a single
// parse. A valid envelope yields both from the structured field; a reply that
// does not parse falls back to the raw text with no calls, so the text and the
// tool calls can never disagree about the empty-structured fallback.
func TestParseEnvelope_TextAndCallsShareOneParse(t *testing.T) {
	names := toolNameSet(sampleTools())

	t.Run("valid envelope", func(t *testing.T) {
		env := envelope("visible", map[string]string{"name": "file_read", "arguments": `{"path":"a.go"}`})
		text, calls := parseEnvelope(json.RawMessage(env), "", names)
		if text != "visible" {
			t.Errorf("text = %q, want %q", text, "visible")
		}
		if len(calls) != 1 || calls[0].Function.Name != "file_read" {
			t.Fatalf("calls = %+v, want one file_read call", calls)
		}
	})

	t.Run("empty structured falls back to raw text", func(t *testing.T) {
		env := envelope("from raw")
		text, calls := parseEnvelope(nil, env, names)
		if text != "from raw" {
			t.Errorf("text = %q, want %q from the raw-text fallback", text, "from raw")
		}
		if calls != nil {
			t.Errorf("calls = %+v, want nil", calls)
		}
	})

	t.Run("unparseable reply returns raw text and no calls", func(t *testing.T) {
		text, calls := parseEnvelope(nil, "not json", names)
		if text != "not json" {
			t.Errorf("text = %q, want the raw text preserved for the review filter", text)
		}
		if calls != nil {
			t.Errorf("calls = %+v, want nil for an unparseable reply", calls)
		}
	})
}

// Test 12: the factory maps the CLI protocols to a CLIClient with the right
// underlying provider.
func TestNewLLMClient_CLIProtocols(t *testing.T) {
	tests := []struct {
		protocol     string
		wantProvider llmdriver.Provider
	}{
		{ProtocolClaudeCLI, llmdriver.ProviderClaude},
		{ProtocolCodexCLI, llmdriver.ProviderOpenAI},
	}
	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			client := NewLLMClient(ResolvedEndpoint{Protocol: tt.protocol, Model: "m"}, nil, nil)
			cc, ok := client.(*CLIClient)
			if !ok {
				t.Fatalf("NewLLMClient returned %T, want *CLIClient", client)
			}
			if cc.provider != tt.wantProvider {
				t.Errorf("provider = %q, want %q", cc.provider, tt.wantProvider)
			}
		})
	}
}
