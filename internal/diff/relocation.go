// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/telemetry"
)

// BuildReLocationMessages renders the re-location prompt for cm against d.
// Returns nil when the task template is absent or empty, which the caller
// treats as "no re-location attempt": no session record, no request.
//
// This is split out of ReLocateComment so the caller can create the
// ReLocationTask session record — and therefore know its RequestNo — before any
// HTTP call happens. It is pure prompt construction: no client, no session, no
// request identity. Keeping it that way is what stops observability concerns
// from sinking into package diff.
func BuildReLocationMessages(cm *model.LlmComment, d *model.Diff, task *template.LlmConversation) []llm.Message {
	if task == nil || len(task.Messages) == 0 {
		return nil
	}

	messages := make([]llm.Message, 0, len(task.Messages))
	for _, m := range task.Messages {
		content := m.Content
		content = strings.ReplaceAll(content, "{diff}", d.Diff)
		content = strings.ReplaceAll(content, "{existing_code}", cm.ExistingCode)
		content = strings.ReplaceAll(content, "{suggestion_content}", cm.Content)
		messages = append(messages, llm.NewTextMessage(m.Role, content))
	}
	return messages
}

// ReLocateComment calls the LLM to regenerate a precise existing_code snippet
// when text-based matching fails, then retries ResolveComment with the new
// snippet. messages comes from BuildReLocationMessages; the caller has already
// recorded it in the session, so only (success, response) are returned here.
// Response is nil when the request failed.
func ReLocateComment(
	ctx context.Context,
	cm *model.LlmComment,
	d *model.Diff,
	client llm.LLMClient,
	messages []llm.Message,
	modelName string,
	maxTokens int,
) (bool, *llm.ChatResponse) {
	if len(messages) == 0 {
		return false, nil
	}

	startTime := time.Now()
	_, llmSpan := telemetry.StartLLMSpan(ctx, modelName)
	resp, err := client.CompletionsWithCtx(ctx, llm.ChatRequest{
		Model:     modelName,
		Messages:  messages,
		MaxTokens: maxTokens,
	})
	duration := time.Since(startTime)
	if err != nil {
		telemetry.RecordLLMResult(llmSpan, duration, 0, err)
		llmSpan.End()
		fmt.Fprintf(stdout.Writer(), "[ocr] Re-location LLM call failed for %s: %v\n", cm.Path, err)
		return false, nil
	}
	var totalTokens int64
	if resp.Usage != nil {
		totalTokens = resp.Usage.TotalTokens
	}
	telemetry.RecordLLMResult(llmSpan, duration, totalTokens, nil)
	llmSpan.End()

	code := extractCodeBlock(resp.Content())
	if code == "" {
		return false, resp
	}

	original := cm.ExistingCode
	cm.ExistingCode = code
	if ResolveComment(cm, d) {
		return true, resp
	}
	cm.ExistingCode = original
	return false, resp
}

// extractCodeBlock extracts the content of the first fenced code block from text.
// Returns empty string if no code block is found.
func extractCodeBlock(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "```")
	if start < 0 {
		return ""
	}
	afterOpen := start + 3
	// Skip optional language tag on the opening fence line.
	if nl := strings.IndexByte(text[afterOpen:], '\n'); nl >= 0 {
		afterOpen += nl + 1
	} else {
		return ""
	}
	end := strings.Index(text[afterOpen:], "```")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(text[afterOpen : afterOpen+end])
}
