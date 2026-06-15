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
	"or npm install -g @cursor-go-sdk/cursor-sdk-bridge@0.0.2"

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

	agent, err := cursor.CreateAgent(ctx, b.agentOptions(model))
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
	if result.Status == cursor.RunStatusError {
		return nil, fmt.Errorf("cursor prompt failed: %s", result.Result)
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

	model := req.Model
	if model == "" {
		model = b.cfg.Model
	}

	customTools := toolDefsToCustomTools(ctx, req.Tools, exec)

	agent, err := cursor.CreateAgent(ctx, b.agentOptions(model))
	if err != nil {
		return wrapCursorError(err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = agent.Close(closeCtx)
	}()

	var rec *session.TaskRecord
	if hooks.AppendTaskRecord != nil {
		rec = hooks.AppendTaskRecord(session.MainTask, append([]llm.Message(nil), req.Messages...))
	}

	start := time.Now()
	usageAcc := &cursorUsageAccumulator{}
	run, err := agent.Send(ctx, messagesToPrompt(req.Messages), cursor.SendOptions{
		Local: &cursor.LocalSendOptions{
			CustomTools: customTools,
		},
		OnDelta: usageAcc.callback(),
	})
	if err != nil {
		if hooks.SetError != nil && rec != nil {
			hooks.SetError(rec, err, time.Since(start).Milliseconds())
		}
		return wrapCursorError(err)
	}

	result, err := run.Wait(ctx)
	duration := time.Since(start)
	if err != nil {
		if hooks.SetError != nil && rec != nil {
			hooks.SetError(rec, err, duration.Milliseconds())
		}
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
		}, duration.Milliseconds())
	}
	usage := usageAcc.usage()
	if usage != nil && hooks.RecordUsage != nil {
		hooks.RecordUsage(usage)
	}
	totalTokens := int64(0)
	if usage != nil {
		totalTokens = usage.TotalTokens
	}
	if hooks.RecordLLMRequest != nil {
		hooks.RecordLLMRequest(duration.Milliseconds(), totalTokens, string(result.Status))
	}

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

func (b *CursorAgentBackend) agentOptions(model string) cursor.AgentOptions {
	sandboxEnabled := true
	return cursor.AgentOptions{
		Model:  model,
		APIKey: b.cfg.APIKey,
		Local: &cursor.LocalAgentOptions{
			CWD:            []string{b.repoDir},
			SettingSources: nil,
			SandboxOptions: &cursor.SandboxOptions{Enabled: &sandboxEnabled},
			CustomTools:    nil,
		},
	}
}

func toolDefsToCustomTools(ctx context.Context, defs []llm.ToolDef, exec ToolExecutor) map[string]cursor.CustomTool {
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
