// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var credentialLinePattern = regexp.MustCompile(`(?i)\b(?:token|secret|password|passwd|api[_-]?key|authorization|cookie|credential|private[_-]?key|access[_-]?key|client[_-]?secret)\b["']?[[:space:]]*[:=]`)

const (
	maxDiscoveryPages   = 64
	maxDiscoveredTools  = 512
	maxToolNameBytes    = 1024
	maxDescriptionBytes = 8 << 10
	maxSchemaBytes      = 256 << 10
	maxSchemaDepth      = 64
	maxCatalogBytes     = 4 << 20
	maxToolResultBytes  = 1 << 20
)

// ToolID is the immutable identity of a tool. Tool names are scoped by server;
// bare names are never used as cross-server identities.
type ToolID struct {
	Server string `json:"server"`
	Name   string `json:"name"`
}

// ToolGrant is the immutable policy snapshot attached to a registered provider.
type ToolGrant struct {
	ID                   ToolID
	ModelAlias           string
	Permission           Permission
	DefinitionSHA256     string
	UntrustedDescription string
}

// DiscoveredTool is the bounded, sanitized catalog record exposed to management
// flows. It contains no server annotations and no resolved connection secrets.
type DiscoveredTool struct {
	Name               string          `json:"name"`
	Description        string          `json:"description,omitempty"`
	InputSchema        json.RawMessage `json:"input_schema"`
	DefinitionSHA256   string          `json:"definition_sha256"`
	ServerProvidedHint bool            `json:"server_provided_hint,omitempty"`
}

// RedactConfigText protects management labels, including legacy configured tool
// names, using the execution client's credential redaction. It never connects.
func RedactConfigText(config MCPServerConfig, text string) string {
	values := commandArgumentRedactions(config.Args)
	entries := append([]string(nil), config.Env...)
	for _, value := range config.Headers {
		entries = append(entries, "header="+value)
	}
	for _, entry := range entries {
		_, value, _ := strings.Cut(entry, "=")
		values = appendRedaction(values, value)
		// A missing reference must not prevent redacting other resolved values.
		os.Expand(value, func(key string) string { values = appendRedaction(values, os.Getenv(key)); return "" })
		expanded, parts, err := expandEnvironmentReferencesWithSecrets(value)
		values = append(values, parts...)
		if err == nil {
			values = appendRedaction(values, expanded)
		}
	}
	return redactText(text, values, config.URL)
}

// ModelAlias returns a deterministic, provider-safe function name. The fixed
// component limits keep every alias at or below 64 ASCII bytes.
func ModelAlias(id ToolID) string {
	server := identifierSlug(id.Server, 19, "server")
	tool := identifierSlug(id.Name, 20, "tool")
	sum := sha256.Sum256([]byte(id.Server + "\x00" + id.Name))
	return fmt.Sprintf("mcp__%s__%s__%s", server, tool, hex.EncodeToString(sum[:8]))
}

func identifierSlug(value string, limit int, fallback string) string {
	var b strings.Builder
	b.Grow(min(len(value), limit))
	lastSeparator := false
	for _, r := range value {
		var out byte
		switch {
		case r >= 'A' && r <= 'Z':
			out = byte(r - 'A' + 'a')
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = byte(r)
		case r == '-' || r == '_':
			out = byte(r)
		default:
			out = '_'
		}
		separator := out == '_' || out == '-'
		if separator && lastSeparator {
			continue
		}
		if b.Len() == 0 && separator {
			continue
		}
		if b.Len() >= limit {
			break
		}
		b.WriteByte(out)
		lastSeparator = separator
	}
	result := strings.TrimRight(b.String(), "_-")
	if result == "" {
		return fallback
	}
	return result
}

func sanitizeDescription(value string) (string, error) {
	if len(value) > maxDescriptionBytes {
		return "", fmt.Errorf("MCP tool description exceeds %d bytes", maxDescriptionBytes)
	}
	return strings.TrimSpace(sanitizeUntrustedText(strings.ReplaceAll(value, "\r\n", "\n"))), nil
}

func sanitizeUntrustedText(value string) string {
	value = stripANSI(value)
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case unicode.IsControl(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func stripANSI(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); {
		if value[i] != 0x1b {
			b.WriteByte(value[i])
			i++
			continue
		}
		i++
		if i >= len(value) {
			break
		}
		switch value[i] {
		case '[':
			i++
			for i < len(value) {
				c := value[i]
				i++
				if c >= 0x40 && c <= 0x7e {
					break
				}
			}
		case ']':
			i++
			for i < len(value) {
				if value[i] == 0x07 {
					i++
					break
				}
				if value[i] == 0x1b && i+1 < len(value) && value[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		default:
			// Skip the single-character escape sequence introducer.
			i++
		}
	}
	return b.String()
}

func redactText(value string, redactions []string, rawURL string) string {
	if rawURL != "" {
		if parsed, err := url.Parse(rawURL); err == nil {
			value = strings.ReplaceAll(value, rawURL, safeEndpointLabel(parsed))
		} else {
			value = strings.ReplaceAll(value, rawURL, "<redacted-url>")
		}
	}
	for _, secret := range sortedRedactions(redactions) {
		value = strings.ReplaceAll(value, secret, "<redacted>")
	}
	return value
}

// sanitizeMCPResultText removes resolved connection/call secrets and performs a
// second, structure-aware pass for credentials newly returned by a business
// tool before the value reaches the model, session history, or console.
func sanitizeMCPResultText(value string, redactions []string, rawURL string) string {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var records []string
	for {
		offset := decoder.InputOffset()
		var decoded any
		err := decoder.Decode(&decoded)
		if err == io.EOF && len(records) > 0 {
			return strings.Join(records, "\n")
		}
		if err != nil {
			// A text suffix must not undo protection of complete JSON records
			// already decoded (including NDJSON and escaped credential keys).
			if len(records) > 0 {
				return strings.Join(records, "\n") + redactResultString(value[offset:], redactions, rawURL)
			}
			return redactResultString(value, redactions, rawURL)
		}
		clean := redactSensitiveResultValue(decoded, "", redactions, rawURL)
		encoded, err := json.Marshal(clean)
		if err != nil {
			return "[invalid MCP result]"
		}
		records = append(records, sanitizeUntrustedText(string(encoded)))
	}
}

func redactResultString(value string, redactions []string, rawURL string) string {
	// Terminal control normalization must not reconstruct a secret after its
	// last exact-value check. Redact both representations before rendering.
	value = redactText(value, redactions, rawURL)
	value = redactText(sanitizeUntrustedText(value), redactions, rawURL)
	return redactPotentialCredentialText(value)
}

func redactSensitiveResultValue(value any, key string, redactions []string, rawURL string) any {
	if key != "" && sensitiveName(key) {
		return "[redacted]"
	}
	switch item := value.(type) {
	case string:
		return redactResultString(item, redactions, rawURL)
	case json.Number:
		if clean := redactResultString(string(item), redactions, rawURL); clean != string(item) {
			return clean
		}
		return item
	case map[string]any:
		result := make(map[string]any, len(item))
		for childKey, child := range item {
			cleanKey := strings.TrimSpace(redactResultString(childKey, redactions, rawURL))
			if cleanKey == "" {
				cleanKey = "[sanitized-key]"
			}
			if _, collision := result[cleanKey]; collision {
				result[cleanKey] = "[redacted key collision]"
				continue
			}
			// Detect sensitive names after control normalization, but before
			// replacing a known secret that might also be part of the key name.
			policyKey := strings.TrimSpace(sanitizeUntrustedText(childKey))
			result[cleanKey] = redactSensitiveResultValue(child, policyKey, redactions, rawURL)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for index, child := range item {
			result[index] = redactSensitiveResultValue(child, key, redactions, rawURL)
		}
		return result
	default:
		return value
	}
}

func redactPotentialCredentialText(value string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		if match := credentialLinePattern.FindStringIndex(line); match != nil {
			lines[index] = line[:match[1]] + "<redacted>"
		}
	}
	return sanitizeUntrustedText(strings.Join(lines, "\n"))
}

func canonicalInputSchema(value any, redactions []string, rawURL string) (json.RawMessage, error) {
	if value == nil {
		value = map[string]any{"type": "object"}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal MCP input schema: %w", err)
	}
	if len(data) > maxSchemaBytes {
		return nil, fmt.Errorf("MCP input schema exceeds %d bytes", maxSchemaBytes)
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, fmt.Errorf("normalize MCP input schema: %w", err)
	}
	normalized, err = sanitizeJSONStrings(normalized, redactions, rawURL)
	if err != nil {
		return nil, err
	}
	if err := validateJSONDepth(normalized, 1); err != nil {
		return nil, err
	}
	object, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("MCP input schema must be a JSON object")
	}
	if schemaType, exists := object["type"]; exists {
		if schemaType != "object" {
			return nil, fmt.Errorf("MCP input schema type must be object")
		}
	} else {
		object["type"] = "object"
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical MCP input schema: %w", err)
	}
	if len(canonical) > maxSchemaBytes {
		return nil, fmt.Errorf("canonical MCP input schema exceeds %d bytes", maxSchemaBytes)
	}
	return canonical, nil
}

func sanitizeJSONStrings(value any, redactions []string, rawURL string) (any, error) {
	switch item := value.(type) {
	case string:
		return sanitizeUntrustedText(redactText(item, redactions, rawURL)), nil
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, child := range item {
			cleanKey := sanitizeUntrustedText(redactText(key, redactions, rawURL))
			if strings.TrimSpace(cleanKey) == "" {
				return nil, fmt.Errorf("MCP input schema contains an empty key after sanitization")
			}
			if _, collision := result[cleanKey]; collision {
				return nil, fmt.Errorf("MCP input schema keys collide after sanitization")
			}
			cleanChild, err := sanitizeJSONStrings(child, redactions, rawURL)
			if err != nil {
				return nil, err
			}
			result[cleanKey] = cleanChild
		}
		return result, nil
	case []any:
		result := make([]any, len(item))
		for i, child := range item {
			cleanChild, err := sanitizeJSONStrings(child, redactions, rawURL)
			if err != nil {
				return nil, err
			}
			result[i] = cleanChild
		}
		return result, nil
	default:
		return value, nil
	}
}

func validateJSONDepth(value any, depth int) error {
	if depth > maxSchemaDepth {
		return fmt.Errorf("MCP input schema exceeds maximum depth %d", maxSchemaDepth)
	}
	switch item := value.(type) {
	case map[string]any:
		for _, child := range item {
			if err := validateJSONDepth(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range item {
			if err := validateJSONDepth(child, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func definitionFingerprint(serverName string, config MCPServerConfig, toolName, description string, schema json.RawMessage) (string, error) {
	connection, err := sanitizedConnectionIdentity(config)
	if err != nil {
		return "", err
	}
	payload := struct {
		Server      string          `json:"server"`
		Connection  any             `json:"connection"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
	}{
		Server:      serverName,
		Connection:  connection,
		Name:        toolName,
		Description: description,
		InputSchema: schema,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("fingerprint MCP tool definition: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type connectionIdentity struct {
	Type              string   `json:"type"`
	Command           string   `json:"command,omitempty"`
	Args              []string `json:"args,omitempty"`
	EnvKeys           []string `json:"env_keys,omitempty"`
	URL               string   `json:"url,omitempty"`
	HeaderNames       []string `json:"header_names,omitempty"`
	AllowInsecureHTTP bool     `json:"allow_insecure_http,omitempty"`
}

func sanitizedConnectionIdentity(config MCPServerConfig) (connectionIdentity, error) {
	typeName := config.Type
	if typeName == "" {
		typeName = "stdio"
	}
	identity := connectionIdentity{Type: typeName}
	switch typeName {
	case "stdio":
		identity.Command = config.Command
		identity.Args = sanitizeCommandArgs(config.Args)
		for _, entry := range config.Env {
			key, _, ok := strings.Cut(entry, "=")
			if !ok || !validEnvKey(key) {
				return connectionIdentity{}, fmt.Errorf("invalid MCP environment entry")
			}
			identity.EnvKeys = append(identity.EnvKeys, key)
		}
		sort.Strings(identity.EnvKeys)
	case "remote":
		parsed, err := validateRemoteURL(config.URL, config.AllowInsecureHTTP)
		if err != nil {
			return connectionIdentity{}, err
		}
		identity.URL = sanitizedURLIdentity(parsed)
		identity.AllowInsecureHTTP = config.AllowInsecureHTTP
		for name := range config.Headers {
			identity.HeaderNames = append(identity.HeaderNames, strings.ToLower(name))
		}
		sort.Strings(identity.HeaderNames)
	default:
		return connectionIdentity{}, fmt.Errorf("unsupported MCP server type %q", config.Type)
	}
	return identity, nil
}

// SafeCommandArguments is the common credential-safe representation for
// connection identity and CLI previews. It never changes execution arguments.
func SafeCommandArguments(args []string) []string { return sanitizeCommandArgs(args) }

// SensitiveArgumentName shares credential-key classification with the CLI.
func SensitiveArgumentName(name string) bool { return sensitiveName(name) }

func sanitizeCommandArgs(args []string) []string {
	result := make([]string, len(args))
	redactNext := false
	for i, arg := range args {
		if redactNext {
			result[i] = "<redacted>"
			redactNext = false
			continue
		}
		if key, _, ok := strings.Cut(arg, "="); ok && sensitiveName(key) {
			result[i] = key + "=<redacted>"
			continue
		}
		if strings.HasPrefix(arg, "-") && !strings.Contains(arg, "=") && sensitiveName(arg) {
			result[i] = arg
			redactNext = true
			continue
		}
		if prefix, parsed := commandArgumentURL(arg); parsed != nil {
			result[i] = prefix + sanitizedURLIdentity(parsed)
			continue
		}
		result[i] = arg
	}
	return result
}

// Parse positional URLs and flag-assignment URLs once for previews, connection
// identity, and runtime credential collection. Execution arguments stay intact.
func commandArgumentURL(argument string) (string, *url.URL) {
	prefix, value := "", argument
	if strings.HasPrefix(argument, "-") {
		if key, assigned, ok := strings.Cut(argument, "="); ok {
			prefix, value = key+"=", assigned
		}
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", nil
	}
	return prefix, parsed
}

func sanitizedURLIdentity(parsed *url.URL) string {
	clean := &url.URL{
		Scheme:  strings.ToLower(parsed.Scheme),
		Host:    strings.ToLower(parsed.Host),
		Path:    parsed.Path,
		RawPath: parsed.RawPath,
	}
	if clean.Path == "" {
		clean.Path = "/"
	}
	if len(parsed.Query()) > 0 {
		keys := make([]string, 0, len(parsed.Query()))
		for key := range parsed.Query() {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		query := url.Values{}
		for _, key := range keys {
			for range parsed.Query()[key] {
				query.Add(key, "<redacted>")
			}
		}
		clean.RawQuery = query.Encode()
	}
	return clean.String()
}

func sensitiveName(value string) bool {
	value = strings.ToLower(strings.TrimLeft(value, "-_"))
	value = strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(value)
	for _, marker := range []string{"token", "secret", "password", "passwd", "apikey", "authorization", "cookie", "credential", "privatekey", "accesskey", "header", "env"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
