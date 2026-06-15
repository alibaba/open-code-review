package reviewbackend

import (
	"fmt"
	"sort"
	"strings"

	"github.com/open-code-review/open-code-review/internal/llm"
)

// buildCursorReviewPrompt formats OCR messages for Cursor agent.send.
// Custom tools are registered as MCP server "custom-user-tools"; the prompt must
// tell the model to call them instead of emitting a markdown review.
func buildCursorReviewPrompt(msgs []llm.Message, toolsPrompt string) string {
	var sb strings.Builder
	sb.WriteString(messagesToPrompt(msgs))
	sb.WriteString("\n\n## OCR review tools (MCP: custom-user-tools)\n")
	sb.WriteString("Do not write a markdown code review. Use the tools below:\n")
	sb.WriteString("- Call `code_comment` for each confirmed issue (use the comments[] schema with existing_code from the diff).\n")
	sb.WriteString("- Call `task_done` when finished.\n")
	sb.WriteString("- Use context tools only when needed to confirm an issue in the current diff.\n")
	if toolsPrompt != "" {
		sb.WriteString("\n")
		sb.WriteString(toolsPrompt)
	}
	return strings.TrimSpace(sb.String())
}

// normalizeCursorToolName strips MCP server prefixes (e.g. custom-user-tools/code_comment).
func normalizeCursorToolName(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if strings.HasPrefix(name, "custom-user-tools__") {
		return strings.TrimPrefix(name, "custom-user-tools__")
	}
	return name
}

// FormatCursorToolDefs renders main-task tool definitions for Cursor MCP prompts.
func FormatCursorToolDefs(defs []llm.ToolDef) string {
	return formatCursorToolDefs(defs)
}

func formatCursorToolDefs(defs []llm.ToolDef) string {
	if len(defs) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, td := range defs {
		fn := &td.Function
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", fn.Name, fn.Description))
		if params, ok := fn.Parameters["properties"].(map[string]any); ok && len(params) > 0 {
			sb.WriteString("  Parameters:\n")
			required := make(map[string]bool)
			if reqList, ok := fn.Parameters["required"].([]any); ok {
				for _, r := range reqList {
					if s, ok := r.(string); ok {
						required[s] = true
					}
				}
			}
			names := make([]string, 0, len(params))
			for name := range params {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				p := params[name]
				suffix := ""
				if required[name] {
					suffix = " (required)"
				}
				if pm, ok := p.(map[string]any); ok {
					desc, _ := pm["description"].(string)
					sb.WriteString(fmt.Sprintf("  - %s: %s%s\n", name, desc, suffix))
				} else {
					sb.WriteString(fmt.Sprintf("  - %s%s\n", name, suffix))
				}
			}
		}
	}
	return sb.String()
}
