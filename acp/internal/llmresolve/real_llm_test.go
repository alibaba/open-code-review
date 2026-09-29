// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmresolve

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/intent"
)

// TestRealEndpointToolCall is opt-in because it calls a real, configured
// endpoint. The core regression never depends on it.
func TestRealEndpointToolCall(t *testing.T) {
	if os.Getenv("OCR_ACP_REAL_LLM") == "" {
		t.Skip("set OCR_ACP_REAL_LLM=1 to call the configured endpoint")
	}
	ep, err := Resolve(Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	c, err := NewClient(ep)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Log(ep.Summary())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	call, err := c.CallTool(ctx, intent.LLMRequest{
		System: "You convert one review request into exactly one submit_intent tool call.",
		User:   "Review my current changes.",
		Tool:   intent.SubmitIntentTool(),
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if call.Name != "submit_intent" {
		t.Fatalf("tool = %q", call.Name)
	}
}

// This optional semantic check sends only synthetic requests through the real
// parser prompt. It neither sends repository content nor starts an OCR review.
func TestRealLatestCommitIntent(t *testing.T) {
	if os.Getenv("OCR_ACP_REAL_LLM") == "" {
		t.Skip("set OCR_ACP_REAL_LLM=1 to call the configured endpoint")
	}
	ep, err := Resolve(Options{})
	if err != nil {
		t.Fatal("real parser configuration is unavailable")
	}
	client, err := NewClient(ep)
	if err != nil {
		t.Fatal("real parser client could not be constructed")
	}
	for _, input := range []string{
		"\u5ba1\u67e5\u4e00\u4e0b\u6700\u65b0\u63d0\u4ea4\u7684commit",
		"\u5ba1\u67e5\u4e00\u4e0b\u6700\u8fd1\u7684\u4e00\u6b21\u63d0\u4ea4",
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		result, err := intent.NewParser(client, nil).Parse(ctx, input, intent.NewState())
		cancel()
		if err != nil || result.Kind != intent.KindIntent || result.Review == nil || result.Review.Commit != "HEAD" {
			t.Fatalf("latest commit was not mapped to a HEAD review (kind=%v)", result.Kind)
		}
	}
}
