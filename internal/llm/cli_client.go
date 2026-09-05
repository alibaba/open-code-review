// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	llmdriver "github.com/lao/llm-driver"
)

// CLIClient drives a locally installed provider CLI (Claude Code's `claude -p`
// or Codex's `codex exec`) as the model, through the llm-driver library. The CLI
// runs as a pure model: its own agentic tools stay disabled, OCR runs the review
// loop itself. Tool definitions from the request become a JSON output schema and
// a tool catalog in the prompt; the CLI's structured JSON reply is turned back
// into synthetic ToolCalls the agent loop understands.
//
// ponytail: the transcript is rebuilt from Content/ToolCalls on every round, so
// there is no prompt-cache reuse across rounds (each round is a fresh process).
// Upgrade path: Claude's `--resume` session reuse (out of scope for v1).
type CLIClient struct {
	cfg          ClientConfig
	provider     llmdriver.Provider // ProviderClaude or ProviderOpenAI
	cliName      string             // "claude" or "codex"; the PATH executable and login command
	baseArgs     []string           // per-provider default flags (before cfg.CLIArgs)
	securityArgs []string           // security-critical flags, appended AFTER cfg.CLIArgs so they cannot be overridden
}

// NewClaudeCLIClient builds a CLIClient backed by `claude -p`. The default flags
// run the CLI as a bare model: --bare strips the agent scaffolding, --tools ""
// disables its tools, and --strict-mcp-config plus an absent MCP config keeps
// external servers out. The system prompt is passed per request via
// --system-prompt (see CompletionsWithCtx), so Request.System stays empty and
// the library never also emits --append-system-prompt.
//
// Security-critical flags (--tools "" and --strict-mcp-config) are appended AFTER
// any user-supplied cfg.CLIArgs so a configured cli_args value cannot supersede
// them.
func NewClaudeCLIClient(cfg ClientConfig) *CLIClient {
	return &CLIClient{
		cfg:          cfg,
		provider:     llmdriver.ProviderClaude,
		cliName:      "claude",
		baseArgs:     []string{"--bare"},
		securityArgs: []string{"--tools", "", "--strict-mcp-config"},
	}
}

// NewCodexCLIClient builds a CLIClient backed by `codex exec`. --ephemeral keeps
// the run non-persistent and --ignore-user-config keeps the user's MCP servers
// and settings out of a review run. shell_environment_policy.inherit="none"
// scrubs the environment handed to the shell commands the model runs, closing the
// direct `env`-in-a-shell channel by which a prompt-injected model could read
// OCR's inherited secrets (e.g. OCR_LLM_TOKEN, GITHUB_TOKEN). It does not remove
// those vars from Codex's own process — the library sets no cmd.Env and its
// runner is not overridable — so under Codex's read-only sandbox (reads allowed,
// writes and network blocked) the secrets remain reachable on Linux via
// /proc/<pid>/environ; that residual host-read exposure is documented, not code-
// closed. The system prompt goes through Request.System, which the library maps
// to `-c developer_instructions=`.
//
// Codex is deliberately not pointed at the repo (`-C`): OCR feeds the diff on
// stdin and serves file reads through its own confined tools, so a working
// directory would hand Codex host access OCR never needs. The process runs in
// its own working directory, which is OCR's — the same directory the earlier
// `-C $(pwd)` fallback resolved to in production, so this drops a stranded knob
// without changing where Codex runs.
//
// Security-critical flags (--ignore-user-config and the shell_environment_policy
// config) are appended AFTER any user-supplied cfg.CLIArgs so a configured
// cli_args value cannot supersede them.
func NewCodexCLIClient(cfg ClientConfig) *CLIClient {
	return &CLIClient{
		cfg:      cfg,
		provider: llmdriver.ProviderOpenAI,
		cliName:  "codex",
		baseArgs: []string{"--ephemeral"},
		securityArgs: []string{
			"--ignore-user-config",
			"-c", `shell_environment_policy.inherit="none"`,
		},
	}
}

// cliMaxTokens is the fallback output-token budget. The library requires a
// positive MaxTokens even though CLI flavors cannot strictly enforce it.
const cliMaxTokens = 8192

// cliErrMessageCap bounds the stderr excerpt carried into an error so a verbose
// CLI (or a huge stack trace) cannot flood the log, while still keeping enough
// to diagnose a logged-out CLI.
const cliErrMessageCap = 500

// CompletionsWithCtx runs one CLI round: it flattens the request into a single
// prompt, constrains the reply to a JSON envelope when tools are offered, and
// maps the structured reply back into a ChatResponse.
func (c *CLIClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	system, rest := splitSystem(req.Messages)

	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = cliMaxTokens
	}

	request := llmdriver.Request{
		Messages:  []llmdriver.Message{llmdriver.User(buildPrompt(rest, req.Tools))},
		MaxTokens: maxTokens,
	}

	// Per-provider argument assembly. Claude takes the system prompt as a flag
	// (leaving Request.System empty so the library adds no --append-system-prompt);
	// Codex takes it through Request.System. User cfg.CLIArgs come after baseArgs
	// but before securityArgs, so they cannot override the security-critical flags.
	args := append([]string(nil), c.baseArgs...)
	if c.provider == llmdriver.ProviderClaude {
		if system != "" {
			args = append(args, "--system-prompt", system)
		}
	} else {
		request.System = system
	}
	args = append(args, c.cfg.CLIArgs...)
	args = append(args, c.securityArgs...)

	toolNames := toolNameSet(req.Tools)
	if len(req.Tools) > 0 {
		request.OutputSchema = &llmdriver.OutputSchema{Name: "ocr_tool_calls", Schema: envelopeSchema(req.Tools)}
	}

	timeout := c.cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := llmdriver.New(llmdriver.Config{
		Provider: c.provider,
		Flavor:   llmdriver.FlavorCLI,
		Model:    model,
		CLIPath:  c.cfg.CLIPath,
		CLIArgs:  args,
	})
	if err != nil {
		return nil, fmt.Errorf("%s CLI configuration: %w", c.cliName, err)
	}

	resp, err := client.Generate(cctx, request)
	if err != nil {
		// A cancelled or timed-out context is returned unchanged so the agent
		// loop's own cancellation handling sees the sentinel it expects.
		if pe := ctx.Err(); pe != nil {
			return nil, pe
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		// A schema was requested but the model replied with non-JSON: the library
		// discards the text, so recovering it is impossible. Return an empty turn
		// with no tool calls so the agent loop retries the round rather than
		// aborting it (the same path a plain-text reply takes).
		var e *llmdriver.Error
		if errors.As(err, &e) && e.Code == "invalid_structured_output" {
			return c.buildResponse("", model, "", nil), nil
		}
		return nil, c.mapError(err)
	}

	// No schema: the reply is plain text with no tool calls.
	if len(req.Tools) == 0 {
		out := c.buildResponse(resp.ID, model, resp.Text, nil)
		out.Usage = usageFrom(resp.Usage)
		return out, nil
	}

	text, toolCalls := parseEnvelope(resp.StructuredOutput, resp.Text, toolNames)
	out := c.buildResponse(resp.ID, model, text, toolCalls)
	out.Usage = usageFrom(resp.Usage)
	return out, nil
}

// buildResponse assembles a ChatResponse from mapped pieces. Content is a nil
// pointer when empty so VisibleContent() reports no text, matching the other
// clients. Native stays empty: the transcript is rebuilt from Content/ToolCalls
// each round, so there is nothing to replay.
func (c *CLIClient) buildResponse(id, model, content string, toolCalls []ToolCall) *ChatResponse {
	var contentPtr *string
	if content != "" {
		contentPtr = &content
	}
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	return &ChatResponse{
		ID:    id,
		Model: model,
		Choices: []Choice{{
			Message: ResponseMessage{
				Role:      "assistant",
				Content:   contentPtr,
				ToolCalls: toolCalls,
			},
			FinishReason: finishReason,
		}},
	}
}

// mapError translates a library failure into an actionable message. A missing
// executable is the common first-run case and gets install/login guidance; a
// process failure keeps the CLI's own stderr excerpt (capped) so a logged-out
// CLI is diagnosable.
func (c *CLIClient) mapError(err error) error {
	if errors.Is(err, llmdriver.ErrExecutableNotFound) {
		exe := c.cliName
		if c.cfg.CLIPath != "" {
			exe = c.cfg.CLIPath
		}
		return fmt.Errorf("%s CLI not found on PATH (looked for %q); install it and run `%s login`, or set providers.<name>.cli_path: %w",
			c.cliName, exe, c.cliName, err)
	}
	var e *llmdriver.Error
	if errors.As(err, &e) {
		msg := strings.TrimSpace(e.Message)
		if len(msg) > cliErrMessageCap {
			msg = msg[:cliErrMessageCap]
		}
		return fmt.Errorf("%s CLI %s failed [%s]: %s", c.cliName, e.Operation, e.Code, msg)
	}
	return err
}

// splitSystem separates the leading system message (if any) from the rest of the
// transcript. Any further system message is dropped from the flattened prompt,
// as the CLI has one system channel.
func splitSystem(messages []Message) (system string, rest []Message) {
	for i := range messages {
		if messages[i].Role == "system" {
			if system == "" {
				system = messages[i].ExtractText()
			}
			continue
		}
		rest = append(rest, messages[i])
	}
	return system, rest
}

// buildPrompt flattens the transcript into a single prompt string. A lone
// tool-less user message is sent raw. Otherwise the prompt carries a tool
// catalog (when tools are offered), the conversation, and the response-format
// instruction (when tools are offered).
func buildPrompt(rest []Message, tools []ToolDef) string {
	if len(tools) == 0 && len(rest) == 1 && rest[0].Role == "user" {
		return rest[0].ExtractText()
	}

	var b strings.Builder
	if len(tools) > 0 {
		b.WriteString("## Tools\n")
		b.WriteString("You can call the tools below by returning JSON (see \"Response format\").\n")
		for _, t := range tools {
			fmt.Fprintf(&b, "\n### %s\n%s\nParameters (JSON Schema): %s\n",
				t.Function.Name, t.Function.Description, compactParams(t.Function.Parameters))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Conversation\n")
	b.WriteString(renderConversation(rest))

	if len(tools) > 0 {
		b.WriteString("\n\n## Response format\n")
		b.WriteString(`Return ONLY a JSON object: {"text": string, "tool_calls": [{"name": string, "arguments": string}]}, ` +
			`where "arguments" is a JSON object encoded as a string. Use "tool_calls": [] when you have nothing to call.`)
	}
	return b.String()
}

// renderConversation flattens the non-system messages into role-labelled blocks,
// renumbering tool-call ids to call_<n> so an assistant tool call and its result
// line up inside this transcript.
func renderConversation(rest []Message) string {
	idToCall := map[string]string{}
	nameByCall := map[string]string{}
	next := 0

	var blocks []string
	for i := range rest {
		msg := &rest[i]
		switch msg.Role {
		case "assistant":
			var sb strings.Builder
			sb.WriteString("[assistant]")
			if text := msg.ExtractText(); text != "" {
				sb.WriteString("\n")
				sb.WriteString(text)
			}
			for _, tc := range msg.ToolCalls {
				next++
				callID := fmt.Sprintf("call_%d", next)
				idToCall[tc.ID] = callID
				nameByCall[callID] = tc.Function.Name
				args := tc.Function.Arguments
				if strings.TrimSpace(args) == "" {
					args = "{}"
				}
				fmt.Fprintf(&sb, "\n{\"id\":%q,\"name\":%q,\"arguments\":%s}", callID, tc.Function.Name, args)
			}
			blocks = append(blocks, sb.String())
		case "tool":
			callID := idToCall[msg.ToolCallID]
			if callID == "" {
				callID = msg.ToolCallID
			}
			name := nameByCall[callID]
			blocks = append(blocks, fmt.Sprintf("[tool_result id=%s name=%s]\n%s", callID, name, msg.ExtractText()))
		default:
			blocks = append(blocks, "[user]\n"+msg.ExtractText())
		}
	}
	return strings.Join(blocks, "\n\n")
}

// cliEnvelope is the JSON reply shape the model is constrained to produce.
// Arguments is a string (a JSON object encoded as a string) because OpenAI
// strict structured output cannot represent an open object.
type cliEnvelope struct {
	Text      string `json:"text"`
	ToolCalls []struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"tool_calls"`
}

// parseEnvelope decodes the structured reply once into the visible text and the
// synthetic tool calls. Structured output is preferred; an empty structured
// field falls back to the trimmed raw text (the plain-text path). A reply that
// does not parse as the envelope yields the raw text and no tool calls, so the
// review filter's text fallback still has input and the agent loop retries. A
// call whose name is not in the offered set is dropped (logged to stdout, as the
// loop does). Text and calls share one parse so the empty-structured fallback
// cannot drift between them.
func parseEnvelope(structured json.RawMessage, rawText string, toolNames map[string]bool) (text string, toolCalls []ToolCall) {
	raw := structured
	if len(raw) == 0 {
		raw = json.RawMessage(strings.TrimSpace(rawText))
	}
	var env cliEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return rawText, nil
	}
	for _, tc := range env.ToolCalls {
		if !toolNames[tc.Name] {
			fmt.Fprintf(os.Stdout, "[ocr] CLI model requested unknown tool %q; dropping\n", tc.Name)
			continue
		}
		args := strings.TrimSpace(tc.Arguments)
		if args == "" {
			args = "{}"
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:   fmt.Sprintf("call_%d", len(toolCalls)+1),
			Type: "function",
			Function: FunctionCall{
				Name:      tc.Name,
				Arguments: args,
			},
		})
	}
	return env.Text, toolCalls
}

// envelopeSchema builds the strict JSON Schema the CLI is constrained to. OpenAI
// structured output requires additionalProperties:false on every object and
// every property in "required", and cannot represent an open object, so
// arguments is a string. The name enum limits the model to the offered tools.
func envelopeSchema(tools []ToolDef) json.RawMessage {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Function.Name)
	}
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
			"tool_calls": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"name":      map[string]any{"type": "string", "enum": names},
						"arguments": map[string]any{"type": "string"},
					},
					"required": []string{"name", "arguments"},
				},
			},
		},
		"required": []string{"text", "tool_calls"},
	}
	out, _ := json.Marshal(schema)
	return out
}

// toolNameSet is the lookup used to drop calls to tools that were not offered.
func toolNameSet(tools []ToolDef) map[string]bool {
	set := make(map[string]bool, len(tools))
	for _, t := range tools {
		set[t.Function.Name] = true
	}
	return set
}

// compactParams renders a tool's JSON Schema parameters as compact JSON for the
// prompt's tool catalog.
func compactParams(params map[string]any) string {
	if params == nil {
		return "{}"
	}
	out, err := json.Marshal(params)
	if err != nil {
		return "{}"
	}
	return string(out)
}

// usageFrom maps the library's token counts onto UsageInfo.
func usageFrom(u llmdriver.Usage) *UsageInfo {
	return &UsageInfo{
		PromptTokens:     int64(u.InputTokens),
		CompletionTokens: int64(u.OutputTokens),
		CacheReadTokens:  int64(u.CachedInputTokens),
		CacheWriteTokens: int64(u.CacheCreationInputTokens),
		TotalTokens:      int64(u.InputTokens + u.OutputTokens),
	}
}
