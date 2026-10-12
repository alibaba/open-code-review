// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"fmt"
	"strings"
)

// chatGPTToolNamespace groups the review tools when they are sent over the
// ChatGPT plan route; replayed function calls must carry the same namespace.
const chatGPTToolNamespace = "ocr"

func checkChatGPTExtraBody(body map[string]any) error {
	for k := range body {
		if k != "reasoning" && k != "text" && k != "prompt_cache_key" {
			return fmt.Errorf("ChatGPT plan usage does not accept extra_body.%s", k)
		}
	}
	return nil
}

func resolveChatGPTProvider(cfg configFile, override string, preset Provider) (ResolvedEndpoint, bool, error) {
	entry := cfg.Providers[preset.Name]
	if strings.TrimSpace(entry.APIKey) != "" || strings.TrimSpace(entry.APIKeyCmd) != "" || entry.URL != "" || entry.Protocol != "" || entry.AuthHeader != "" || len(entry.ExtraHeaders) != 0 {
		return ResolvedEndpoint{}, false, fmt.Errorf("provider %q uses managed OAuth and the public Responses endpoint; remove API key, URL, protocol and header overrides", preset.Name)
	}
	model := cfg.Model
	if entry.Model != "" {
		model = entry.Model
	}
	if override != "" {
		model = override
	}
	if model == "" {
		return ResolvedEndpoint{}, false, fmt.Errorf("provider %q has no model configured; run 'ocr llm login openai-chatgpt'", preset.Name)
	}
	timeout, err := ValidateTimeoutSec(entry.TimeoutSec)
	if err != nil {
		return ResolvedEndpoint{}, false, err
	}
	if err := checkChatGPTExtraBody(entry.ExtraBody); err != nil {
		return ResolvedEndpoint{}, false, err
	}
	return ResolvedEndpoint{URL: preset.BaseURL, Model: model, Provider: preset.Name, Protocol: preset.Protocol,
		ChatGPT: true, Source: "provider:" + preset.Name, Timeout: timeout, ExtraBody: entry.ExtraBody}, true, nil
}
