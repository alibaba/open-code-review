package reviewbackend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/session"
	"github.com/remdev/cursor-go-sdk/cursor"
)

const bridgeSetupHint = "install Cursor bridge: go run github.com/remdev/cursor-go-sdk/cmd/setup@latest " +
	"or npm install -g @cursor-go-sdk/cursor-sdk-bridge@0.0.3"

// CursorAgentBackend runs review via Cursor Agent SDK local runtime and custom tools.
type CursorAgentBackend struct {
	cfg     CursorConfig
	repoDir string
}

// NewCursorAgentBackend validates prerequisites and returns a Cursor backend.
func NewCursorAgentBackend(ctx context.Context, cfg CursorConfig, repoDir string) (*CursorAgentBackend, error) {
	if err := cursor.EnsureBridgeInstalled(ctx); err != nil {
		return nil, fmt.Errorf("cursor bridge not ready: %w; %s", err, bridgeSetupHint)
	}
	if cfg.APIKey == "" {
		return nil, errors.New("cursor api key is required (set providers.cursor.api_key or CURSOR_API_KEY)")
	}
	if cfg.Model == "" {
		return nil, errors.New("cursor model is required")
	}
	if repoDir == "" {
		return nil, errors.New("cursor local agent requires repo directory")
	}
	return &CursorAgentBackend{cfg: cfg, repoDir: repoDir}, nil
}

func (b *CursorAgentBackend) Kind() Kind { return KindCursorAgent }

func (b *CursorAgentBackend) Model() string { return b.cfg.Model }

func (b *CursorAgentBackend) Source() string { return b.cfg.Source }

func (b *CursorAgentBackend) Complete(ctx context.Context, req CompleteRequest) (*CompleteResponse, error) {
	model := req.Model
	if model == "" {
		model = b.cfg.Model
	}

	agent, err := cursor.CreateAgent(ctx, b.agentOptions(model, nil))
	if err != nil {
		return nil, wrapCursorError(err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = agent.Close(closeCtx)
	}()

	usageAcc := &cursorUsageAccumulator{}
	run, err := agent.Send(ctx, messagesToPrompt(req.Messages), cursor.SendOptions{
		OnDelta: usageAcc.callback(),
	})
	if err != nil {
		return nil, wrapCursorError(err)
	}

	result, err := run.Wait(ctx)
	if err != nil {
		return nil, wrapCursorError(err)
	}
	if err := cursorRunStatusError(result); err != nil {
		return nil, err
	}

	return &CompleteResponse{
		Content: result.Result,
		Model:   model,
		Usage:   usageAcc.usage(),
	}, nil
}

func (b *CursorAgentBackend) ReviewFile(ctx context.Context, req ReviewFileRequest, exec ToolExecutor, hooks *ReviewHooks) error {
	if hooks == nil {
		hooks = &ReviewHooks{}
	}
	logf := hooks.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}

	model := req.Model
	if model == "" {
		model = b.cfg.Model
	}

	tracker := &cursorReviewTracker{}
	mcpExec := func(callCtx context.Context, call ToolCallInput) ToolCallOutput {
		tracker.markTool(call.Name)
		out := exec(callCtx, call)
		if out.Completed {
			tracker.taskDone = true
		}
		return out
	}

	customTools := toolDefsToCustomTools(ctx, req.Tools, req.FilePath, mcpExec)

	agent, err := cursor.CreateAgent(ctx, b.agentOptions(model, customTools))
	if err != nil {
		return wrapCursorError(err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = agent.Close(closeCtx)
	}()

	maxRounds := req.MaxToolRounds
	if maxRounds <= 0 {
		maxRounds = 1
	}

	prompt := buildCursorReviewPrompt(req.Messages, req.ToolsPrompt)
	basePrompt := prompt

	for round := 0; round < maxRounds; round++ {
		tracker.roundToolInvoked = false

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		usageAcc := &cursorUsageAccumulator{}
		sendOpts := cursor.SendOptions{
			Mode:    cursor.AgentModeAgent,
			OnDelta: usageAcc.callback(),
		}

		var rec *session.TaskRecord
		if hooks.AppendTaskRecord != nil {
			msgs := req.Messages
			if round > 0 {
				msgs = []llm.Message{llm.NewTextMessage("user", prompt)}
			}
			rec = hooks.AppendTaskRecord(session.MainTask, append([]llm.Message(nil), msgs...))
		}

		start := time.Now()
		run, err := agent.Send(ctx, prompt, sendOpts)
		if err != nil {
			durationMs := time.Since(start).Milliseconds()
			if hooks.SetError != nil && rec != nil {
				hooks.SetError(rec, err, durationMs)
			}
			recordCursorRoundMetrics(hooks, usageAcc, durationMs, "error")
			return wrapCursorError(err)
		}

		result, err := run.Wait(ctx)
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			if hooks.SetError != nil && rec != nil {
				hooks.SetError(rec, err, durationMs)
			}
			recordCursorRoundMetrics(hooks, usageAcc, durationMs, "error")
			return wrapCursorError(err)
		}

		if hooks.SetResponse != nil && rec != nil {
			content := result.Result
			hooks.SetResponse(rec, &llm.ChatResponse{
				Model: model,
				Choices: []llm.Choice{{
					Message:      llm.ResponseMessage{Role: "assistant", Content: &content},
					FinishReason: string(result.Status),
				}},
				Usage: usageAcc.usage(),
			}, durationMs)
		}

		if err := cursorRunStatusError(result); err != nil {
			if hooks.SetError != nil && rec != nil {
				hooks.SetError(rec, err, durationMs)
			}
			recordCursorRoundMetrics(hooks, usageAcc, durationMs, "error")
			return err
		}
		recordCursorRoundMetrics(hooks, usageAcc, durationMs, string(result.Status))

		if tracker.taskDone {
			return nil
		}
		if tracker.roundToolInvoked {
			break
		}
		if round+1 >= maxRounds {
			break
		}
		logf("[ocr] Cursor agent replied without MCP tools for %s, retrying...\n", req.FilePath)
		prompt = basePrompt + "\n\n" + cursorReviewNudgeNoTools()
	}

	if tracker.taskDone || tracker.commentCalls > 0 {
		return nil
	}
	logf("[ocr] Cursor review for %s produced no comments.\n", req.FilePath)
	return fmt.Errorf("cursor review incomplete for %s", req.FilePath)
}

type cursorReviewTracker struct {
	taskDone         bool
	commentCalls     int
	roundToolInvoked bool
}

func (t *cursorReviewTracker) markTool(name string) {
	t.roundToolInvoked = true
	if name == "code_comment" {
		t.commentCalls++
	}
}

func recordCursorRoundMetrics(hooks *ReviewHooks, usageAcc *cursorUsageAccumulator, durationMs int64, status string) {
	if hooks == nil {
		return
	}
	if usage := usageAcc.usage(); usage != nil && hooks.RecordUsage != nil {
		hooks.RecordUsage(usage)
	}
	totalTokens := int64(0)
	if usage := usageAcc.usage(); usage != nil {
		totalTokens = usage.TotalTokens
	}
	if hooks.RecordLLMRequest != nil {
		hooks.RecordLLMRequest(durationMs, totalTokens, status)
	}
}

func cursorReviewNudgeNoTools() string {
	return "You must not reply with a markdown review. For each confirmed issue in the diff, call the code_comment tool with structured comments (path, start_line, end_line, content). When finished, call task_done."
}

func cursorRunStatusError(result cursor.RunResult) error {
	switch result.Status {
	case cursor.RunStatusFinished:
		return nil
	case cursor.RunStatusCancelled:
		return fmt.Errorf("cursor review cancelled")
	case cursor.RunStatusExpired:
		return fmt.Errorf("cursor review expired")
	case cursor.RunStatusError:
		if result.Result != "" {
			return fmt.Errorf("cursor review failed: %s", result.Result)
		}
		return errors.New("cursor review failed")
	default:
		return nil
	}
}

func (b *CursorAgentBackend) agentOptions(model string, customTools map[string]cursor.CustomTool) cursor.AgentOptions {
	// Sandbox is disabled so MCP custom-user-tools can run. Operators should treat
	// the local Cursor agent as trusted code with full repo access (see Cursor SDK docs).
	sandboxEnabled := false
	return cursor.AgentOptions{
		Model:  model,
		APIKey: b.cfg.APIKey,
		Mode:   cursor.AgentModeAgent,
		Local: &cursor.LocalAgentOptions{
			CWD:            []string{b.repoDir},
			SettingSources: nil,
			SandboxOptions: &cursor.SandboxOptions{Enabled: &sandboxEnabled},
			CustomTools:    customTools,
		},
	}
}

func toolDefsToCustomTools(ctx context.Context, defs []llm.ToolDef, filePath string, exec ToolExecutor) map[string]cursor.CustomTool {
	if len(defs) == 0 {
		return nil
	}
	out := make(map[string]cursor.CustomTool, len(defs))
	for _, def := range defs {
		name := def.Function.Name
		fn := def.Function
		out[name] = cursor.CustomTool{
			Description: fn.Description,
			InputSchema: fn.Parameters,
			Execute: func(args map[string]any, tctx cursor.CustomToolContext) (any, error) {
				if name == "code_comment" {
					args = normalizeCodeCommentArgs(args, filePath)
				}
				raw, err := json.Marshal(args)
				if err != nil {
					return nil, err
				}
				result := exec(ctx, ToolCallInput{
					ID:        tctx.ToolCallID,
					Name:      name,
					Arguments: string(raw),
				})
				if result.Completed {
					return "Task completed successfully.", nil
				}
				if result.Result == "" {
					return "Error: Tool execution returned no result.", nil
				}
				return result.Result, nil
			},
		}
	}
	return out
}

func messagesToPrompt(msgs []llm.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		role := m.Role
		switch role {
		case "system":
			role = "System"
		case "assistant":
			role = "Assistant"
		case "user":
			role = "User"
		case "tool":
			role = "Tool"
		default:
			if role == "" {
				role = "User"
			} else {
				role = strings.ToUpper(role[:1]) + role[1:]
			}
		}
		sb.WriteString(role)
		sb.WriteString(":\n")
		sb.WriteString(m.ExtractText())
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

func wrapCursorError(err error) error {
	var ae *cursor.AgentError
	if errors.As(err, &ae) {
		return fmt.Errorf("cursor agent error (%s): %s; %s", ae.Code, ae.Message, bridgeSetupHint)
	}
	if strings.Contains(strings.ToLower(err.Error()), "sandbox") ||
		strings.Contains(strings.ToLower(err.Error()), "configuration") {
		return fmt.Errorf("%w; cursor sandbox may be unavailable on this host — %s", err, bridgeSetupHint)
	}
	return err
}
