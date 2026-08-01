package tool

import (
	"context"
	"errors"
	"fmt"
)

// Tool represents a single review tool.
type Tool struct {
	name string
}

var (
	Unknown      = Tool{name: "unknown"}
	TaskDone     = Tool{name: "task_done"}
	CodeComment  = Tool{name: "code_comment"}
	FileRead     = Tool{name: "file_read"}
	FileFind     = Tool{name: "file_find"}
	FileReadDiff = Tool{name: "file_read_diff"}
	CodeSearch   = Tool{name: "code_search"}
)

func OfName(name string) Tool {
	for _, t := range allTools() {
		if t.name == name {
			return t
		}
	}
	return Unknown
}

func allTools() []Tool {
	return []Tool{Unknown, TaskDone, CodeComment, FileRead, FileFind, FileReadDiff, CodeSearch}
}

// IsReserved reports whether name matches any built-in tool name (including Unknown).
func IsReserved(name string) bool {
	for _, t := range allTools() {
		if t.name == name {
			return true
		}
	}
	return false
}

// Dynamic creates a Tool with the given name for dynamically discovered tools (e.g. MCP).
func Dynamic(name string) Tool {
	if name == "" {
		panic("tool: Dynamic called with empty name")
	}
	if IsReserved(name) {
		panic(fmt.Sprintf("tool: Dynamic called with reserved tool name %q", name))
	}
	return Tool{name: name}
}

// Name returns the tool's identifier name.
func (t Tool) Name() string { return t.name }

// IsKnown reports whether the tool is not UNKNOWN.
func (t Tool) IsKnown() bool {
	return t != Unknown
}

// Provider is the interface that all concrete tool implementations satisfy.
// Each tool handles one specific capability (read file, search code, etc.).
type Provider interface {
	// Tool returns which tool this provider implements.
	Tool() Tool
	// Execute runs the tool with the given arguments and returns the result string.
	Execute(ctx context.Context, args map[string]any) (string, error)
}

// Registry holds tool providers. It is safe for concurrent reads after Freeze.
type Registry struct {
	providers map[string]Provider
	frozen    bool
}

// NewRegistry creates an empty, mutable registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a tool provider. Panics if the registry is frozen.
func (r *Registry) Register(p Provider) {
	if r.frozen {
		panic("tool: Register called on frozen registry")
	}
	r.providers[p.Tool().name] = p
}

// Get returns the provider registered under name.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// Freeze prevents further mutations. Call once setup is complete.
func (r *Registry) Freeze() {
	r.frozen = true
}

// ErrToolNotFound is returned when a tool alias cannot be resolved.
var ErrToolNotFound = errors.New("tool not found")

// NotAvailableError is the standard message returned when a tool is not registered.
const NotAvailableMsg = "Error: Tool not found. The tool you attempted to call does not exist or is not available. Please check the tool name and try again with a valid tool."
