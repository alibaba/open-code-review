// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package reviewtask

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type countingToolProvider struct {
	name  string
	calls int
}

func (p *countingToolProvider) Tool() tool.Tool {
	if tool.IsReserved(p.name) {
		return tool.OfName(p.name)
	}
	return tool.Dynamic(p.name)
}
func (p *countingToolProvider) Execute(context.Context, map[string]any) (string, error) {
	p.calls++
	return "executed", nil
}

func toolCallResponse(name, args string) *llm.ChatResponse {
	response := taskDoneResponseWithArguments(args)
	response.Choices[0].Message.ToolCalls[0].Function.Name = name
	return response
}

func TestRunMainTaskRejectsUnadvertisedTools(t *testing.T) {
	for _, tc := range []struct {
		name, arguments string
		noTools         bool
	}{
		{"file_read", `{}`, false},
		{"file_read_diff", `{}`, false},
		{"external_lookup", `{}`, false},
		{"task_done", `{}`, false},
		{"task_done", `{"state":"FAILED"}`, false},
		{"code_comment", `{"comments":[{"content":"issue","existing_code":"x"}]}`, false},
		{"file_read", `{}`, true},
	} {
		t.Run(tc.name+tc.arguments, func(t *testing.T) {
			client := &fakeClient{responses: []*llm.ChatResponse{
				toolCallResponse(tc.name, tc.arguments), toolCallResponse("allowed_tool", `{}`),
			}}
			deps := newTestDeps(client)
			deps.Template.MaxToolRequestTimes = 2
			deps.Session = session.New(t.TempDir(), "main", "fake", session.SessionOptions{})
			deps.MainToolDefs = []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "allowed_tool"}}}
			if tc.noTools {
				deps.MainToolDefs = nil
			}
			rejected := &countingToolProvider{name: tc.name}
			allowed := &countingToolProvider{name: "allowed_tool"}
			deps.Tools.Register(rejected)
			deps.Tools.Register(allowed)
			runner := NewRunner(deps)
			completed, _, err := runner.RunMainTask(context.Background(), []llm.Message{llm.NewTextMessage("user", "review")}, "main.go")
			if err != nil || completed || len(client.requests) != 2 {
				t.Fatalf("completed=%v, err=%v, requests=%d", completed, err, len(client.requests))
			}
			messages := client.requests[1].Messages
			if got := messages[len(messages)-1].ExtractText(); got != tool.NotAvailableMsg {
				t.Fatalf("rejected tool result = %q", got)
			}
			if rejected.calls != 0 || len(runner.CollectPendingComments()) != 0 {
				t.Fatal("unadvertised tool executed or collected comments")
			}
			wantAllowed := 1
			if tc.noTools {
				wantAllowed = 0
			}
			if allowed.calls != wantAllowed {
				t.Fatalf("allowed tool calls = %d, want %d", allowed.calls, wantAllowed)
			}
		})
	}
}

func TestRunMainTaskMCPAndGraceAvailability(t *testing.T) {
	var mcpCalls atomic.Int64
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
	server.AddTool(&sdkmcp.Tool{Name: "external_lookup", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			mcpCalls.Add(1)
			return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "external result"}}}, nil
		})
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return server }, nil))
	defer httpServer.Close()
	mcpClient, err := ocrmcp.NewRemoteClient(context.Background(), "test", httpServer.URL, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer mcpClient.Close()
	for _, mode := range []string{"advertised", "unadvertised", "grace"} {
		t.Run(mode, func(t *testing.T) {
			mcpCalls.Store(0)
			client := &fakeClient{}
			deps := newTestDeps(client)
			deps.Session = session.New(t.TempDir(), "main", "fake", session.SessionOptions{})
			reader := &countingToolProvider{name: "file_read"}
			deps.Tools.Register(reader)
			deps.Tools.Register(&tool.CodeCommentProvider{Collector: deps.CommentCollector})
			ocrmcp.RegisterAll(deps.Tools, mcpClient, nil)
			if mode != "unadvertised" {
				deps.MainToolDefs = append(deps.MainToolDefs, ocrmcp.CollectToolDefs([]*ocrmcp.Client{mcpClient}, deps.Tools)...)
			}
			deps.MainToolDefs = append(deps.MainToolDefs, llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}})
			calls := toolCallResponse("external_lookup", `{}`)
			calls.Choices[0].Message.ToolCalls = append(calls.ToolCalls(), fileReadToolCallResponse("read", `{}`).ToolCalls()...)
			if mode == "grace" {
				deps.Template.MaxToolRequestTimes = 1
				calls.Choices[0].Message.ToolCalls = append(calls.ToolCalls(), codeCommentResponse("", "").ToolCalls()...)
				calls.Choices[0].Message.ToolCalls = append(calls.ToolCalls(), taskDoneResponse().ToolCalls()...)
				client.responses = []*llm.ChatResponse{{}, calls}
			} else {
				client.responses = []*llm.ChatResponse{calls, taskDoneResponse()}
			}
			runner := NewRunner(deps)
			completed, _, err := runner.RunMainTask(context.Background(), []llm.Message{llm.NewTextMessage("user", "review")}, "main.go")
			if err != nil || !completed || len(client.requests) != 2 {
				t.Fatalf("completed=%v, err=%v, requests=%d", completed, err, len(client.requests))
			}
			wantMCP, wantRead, wantComments := int64(0), 1, 0
			if mode == "advertised" {
				wantMCP = 1
			}
			if mode == "grace" {
				wantRead, wantComments = 0, 1
				for _, def := range client.requests[1].Tools {
					if def.Function.Name != "code_comment" && def.Function.Name != "task_done" {
						t.Fatalf("unexpected grace definition: %+v", def)
					}
				}
			} else {
				wantResult := "external result"
				if mode == "unadvertised" {
					wantResult = tool.NotAvailableMsg
				}
				messages := client.requests[1].Messages
				if got := messages[len(messages)-2].ExtractText(); got != wantResult {
					t.Fatalf("MCP result = %q, want %q", got, wantResult)
				}
			}
			if mcpCalls.Load() != wantMCP || reader.calls != wantRead || len(runner.CollectPendingComments()) != wantComments {
				t.Fatalf("MCP/read/comments = %d/%d/%d, want %d/%d/%d", mcpCalls.Load(), reader.calls, len(runner.CollectPendingComments()), wantMCP, wantRead, wantComments)
			}
		})
	}
}
