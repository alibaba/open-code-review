// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import "testing"

func TestChatGPTOnlySubscriptionPreset(t *testing.T) {
	if _, ok := LookupProvider("codex"); ok {
		t.Fatal("undocumented Codex preset must not ship")
	}
	p, ok := LookupProvider("chatgpt")
	if !ok {
		t.Fatal("chatgpt preset not found")
	}
	if p.Protocol != ProtocolOpenAIResponses || p.BaseURL != "https://api.openai.com/v1" || p.EnvVar != "" || !p.ExternalAuth || !p.RequiresStreaming || !p.RejectsSamplingParams || len(p.Models) != 0 {
		t.Fatalf("unexpected chatgpt preset: %+v", p)
	}
}
