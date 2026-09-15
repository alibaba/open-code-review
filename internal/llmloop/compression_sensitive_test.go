// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestCompressionRedactsSensitiveTurnsBeforeBuildingXML(t *testing.T) {
	const secret = "compression-secret-sentinel"
	const name = "mcp__fixture__probe"
	summary := "safe summary"
	client := &scriptedLLMClient{responses: []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}}}
	r, messages := newCompressionRunner(t, client, nil)
	registry := tool.NewRegistry()
	registry.Register(&sensitiveArgsProvider{argsCapturingProvider: argsCapturingProvider{tool: tool.Dynamic(name)}})
	registry.Freeze()
	r.deps.Tools = registry
	r.deps.MainToolDefs = []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: name}}}
	calls := []llm.ToolCall{{ID: "sensitive", Function: llm.FunctionCall{Name: name, Arguments: `{"token":"` + secret + `"}`}}}
	messages[2] = llm.NewToolCallMessage(secret, calls, llm.NativeTurn{Family: "anthropic", Payload: map[string]any{"input": secret}}, secret)
	messages[3] = llm.NewTextMessage("tool", "safe tool result")

	if _, err := r.runCompression(context.Background(), messages, "test.go"); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("compression requests = %d, want 1", len(client.requests))
	}
	request, err := json.Marshal(client.requests[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(request), secret) {
		t.Fatal("sensitive arguments, reasoning or native replay leaked into compression input")
	}
	if !strings.Contains(string(request), "[sensitive tool turn]") || !strings.Contains(string(request), "safe tool result") {
		t.Fatal("compression input lost its safe tool context")
	}
	if messages[2].Native.Payload == nil || messages[2].ExtractText() != secret || !strings.Contains(calls[0].Function.Arguments, secret) {
		t.Fatal("compression modified live replay state")
	}
	if err := r.deps.Session.Finalize(); err != nil {
		t.Fatal(err)
	}
}
