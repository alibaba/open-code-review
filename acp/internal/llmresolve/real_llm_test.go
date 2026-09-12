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
