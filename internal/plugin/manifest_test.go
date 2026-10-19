// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestManifestValidation(t *testing.T) {
	valid := testManifest("example", "1.0.0")
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseManifest(data)
	if err != nil || parsed.ID != valid.ID {
		t.Fatalf("ParseManifest = %+v, %v", parsed, err)
	}
	for _, id := range []string{"", "../escape", "a/b", "Example", "nul", "con", "com1", "lpt9", "aux.txt", "bad.", "bad-", "-bad", strings.Repeat("a", 65)} {
		t.Run("id/"+id, func(t *testing.T) {
			m := valid
			m.ID = id
			requireError(t, m.Validate(), "invalid plugin id")
		})
	}
	for _, id := range []string{"a", "example-tools", "go.rules", "pack_2", "com10", strings.Repeat("a", 64)} {
		m := valid
		m.ID = id
		if err := m.Validate(); err != nil {
			t.Fatalf("portable id %q rejected: %v", id, err)
		}
	}
	tests := []struct {
		name string
		edit func(*Manifest)
		want string
	}{
		{"schema", func(m *Manifest) { m.SchemaVersion = 2 }, "unsupported"},
		{"name", func(m *Manifest) { m.Name = " " }, "required"},
		{"version", func(m *Manifest) { m.Version = "" }, "required"},
		{"empty", func(m *Manifest) { m.Rules = nil }, "must contribute"},
		{"duplicate", func(m *Manifest) { m.Rules = []string{"rules.json", "rules.json"} }, "duplicate"},
		{"case duplicate", func(m *Manifest) { m.Rules = []string{"rules.json", "RULES.json"} }, "duplicate"},
		{"extension", func(m *Manifest) { m.Rules = []string{"rules.txt"} }, "JSON file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := valid
			test.edit(&m)
			requireError(t, m.Validate(), test.want)
		})
	}
	for _, filename := range []string{"/rules.json", "../rules.json", "rules/../rules.json", "rules//rules.json", `rules\rules.json`, "C:/rules.json", "nul/rules.json", "con.txt/rules.json", "COM\u00b9/rules.json", "bad./rules.json", "bad /rules.json", "bad?/rules.json", "bad\x00/rules.json", "bad\n/rules.json", "."} {
		t.Run("path/"+filename, func(t *testing.T) {
			m := valid
			m.Rules = []string{filename}
			requireError(t, m.Validate(), "invalid rule path")
		})
	}
}

func TestMCPManifestValidation(t *testing.T) {
	for _, server := range []MCPServer{
		{Command: "node", Args: []string{"${plugin_dir}/server.js"}, Env: []string{"TOKEN=${TOKEN}"}, Tools: []string{"lookup"}},
		{Type: "stdio", Command: "mcp-server"},
		{Type: "remote", URL: "https://example.com/mcp", Headers: map[string]string{"Authorization": "Bearer ${TOKEN}"}},
		{Type: "remote", URL: "http://localhost:8080/mcp"},
		{Type: "remote", URL: "http://127.0.0.1:8080/mcp"},
		{Type: "remote", URL: "http://[::1]:8080/mcp"},
	} {
		m := testManifest("example", "1")
		m.Rules = nil
		m.MCPServers = map[string]MCPServer{"tools": server}
		if err := m.Validate(); err != nil {
			t.Fatalf("valid MCP server rejected: %+v, %v", server, err)
		}
	}
	for _, server := range []MCPServer{
		{}, {Type: "unsupported"}, {Command: "node", URL: "https://example.com"},
		{Command: "node", Headers: map[string]string{"A": "B"}},
		{Type: "remote"}, {Type: "remote", URL: "ftp://example.com"},
		{Type: "remote", URL: "http://example.com/mcp"},
		{Type: "remote", URL: "http://192.168.1.2/mcp"},
		{Type: "remote", URL: "http://localhost.example.com/mcp"},
		{Type: "remote", URL: "https://user:pass@example.com"},
		{Type: "remote", URL: "https://example.com/#fragment"},
		{Type: "remote", URL: "https://example.com", Command: "node"},
		{Type: "remote", URL: "https://example.com", Args: []string{"x"}},
		{Type: "remote", URL: "https://example.com", Env: []string{"A=B"}},
		{Command: "node", Env: []string{"EMPTY"}},
		{Command: "node", Env: []string{"=value"}},
		{Command: "node", Env: []string{"A=B\n"}},
		{Command: "node", Args: []string{"bad\x00arg"}},
		{Type: "remote", URL: "https://example.com", Headers: map[string]string{"": "value"}},
		{Type: "remote", URL: "https://example.com", Headers: map[string]string{"A": "value\r\ninjected"}},
	} {
		m := testManifest("example", "1")
		m.MCPServers = map[string]MCPServer{"tools": server}
		if err := m.Validate(); err == nil {
			t.Fatalf("invalid MCP server accepted: %+v", server)
		}
	}
	m := testManifest("example", "1")
	m.MCPServers = map[string]MCPServer{"../bad": {Command: "node"}}
	requireError(t, m.Validate(), "invalid MCP server name")
}

func TestStrictJSONContract(t *testing.T) {
	for _, data := range []string{
		`{"schema_version":1,"id":"example","name":"Example","version":"1","rules":["rules.json"],"setup":"do something"}`,
		`{"schema_version":1,"id":"example","name":"Example","version":"1","mcp_servers":{"tools":{"command":"node","setup":"do something"}}}`,
		`{"schema_version":1,"id":"example","id":"different","name":"Example","version":"1","rules":["rules.json"]}`,
		`{"schema_version":1,"id":"example","name":"Example","version":"1","rules":["rules.json"]} {}`,
		`{"schema_version":1,"id":"example","name":"Example","version":"1","rules":["rules.json"]} trailing`,
		`{"id": }`, `{"id":"example",}`, `{"id"}`, "", "null", "[]", `{"id":[1,2]}`,
		strings.Repeat("[", 66) + "1" + strings.Repeat("]", 66),
	} {
		if _, err := ParseManifest([]byte(data)); err == nil {
			t.Fatalf("invalid JSON contract accepted: %s", data)
		}
	}
}

func TestLegacyHTTPValidationRetainsManifestChecks(t *testing.T) {
	m := testManifest("legacy", "1")
	m.MCPServers = map[string]MCPServer{"remote": {Type: "remote", URL: "http://example.com/mcp"}}
	if err := m.validate(false); err != nil {
		t.Fatalf("legacy HTTP metadata rejected: %v", err)
	}
	for _, server := range []MCPServer{
		{Type: "remote", URL: "ftp://example.com/mcp"},
		{Type: "remote", URL: "http://user:pass@example.com/mcp"},
		{Type: "remote", URL: "http://example.com/mcp#fragment"},
		{Type: "remote", URL: "http://example.com/mcp", Command: "node"},
		{Type: "remote", URL: "http://example.com/mcp", Headers: map[string]string{"A": "bad\r\nheader"}},
	} {
		m.MCPServers["remote"] = server
		if err := m.validate(false); err == nil {
			t.Fatalf("invalid legacy metadata accepted: %+v", server)
		}
	}
}
