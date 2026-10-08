// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func TestChatGPTModelSelectionContract(t *testing.T) {
	oldCredentials, oldModels := chatGPTCredentials, chatGPTModels
	defer func() { chatGPTCredentials = oldCredentials; chatGPTModels = oldModels }()
	chatGPTCredentials = func(context.Context, string) (*chatgptauth.Auth, error) {
		return &chatgptauth.Auth{ClientID: "account", AccessToken: "fixture"}, nil
	}
	var calls int
	chatGPTModels = func(context.Context, *chatgptauth.Auth) ([]chatgptauth.Model, error) {
		calls++
		return []chatgptauth.Model{{Slug: "server-second"}, {Slug: "server-first"}}, nil
	}
	for _, tc := range []struct {
		name, global, entry, override, want string
		catalog                             bool
	}{
		{name: "default-order", want: "server-second", catalog: true},
		{name: "global-explicit", global: "global-unlisted", want: "global-unlisted"},
		{name: "provider-explicit", global: "global", entry: "gpt-6-luna", want: "gpt-6-luna"},
		{name: "override-explicit", global: "global", entry: "entry", override: "user-slug", want: "user-slug"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls = 0
			ep, ok, err := resolveChatGPT(configFile{Model: tc.global}, providerEntryConfig{Model: tc.entry}, tc.override, context.Background())
			wantCalls := 0
			if tc.catalog {
				wantCalls = 1
			}
			if err != nil || !ok || ep.Model != tc.want || calls != wantCalls {
				t.Fatal(ep, calls, err)
			}
		})
	}
	chatGPTModels = func(context.Context, *chatgptauth.Auth) ([]chatgptauth.Model, error) { return nil, nil }
	if _, _, err := resolveChatGPT(configFile{}, providerEntryConfig{}, "", context.Background()); err == nil {
		t.Fatal("empty default catalog accepted")
	}
	chatGPTModels = func(context.Context, *chatgptauth.Auth) ([]chatgptauth.Model, error) {
		return nil, errors.New("catalog denied")
	}
	if _, _, err := resolveChatGPT(configFile{}, providerEntryConfig{}, "", context.Background()); err == nil || !strings.Contains(err.Error(), "catalog denied") {
		t.Fatal(err)
	}
	if _, err := ChatGPTModels(context.Background()); err == nil {
		t.Fatal("list command bypassed live catalog")
	}
}

func TestChatGPTExplicitModelServerRejection(t *testing.T) {
	oldCredentials, oldModels := chatGPTCredentials, chatGPTModels
	defer func() { chatGPTCredentials = oldCredentials; chatGPTModels = oldModels }()
	chatGPTCredentials = func(context.Context, string) (*chatgptauth.Auth, error) {
		return &chatgptauth.Auth{ClientID: "account", AccessToken: "fixture"}, nil
	}
	chatGPTModels = func(context.Context, *chatgptauth.Auth) ([]chatgptauth.Model, error) {
		t.Fatal("explicit resolution queried catalog")
		return nil, nil
	}
	ep, _, err := resolveChatGPT(configFile{}, providerEntryConfig{Model: "server-rejected"}, "", context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c := NewOpenAIResponsesClient(ClientConfig{URL: ep.URL, ChatGPTPlan: true, RequiresStreaming: true})
	c.chatGPTToken = func(context.Context) (string, error) { return "fixture", nil }
	var calls int
	c.sdk = openai.NewClient(option.WithBaseURL(ep.URL), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: resolverRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"server-rejected"`) {
			t.Error("changed user model", string(body))
		}
		return &http.Response{StatusCode: 403, Header: http.Header{"Content-Type": {"application/json"}, "X-Request-Id": {"req-model"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"subscription_sharing_model_not_eligible","param":"model","message":"model not eligible"}}`))}, nil
	})}))
	_, err = c.CompletionsWithCtx(context.Background(), ChatRequest{Model: ep.Model, Messages: []Message{NewTextMessage("user", "hello")}})
	var api *openai.Error
	if !errors.As(err, &api) || api.StatusCode != 403 || api.Code != "subscription_sharing_model_not_eligible" || api.Param != "model" || api.Response.Header.Get("x-request-id") != "req-model" || calls != 1 {
		t.Fatal("server rejection lost or retried", calls, err)
	}
}
