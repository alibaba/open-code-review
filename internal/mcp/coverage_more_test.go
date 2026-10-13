// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/tool"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCoverageMoreConnectionValidation(t *testing.T) {
	tests := []struct {
		name   string
		config MCPServerConfig
		valid  bool
	}{
		{name: "default stdio", config: MCPServerConfig{Command: "server"}, valid: true},
		{name: "empty command", config: MCPServerConfig{Type: "stdio"}},
		{name: "stdio remote fields", config: MCPServerConfig{Type: "stdio", Command: "server", URL: "https://example.test"}},
		{name: "stdio NUL argument", config: MCPServerConfig{Type: "stdio", Command: "server", Args: []string{"bad\x00arg"}}},
		{name: "stdio malformed env", config: MCPServerConfig{Type: "stdio", Command: "server", Env: []string{"NO_EQUALS"}}},
		{name: "stdio NUL env", config: MCPServerConfig{Type: "stdio", Command: "server", Env: []string{"KEY=bad\x00value"}}},
		{name: "stdio duplicate env", config: MCPServerConfig{Type: "stdio", Command: "server", Env: []string{"KEY=one", "KEY=two"}}},
		{name: "remote stdio fields", config: MCPServerConfig{Type: "remote", URL: "https://example.test", Command: "server"}},
		{name: "remote invalid URL", config: MCPServerConfig{Type: "remote", URL: "://bad"}},
		{name: "remote invalid header name", config: MCPServerConfig{Type: "remote", URL: "https://example.test", Headers: map[string]string{"Bad Header": "value"}}},
		{name: "remote reserved header", config: MCPServerConfig{Type: "remote", URL: "https://example.test", Headers: map[string]string{"Accept": "value"}}},
		{name: "remote empty header", config: MCPServerConfig{Type: "remote", URL: "https://example.test", Headers: map[string]string{"X-Test": " "}}},
		{name: "remote control header", config: MCPServerConfig{Type: "remote", URL: "https://example.test", Headers: map[string]string{"X-Test": "bad\nvalue"}}},
		{name: "remote duplicate header", config: MCPServerConfig{Type: "remote", URL: "https://example.test", Headers: map[string]string{"X-Test": "one", "x-test": "two"}}},
		{name: "unsupported", config: MCPServerConfig{Type: "socket"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMCPConnection(tt.config)
			if (err == nil) != tt.valid {
				t.Fatalf("ValidateMCPConnection() error = %v, valid=%v", err, tt.valid)
			}
		})
	}

	if _, err := NewClient(context.Background(), "wrapper", "", nil, nil, "", "test"); err == nil {
		t.Fatal("NewClient compatibility wrapper accepted an empty command")
	}
	if _, err := NewRemoteClient(context.Background(), "wrapper", "file:///tmp/mcp", nil, "test"); err == nil {
		t.Fatal("NewRemoteClient compatibility wrapper accepted an invalid URL")
	}
	if _, err := NewConfiguredClient(context.Background(), "wrapper", MCPServerConfig{Type: "socket"}, "", "test"); err == nil {
		t.Fatal("NewConfiguredClient accepted an unsupported transport")
	}
}

func TestCoverageMoreDirectClientFailureBranches(t *testing.T) {
	if _, err := newStdioClient(context.Background(), "server", MCPServerConfig{Command: ""}, "", "test"); err == nil {
		t.Fatal("newStdioClient accepted an empty command")
	}
	if _, err := newStdioClient(context.Background(), "server", MCPServerConfig{Command: "server", Args: []string{"bad\x00arg"}}, "", "test"); err == nil {
		t.Fatal("newStdioClient accepted a NUL argument")
	}
	if _, err := newStdioClient(context.Background(), "server", MCPServerConfig{Command: "server", Env: []string{"KEY=$OCR_MCP_COVERAGE_MISSING"}}, "", "test"); err == nil {
		t.Fatal("newStdioClient accepted a missing environment reference")
	}
	if _, err := newRemoteClient(context.Background(), "server", MCPServerConfig{Type: "remote", URL: "file:///tmp/mcp"}, "test"); err == nil {
		t.Fatal("newRemoteClient accepted an invalid URL")
	}
	if _, err := newRemoteClient(context.Background(), "server", MCPServerConfig{
		Type: "remote", URL: "https://example.test", Headers: map[string]string{"X-Test": "$OCR_MCP_COVERAGE_MISSING"},
	}, "test"); err == nil {
		t.Fatal("newRemoteClient accepted an unresolved header")
	}
}

func TestCoverageMoreCatalogCopiesAndCallFailures(t *testing.T) {
	client := &Client{tools: []*sdkmcp.Tool{{
		Name: "one", Description: "description",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}},
	}}}
	first := client.Tools()
	first[0].Name = "changed"
	first[0].InputSchema.(map[string]any)["type"] = "string"
	second := client.Tools()
	if second[0].Name != "one" || second[0].InputSchema.(map[string]any)["type"] != "object" {
		t.Fatal("Tools returned mutable client-owned state")
	}

	if _, err := (&Client{}).callTool(context.Background(), "tool", nil); err == nil {
		t.Fatal("callTool accepted an empty callable session")
	}
	client = &Client{callToolFunc: func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
		return nil, nil
	}}
	if _, err := client.callTool(context.Background(), "tool", nil); err == nil {
		t.Fatal("callTool accepted an empty result")
	}
	client.callToolFunc = func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
		return &sdkmcp.CallToolResult{StructuredContent: func() {}}, nil
	}
	if _, err := client.callTool(context.Background(), "tool", nil); err == nil {
		t.Fatal("callTool accepted an unencodable result")
	}
	client.callToolFunc = func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
		return &sdkmcp.CallToolResult{IsError: true}, nil
	}
	_, err := client.callTool(context.Background(), "tool", nil)
	if err == nil || !strings.Contains(err.Error(), "unspecified") {
		t.Fatalf("empty ToolExecutionError = %v", err)
	}

	if _, err := contentToText([]sdkmcp.Content{
		&sdkmcp.TextContent{Text: "x"},
		&sdkmcp.TextContent{Text: strings.Repeat("y", maxToolResultBytes)},
	}); err == nil {
		t.Fatal("contentToText did not count its separator")
	}
}

type coverageRoundTripFunc func(*http.Request) (*http.Response, error)

func (f coverageRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCoverageMoreHeaderTransportBranches(t *testing.T) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNoContent} {
		transport := &headerTransport{
			origin: "https://example.test", serverName: "server",
			base: coverageRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Header.Get("X-Test") != "value" {
					t.Errorf("configured header = %q", request.Header.Get("X-Test"))
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("body")), Header: make(http.Header)}, nil
			}),
			headers: map[string]string{"X-Test": "value"},
		}
		response, roundTripErr := transport.RoundTrip(request)
		if status == http.StatusNoContent {
			if roundTripErr != nil || response == nil {
				t.Fatalf("RoundTrip(%d) = (%v, %v)", status, response, roundTripErr)
			}
			_ = response.Body.Close()
		} else if roundTripErr == nil || response != nil {
			t.Fatalf("RoundTrip(%d) = (%v, %v), want transport error", status, response, roundTripErr)
		}
	}

	transport := &headerTransport{
		origin: "https://example.test",
		base: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network failed")
		}),
	}
	if _, err := transport.RoundTrip(request); err == nil {
		t.Fatal("RoundTrip dropped its base transport error")
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	endpoint, _ := url.Parse(server.URL)
	transport = &headerTransport{origin: endpointOrigin(endpoint)}
	localRequest, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	response, err := transport.RoundTrip(localRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if endpointOrigin(nil) != "" {
		t.Fatal("nil URL unexpectedly had an origin")
	}
}

func TestCoverageMoreEnvironmentHeaderAndRedactionHelpers(t *testing.T) {
	t.Setenv("OCR_MCP_COVERAGE_EMPTY", "")
	if _, _, err := validateAndExpandHeaders(map[string]string{"X-Test": "$OCR_MCP_COVERAGE_EMPTY"}); err == nil {
		t.Fatal("empty expanded header was accepted")
	}
	if validHeaderName("") {
		t.Fatal("empty header name was accepted")
	}
	if _, _, err := buildStdioEnv([]string{"KEY=$OCR_MCP_COVERAGE_MISSING"}); err == nil {
		t.Fatal("missing environment reference was accepted")
	}
	if _, _, err := buildStdioEnv([]string{"KEY=bad\x00value"}); err == nil {
		t.Fatal("NUL environment value was accepted")
	}
	if safeInheritedEnv("OCR_MCP_RANDOM_SECRET") {
		t.Fatal("unsafe ambient environment key was inherited")
	}
	if redactError(nil, nil, "") != "" {
		t.Fatal("nil error redaction was non-empty")
	}
	if got := redactError(errors.New("bad %zz secret: abc"), []string{"abc"}, "%zz"); strings.Contains(got, "abc") || strings.Contains(got, "%zz") {
		t.Fatalf("malformed URL error was not redacted: %q", got)
	}

	arguments := []string{"--token", "one", "--api-key=two", "https://EXAMPLE.test/mcp?q=three", "plain", "--secret"}
	identity := sanitizeCommandArgs(arguments)
	if identity[1] != "<redacted>" || !strings.Contains(identity[2], "<redacted>") || strings.Contains(identity[3], "three") || identity[4] != "plain" {
		t.Fatalf("sanitizeCommandArgs() = %#v", identity)
	}
	redactions := commandArgumentRedactions(arguments)
	for _, secret := range []string{"one", "two", "three"} {
		if !containsString(redactions, secret) {
			t.Errorf("command redactions %#v omit %q", redactions, secret)
		}
	}

	values := appendSensitiveArgumentRedactions(nil, map[string]any{
		"credentials": map[string]any{
			"string": "one", "bytes": []byte("two"), "number": json.Number("3"),
			"float": 4.5, "map": map[string]any{"nested": "five"}, "list": []any{"six"}, "ignored": true,
		},
		"items": []any{map[string]any{"api_key": "seven"}},
	}, "", 0)
	for _, secret := range []string{"one", "two", "3", "4.5", "five", "six", "seven"} {
		if !containsString(values, secret) {
			t.Errorf("argument redactions %#v omit %q", values, secret)
		}
	}
	if got := appendSensitiveArgumentRedactions([]string{"kept"}, "secret", "api_key", maxSchemaDepth+1); len(got) != 1 {
		t.Fatalf("depth-limited redactions = %#v", got)
	}
	if got := appendCredentialLeaves([]string{"kept"}, "secret", maxSchemaDepth+1); len(got) != 1 {
		t.Fatalf("depth-limited credential leaves = %#v", got)
	}
	if got := appendRedaction([]string{"kept"}, ""); len(got) != 1 {
		t.Fatalf("empty redaction changed list: %#v", got)
	}
	if got := appendRedaction(nil, "Bearer token"); len(got) != 2 || got[1] != "token" {
		t.Fatalf("credential scheme split = %#v", got)
	}
	sorted := sortedRedactions([]string{"", "bb", "a", "bb", "cc"})
	if strings.Join(sorted, ",") != "bb,cc,a" {
		t.Fatalf("sortedRedactions() = %#v", sorted)
	}
}

func TestCoverageMoreIdentitySanitizationBranches(t *testing.T) {
	if got := identifierSlug("__A--B!!C", 4, "fallback"); got != "a-b" {
		t.Fatalf("identifierSlug() = %q", got)
	}
	if got := identifierSlug("!!!", 4, "fallback"); got != "fallback" {
		t.Fatalf("identifierSlug fallback = %q", got)
	}
	ansi := "a\x1b[31mred\x1b]0;title\x07b\x1b]x\x1b\\c\x1bXd\x1b"
	if got := stripANSI(ansi); strings.Contains(got, "\x1b") || !strings.Contains(got, "aredbcd") {
		t.Fatalf("stripANSI() = %q", got)
	}
	if got := redactText("bad %zz and secret", []string{"secret"}, "%zz"); strings.Contains(got, "%zz") || strings.Contains(got, "secret") {
		t.Fatalf("redactText() = %q", got)
	}

	clean := sanitizeMCPResultText(`{"api_key":"secret","normal":"ok","nested":[{"token":"x"}],"\u0000":"one"," ":"two"}`, nil, "")
	if strings.Contains(clean, "secret") || strings.Contains(clean, `"x"`) || !strings.Contains(clean, "redacted") {
		t.Fatalf("structured result sanitization = %q", clean)
	}
	if got := sanitizeMCPResultText("Authorization: Bearer secret\nnormal", nil, ""); strings.Contains(got, "Bearer secret") {
		t.Fatalf("line result sanitization = %q", got)
	}
	if got := sanitizeMCPResultText(`{"ok":1} trailing`, nil, ""); got == "" {
		t.Fatal("trailing JSON fallback returned empty output")
	}
	for _, value := range []any{"token: value", []any{"secret=x"}, 42} {
		if redactSensitiveResultValue(value, "", nil, "") == nil {
			t.Fatalf("redactSensitiveResultValue(%#v) returned nil", value)
		}
	}

	if _, err := canonicalInputSchema(map[string]any{"bad": func() {}}, nil, ""); err == nil {
		t.Fatal("unencodable schema was accepted")
	}
	if _, err := sanitizeJSONStrings(map[string]any{"\x00": "value"}, nil, ""); err == nil {
		t.Fatal("schema key empty after sanitization was accepted")
	}
	if _, err := sanitizeJSONStrings(map[string]any{"a\x00": "one", "a ": "two"}, nil, ""); err == nil {
		t.Fatal("colliding sanitized schema keys were accepted")
	}
	if _, err := sanitizeJSONStrings([]any{map[string]any{"\x00": "value"}}, nil, ""); err == nil {
		t.Fatal("nested invalid schema key was accepted")
	}
	if value, err := sanitizeJSONStrings([]any{"ok", 1}, nil, ""); err != nil || len(value.([]any)) != 2 {
		t.Fatalf("array schema sanitization = (%#v, %v)", value, err)
	}

	if _, err := definitionFingerprint("server", MCPServerConfig{Type: "socket"}, "tool", "description", json.RawMessage(`{"type":"object"}`)); err == nil {
		t.Fatal("fingerprint accepted an unsupported connection")
	}
	if _, err := sanitizedConnectionIdentity(MCPServerConfig{Type: "stdio", Env: []string{"bad"}}); err == nil {
		t.Fatal("identity accepted malformed environment")
	}
	if _, err := sanitizedConnectionIdentity(MCPServerConfig{Type: "remote", URL: "file:///tmp/mcp"}); err == nil {
		t.Fatal("identity accepted invalid remote URL")
	}
	if _, err := sanitizedConnectionIdentity(MCPServerConfig{Type: "socket"}); err == nil {
		t.Fatal("identity accepted unsupported transport")
	}

	for _, name := range []string{"", strings.Repeat("x", maxToolNameBytes+1), "bad\nname"} {
		if err := validateToolName(name); err == nil {
			t.Errorf("validateToolName(%q) unexpectedly succeeded", name)
		}
	}
}

func TestCoverageMoreConfigValidationAndCloning(t *testing.T) {
	for _, config := range []MCPConfig{
		{Version: -1}, {Version: 2}, {ApprovalTimeoutSeconds: 601}, {DefaultPermission: "invalid"},
	} {
		if err := ValidateMCPConfig(config); err == nil {
			t.Errorf("ValidateMCPConfig(%#v) unexpectedly succeeded", config)
		}
	}
	for _, config := range []MCPServerConfig{
		{DefaultPermission: "invalid"},
		{Tools: []string{"tool"}, ToolPermissions: map[string]Permission{"": PermissionAsk}},
	} {
		if err := ValidateMCPServerConfig(config); err == nil {
			t.Errorf("ValidateMCPServerConfig(%#v) unexpectedly succeeded", config)
		}
	}
	if permission, err := ResolvePermission(MCPConfig{Version: 2}, MCPServerConfig{Tools: []string{"tool"}}, "tool"); err == nil || permission != PermissionDeny {
		t.Fatalf("invalid global permission resolution = (%q, %v)", permission, err)
	}
	if permission, err := ResolvePermission(MCPConfig{}, MCPServerConfig{Tools: []string{"tool"}, DefaultPermission: "invalid"}, "tool"); err == nil || permission != PermissionDeny {
		t.Fatalf("invalid server permission resolution = (%q, %v)", permission, err)
	}

	original := MCPServerConfig{
		Args: []string{"arg"}, Env: []string{"KEY=value"}, Tools: []string{"tool"},
		Headers:              map[string]string{"X-Test": "value"},
		ToolPermissions:      map[string]Permission{"tool": PermissionAsk},
		ToolDefinitionSHA256: map[string]string{"tool": strings.Repeat("a", 64)},
	}
	cloned := cloneServerConfig(original)
	cloned.Args[0], cloned.Env[0], cloned.Tools[0] = "changed", "OTHER=value", "other"
	cloned.Headers["X-Test"] = "changed"
	cloned.ToolPermissions["tool"] = PermissionDeny
	cloned.ToolDefinitionSHA256["tool"] = strings.Repeat("b", 64)
	if original.Args[0] != "arg" || original.Env[0] != "KEY=value" || original.Tools[0] != "tool" || original.Headers["X-Test"] != "value" || original.ToolPermissions["tool"] != PermissionAsk || original.ToolDefinitionSHA256["tool"] != strings.Repeat("a", 64) {
		t.Fatal("cloneServerConfig shared mutable collection state")
	}
}

func TestCoverageMoreApprovalAndCloneBranches(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(approvalContextError(ctx), context.Canceled) {
		t.Fatal("approvalContextError did not preserve cancellation")
	}
	originalBytes := []byte("secret")
	original := map[string]any{
		"list":   []any{map[string]any{"value": "one"}},
		"bytes":  originalBytes,
		"scalar": 42,
	}
	cloned := cloneArgumentValue(original).(map[string]any)
	cloned["list"].([]any)[0].(map[string]any)["value"] = "changed"
	cloned["bytes"].([]byte)[0] = 'X'
	if original["list"].([]any)[0].(map[string]any)["value"] != "one" || string(originalBytes) != "secret" || cloned["scalar"] != 42 {
		t.Fatal("cloneArgumentValue did not isolate nested values")
	}
}

func TestCoverageMoreRegistrationFailureBranches(t *testing.T) {
	authorizer := NewRuntimeAuthorizer(false, time.Second, nil)
	if _, err := RegisterSelected(nil, nil, MCPConfig{}, nil, authorizer, false); err == nil {
		t.Fatal("nil registry was accepted")
	}
	if _, err := RegisterSelected(tool.NewRegistry(), nil, MCPConfig{}, nil, nil, false); err == nil {
		t.Fatal("nil authorizer was accepted")
	}
	if _, err := RegisterSelected(tool.NewRegistry(), nil, MCPConfig{Version: 2}, nil, authorizer, false); err == nil {
		t.Fatal("invalid global config was accepted")
	}
	disabled := false
	registration, err := RegisterSelected(tool.NewRegistry(), nil, MCPConfig{Enabled: &disabled}, nil, authorizer, false)
	if err != nil || len(registration.ToolDefs) != 0 {
		t.Fatalf("globally disabled registration = (%#v, %v)", registration, err)
	}
	if _, err := RegisterSelected(tool.NewRegistry(), []*Client{nil}, MCPConfig{}, nil, authorizer, false); err == nil {
		t.Fatal("nil client was accepted")
	}

	client, server := testCatalogClient(t, "server", "tool", "description")
	if _, err := RegisterSelected(tool.NewRegistry(), []*Client{client, client}, MCPConfig{}, map[string]MCPServerConfig{"server": server}, authorizer, false); err == nil {
		t.Fatal("duplicate client was accepted")
	}
	if _, err := RegisterSelected(tool.NewRegistry(), []*Client{client}, MCPConfig{}, nil, authorizer, false); err == nil {
		t.Fatal("missing server policy was accepted")
	}
	invalidPolicy := server
	invalidPolicy.Tools = []string{"tool", "tool"}
	if _, err := RegisterSelected(tool.NewRegistry(), []*Client{client}, MCPConfig{}, map[string]MCPServerConfig{"server": invalidPolicy}, authorizer, false); err == nil {
		t.Fatal("invalid server policy was accepted")
	}

	serverDisabled := server
	serverDisabled.Enabled = &disabled
	registration, err = RegisterSelected(tool.NewRegistry(), []*Client{client}, MCPConfig{}, map[string]MCPServerConfig{"server": serverDisabled}, authorizer, false)
	if err != nil || len(registration.ToolDefs) != 0 {
		t.Fatalf("disabled server registration = (%#v, %v)", registration, err)
	}

	denied := server
	denied.DefaultPermission = PermissionDeny
	registration, err = RegisterSelected(tool.NewRegistry(), []*Client{client}, MCPConfig{}, map[string]MCPServerConfig{"server": denied}, authorizer, false)
	if err != nil || len(registration.Skipped) != 1 {
		t.Fatalf("denied registration = (%#v, %v)", registration, err)
	}

	alias := ModelAlias(ToolID{Server: "server", Name: "tool"})
	registry := tool.NewRegistry()
	registry.Register(tool.NewStub(tool.Dynamic(alias)))
	if _, err := RegisterSelected(registry, []*Client{client}, MCPConfig{Version: 1, DefaultPermission: PermissionAllow}, map[string]MCPServerConfig{"server": server}, authorizer, false); err == nil {
		t.Fatal("existing provider alias collision was accepted")
	}

	badFingerprintClient := *client
	badFingerprintClient.discovered = append([]DiscoveredTool(nil), client.discovered...)
	badFingerprintClient.discovered[0].InputSchema = json.RawMessage(`{`)
	badFingerprintServer := server
	badFingerprintServer.ToolDefinitionSHA256 = map[string]string{"tool": strings.Repeat("a", 64)}
	if _, err := RegisterSelected(tool.NewRegistry(), []*Client{&badFingerprintClient}, MCPConfig{Version: 1, DefaultPermission: PermissionAllow}, map[string]MCPServerConfig{"server": badFingerprintServer}, authorizer, false); err == nil {
		t.Fatal("unencodable fingerprint input was accepted")
	}

	arraySchemaClient := *client
	arraySchemaClient.discovered = append([]DiscoveredTool(nil), client.discovered...)
	arraySchemaClient.discovered[0].InputSchema = json.RawMessage(`[]`)
	arrayFingerprint, err := definitionFingerprint("server", server, "tool", "description", json.RawMessage(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	arrayServer := server
	arrayServer.ToolDefinitionSHA256 = map[string]string{"tool": arrayFingerprint}
	if _, err := RegisterSelected(tool.NewRegistry(), []*Client{&arraySchemaClient}, MCPConfig{Version: 1, DefaultPermission: PermissionAllow}, map[string]MCPServerConfig{"server": arrayServer}, authorizer, false); err == nil {
		t.Fatal("non-object binding schema was accepted")
	}

	if err := verifyConnectionIdentity(MCPServerConfig{Type: "socket"}, server); err == nil {
		t.Fatal("invalid actual connection identity was accepted")
	}
	if err := verifyConnectionIdentity(server, MCPServerConfig{Type: "socket"}); err == nil {
		t.Fatal("invalid configured connection identity was accepted")
	}
	if err := validateServerName(strings.Repeat("x", maxToolNameBytes+1)); err == nil {
		t.Fatal("oversized server name was accepted")
	}
}

func TestCoverageMoreDiscoveryFailureBranches(t *testing.T) {
	toolDefinition := func(name string, schema any) *sdkmcp.Tool {
		return &sdkmcp.Tool{Name: name, InputSchema: schema}
	}
	tests := []struct {
		name   string
		config MCPServerConfig
		result *sdkmcp.ListToolsResult
	}{
		{name: "unencodable raw definition", result: &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{toolDefinition("tool", func() {})}}},
		{name: "raw catalog too large", result: &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{{Name: "tool", Title: strings.Repeat("x", maxCatalogBytes+1), InputSchema: map[string]any{"type": "object"}}}}},
		{name: "empty tool name", result: &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{toolDefinition("", map[string]any{"type": "object"})}}},
		{name: "control tool name", result: &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{toolDefinition("bad\nname", map[string]any{"type": "object"})}}},
		{name: "long description", result: &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{{Name: "tool", Description: strings.Repeat("x", maxDescriptionBytes+1), InputSchema: map[string]any{"type": "object"}}}}},
		{name: "invalid schema", result: &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{toolDefinition("tool", []any{"not-object"})}}},
		{name: "fingerprint connection failure", config: MCPServerConfig{Type: "socket"}, result: &sdkmcp.ListToolsResult{Tools: []*sdkmcp.Tool{toolDefinition("tool", map[string]any{"type": "object"})}}},
		{name: "cursor catalog too large", result: &sdkmcp.ListToolsResult{NextCursor: strings.Repeat("x", maxCatalogBytes+1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := discoverToolPages(context.Background(), "server", tt.config, nil, "", func(context.Context, *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error) {
				return tt.result, nil
			})
			if err == nil {
				t.Fatal("malformed discovery unexpectedly succeeded")
			}
		})
	}
}

func TestCoverageMoreNoAmbientMutation(t *testing.T) {
	// Exercise explicit expansion without relying on any developer-machine value.
	const key = "OCR_MCP_COVERAGE_VALUE"
	if err := os.Setenv(key, "value"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv(key) })
	value, err := expandEnvironmentReferences("before-$" + key + "-after")
	if err != nil || value != "before-value-after" {
		t.Fatalf("expandEnvironmentReferences() = (%q, %v)", value, err)
	}
}
