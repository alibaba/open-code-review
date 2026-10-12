// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

// copilotConversation keeps the CLI's native history between OCR tool rounds.
// OCR still executes every tool and decides whether another model call is allowed.
type copilotConversation struct {
	mu          sync.Mutex
	cancelMu    sync.Mutex
	cancel      context.CancelFunc
	closed      bool
	client      *copilot.Client
	session     *copilot.Session
	stateDir    string
	unsubscribe func()
	events      chan copilot.SessionEvent
	overflow    chan struct{}
	model       string
	system      string
	toolsKey    string
	maxTokens   int
	history     []copilotReplayMessage
	pending     map[string]string // tool call ID -> SDK request ID
}

func (c *CopilotClient) complete(ctx context.Context, req ChatRequest, model, system, prompt string, tools []copilot.Tool, allowed []string) (*ChatResponse, error) {
	toolBytes, err := json.Marshal(struct {
		Defs   []ToolDef
		Choice string
	}{req.Tools, req.ToolChoice})
	if err != nil {
		return nil, fmt.Errorf("encode Copilot tools: %w", err)
	}
	toolsKey := string(toolBytes)
	baseSystem := copilotBaseSystem(req.Messages)

	conversation := &copilotConversation{}
	if req.SessionID != "" {
		value, _ := c.sessions.LoadOrStore(req.SessionID, conversation)
		conversation = value.(*copilotConversation)
	}
	conversation.mu.Lock()
	defer conversation.mu.Unlock()
	ctx, finishRequest := conversation.requestContext(ctx)
	defer finishRequest()
	if err := ctx.Err(); err != nil {
		conversation.closeLocked()
		c.discardConversation(req.SessionID, conversation)
		return nil, err
	}

	// OCR compression or the grace round can replace the transcript or tool
	// allowlist. In those cases a fresh session receives OCR's authoritative
	// transcript; ordinary tool rounds keep their native SDK conversation.
	results, canContinue := conversation.continuation(req, model, baseSystem, toolsKey)
	if conversation.session != nil && !canContinue {
		conversation.closeLocked()
	}
	if conversation.session == nil {
		if err := conversation.start(ctx, c, model, system, baseSystem, toolsKey, req.MaxTokens, tools, allowed); err != nil {
			c.discardConversation(req.SessionID, conversation)
			return nil, err
		}
		if _, err := conversation.session.Send(ctx, copilot.MessageOptions{Prompt: prompt}); err != nil {
			conversation.closeLocked()
			c.discardConversation(req.SessionID, conversation)
			return nil, fmt.Errorf("send Copilot prompt: %w", err)
		}
	} else {
		if err := conversation.resolveTools(ctx, results); err != nil {
			conversation.closeLocked()
			c.discardConversation(req.SessionID, conversation)
			return nil, err
		}
	}

	message, usage, pending, err := conversation.wait(ctx)
	if err != nil {
		conversation.closeLocked()
		c.discardConversation(req.SessionID, conversation)
		return nil, err
	}
	response, err := copilotResponse(message, usage, system, prompt, tools, allowed, model)
	if err != nil {
		conversation.closeLocked()
		c.discardConversation(req.SessionID, conversation)
		return nil, err
	}
	conversation.history = copilotHistorySnapshot(req.Messages)
	conversation.pending = pending
	if req.SessionID == "" || len(pending) == 0 || hasCopilotTaskDone(response) {
		conversation.closeLocked()
		c.discardConversation(req.SessionID, conversation)
	}
	return response, nil
}

func hasCopilotTaskDone(response *ChatResponse) bool {
	for _, call := range response.ToolCalls() {
		if call.Function.Name == "task_done" {
			return true
		}
	}
	return false
}

func (c *CopilotClient) discardConversation(id string, conversation *copilotConversation) {
	if id != "" {
		c.sessions.CompareAndDelete(id, conversation)
	}
}

// CloseSession releases a review conversation even when OCR stops at its
// budget, timeout, compression, or task_done boundary.
func (c *CopilotClient) CloseSession(id string) {
	if id == "" {
		return
	}
	value, ok := c.sessions.LoadAndDelete(id)
	if !ok {
		return
	}
	conversation := value.(*copilotConversation)
	conversation.cancelRequest()
	conversation.mu.Lock()
	defer conversation.mu.Unlock()
	conversation.closeLocked()
}

func (conversation *copilotConversation) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	conversation.cancelMu.Lock()
	if conversation.closed {
		cancel()
	} else {
		conversation.cancel = cancel
	}
	conversation.cancelMu.Unlock()
	return ctx, func() {
		cancel()
		conversation.cancelMu.Lock()
		conversation.cancel = nil
		conversation.cancelMu.Unlock()
	}
}

func (conversation *copilotConversation) cancelRequest() {
	// Closing must interrupt the active request before waiting for its state lock.
	conversation.cancelMu.Lock()
	conversation.closed = true
	if conversation.cancel != nil {
		conversation.cancel()
	}
	conversation.cancelMu.Unlock()
}

func (conversation *copilotConversation) continuation(req ChatRequest, model, system, toolsKey string) (map[string]string, bool) {
	if conversation.session == nil || conversation.model != model || conversation.system != system || conversation.toolsKey != toolsKey || conversation.maxTokens != req.MaxTokens {
		return nil, false
	}
	if len(conversation.history) >= len(req.Messages) {
		return nil, false
	}
	for i, previous := range conversation.history {
		current := req.Messages[i]
		if previous.Role != current.Role || previous.Content != current.ExtractText() || previous.ToolCallID != current.ToolCallID || !slices.EqualFunc(previous.ToolCalls, current.ToolCalls, func(a, b ToolCall) bool {
			return a.ID == b.ID && a.Type == b.Type && a.Function == b.Function
		}) {
			return nil, false
		}
	}
	if len(conversation.pending) == 0 {
		return nil, false
	}
	results := make(map[string]string, len(conversation.pending))
	for _, message := range req.Messages[len(conversation.history):] {
		if message.Role == "tool" {
			if _, expected := conversation.pending[message.ToolCallID]; !expected {
				return nil, false
			}
			results[message.ToolCallID] = message.ExtractText()
		}
	}
	return results, len(results) == len(conversation.pending)
}

func (conversation *copilotConversation) start(ctx context.Context, owner *CopilotClient, model, system, baseSystem, toolsKey string, maxTokens int, tools []copilot.Tool, allowed []string) error {
	cliPath, err := copilotRuntimePath(owner.cliPath)
	if err != nil {
		return err
	}
	stateDir, err := os.MkdirTemp("", "ocr-copilot-")
	if err != nil {
		return fmt.Errorf("create Copilot session directory: %w", err)
	}
	conversation.stateDir = stateDir
	conversation.client = copilot.NewClient(&copilot.ClientOptions{
		Mode: copilot.ModeEmpty, BaseDirectory: stateDir,
		Connection: copilot.StdioConnection{Path: cliPath},
	})
	if err := conversation.client.Start(ctx); err != nil {
		conversation.closeLocked()
		return fmt.Errorf("start Copilot CLI: %w", err)
	}
	session, err := conversation.client.CreateSession(ctx, &copilot.SessionConfig{
		ClientName:              "open-code-review",
		Model:                   model,
		Provider:                owner.provider,
		Tools:                   tools,
		AvailableTools:          allowed,
		SystemMessage:           &copilot.SystemMessageConfig{Mode: "replace", Content: system},
		EnableConfigDiscovery:   copilot.Bool(false),
		EnableSkills:            copilot.Bool(false),
		EnableHostGitOperations: copilot.Bool(false),
	})
	if err != nil {
		conversation.closeLocked()
		return fmt.Errorf("create Copilot session: %w", err)
	}
	conversation.session = session
	conversation.events = make(chan copilot.SessionEvent, 128)
	conversation.overflow = make(chan struct{}, 1)
	events := conversation.events
	overflow := conversation.overflow
	conversation.unsubscribe = session.On(func(event copilot.SessionEvent) {
		switch event.Data.(type) {
		case *copilot.AssistantMessageData, *copilot.AssistantUsageData,
			*copilot.ExternalToolRequestedData, *copilot.SessionIdleData,
			*copilot.SessionErrorData:
			select {
			case events <- event:
			default:
				select {
				case overflow <- struct{}{}:
				default:
				}
			}
		}
	})
	conversation.model = model
	conversation.system = baseSystem
	conversation.toolsKey = toolsKey
	conversation.maxTokens = maxTokens
	return nil
}

func (conversation *copilotConversation) resolveTools(ctx context.Context, results map[string]string) error {
	for callID, requestID := range conversation.pending {
		result, err := conversation.session.RPC.Tools.HandlePendingToolCall(ctx, &rpc.HandlePendingToolCallRequest{
			RequestID: requestID, Result: rpc.ExternalToolStringResult(results[callID]),
		})
		if err != nil {
			return fmt.Errorf("return OCR tool result %q to Copilot: %w", callID, err)
		}
		if result == nil || !result.Success {
			return fmt.Errorf("Copilot rejected OCR tool result %q", callID)
		}
	}
	conversation.pending = nil
	return nil
}

func (conversation *copilotConversation) wait(ctx context.Context) (*copilot.AssistantMessageData, []*copilot.AssistantUsageData, map[string]string, error) {
	var message copilot.AssistantMessageData
	var usages []*copilot.AssistantUsageData
	var haveMessage bool
	var messageComplete bool
	pending := make(map[string]string)
	for {
		select {
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		case <-conversation.overflow:
			return nil, nil, nil, errors.New("Copilot session event buffer overflowed")
		case event := <-conversation.events:
			switch data := event.Data.(type) {
			case *copilot.AssistantMessageData:
				sameCall := message.APICallID != nil && data.APICallID != nil && *message.APICallID != "" && *message.APICallID == *data.APICallID
				if haveMessage && message.MessageID != "" && data.MessageID != message.MessageID && len(message.ToolRequests) > 0 && !sameCall {
					return nil, nil, nil, errors.New("Copilot advanced past an OCR tool request before OCR returned its result")
				}
				haveMessage = true
				message.MessageID = data.MessageID
				message.APICallID = data.APICallID
				message.Content += data.Content
				message.ToolRequests = append(message.ToolRequests, data.ToolRequests...)
				// This is the API-call total, so repeated chunks must not be summed.
				if data.OutputTokens != nil && (message.OutputTokens == nil || *data.OutputTokens > *message.OutputTokens) {
					total := *data.OutputTokens
					message.OutputTokens = &total
				}
				messageComplete = data.ChunkCount == nil || data.ChunkIndex == nil || *data.ChunkIndex >= *data.ChunkCount-1
			case *copilot.AssistantUsageData:
				usages = append(usages, data)
			case *copilot.ExternalToolRequestedData:
				if data.ToolCallID == "" || data.RequestID == "" {
					return nil, nil, nil, errors.New("Copilot returned an invalid pending tool request")
				}
				if haveMessage && messageComplete {
					found := false
					for _, call := range message.ToolRequests {
						if call.ToolCallID == data.ToolCallID {
							found = true
							break
						}
					}
					if !found {
						return nil, nil, nil, fmt.Errorf("Copilot requested an unannounced tool call %q", data.ToolCallID)
					}
				}
				pending[data.ToolCallID] = data.RequestID
			case *copilot.SessionIdleData:
				if !haveMessage {
					return nil, nil, nil, errors.New("Copilot session finished without an assistant message")
				}
				if !messageComplete || !copilotPendingMatches(message.ToolRequests, pending) {
					return nil, nil, nil, errors.New("Copilot session stopped without all pending OCR tool requests")
				}
				return &message, usages, pending, nil
			case *copilot.SessionErrorData:
				return nil, nil, nil, fmt.Errorf("Copilot session error: %s", data.Message)
			}
			if haveMessage && messageComplete && len(message.ToolRequests) > 0 && copilotPendingMatches(message.ToolRequests, pending) {
				return &message, usages, pending, nil
			}
		}
	}
}

func copilotPendingMatches(requests []copilot.AssistantMessageToolRequest, pending map[string]string) bool {
	if len(requests) != len(pending) {
		return false
	}
	for _, call := range requests {
		if pending[call.ToolCallID] == "" {
			return false
		}
	}
	return true
}

func (conversation *copilotConversation) closeLocked() {
	if conversation.unsubscribe != nil {
		conversation.unsubscribe()
		conversation.unsubscribe = nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if conversation.session != nil {
		_ = conversation.session.Abort(cleanupCtx)
		_ = conversation.session.Disconnect()
		if conversation.client != nil {
			_ = conversation.client.DeleteSession(cleanupCtx, conversation.session.SessionID)
		}
		conversation.session = nil
	}
	if conversation.client != nil {
		_ = conversation.client.Stop()
		conversation.client = nil
	}
	if conversation.stateDir != "" {
		_ = os.RemoveAll(conversation.stateDir)
		conversation.stateDir = ""
	}
	conversation.events = nil
	conversation.overflow = nil
	conversation.pending = nil
	conversation.history = nil
}
