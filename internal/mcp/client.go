// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"unicode"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client wraps one MCP connection and its bounded discovery snapshot.
type Client struct {
	name         string
	session      *sdkmcp.ClientSession
	tools        []*sdkmcp.Tool
	discovered   []DiscoveredTool
	connection   MCPServerConfig
	redactValues []string
	redactURL    string
	callToolFunc func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error)
	closeFunc    func() error
}

// NewConfiguredClient connects to either a stdio or remote server according to
// config, then performs bounded discovery without invoking a business tool.
func NewConfiguredClient(ctx context.Context, name string, config MCPServerConfig, dir, version string) (*Client, error) {
	if err := validateServerName(name); err != nil {
		return nil, err
	}
	config = cloneServerConfig(config)
	if err := ValidateMCPConnection(config); err != nil {
		return nil, fmt.Errorf("invalid MCP server %q connection: %w", name, err)
	}
	switch config.Type {
	case "", "stdio":
		config.Type = "stdio"
		return newStdioClient(ctx, name, config, dir, version)
	case "remote":
		return newRemoteClient(ctx, name, config, version)
	default:
		return nil, fmt.Errorf("unsupported MCP server type %q", config.Type)
	}
}

// ValidateMCPConnection validates transport configuration without resolving or
// exposing credential values. Management commands and the runtime share this
// exact fail-closed structural contract.
func ValidateMCPConnection(config MCPServerConfig) error {
	typeName := config.Type
	if typeName == "" {
		typeName = "stdio"
	}
	switch typeName {
	case "stdio":
		if strings.TrimSpace(config.Command) == "" || strings.IndexByte(config.Command, 0) >= 0 {
			return errors.New("stdio command cannot be empty or contain a NUL byte")
		}
		if config.URL != "" || len(config.Headers) != 0 || config.AllowInsecureHTTP {
			return errors.New("stdio connection contains remote-only fields")
		}
		for _, arg := range config.Args {
			if strings.IndexByte(arg, 0) >= 0 {
				return errors.New("stdio arguments must not contain NUL bytes")
			}
		}
		seenEnv := make(map[string]struct{}, len(config.Env))
		for _, entry := range config.Env {
			key, value, ok := strings.Cut(entry, "=")
			if !ok || !validEnvKey(key) || strings.IndexByte(value, 0) >= 0 {
				return errors.New("environment entries must use a valid KEY=VALUE form without NUL bytes")
			}
			normalized := normalizedEnvKey(key)
			if _, duplicate := seenEnv[normalized]; duplicate {
				return fmt.Errorf("environment variable %q is configured more than once", key)
			}
			seenEnv[normalized] = struct{}{}
		}
	case "remote":
		if config.Command != "" || len(config.Args) != 0 || len(config.Env) != 0 {
			return errors.New("remote connection contains stdio-only fields")
		}
		if _, err := validateRemoteURL(config.URL, config.AllowInsecureHTTP); err != nil {
			return err
		}
		seenHeaders := make(map[string]struct{}, len(config.Headers))
		for name, value := range config.Headers {
			identity := strings.ToLower(name)
			if !validHeaderName(name) || forbiddenMCPHeader(name) || strings.TrimSpace(value) == "" || containsInvalidHeaderValue(value) {
				return fmt.Errorf("invalid MCP header %q", name)
			}
			if _, duplicate := seenHeaders[identity]; duplicate {
				return fmt.Errorf("header %q is configured more than once", name)
			}
			seenHeaders[identity] = struct{}{}
		}
	default:
		return fmt.Errorf("unsupported MCP server type %q", config.Type)
	}
	return nil
}

// NewClient is the compatibility wrapper for a stdio connection.
func NewClient(ctx context.Context, name, command string, args, env []string, dir, version string) (*Client, error) {
	return NewConfiguredClient(ctx, name, MCPServerConfig{
		Type: "stdio", Command: command, Args: args, Env: env,
	}, dir, version)
}

func newStdioClient(ctx context.Context, name string, config MCPServerConfig, dir, version string) (*Client, error) {
	if strings.TrimSpace(config.Command) == "" || strings.IndexByte(config.Command, 0) >= 0 {
		return nil, fmt.Errorf("MCP server %q has an invalid command", name)
	}
	for _, arg := range config.Args {
		if strings.IndexByte(arg, 0) >= 0 {
			return nil, fmt.Errorf("MCP server %q has an invalid command argument", name)
		}
	}
	childEnv, redactions, err := buildStdioEnv(config.Env)
	if err != nil {
		return nil, fmt.Errorf("prepare environment for MCP server %q: %w", name, err)
	}
	redactions = append(redactions, commandArgumentRedactions(config.Args)...)

	cmd := exec.Command(config.Command, config.Args...)
	cmd.Env = childEnv
	if dir != "" {
		cmd.Dir = dir
	}
	client := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "open-code-review", Version: version},
		&sdkmcp.ClientOptions{Capabilities: &sdkmcp.ClientCapabilities{}},
	)
	session, err := client.Connect(ctx, &sdkmcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to MCP server %q: %s", name, redactError(err, redactions, ""))
	}
	return newDiscoveredClient(ctx, name, session, config, redactions, "")
}

// NewRemoteClient is the compatibility wrapper for a secure remote connection.
// Non-loopback plain HTTP requires NewConfiguredClient with AllowInsecureHTTP.
func NewRemoteClient(ctx context.Context, name, rawURL string, headers map[string]string, version string) (*Client, error) {
	return NewConfiguredClient(ctx, name, MCPServerConfig{
		Type: "remote", URL: rawURL, Headers: headers,
	}, "", version)
}

func newRemoteClient(ctx context.Context, name string, config MCPServerConfig, version string) (*Client, error) {
	endpoint, err := validateRemoteURL(config.URL, config.AllowInsecureHTTP)
	if err != nil {
		return nil, fmt.Errorf("invalid remote MCP server %q URL: %w", name, err)
	}
	expanded, redactions, err := validateAndExpandHeaders(config.Headers)
	if err != nil {
		return nil, fmt.Errorf("invalid remote MCP server %q headers: %w", name, err)
	}
	for _, values := range endpoint.Query() {
		for _, value := range values {
			if value != "" {
				redactions = appendRedaction(redactions, value)
			}
		}
	}
	transport := &headerTransport{
		base: http.DefaultTransport, headers: expanded, serverName: name,
		origin: endpointOrigin(endpoint),
	}
	httpClient := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects from remote MCP server")
			}
			if endpointOrigin(req.URL) != transport.origin {
				return errors.New("remote MCP server attempted a cross-origin redirect")
			}
			return nil
		},
	}
	client := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "open-code-review", Version: version},
		&sdkmcp.ClientOptions{Capabilities: &sdkmcp.ClientCapabilities{}},
	)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{
		Endpoint: endpoint.String(), HTTPClient: httpClient,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to remote MCP server %q at %s: %s", name, safeEndpointLabel(endpoint), redactError(err, redactions, config.URL))
	}
	return newDiscoveredClient(ctx, name, session, config, redactions, config.URL)
}

func newDiscoveredClient(ctx context.Context, name string, session *sdkmcp.ClientSession, config MCPServerConfig, redactions []string, rawURL string) (*Client, error) {
	var success bool
	defer func() {
		if !success {
			_ = session.Close()
		}
	}()
	tools, discovered, err := discoverTools(ctx, name, session, config, redactions, rawURL)
	if err != nil {
		return nil, err
	}
	success = true
	return &Client{
		name: name, session: session, tools: tools, discovered: discovered,
		connection: config, redactValues: append([]string(nil), redactions...),
		redactURL: rawURL, callToolFunc: session.CallTool, closeFunc: session.Close,
	}, nil
}

func discoverTools(ctx context.Context, serverName string, session *sdkmcp.ClientSession, config MCPServerConfig, redactions []string, rawURL string) ([]*sdkmcp.Tool, []DiscoveredTool, error) {
	return discoverToolPages(ctx, serverName, config, redactions, rawURL, session.ListTools)
}

type listToolsFunc func(context.Context, *sdkmcp.ListToolsParams) (*sdkmcp.ListToolsResult, error)

func discoverToolPages(ctx context.Context, serverName string, config MCPServerConfig, redactions []string, rawURL string, list listToolsFunc) ([]*sdkmcp.Tool, []DiscoveredTool, error) {
	seenNames := make(map[string]struct{})
	seenCursors := map[string]struct{}{"": {}}
	var tools []*sdkmcp.Tool
	var discovered []DiscoveredTool
	var catalogBytes int
	cursor := ""
	for page := 0; page < maxDiscoveryPages; page++ {
		result, err := list(ctx, &sdkmcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, nil, fmt.Errorf("list tools from MCP server %q: %s", serverName, redactError(err, redactions, rawURL))
		}
		if result == nil {
			return nil, nil, fmt.Errorf("list tools from MCP server %q returned an empty response", serverName)
		}
		for _, rawTool := range result.Tools {
			if rawTool == nil {
				return nil, nil, fmt.Errorf("MCP server %q returned a null tool definition", serverName)
			}
			rawDefinition, err := json.Marshal(rawTool)
			if err != nil {
				return nil, nil, fmt.Errorf("MCP server %q returned a tool definition that cannot be encoded", serverName)
			}
			catalogBytes += len(rawDefinition)
			if catalogBytes > maxCatalogBytes {
				return nil, nil, fmt.Errorf("MCP server %q exceeds the %d-byte discovery catalog limit", serverName, maxCatalogBytes)
			}
			// Names are protocol identities, not mutable display text. Reject a
			// secret-bearing identity before any name-bearing error or catalog.
			if redactText(rawTool.Name, redactions, rawURL) != rawTool.Name {
				return nil, nil, errors.New("MCP server returned a tool name containing a connection credential")
			}
			if err := validateToolName(rawTool.Name); err != nil {
				return nil, nil, fmt.Errorf("MCP server %q returned an invalid tool: %w", serverName, err)
			}
			if _, duplicate := seenNames[rawTool.Name]; duplicate {
				return nil, nil, fmt.Errorf("MCP server %q returned duplicate tool name %q", serverName, rawTool.Name)
			}
			if len(tools) >= maxDiscoveredTools {
				return nil, nil, fmt.Errorf("MCP server %q exceeds the %d-tool discovery limit", serverName, maxDiscoveredTools)
			}
			if len(rawTool.Description) > maxDescriptionBytes {
				return nil, nil, fmt.Errorf("MCP server %q tool %q description exceeds %d bytes", serverName, rawTool.Name, maxDescriptionBytes)
			}
			description, err := sanitizeDescription(redactText(rawTool.Description, redactions, rawURL))
			if err != nil {
				return nil, nil, fmt.Errorf("MCP server %q tool %q: %w", serverName, rawTool.Name, err)
			}
			schema, err := canonicalInputSchema(rawTool.InputSchema, redactions, rawURL)
			if err != nil {
				return nil, nil, fmt.Errorf("MCP server %q tool %q: %w", serverName, rawTool.Name, err)
			}
			fingerprint, err := definitionFingerprint(serverName, config, rawTool.Name, description, schema)
			if err != nil {
				return nil, nil, fmt.Errorf("fingerprint MCP server %q tool %q: %w", serverName, rawTool.Name, err)
			}
			var schemaMap map[string]any
			if err := json.Unmarshal(schema, &schemaMap); err != nil {
				return nil, nil, fmt.Errorf("decode canonical schema for MCP server %q tool %q: %w", serverName, rawTool.Name, err)
			}
			tools = append(tools, &sdkmcp.Tool{
				Name: rawTool.Name, Description: description, InputSchema: schemaMap,
			})
			discovered = append(discovered, DiscoveredTool{
				Name: rawTool.Name, Description: description,
				InputSchema: append([]byte(nil), schema...), DefinitionSHA256: fingerprint,
				ServerProvidedHint: rawTool.Annotations != nil || rawTool.Title != "" || len(rawTool.Icons) > 0,
			})
			seenNames[rawTool.Name] = struct{}{}
		}
		next := result.NextCursor
		if next == "" {
			return tools, discovered, nil
		}
		catalogBytes += len(next)
		if catalogBytes > maxCatalogBytes {
			return nil, nil, fmt.Errorf("MCP server %q exceeds the %d-byte discovery catalog limit", serverName, maxCatalogBytes)
		}
		if _, repeated := seenCursors[next]; repeated {
			return nil, nil, fmt.Errorf("MCP server %q repeated tools/list cursor", serverName)
		}
		seenCursors[next] = struct{}{}
		cursor = next
	}
	return nil, nil, fmt.Errorf("MCP server %q exceeds the %d-page discovery limit", serverName, maxDiscoveryPages)
}

// Name returns the configured server identifier.
func (c *Client) Name() string { return c.name }

// Tools returns a defensive copy for catalog display compatibility.
// Registration must use RegisterSelected, not this raw-name view.
func (c *Client) Tools() []*sdkmcp.Tool {
	result := make([]*sdkmcp.Tool, 0, len(c.tools))
	for _, item := range c.tools {
		data, _ := json.Marshal(item.InputSchema)
		var schema map[string]any
		_ = json.Unmarshal(data, &schema)
		result = append(result, &sdkmcp.Tool{
			Name: item.Name, Description: item.Description, InputSchema: schema,
		})
	}
	return result
}

// DiscoveredTools returns a defensive copy of the safe management catalog.
func (c *Client) DiscoveredTools() []DiscoveredTool {
	result := make([]DiscoveredTool, len(c.discovered))
	for i, item := range c.discovered {
		result[i] = item
		result[i].InputSchema = append([]byte(nil), item.InputSchema...)
	}
	return result
}

// callTool is deliberately private. Provider.Execute authorizes immediately
// before reaching this method.
func (c *Client) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if c.callToolFunc == nil {
		return "", errors.New("MCP client has no callable session")
	}
	redactions := append([]string(nil), c.redactValues...)
	redactions = appendSensitiveArgumentRedactions(redactions, args, "", 0)
	result, err := c.callToolFunc(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", fmt.Errorf("call MCP tool %q: %s", name, redactError(err, redactions, c.redactURL))
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if result == nil {
		return "", fmt.Errorf("call MCP tool %q returned an empty response", name)
	}
	rawResult, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return "", fmt.Errorf("call MCP tool %q returned an invalid response", name)
	}
	if len(rawResult) > maxToolResultBytes {
		return "", fmt.Errorf("MCP tool result exceeds %d bytes", maxToolResultBytes)
	}
	text, err := contentToTextSanitized(result.Content, func(text string) string {
		return sanitizeMCPResultText(text, redactions, c.redactURL)
	})
	if err != nil {
		return "", fmt.Errorf("call MCP tool %q: %w", name, err)
	}
	text = sanitizeMCPResultText(text, redactions, c.redactURL)
	if result.IsError {
		return "", &ToolExecutionError{Tool: name, Message: text}
	}
	return text, nil
}

// ToolExecutionError reports a server-declared CallToolResult error.
type ToolExecutionError struct {
	Tool    string
	Message string
}

func (e *ToolExecutionError) Error() string {
	message := e.Message
	if message == "" {
		message = "server returned an unspecified error"
	}
	return fmt.Sprintf("MCP tool %q returned an error: %s", e.Tool, message)
}

// Close closes the MCP session and transport.
func (c *Client) Close() error {
	if c == nil || c.closeFunc == nil {
		return nil
	}
	if err := c.closeFunc(); err != nil {
		return fmt.Errorf("close MCP server %q: %s", c.name, redactError(err, c.redactValues, c.redactURL))
	}
	return nil
}

func contentToText(contents []sdkmcp.Content) (string, error) {
	return contentToTextSanitized(contents, nil)
}

func contentToTextSanitized(contents []sdkmcp.Content, sanitize func(string) string) (string, error) {
	var b strings.Builder
	for _, item := range contents {
		var text string
		switch value := item.(type) {
		case *sdkmcp.TextContent:
			text = value.Text
		default:
			text = fmt.Sprintf("[unsupported content type: %T]", item)
		}
		if sanitize != nil {
			text = sanitize(text)
		}
		separator := 0
		if b.Len() > 0 {
			separator = 1
		}
		if b.Len()+separator+len(text) > maxToolResultBytes {
			return "", fmt.Errorf("MCP tool result exceeds %d bytes", maxToolResultBytes)
		}
		if separator != 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text)
	}
	return b.String(), nil
}

type headerTransport struct {
	base       http.RoundTripper
	headers    map[string]string
	serverName string
	origin     string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if endpointOrigin(req.URL) != t.origin {
		return nil, fmt.Errorf("remote MCP server %q attempted a cross-origin request", t.serverName)
	}
	cloned := req.Clone(req.Context())
	for key, value := range t.headers {
		cloned.Header.Set(key, value)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(cloned)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		_ = resp.Body.Close()
		return nil, fmt.Errorf("remote MCP server %q returned HTTP 401 Unauthorized", t.serverName)
	case http.StatusForbidden:
		_ = resp.Body.Close()
		return nil, fmt.Errorf("remote MCP server %q returned HTTP 403 Forbidden", t.serverName)
	}
	return resp, nil
}

func validateRemoteURL(raw string, allowInsecure bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, errors.New("URL cannot be parsed")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("URL must use http or https")
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return nil, errors.New("URL must include a host")
	}
	if parsed.User != nil {
		return nil, errors.New("URL user information is not allowed; use a configured header")
	}
	if parsed.Fragment != "" {
		return nil, errors.New("URL fragments are not allowed")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) && !allowInsecure {
		return nil, errors.New("plain HTTP for a non-loopback MCP server requires allow_insecure_http (CLI: --allow-insecure-http)")
	}
	return parsed, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func safeEndpointLabel(endpoint *url.URL) string {
	return (&url.URL{Scheme: endpoint.Scheme, Host: endpoint.Host, Path: endpoint.Path}).String()
}

func endpointOrigin(endpoint *url.URL) string {
	if endpoint == nil {
		return ""
	}
	return strings.ToLower(endpoint.Scheme) + "://" + strings.ToLower(endpoint.Host)
}

func validateAndExpandHeaders(headers map[string]string) (map[string]string, []string, error) {
	expanded := make(map[string]string, len(headers))
	seen := make(map[string]struct{}, len(headers))
	var redactions []string
	for rawName, rawValue := range headers {
		if !validHeaderName(rawName) {
			return nil, nil, fmt.Errorf("invalid header name %q", rawName)
		}
		name := http.CanonicalHeaderKey(rawName)
		identity := strings.ToLower(name)
		if _, duplicate := seen[identity]; duplicate {
			return nil, nil, fmt.Errorf("header %q is configured more than once", name)
		}
		seen[identity] = struct{}{}
		if forbiddenMCPHeader(name) {
			return nil, nil, fmt.Errorf("header %q cannot be configured for an MCP connection", name)
		}
		value, components, err := expandEnvironmentReferencesWithSecrets(rawValue)
		if err != nil {
			return nil, nil, fmt.Errorf("header %q: %w", name, err)
		}
		if strings.TrimSpace(value) == "" {
			return nil, nil, fmt.Errorf("header %q expands to an empty value", name)
		}
		if containsInvalidHeaderValue(value) {
			return nil, nil, fmt.Errorf("header %q contains invalid control characters", name)
		}
		expanded[name] = value
		redactions = append(redactions, components...)
		redactions = appendRedaction(redactions, value)
	}
	return expanded, redactions, nil
}

func validHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			continue
		}
		if strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c)) {
			continue
		}
		return false
	}
	return true
}

func forbiddenMCPHeader(name string) bool {
	switch strings.ToLower(name) {
	case "host", "content-length", "transfer-encoding", "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "upgrade", "mcp-protocol-version", "mcp-session-id", "accept", "content-type":
		return true
	default:
		return false
	}
}

func containsInvalidHeaderValue(value string) bool {
	for _, r := range value {
		if r == '\r' || r == '\n' || r == 0 || (unicode.IsControl(r) && r != '\t') {
			return true
		}
	}
	return false
}

func buildStdioEnv(explicit []string) ([]string, []string, error) {
	type envValue struct {
		key   string
		value string
	}
	values := make(map[string]envValue)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && safeInheritedEnv(key) {
			values[normalizedEnvKey(key)] = envValue{key: key, value: value}
		}
	}
	var redactions []string
	seenExplicit := make(map[string]struct{}, len(explicit))
	for _, entry := range explicit {
		key, rawValue, ok := strings.Cut(entry, "=")
		if !ok || !validEnvKey(key) {
			return nil, nil, errors.New("environment entries must use a valid KEY=VALUE form")
		}
		value, components, err := expandEnvironmentReferencesWithSecrets(rawValue)
		if err != nil {
			return nil, nil, fmt.Errorf("environment variable %q: %w", key, err)
		}
		if strings.IndexByte(value, 0) >= 0 {
			return nil, nil, fmt.Errorf("environment variable %q contains a NUL byte", key)
		}
		normalized := normalizedEnvKey(key)
		if _, duplicate := seenExplicit[normalized]; duplicate {
			return nil, nil, fmt.Errorf("environment variable %q is configured more than once", key)
		}
		seenExplicit[normalized] = struct{}{}
		values[normalized] = envValue{key: key, value: value}
		redactions = append(redactions, components...)
		redactions = appendRedaction(redactions, value)
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, normalized := range keys {
		entry := values[normalized]
		result = append(result, entry.key+"="+entry.value)
	}
	return result, redactions, nil
}

func safeInheritedEnv(key string) bool {
	lookup := key
	if runtime.GOOS == "windows" {
		lookup = strings.ToUpper(key)
	}
	switch lookup {
	case "PATH", "HOME", "USER", "LOGNAME", "SHELL", "TMPDIR", "TMP", "TEMP", "LANG", "LANGUAGE",
		"LC_ALL", "LC_CTYPE", "LC_COLLATE", "LC_MESSAGES", "LC_MONETARY", "LC_NUMERIC", "LC_TIME",
		"LC_PAPER", "LC_NAME", "LC_ADDRESS", "LC_TELEPHONE", "LC_MEASUREMENT", "LC_IDENTIFICATION",
		"SYSTEMROOT", "SystemRoot", "COMSPEC", "PATHEXT", "USERPROFILE", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS":
		return true
	default:
		return false
	}
}

func normalizedEnvKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}

func validEnvKey(key string) bool {
	if key == "" || !((key[0] >= 'A' && key[0] <= 'Z') || (key[0] >= 'a' && key[0] <= 'z') || key[0] == '_') {
		return false
	}
	for i := 1; i < len(key); i++ {
		c := key[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			continue
		}
		return false
	}
	return true
}

func expandEnvironmentReferences(value string) (string, error) {
	expanded, _, err := expandEnvironmentReferencesWithSecrets(value)
	return expanded, err
}

func expandEnvironmentReferencesWithSecrets(value string) (string, []string, error) {
	var missing string
	var secrets []string
	expanded := os.Expand(value, func(key string) string {
		resolved, ok := os.LookupEnv(key)
		if !ok && missing == "" {
			missing = key
		}
		secrets = appendRedaction(secrets, resolved)
		return resolved
	})
	if missing != "" {
		return "", nil, fmt.Errorf("references unset environment variable %q", missing)
	}
	return expanded, secrets, nil
}

func redactError(err error, values []string, rawURL string) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if rawURL != "" {
		if parsed, parseErr := url.Parse(rawURL); parseErr == nil {
			message = strings.ReplaceAll(message, rawURL, safeEndpointLabel(parsed))
		} else {
			message = strings.ReplaceAll(message, rawURL, "<redacted-url>")
		}
	}
	for _, value := range sortedRedactions(values) {
		message = strings.ReplaceAll(message, value, "<redacted>")
	}
	return redactPotentialCredentialText(message)
}

func commandArgumentRedactions(args []string) []string {
	var result []string
	redactNext := false
	for _, arg := range args {
		if redactNext {
			result = appendCommandCredential(result, arg)
			redactNext = false
			continue
		}
		if key, value, ok := strings.Cut(arg, "="); ok && sensitiveName(key) {
			result = appendCommandCredential(result, value)
			continue
		}
		if strings.HasPrefix(arg, "-") && !strings.Contains(arg, "=") && sensitiveName(arg) {
			redactNext = true
			continue
		}
		if _, parsed := commandArgumentURL(arg); parsed != nil {
			if parsed.User != nil {
				result = appendRedaction(result, parsed.User.Username())
				password, _ := parsed.User.Password()
				result = appendRedaction(result, password)
			}
			result = appendRedaction(result, parsed.Fragment)
			for _, values := range parsed.Query() {
				for _, value := range values {
					result = appendRedaction(result, value)
				}
			}
		}
	}
	return result
}

// Header and environment arguments wrap credentials in another key/value
// representation (Authorization: Bearer TOKEN, API_TOKEN=TOKEN). Collect each
// nested value, without changing the argument passed to the process.
func appendCommandCredential(values []string, value string) []string {
	values = appendRedaction(values, value)
	for depth := 0; depth < maxSchemaDepth; depth++ {
		index := strings.IndexAny(value, "=: \t")
		if index < 0 {
			break
		}
		value = strings.TrimSpace(value[index+1:])
		if value == "" {
			break
		}
		values = appendRedaction(values, value)
	}
	return values
}

func appendSensitiveArgumentRedactions(values []string, value any, key string, depth int) []string {
	if depth > maxSchemaDepth {
		return values
	}
	if key != "" && sensitiveName(key) {
		return appendCredentialLeaves(values, value, depth)
	}
	switch item := value.(type) {
	case map[string]any:
		for childKey, child := range item {
			values = appendSensitiveArgumentRedactions(values, child, childKey, depth+1)
		}
	case []any:
		for _, child := range item {
			values = appendSensitiveArgumentRedactions(values, child, key, depth+1)
		}
	}
	return values
}

func appendCredentialLeaves(values []string, value any, depth int) []string {
	if depth > maxSchemaDepth {
		return values
	}
	switch item := value.(type) {
	case string:
		return appendRedaction(values, item)
	case []byte:
		return appendRedaction(values, string(item))
	case json.Number:
		return appendRedaction(values, item.String())
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return appendRedaction(values, fmt.Sprint(item))
	case map[string]any:
		for _, child := range item {
			values = appendCredentialLeaves(values, child, depth+1)
		}
	case []any:
		for _, child := range item {
			values = appendCredentialLeaves(values, child, depth+1)
		}
	}
	return values
}

func appendRedaction(values []string, value string) []string {
	if value == "" {
		return values
	}
	values = append(values, value)
	if _, credential, ok := strings.Cut(value, " "); ok && credential != "" {
		values = append(values, strings.TrimSpace(credential))
	}
	return values
}

func sortedRedactions(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if len(result[i]) == len(result[j]) {
			return result[i] < result[j]
		}
		return len(result[i]) > len(result[j])
	})
	return result
}

func validateToolName(name string) error {
	if name == "" {
		return errors.New("tool name is empty")
	}
	if len(name) > maxToolNameBytes {
		return fmt.Errorf("tool name exceeds %d bytes", maxToolNameBytes)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("tool name contains control characters")
		}
	}
	return nil
}
