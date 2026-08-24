// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/telemetry"
)

type promptReplacement struct {
	token string
	value string
}

const candidateReLocationRetryPrompt = `The previous answer was not valid for candidate selection. Return exactly one JSON object and nothing else.

Use only a candidate_id from the candidate list. Return {"candidate_id":null} only when no candidate is clearly correct. Do not use Markdown, labels, or prose.`

// BuildReLocationMessages renders the snippet-based re-location prompt.
// It returns nil when the task template is absent or empty, which the caller
// treats as "no re-location attempt": no session record, no request.
//
// Prompt rendering stays separate from ReLocateComment so the caller can create
// the ReLocationTask session record, including RequestNo, before the HTTP call.
// Keeping this function pure also keeps session and request identity concerns
// out of package diff.
func BuildReLocationMessages(cm *model.LlmComment, d *model.Diff, task *template.LlmConversation) []llm.Message {
	return renderPromptMessages(task, []promptReplacement{
		{token: "{diff}", value: d.Diff},
		{token: "{existing_code}", value: cm.ExistingCode},
		{token: "{suggestion_content}", value: cm.Content},
	})
}

// BuildCandidateReLocationMessages renders the candidate-selection prompt.
func BuildCandidateReLocationMessages(cm *model.LlmComment, candidates []CommentLocationCandidate, task *template.LlmConversation) []llm.Message {
	if len(candidates) == 0 {
		return nil
	}

	return renderPromptMessages(task, []promptReplacement{
		{token: "{suggestion_content}", value: cm.Content},
		{token: "{existing_code}", value: cm.ExistingCode},
		{token: "{suggestion_code}", value: strings.TrimSpace(cm.SuggestionCode)},
		{token: "{thinking}", value: strings.TrimSpace(cm.Thinking)},
		{token: "{candidates}", value: renderCandidateList(candidates)},
	})
}

// BuildCandidateReLocationRetryMessages appends a local repair turn after the
// model answered in the wrong format. The retry conversation is scoped to the
// re-location task only and is never appended to the main review loop.
func BuildCandidateReLocationRetryMessages(messages []llm.Message, previous string) []llm.Message {
	out := append([]llm.Message(nil), messages...)
	if previous = strings.TrimSpace(previous); previous != "" {
		out = append(out, llm.NewTextMessage("assistant", previous))
	}
	out = append(out, llm.NewTextMessage("user", candidateReLocationRetryPrompt))
	return out
}

func renderPromptMessages(task *template.LlmConversation, replacements []promptReplacement) []llm.Message {
	if task == nil || len(task.Messages) == 0 {
		return nil
	}

	messages := make([]llm.Message, 0, len(task.Messages))
	for _, m := range task.Messages {
		messages = append(messages, llm.NewTextMessage(m.Role, replacePromptTokens(m.Content, replacements)))
	}
	return messages
}

func replacePromptTokens(content string, replacements []promptReplacement) string {
	args := make([]string, 0, len(replacements)*2)
	for _, r := range replacements {
		args = append(args, r.token, r.value)
	}
	return strings.NewReplacer(args...).Replace(content)
}

func renderCandidateList(candidates []CommentLocationCandidate) string {
	type candidatePromptItem struct {
		CandidateID string `json:"candidate_id"`
		Path        string `json:"path"`
		StartLine   int    `json:"start_line"`
		EndLine     int    `json:"end_line"`
		MatchedCode string `json:"matched_code"`
		Context     string `json:"context,omitempty"`
	}

	items := make([]candidatePromptItem, 0, len(candidates))
	for _, c := range candidates {
		item := candidatePromptItem{
			CandidateID: c.ID,
			Path:        c.Path,
			StartLine:   c.StartLine,
			EndLine:     c.EndLine,
			MatchedCode: c.Snippet,
		}
		if strings.TrimSpace(c.Context) != "" && c.Context != c.Snippet {
			item.Context = c.Context
		}
		items = append(items, item)
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

// ReLocateComment asks the LLM to regenerate a precise existing_code snippet
// when text matching fails, then retries ResolveComment with the new snippet.
// The caller owns session recording, so this returns only success and response.
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

// ReLocateCommentCandidate asks the LLM to choose a precomputed candidate.
func ReLocateCommentCandidate(
	ctx context.Context,
	cm *model.LlmComment,
	candidates []CommentLocationCandidate,
	client llm.LLMClient,
	messages []llm.Message,
	modelName string,
	maxTokens int,
) (bool, *llm.ChatResponse) {
	if len(messages) == 0 || len(candidates) == 0 {
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
		fmt.Fprintf(stdout.Writer(), "[ocr] Re-location candidate selection failed for %s: %v\n", cm.Path, err)
		return false, nil
	}
	var totalTokens int64
	if resp.Usage != nil {
		totalTokens = resp.Usage.TotalTokens
	}
	telemetry.RecordLLMResult(llmSpan, duration, totalTokens, nil)
	llmSpan.End()

	id, ok := ParseCandidateID(resp.Content())
	if !ok || id == "" {
		return false, resp
	}
	for _, c := range candidates {
		if c.ID == id {
			ApplyCandidate(cm, c)
			return true, resp
		}
	}
	return false, resp
}

// ParseCandidateID reports whether content follows the strict candidate output
// contract: a JSON object with candidate_id.
func ParseCandidateID(content string) (string, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", false
	}
	if id, ok := parseCandidateIDJSON(content); ok {
		return id, true
	}
	if block := extractWholeCodeBlock(content); block != "" {
		return parseCandidateIDJSON(block)
	}
	return "", false
}

func parseCandidateIDJSON(content string) (string, bool) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &payload); err == nil && payload != nil {
		raw, ok := payload["candidate_id"]
		if !ok {
			return "", false
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", false
		}
		return normalizeCandidateID(value)
	}
	return "", false
}

func extractWholeCodeBlock(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return ""
	}
	afterOpen := 3
	if nl := strings.IndexByte(text[afterOpen:], '\n'); nl >= 0 {
		afterOpen += nl + 1
	} else {
		return ""
	}
	end := strings.Index(text[afterOpen:], "```")
	if end < 0 {
		return ""
	}
	afterClose := afterOpen + end + 3
	if strings.TrimSpace(text[afterClose:]) != "" {
		return ""
	}
	return strings.TrimSpace(text[afterOpen : afterOpen+end])
}

func normalizeCandidateID(value any) (string, bool) {
	switch v := value.(type) {
	case nil:
		return "", true
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return "", false
		}
		if !isPositiveInteger(v) {
			return "", false
		}
		return v, true
	case float64:
		n := int64(v)
		if v < 1 || v != float64(n) {
			return "", false
		}
		return fmt.Sprintf("%d", n), true
	default:
		return "", false
	}
}

func isPositiveInteger(s string) bool {
	for i, r := range s {
		if r < '0' || r > '9' {
			return false
		}
		if i == 0 && r == '0' {
			return false
		}
	}
	return s != ""
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
