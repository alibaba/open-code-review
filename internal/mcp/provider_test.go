// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/tool"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type authorizerFunc func(context.Context, Invocation) (Decision, error)

func (f authorizerFunc) Authorize(ctx context.Context, invocation Invocation) (Decision, error) {
	return f(ctx, invocation)
}

func TestProviderAuthorizesImmediatelyBeforeCall(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	client := &Client{callToolFunc: func(_ context.Context, params *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
		calls.Add(1)
		if params.Name != "raw-name" {
			t.Errorf("called raw name %q, want raw-name", params.Name)
		}
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "ok"}}}, nil
	}}
	grant := ToolGrant{
		ID: ToolID{Server: "server", Name: "raw-name"}, ModelAlias: ModelAlias(ToolID{Server: "server", Name: "raw-name"}),
		Permission: PermissionAsk,
	}

	t.Run("allow", func(t *testing.T) {
		before := calls.Load()
		provider := &Provider{grant: grant, client: client, authorizer: authorizerFunc(func(_ context.Context, invocation Invocation) (Decision, error) {
			if invocation.Grant.ID != grant.ID {
				t.Fatalf("authorized grant = %#v, want %#v", invocation.Grant.ID, grant.ID)
			}
			return DecisionAllowOnce, nil
		})}
		got, err := provider.Execute(context.Background(), map[string]any{"q": "value"})
		if err != nil || got != "ok" || calls.Load() != before+1 {
			t.Fatalf("Execute() = (%q, %v), calls=%d", got, err, calls.Load())
		}
		if provider.Tool().Name() != grant.ModelAlias || !provider.SensitiveToolCall() {
			t.Fatal("provider alias or sensitive marker is incorrect")
		}
	})

	for _, tt := range []struct {
		name       string
		authorizer Authorizer
		wantErr    error
	}{
		{name: "nil authorizer", wantErr: ErrInteractionUnavailable},
		{name: "authorization error", authorizer: authorizerFunc(func(context.Context, Invocation) (Decision, error) {
			return DecisionDenyOnce, ErrPermissionDenied
		}), wantErr: ErrPermissionDenied},
		{name: "non-allow decision", authorizer: authorizerFunc(func(context.Context, Invocation) (Decision, error) {
			return DecisionDenyOnce, nil
		}), wantErr: ErrPermissionDenied},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := calls.Load()
			provider := &Provider{grant: grant, client: client, authorizer: tt.authorizer}
			got, err := provider.Execute(context.Background(), nil)
			if got != "" || !errors.Is(err, tt.wantErr) || calls.Load() != before {
				t.Fatalf("Execute() = (%q, %v), calls=%d; tool must not execute", got, err, calls.Load())
			}
		})
	}
}

func TestProviderExecutesTheExactAuthorizedArgumentSnapshot(t *testing.T) {
	t.Parallel()

	arguments := map[string]any{"nested": map[string]any{"value": "original"}}
	client := &Client{callToolFunc: func(_ context.Context, params *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
		nested := params.Arguments.(map[string]any)["nested"].(map[string]any)
		if nested["value"] != "original" {
			t.Fatalf("executed arguments = %#v, want authorized snapshot", params.Arguments)
		}
		return &sdkmcp.CallToolResult{}, nil
	}}
	provider := &Provider{
		grant:  ToolGrant{ID: ToolID{Server: "server", Name: "tool"}, Permission: PermissionAsk},
		client: client,
		authorizer: authorizerFunc(func(_ context.Context, invocation Invocation) (Decision, error) {
			invocation.Arguments["nested"].(map[string]any)["value"] = "tampered"
			return DecisionAllowOnce, nil
		}),
	}
	if _, err := provider.Execute(context.Background(), arguments); err != nil {
		t.Fatal(err)
	}
	if got := arguments["nested"].(map[string]any)["value"]; got != "original" {
		t.Fatalf("caller arguments were mutated: %v", got)
	}
}

func TestProviderCancellationChecksBeforeAndAfterAuthorizationAndCall(t *testing.T) {
	t.Parallel()

	t.Run("before authorization", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var authorizations atomic.Int32
		provider := &Provider{authorizer: authorizerFunc(func(context.Context, Invocation) (Decision, error) {
			authorizations.Add(1)
			return DecisionAllowOnce, nil
		}), client: &Client{}}
		if _, err := provider.Execute(ctx, nil); !errors.Is(err, context.Canceled) || authorizations.Load() != 0 {
			t.Fatalf("Execute() error = %v, authorizations=%d", err, authorizations.Load())
		}
	})

	t.Run("after authorization", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var calls atomic.Int32
		provider := &Provider{
			client: &Client{callToolFunc: func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
				calls.Add(1)
				return &sdkmcp.CallToolResult{}, nil
			}},
			authorizer: authorizerFunc(func(context.Context, Invocation) (Decision, error) {
				cancel()
				return DecisionAllowOnce, nil
			}),
		}
		if _, err := provider.Execute(ctx, nil); !errors.Is(err, context.Canceled) || calls.Load() != 0 {
			t.Fatalf("Execute() error = %v, calls=%d", err, calls.Load())
		}
	})

	t.Run("after call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		provider := &Provider{
			client: &Client{callToolFunc: func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
				cancel()
				return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "discard"}}}, nil
			}},
			authorizer: authorizerFunc(func(context.Context, Invocation) (Decision, error) {
				return DecisionAllowOnce, nil
			}),
		}
		if got, err := provider.Execute(ctx, nil); got != "" || !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() = (%q, %v), want canceled without output", got, err)
		}
	})
}

func TestProviderBrokerFailuresNeverReachCallTool(t *testing.T) {
	t.Parallel()

	grant := testInvocation(PermissionAsk, "tool").Grant
	var calls atomic.Int32
	client := &Client{callToolFunc: func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
		calls.Add(1)
		return &sdkmcp.CallToolResult{}, nil
	}}
	tests := []struct {
		name    string
		broker  *ApprovalBroker
		context func() (context.Context, context.CancelFunc)
	}{
		{
			name:   "non-interactive",
			broker: NewRuntimeAuthorizer(false, time.Second, nil),
		},
		{
			name: "terminal EOF",
			broker: NewRuntimeAuthorizer(true, time.Second, PromptFunc(func(context.Context, Invocation) (Decision, error) {
				return DecisionDenyOnce, io.EOF
			})),
		},
		{
			name: "explicit denial",
			broker: NewRuntimeAuthorizer(true, time.Second, PromptFunc(func(context.Context, Invocation) (Decision, error) {
				return DecisionDenyOnce, nil
			})),
		},
		{
			name: "approval timeout",
			broker: NewRuntimeAuthorizer(true, time.Second, PromptFunc(func(ctx context.Context, _ Invocation) (Decision, error) {
				<-ctx.Done()
				return DecisionAllowOnce, ctx.Err()
			})),
			context: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 15*time.Millisecond)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cancel := func() {}
			if tt.context != nil {
				ctx, cancel = tt.context()
			}
			defer cancel()
			before := calls.Load()
			provider := &Provider{grant: grant, client: client, authorizer: tt.broker}
			if _, err := provider.Execute(ctx, nil); err == nil {
				t.Fatal("fail-closed broker outcome returned no error")
			}
			if got := calls.Load(); got != before {
				t.Fatalf("tools/call count changed from %d to %d", before, got)
			}
		})
	}
}

func TestRegisterSelectedScopesVisibilityAndNames(t *testing.T) {
	t.Parallel()

	clientOne, serverOne := testCatalogClient(t, "one", "shared", "first description")
	clientTwo, serverTwo := testCatalogClient(t, "two", "shared", "second description")
	clientBuiltIn, serverBuiltIn := testCatalogClient(t, "three", "file_read", "looks built in")
	reg := tool.NewRegistry()
	authorizer := NewRuntimeAuthorizer(false, time.Second, nil)
	registration, err := RegisterSelected(
		reg, []*Client{clientTwo, clientBuiltIn, clientOne},
		MCPConfig{Version: 1, DefaultPermission: PermissionAllow},
		map[string]MCPServerConfig{"one": serverOne, "two": serverTwo, "three": serverBuiltIn},
		authorizer, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(registration.Bindings) != 3 || len(registration.ToolDefs) != 3 || len(registration.Skipped) != 0 {
		t.Fatalf("registration sizes = %d/%d/%d", len(registration.Bindings), len(registration.ToolDefs), len(registration.Skipped))
	}
	aliases := make(map[string]struct{})
	for i, binding := range registration.Bindings {
		alias := binding.Grant.ModelAlias
		if alias == binding.Grant.ID.Name || binding.Definition.Function.Name != alias || registration.ToolDefs[i].Function.Name != alias {
			t.Fatalf("binding/definition alias mismatch: %#v / %#v", binding, registration.ToolDefs[i])
		}
		if _, duplicate := aliases[alias]; duplicate {
			t.Fatalf("cross-server tools collided on alias %q", alias)
		}
		aliases[alias] = struct{}{}
		if _, ok := reg.Get(alias); !ok {
			t.Fatalf("alias %q not registered", alias)
		}
		if !strings.Contains(binding.Definition.Function.Description, "UNTRUSTED") || binding.Grant.UntrustedDescription == "" {
			t.Fatalf("untrusted metadata was not labeled/preserved: %#v", binding)
		}
	}
	if _, ok := reg.Get("shared"); ok {
		t.Fatal("raw MCP name was registered")
	}
	if _, ok := reg.Get("file_read"); ok {
		t.Fatal("MCP tool shadowed a built-in name")
	}
}

func TestRegisterSelectedVisibilityAndDrift(t *testing.T) {
	t.Parallel()

	t.Run("empty allowlist exposes nothing", func(t *testing.T) {
		client, server := testCatalogClient(t, "server", "tool", "description")
		server.Tools = nil
		server.ToolDefinitionSHA256 = nil
		registration, err := RegisterSelected(
			tool.NewRegistry(), []*Client{client}, MCPConfig{Version: 1},
			map[string]MCPServerConfig{"server": server}, NewRuntimeAuthorizer(true, time.Second, nil), true,
		)
		if err != nil || len(registration.ToolDefs) != 0 {
			t.Fatalf("registration = %#v, error=%v", registration, err)
		}
	})

	t.Run("ask hidden in CI", func(t *testing.T) {
		client, server := testCatalogClient(t, "server", "tool", "description")
		registration, err := RegisterSelected(
			tool.NewRegistry(), []*Client{client}, MCPConfig{Version: 1},
			map[string]MCPServerConfig{"server": server}, NewRuntimeAuthorizer(false, time.Second, nil), false,
		)
		if err != nil || len(registration.ToolDefs) != 0 || len(registration.Skipped) != 1 {
			t.Fatalf("registration = %#v, error=%v", registration, err)
		}
	})

	t.Run("legacy missing fingerprint is forced to ask", func(t *testing.T) {
		client, server := testCatalogClient(t, "server", "tool", "description")
		server.ToolDefinitionSHA256 = nil
		server.DefaultPermission = PermissionAllow
		registration, err := RegisterSelected(
			tool.NewRegistry(), []*Client{client}, MCPConfig{Version: 0, DefaultPermission: PermissionAllow},
			map[string]MCPServerConfig{"server": server}, NewRuntimeAuthorizer(true, time.Second, nil), true,
		)
		if err != nil || len(registration.Bindings) != 1 || registration.Bindings[0].Grant.Permission != PermissionAsk {
			t.Fatalf("legacy registration = %#v, error=%v", registration, err)
		}
	})

	for _, tt := range []struct {
		name   string
		mutate func(*Client, *MCPServerConfig)
	}{
		{name: "missing v1 fingerprint", mutate: func(_ *Client, server *MCPServerConfig) { server.ToolDefinitionSHA256 = nil }},
		{name: "changed fingerprint", mutate: func(_ *Client, server *MCPServerConfig) {
			server.ToolDefinitionSHA256["tool"] = strings.Repeat("b", 64)
		}},
		{name: "selected tool disappeared", mutate: func(client *Client, _ *MCPServerConfig) { client.discovered = nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, server := testCatalogClient(t, "server", "tool", "description")
			tt.mutate(client, &server)
			registration, err := RegisterSelected(
				tool.NewRegistry(), []*Client{client}, MCPConfig{Version: 1, DefaultPermission: PermissionAllow},
				map[string]MCPServerConfig{"server": server}, NewRuntimeAuthorizer(false, time.Second, nil), false,
			)
			if err != nil || len(registration.ToolDefs) != 0 || len(registration.Skipped) != 1 || !registration.Skipped[0].NeedsReview {
				t.Fatalf("drift registration = %#v, error=%v", registration, err)
			}
		})
	}
}

func TestRegisterSelectedIsAtomicAndBindsConnectionIdentity(t *testing.T) {
	t.Parallel()

	t.Run("connection mismatch", func(t *testing.T) {
		client, server := testCatalogClient(t, "server", "tool", "description")
		server.Command = "different-command"
		reg := tool.NewRegistry()
		_, err := RegisterSelected(
			reg, []*Client{client}, MCPConfig{Version: 1, DefaultPermission: PermissionAllow},
			map[string]MCPServerConfig{"server": server}, NewRuntimeAuthorizer(false, time.Second, nil), false,
		)
		if err == nil {
			t.Fatal("expected connection identity mismatch")
		}
		if _, ok := reg.Get(ModelAlias(ToolID{Server: "server", Name: "tool"})); ok {
			t.Fatal("registry was mutated after failed validation")
		}
	})

	t.Run("existing definition collision", func(t *testing.T) {
		client, server := testCatalogClient(t, "server", "tool", "description")
		alias := ModelAlias(ToolID{Server: "server", Name: "tool"})
		reg := tool.NewRegistry()
		_, err := RegisterSelected(
			reg, []*Client{client}, MCPConfig{Version: 1, DefaultPermission: PermissionAllow},
			map[string]MCPServerConfig{"server": server}, NewRuntimeAuthorizer(false, time.Second, nil), false,
			[]llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: alias}}},
		)
		if err == nil {
			t.Fatal("expected definition collision")
		}
		if _, ok := reg.Get(alias); ok {
			t.Fatal("registry was mutated after collision")
		}
	})
}

func TestRegisteredProviderExecutesExactGrantedTool(t *testing.T) {
	t.Parallel()

	client, server := testCatalogClient(t, "server", "tool", "description")
	client.callToolFunc = func(_ context.Context, params *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
		if params.Name != "tool" {
			t.Fatalf("CallTool name = %q, want exact raw name", params.Name)
		}
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "executed"}}}, nil
	}
	reg := tool.NewRegistry()
	registration, err := RegisterSelected(
		reg, []*Client{client}, MCPConfig{Version: 1, DefaultPermission: PermissionAllow},
		map[string]MCPServerConfig{"server": server}, NewRuntimeAuthorizer(false, time.Second, nil), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := reg.Get(registration.ToolDefs[0].Function.Name)
	if !ok {
		t.Fatal("registered provider not found")
	}
	got, err := provider.Execute(context.Background(), nil)
	if err != nil || got != "executed" {
		t.Fatalf("Execute() = (%q, %v)", got, err)
	}
}

func testCatalogClient(t *testing.T, serverName, toolName, description string) (*Client, MCPServerConfig) {
	t.Helper()
	server := MCPServerConfig{
		Type: "stdio", Command: "test-command", Tools: []string{toolName},
		ToolDefinitionSHA256: make(map[string]string),
	}
	schema, err := canonicalInputSchema(map[string]any{
		"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}},
	}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := definitionFingerprint(serverName, server, toolName, description, schema)
	if err != nil {
		t.Fatal(err)
	}
	server.ToolDefinitionSHA256[toolName] = fingerprint
	client := &Client{
		name: serverName, connection: cloneServerConfig(server),
		discovered: []DiscoveredTool{{
			Name: toolName, Description: description, InputSchema: schema, DefinitionSHA256: fingerprint,
		}},
	}
	return client, server
}
