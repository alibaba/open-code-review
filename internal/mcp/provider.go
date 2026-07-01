package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/tool"
)

// Provider adapts a single MCP tool to the tool.Provider interface.
type Provider struct {
	toolName string
	client   *Client
}

func (p *Provider) Tool() tool.Tool {
	return tool.Dynamic(p.toolName)
}

func (p *Provider) Execute(ctx context.Context, args map[string]any) (string, error) {
	return p.client.CallTool(ctx, p.toolName, args)
}

// RegisterAll registers tools from the MCP client into the tool registry.
// When allowedTools is non-empty, only tools whose names appear in the list are registered.
// Tools whose names conflict with built-in or already-registered tools are skipped with a warning.
func RegisterAll(reg *tool.Registry, c *Client, allowedTools []string) {
	allowed := make(map[string]struct{}, len(allowedTools))
	for _, name := range allowedTools {
		allowed[name] = struct{}{}
	}
	filtering := len(allowed) > 0

	matched := make(map[string]struct{})
	for _, t := range c.Tools() {
		if filtering {
			if _, ok := allowed[t.Name]; !ok {
				continue
			}
			matched[t.Name] = struct{}{}
		}
		if tool.IsReserved(t.Name) {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: MCP server %q tool %q conflicts with built-in tool, skipping\n", c.Name(), t.Name)
			continue
		}
		if _, exists := reg.Get(t.Name); exists {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: MCP server %q tool %q conflicts with already-registered tool, skipping\n", c.Name(), t.Name)
			continue
		}
		reg.Register(&Provider{
			toolName: t.Name,
			client:   c,
		})
	}

	for name := range allowed {
		if _, ok := matched[name]; !ok {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: MCP server %q allowed tool %q not found in server's tool list\n", c.Name(), name)
		}
	}
}

// ToToolDef converts an MCP tool definition to an llm.ToolDef.
func ToToolDef(t *mcp.Tool) llm.ToolDef {
	params := map[string]any{"type": "object"}

	switch schema := t.InputSchema.(type) {
	case map[string]any:
		for k, v := range schema {
			params[k] = v
		}
		if _, ok := params["type"]; !ok {
			params["type"] = "object"
		}
	case nil:
		// No schema — keep the default {"type": "object"}.
	default:
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: MCP tool %q has unexpected InputSchema type %T, using empty object schema\n", t.Name, t.InputSchema)
	}

	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  params,
		},
	}
}

// CollectToolDefs gathers tool definitions from MCP clients, filtering out
// tools that conflict with built-in tools or were not successfully registered.
func CollectToolDefs(clients []*Client, reg *tool.Registry) []llm.ToolDef {
	var defs []llm.ToolDef
	seen := make(map[string]struct{})
	for _, c := range clients {
		for _, t := range c.Tools() {
			if tool.IsReserved(t.Name) {
				continue
			}
			if _, exists := reg.Get(t.Name); !exists {
				continue
			}
			if _, dup := seen[t.Name]; dup {
				continue
			}
			seen[t.Name] = struct{}{}
			defs = append(defs, ToToolDef(t))
		}
	}
	return defs
}
