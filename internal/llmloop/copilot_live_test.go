// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestCopilotSignedInHistoryModes(t *testing.T) {
	if os.Getenv("OCR_COPILOT_LIVE_TEST") != "1" {
		t.Skip("set OCR_COPILOT_LIVE_TEST=1 to use signed-in Copilot allowance")
	}
	const patch = `arith.go: Add(a,b) must add. Before: func Add(a,b int) int { return a+b }. After: func Add(a,b int) int { return a-b }.
bounds.go: First(xs) must reject empty slices. Before: func First(xs []int)(int,bool) { if len(xs)==0 {return 0,false}; return xs[0],true }. After: func First(xs []int)(int,bool) { if len(xs)>0 {return 0,false}; return xs[0],true }.
copy.go: Clone(xs) must copy without modifying xs. Before: func Clone(xs []int)[]int {out:=make([]int,len(xs)); copy(out,xs);return out}. After: func Clone(xs []int)[]int {out:=make([]int,len(xs)); copy(xs,out);return out}.
guard.go: IsAdmin(role) must allow only admin. Before: func IsAdmin(role string)bool{return role=="admin"}. After: func IsAdmin(role string)bool{return role!="admin"}.
prices.go: Discount(total,percent) must return the remaining amount. Before: func Discount(total,percent int)int{return total*(100-percent)/100}. After: func Discount(total,percent int)int{return total*percent/100}.
queue.go: Pop(q) is FIFO for nonempty queues. Before: func Pop(q []int)(int,[]int){return q[0],q[1:]}. After: func Pop(q []int)(int,[]int){return q[len(q)-1],q[1:]}.`
	paths := []string{"arith.go", "bounds.go", "copy.go", "guard.go", "prices.go", "queue.go"}
	read := llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: "inspect_patch", Description: "Get the six-file patch", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}}
	done := llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "Submit all confirmed findings and finish", Parameters: map[string]any{
		"type": "object", "required": []string{"findings"}, "properties": map[string]any{"findings": map[string]any{"type": "array", "items": map[string]any{
			"type": "object", "required": []string{"path", "issue"}, "properties": map[string]any{"path": map[string]any{"type": "string"}, "issue": map[string]any{"type": "string"}},
		}}},
	}}}
	for _, mode := range []string{"native", "replay", "compression", "grace"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			client := llm.NewCopilotClient(llm.ClientConfig{Model: "auto", Timeout: time.Minute})
			defer client.CloseSession(mode)
			messages := []llm.Message{llm.NewTextMessage("system", "Review changes using only the supplied tools. First call inspect_patch and wait for its result. Then submit one task_done containing each confirmed bug's path, trigger and consequence. Do not finish before inspecting the patch."), llm.NewTextMessage("user", "Inspect the six-file patch and report the defects.")}
			tools := []llm.ToolDef{read, done}
			first, err := client.CompletionsWithCtx(ctx, llm.ChatRequest{SessionID: mode, Messages: messages, Tools: tools, MaxTokens: 4096})
			if err != nil {
				t.Fatal(err)
			}
			calls := first.ToolCalls()
			if len(calls) != 1 || calls[0].Function.Name != "inspect_patch" {
				t.Fatalf("first response did not request the patch: %#v", calls)
			}
			messages = append(messages, llm.NewToolCallMessage(first.VisibleContent(), calls, first.Native(), first.ReasoningContent()), llm.NewToolResultMessage(calls[0].ID, patch))
			if mode == "replay" {
				client.CloseSession(mode)
			}
			if mode == "compression" {
				runner := NewRunner(Deps{LLMClient: client, Model: "auto", Session: session.New(t.TempDir(), "main", "auto", session.SessionOptions{})}, Task{
					MaxContextTokens: 4096, MaxCompletionTokens: 4096, Compression: CompressionPolicy{
						Messages: []MessageTemplate{{Role: "user", Content: "Summarize this code-review transcript. Preserve every filename, changed expression, required behavior and outstanding work. Do not discard defects. Transcript:\n{{context}}"}}, SummaryTemplate: "\nReview memory:\n{{summary}}",
					},
				})
				messages, err = runner.runCompression(ctx, messages, "six-files")
				if err != nil {
					t.Fatal(err)
				}
				if len(messages) != 2 {
					t.Fatalf("compression did not replace the tool history: %d messages", len(messages))
				}
				t.Logf("compressed memory=%s", messages[1].ExtractText())
				for _, path := range paths {
					if !strings.Contains(messages[1].ExtractText(), path) {
						t.Fatalf("compression dropped %s", path)
					}
				}
			}
			if mode == "grace" {
				tools = []llm.ToolDef{done}
			}
			response, err := client.CompletionsWithCtx(ctx, llm.ChatRequest{SessionID: mode, Messages: messages, Tools: tools, MaxTokens: 4096})
			if err != nil {
				t.Fatal(err)
			}
			calls = response.ToolCalls()
			if len(calls) != 1 || calls[0].Function.Name != "task_done" {
				t.Fatalf("review did not finish after reading the patch: %#v", calls)
			}
			var report struct {
				Findings []struct{ Path, Issue string } `json:"findings"`
			}
			if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &report); err != nil {
				t.Fatal(err)
			}
			seen := make(map[string]bool)
			for _, finding := range report.Findings {
				if finding.Issue == "" {
					t.Fatalf("empty finding: %+v", finding)
				}
				seen[finding.Path] = true
			}
			for _, path := range paths {
				if !seen[path] {
					t.Fatalf("review missed %s: %s", path, calls[0].Function.Arguments)
				}
			}
			t.Logf("findings=%s", calls[0].Function.Arguments)
		})
	}
}
