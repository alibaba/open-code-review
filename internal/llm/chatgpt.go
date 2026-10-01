// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
)

var chatGPTCredentials = func(ctx context.Context, account string) (*chatgptauth.Auth, error) {
	s, err := chatgptauth.DefaultStore()
	if err != nil {
		return nil, err
	}
	return chatgptauth.NewClient().Credentials(ctx, s, account)
}
var chatGPTModels = func(ctx context.Context, auth *chatgptauth.Auth) ([]chatgptauth.Model, error) {
	return chatgptauth.NewClient().Models(ctx, auth)
}

func ChatGPTModels(ctx context.Context) ([]chatgptauth.Model, error) {
	auth, err := chatGPTCredentials(ctx, "")
	if err != nil {
		return nil, err
	}
	return chatGPTModels(ctx, auth)
}
func resolveChatGPT(cfg configFile, entry providerEntryConfig, override string, ctx context.Context) (ResolvedEndpoint, bool, error) {
	if entry.URL != "" || entry.Protocol != "" || entry.APIKey != "" || entry.APIKeyCmd != "" || entry.AuthHeader != "" {
		return ResolvedEndpoint{}, false, fmt.Errorf("chatgpt provider uses only its protected SIWC credential store and public Responses endpoint; url, protocol and credential overrides are not allowed")
	}
	if err := validateChatGPTBody(entry.ExtraBody); err != nil {
		return ResolvedEndpoint{}, false, err
	}
	for k := range entry.ExtraHeaders {
		if reservedHeaders[strings.ToLower(k)] || strings.EqualFold(k, "Host") {
			return ResolvedEndpoint{}, false, fmt.Errorf("chatgpt extra_headers cannot override %s", k)
		}
	}
	timeout, err := ValidateTimeoutSec(entry.TimeoutSec)
	if err != nil {
		return ResolvedEndpoint{}, false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	auth, err := chatGPTCredentials(ctx, "")
	if err != nil {
		return ResolvedEndpoint{}, false, err
	}
	model := cfg.Model
	if entry.Model != "" {
		model = entry.Model
	}
	if override != "" {
		model = override
	}
	if model == "" {
		models, err := chatGPTModels(ctx, auth)
		if err != nil {
			return ResolvedEndpoint{}, false, err
		}
		if len(models) == 0 {
			return ResolvedEndpoint{}, false, fmt.Errorf("selected ChatGPT account catalog is empty; configure an explicit model or run ocr llm models")
		}
		model = models[0].Slug
	}
	return ResolvedEndpoint{URL: chatgptauth.Resource, Token: auth.AccessToken, Provider: "chatgpt", Source: "provider:chatgpt (Using ChatGPT plan; Manage usage: " + chatgptauth.UsageURL + ")", Protocol: ProtocolOpenAIResponses, Model: model, Timeout: timeout, ExtraBody: entry.ExtraBody, ExtraHeaders: entry.ExtraHeaders, RequiresStreaming: true, RejectsSamplingParams: true, ChatGPTPlan: true, ChatGPTAccount: auth.ClientID}, true, nil
}
func validateChatGPTBody(body map[string]any) error {
	for k := range body {
		switch k {
		case "reasoning", "text", "prompt_cache_key":
		default:
			return fmt.Errorf("chatgpt preview does not accept extra_body.%s; supported overrides: reasoning, text, prompt_cache_key", k)
		}
	}
	return nil
}
