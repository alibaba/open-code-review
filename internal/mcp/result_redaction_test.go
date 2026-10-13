// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestResultRedactionAcrossBlocksAndNormalization(t *testing.T) {
	for _, tc := range []struct {
		name    string
		blocks  []string
		secrets []string
		absent  string
	}{
		{"blocks", []string{`{"token":"issued_sentinel"}`, `{"status":"ok"}`}, nil, "issued_sentinel"},
		{"mixed", []string{`{"token":"issued_sentinel"}`, "status ok"}, nil, "issued_sentinel"},
		{"ndjson", []string{"{\"access_token\":\"issued_sentinel\"}\n{\"status\":\"ok\"}"}, nil, "issued_sentinel"},
		{"ndjson-escaped-key", []string{"{\"to\\u006ben\":\"issued_sentinel\"}\n{\"status\":\"ok\"}"}, nil, "issued_sentinel"},
		{"json-text-suffix", []string{`{"access_token":"issued_sentinel"} status ok`}, nil, "issued_sentinel"},
		{"escaped-value", []string{`{"value":"connection\u005fsentinel"}`}, []string{"connection_sentinel"}, "connection_sentinel"},
		{"escaped-key", []string{`{"connection\u005fsentinel":"ok"}`}, []string{"connection_sentinel"}, "connection_sentinel"},
		{"nested", []string{`{"values":[{"answer":"connection\u005fsentinel"}]}`}, []string{"connection_sentinel"}, "connection_sentinel"},
		{"terminal-controls", []string{`{"value":"connection\u001b[31m_sentinel"}`}, []string{"connection_sentinel"}, "connection_sentinel"},
		{"normalized-sensitive-key", []string{`{"to\u0000ken":"issued_sentinel"}`}, nil, "issued_sentinel"},
		{"numeric-secret", []string{`{"value":123456789}`}, []string{"123456789"}, "123456789"},
		{"json-punctuation", []string{`{"value":"abc\"def"}`}, []string{`abc"def`}, `abc\"def`},
		{"quoted-text", []string{`status: {"token":"issued_sentinel"}`}, nil, "issued_sentinel"},
	} {
		for _, isError := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/success", true: "/error"}[isError], func(t *testing.T) {
				c := &Client{redactValues: tc.secrets, callToolFunc: func(context.Context, *sdkmcp.CallToolParams) (*sdkmcp.CallToolResult, error) {
					r := &sdkmcp.CallToolResult{IsError: isError}
					for _, block := range tc.blocks {
						r.Content = append(r.Content, &sdkmcp.TextContent{Text: block})
					}
					return r, nil
				}}
				got, err := c.callTool(context.Background(), "test", nil)
				if isError {
					if err == nil {
						t.Fatal("lost server error")
					}
					got += err.Error()
				} else if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(got, tc.absent) {
					t.Fatalf("credential survived: %s", got)
				}
			})
		}
	}
	const ordinary = `{"count":9007199254740993,"status":"ok","values":[1,true,null]}`
	if got := sanitizeMCPResultText(ordinary, nil, ""); got != ordinary {
		t.Fatalf("ordinary JSON changed: %s", got)
	}
	const records = "{\"count\":9007199254740993}\n{\"status\":\"ok\"}"
	if got := sanitizeMCPResultText(records, nil, ""); got != records {
		t.Fatalf("ordinary records changed: %s", got)
	}
}
