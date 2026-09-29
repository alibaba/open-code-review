// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: scan_mcp_e2e mcp|llm")
	}
	switch os.Args[1] {
	case "mcp":
		runMCP()
	case "llm":
		runLLM()
	default:
		panic("usage: scan_mcp_e2e mcp|llm")
	}
}

func runMCP() {
	server := mcp.NewServer(&mcp.Implementation{Name: "scan-e2e", Version: "v1"}, nil)
	server.AddTool(&mcp.Tool{
		Name: "lookup_context", InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := os.WriteFile(os.Getenv("OCR_DEMO_TOOL_MARKER"), []byte("MCP_TOOL_CALLED\n"), 0o600); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "scan-mcp-context"}}}, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		panic(err)
	}
}

func runLLM() {
	http.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
			Messages json.RawMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		available := false
		for _, tool := range request.Tools {
			available = available || tool.Name == "lookup_context"
		}
		if !available {
			http.Error(w, "lookup_context not offered to scan", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(request.Messages), "scan-mcp-context") {
			fmt.Fprintln(os.Stderr, "MCP_RESULT_REACHED_SCAN_AGENT")
			fmt.Fprint(w, `{"id":"done","type":"message","role":"assistant","model":"claude-test","content":[{"type":"tool_use","id":"done_1","name":"task_done","input":{"state":"DONE"}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`)
			return
		}
		fmt.Fprint(w, `{"id":"lookup","type":"message","role":"assistant","model":"claude-test","content":[{"type":"tool_use","id":"lookup_1","name":"lookup_context","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(os.Getenv("OCR_DEMO_URL_FILE"), []byte("http://"+listener.Addr().String()+"/v1/messages"), 0o600); err != nil {
		panic(err)
	}
	if err := http.Serve(listener, nil); err != nil {
		panic(err)
	}
}
