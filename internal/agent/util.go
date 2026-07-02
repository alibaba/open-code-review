package agent

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/session"
)

// planBlockPattern matches the optional "Review Plan" section in a MAIN_TASK
// template user message: a header line beginning with "### " whose text
// contains "Review Plan" or "审查计划" (with optional ASCII "(Optional)" /
// Chinese "（可选）" suffix), the {{plan_guidance}} placeholder on its own
// line, and one trailing blank line. The ASCII and Chinese header forms
// are matched separately because Go's regexp engine does not define \b
// around CJK ideographs.
var planBlockPattern = regexp.MustCompile(
	`(?m)^### [^\n]*(?:Review Plan|审查计划)[^\n]*\n\{\{plan_guidance\}\}\n\n?`)

// stripEmptyPlanBlock removes the "### Review Plan …\n{{plan_guidance}}\n\n"
// wrapper from a MAIN_TASK user message when the plan phase produced no
// guidance. The previous implementation hard-coded a single Chinese literal,
// which did not match the actual English template shipped in
// task_template.json, so the literal token "{{plan_guidance}}" leaked into
// the rendered prompt on every review where the plan phase was skipped or
// failed. Strip is a no-op when the wrapper is absent.
func stripEmptyPlanBlock(content string) string {
	return planBlockPattern.ReplaceAllString(content, "")
}

// stripMarkdownFences removes ```json and ``` wrappers that some models
// add around structured outputs.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		} else {
			s = strings.TrimPrefix(s, "```json")
			s = strings.TrimPrefix(s, "```")
		}
	}
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	return s
}

func buildMessageXML(msgs []llm.Message) string {
	var sb strings.Builder
	for i, m := range msgs {
		sb.WriteString(fmt.Sprintf("<message id=\"%d\" role=\"%s\">\n", i, m.Role))
		sb.WriteString("    <content>\n")
		sb.WriteString(fmt.Sprintf("      %s\n", m.ExtractText()))
		sb.WriteString("    </content>\n")
		sb.WriteString("</message>")
		if i < len(msgs)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func copyMessages(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, len(msgs))
	for i, m := range msgs {
		out[i] = llm.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			ToolCalls:  append([]llm.ToolCall(nil), m.ToolCalls...),
		}
	}
	return out
}

func countMessagesTokens(msgs []llm.Message) int {
	var total int
	for _, m := range msgs {
		total += llm.CountTokens(m.ExtractText())
	}
	return total
}

func reviewModeString(from, to, commit string) string {
	if commit != "" {
		return session.ReviewModeCommit
	}
	if from != "" && to != "" {
		return session.ReviewModeRange
	}
	return session.ReviewModeWorkspace
}

// detectGitBranch returns the current git branch name for the given repo, or empty string on failure.
func detectGitBranch(ctx context.Context, repoDir string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return ""
	}
	return strings.TrimSpace(string(out))
}
