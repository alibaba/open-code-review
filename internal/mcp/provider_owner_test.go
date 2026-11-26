// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCollectToolDefs_UsesRegisteredOwner(t *testing.T) {
	first := newTestClient("first", []*mcp.Tool{
		{Name: "shared", Description: "First server", InputSchema: map[string]any{"type": "object", "required": []string{"source"}}},
		{Name: "allowed"},
	})
	second := newTestClient("second", []*mcp.Tool{
		{Name: "shared", Description: "Second server", InputSchema: map[string]any{"type": "object", "required": []string{"query"}}},
	})
	reg := tool.NewRegistry()
	RegisterAll(reg, first, []string{"allowed"})
	RegisterAll(reg, second, nil)

	for _, clients := range [][]*Client{{first, second}, {second, first}} {
		defs := CollectToolDefs(clients, reg)
		if len(defs) != 2 {
			t.Fatalf("got %d definitions, want 2", len(defs))
		}
		for _, def := range defs {
			if def.Function.Name == "shared" && !reflect.DeepEqual(def, ToToolDef(second.tools[0])) {
				t.Errorf("shared definition = %+v, want the registered second server's schema", def)
			}
		}
	}
}

func TestCollectToolDefs_SkipsNonMCPProvider(t *testing.T) {
	c := newTestClient("server", []*mcp.Tool{{Name: "shared"}})
	reg := tool.NewRegistry()
	reg.Register(tool.NewStub(tool.Dynamic("shared")))
	RegisterAll(reg, c, nil)
	if defs := CollectToolDefs([]*Client{c}, reg); len(defs) != 0 {
		t.Errorf("definitions = %+v, want no schema for a skipped MCP provider", defs)
	}
}

func TestCollectToolDefs_SchemaMatchesExecutedServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	first := newOwnerTestClient(t, ctx, "first", "source")
	second := newOwnerTestClient(t, ctx, "second", "query")
	reg := tool.NewRegistry()
	RegisterAll(reg, first, []string{"allowed"})
	RegisterAll(reg, second, []string{"shared"})
	reg.Freeze()

	for _, def := range CollectToolDefs([]*Client{first, second}, reg) {
		if def.Function.Name != "shared" {
			continue
		}
		args := make(map[string]any)
		for field := range def.Function.Parameters["properties"].(map[string]any) {
			args[field] = "needle"
		}
		provider, ok := reg.Get(def.Function.Name)
		if !ok {
			t.Fatal("advertised tool was not registered")
		}
		result, err := provider.Execute(ctx, args)
		if err != nil || result != "second: needle" {
			t.Fatalf("Execute with advertised arguments = %q, %v; want second: needle", result, err)
		}
		return
	}
	t.Fatal("registered shared tool was not advertised")
}

func newOwnerTestClient(t *testing.T, ctx context.Context, name, argument string) *Client {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: name, Version: "test"}, nil)
	server.AddTool(&mcp.Tool{
		Name: "shared", Description: name,
		InputSchema: map[string]any{
			"type": "object", "required": []string{argument},
			"properties": map[string]any{argument: map[string]any{"type": "string"}},
		},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]string
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		if args[argument] == "" {
			return nil, fmt.Errorf("missing %s argument", argument)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: name + ": " + args[argument]}}}, nil
	})
	server.AddTool(&mcp.Tool{Name: "allowed", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	sdkClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := sdkClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	tools, err := listAllTools(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	return &Client{name: name, session: session, tools: tools}
}
