package mcp

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/tool"
)

func newTestClient(name string, tools []*mcp.Tool) *Client {
	return &Client{name: name, tools: tools}
}

func TestProvider_Tool(t *testing.T) {
	c := newTestClient("srv", nil)
	p := &Provider{toolName: "my_tool", client: c}
	got := p.Tool()
	if got.Name() != "my_tool" {
		t.Errorf("Tool().Name() = %q, want %q", got.Name(), "my_tool")
	}
}

func TestRegisterAll_Basic(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "alpha"},
		{Name: "beta"},
	}
	c := newTestClient("srv", tools)
	reg := tool.NewRegistry()

	RegisterAll(reg, c, nil)

	for _, name := range []string{"alpha", "beta"} {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}

func TestRegisterAll_AllowedFilter(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma"},
	}
	c := newTestClient("srv", tools)
	reg := tool.NewRegistry()

	RegisterAll(reg, c, []string{"alpha", "gamma"})

	if _, ok := reg.Get("alpha"); !ok {
		t.Error("expected alpha to be registered")
	}
	if _, ok := reg.Get("beta"); ok {
		t.Error("expected beta to be filtered out")
	}
	if _, ok := reg.Get("gamma"); !ok {
		t.Error("expected gamma to be registered")
	}
}

func TestRegisterAll_SkipsReservedTools(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "file_read"},
		{Name: "custom_tool"},
	}
	c := newTestClient("srv", tools)
	reg := tool.NewRegistry()

	RegisterAll(reg, c, nil)

	if _, ok := reg.Get("file_read"); ok {
		t.Error("reserved tool file_read should not be registered")
	}
	if _, ok := reg.Get("custom_tool"); !ok {
		t.Error("expected custom_tool to be registered")
	}
}

func TestRegisterAll_SkipsDuplicateTools(t *testing.T) {
	tools := []*mcp.Tool{{Name: "dup_tool"}}
	c := newTestClient("srv", tools)
	reg := tool.NewRegistry()

	stub := tool.NewStub(tool.Dynamic("dup_tool"))
	reg.Register(stub)

	RegisterAll(reg, c, nil)

	got, ok := reg.Get("dup_tool")
	if !ok {
		t.Fatal("expected dup_tool to be registered")
	}
	if _, isProvider := got.(*Provider); isProvider {
		t.Error("expected dup_tool to remain the original stub, not MCP provider")
	}
}

func TestRegisterAll_WarnsUnmatchedAllowed(t *testing.T) {
	tools := []*mcp.Tool{{Name: "alpha"}}
	c := newTestClient("srv", tools)
	reg := tool.NewRegistry()

	RegisterAll(reg, c, []string{"alpha", "nonexistent"})

	if _, ok := reg.Get("alpha"); !ok {
		t.Error("expected alpha to be registered")
	}
}

func TestToToolDef_MapSchema(t *testing.T) {
	mt := &mcp.Tool{
		Name:        "search",
		Description: "Search things",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
	}

	got := ToToolDef(mt)

	want := llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "search",
			Description: "Search things",
		},
	}
	if got.Type != want.Type || got.Function.Name != want.Function.Name || got.Function.Description != want.Function.Description {
		t.Errorf("ToToolDef() basic fields mismatch: got %+v", got)
	}
	if got.Function.Parameters["type"] != "object" {
		t.Errorf("expected type=object, got %v", got.Function.Parameters["type"])
	}
	if got.Function.Parameters["properties"] == nil {
		t.Error("expected properties to be set")
	}
}

func TestToToolDef_NilSchema(t *testing.T) {
	mt := &mcp.Tool{
		Name:        "noop",
		InputSchema: nil,
	}

	got := ToToolDef(mt)

	if got.Function.Parameters["type"] != "object" {
		t.Errorf("expected default type=object, got %v", got.Function.Parameters["type"])
	}
}

func TestToToolDef_UnexpectedSchemaType(t *testing.T) {
	mt := &mcp.Tool{
		Name:        "weird",
		InputSchema: "not-a-map",
	}

	got := ToToolDef(mt)

	if got.Function.Parameters["type"] != "object" {
		t.Errorf("expected fallback type=object, got %v", got.Function.Parameters["type"])
	}
}

func TestCollectToolDefs_Basic(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "alpha", Description: "A"},
		{Name: "beta", Description: "B"},
	}
	c := newTestClient("srv", tools)
	reg := tool.NewRegistry()
	RegisterAll(reg, c, nil)

	defs := CollectToolDefs([]*Client{c}, reg)

	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Function.Name] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("expected alpha and beta in defs, got %v", names)
	}
}

func TestCollectToolDefs_FiltersReserved(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "file_read"},
		{Name: "custom"},
	}
	c := newTestClient("srv", tools)
	reg := tool.NewRegistry()
	RegisterAll(reg, c, nil)

	defs := CollectToolDefs([]*Client{c}, reg)

	for _, d := range defs {
		if d.Function.Name == "file_read" {
			t.Error("reserved tool file_read should not appear in defs")
		}
	}
}

func TestCollectToolDefs_FiltersUnregistered(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "registered_tool"},
		{Name: "unregistered_tool"},
	}
	c := newTestClient("srv", tools)
	reg := tool.NewRegistry()
	reg.Register(&Provider{toolName: "registered_tool", client: c})

	defs := CollectToolDefs([]*Client{c}, reg)

	if len(defs) != 1 {
		t.Fatalf("expected 1 def, got %d", len(defs))
	}
	if defs[0].Function.Name != "registered_tool" {
		t.Errorf("expected registered_tool, got %s", defs[0].Function.Name)
	}
}

func TestCollectToolDefs_Dedup(t *testing.T) {
	tools := []*mcp.Tool{{Name: "shared", Description: "A"}}
	c1 := newTestClient("srv1", tools)
	c2 := newTestClient("srv2", tools)
	reg := tool.NewRegistry()
	reg.Register(&Provider{toolName: "shared", client: c1})

	defs := CollectToolDefs([]*Client{c1, c2}, reg)

	if len(defs) != 1 {
		t.Errorf("expected 1 deduped def, got %d", len(defs))
	}
}
