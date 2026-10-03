// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/alibaba/open-code-review/internal/chatgpt"
	"github.com/alibaba/open-code-review/internal/llm"
)

type fakeChatGPTClient struct {
	profiles []chatgpt.Profile
	models   []chatgpt.Model
	profile  chatgpt.Profile
	err      error
	loginErr error
	loginID  string
	selected string
}

func (f *fakeChatGPTClient) Profiles() ([]chatgpt.Profile, error)            { return f.profiles, f.err }
func (f *fakeChatGPTClient) Models(context.Context) ([]chatgpt.Model, error) { return f.models, f.err }
func (f *fakeChatGPTClient) Logout(context.Context) error                    { return f.err }
func (f *fakeChatGPTClient) Select(_ context.Context, id string) error       { f.selected = id; return f.err }
func (f *fakeChatGPTClient) Login(_ context.Context, id string, open func(string) error) (chatgpt.Profile, error) {
	f.loginID = id
	if err := open("https://example.test/authorize?state=test"); err != nil {
		return chatgpt.Profile{}, err
	}
	return f.profile, f.loginErr
}

func installFakeChatGPT(t *testing.T) *fakeChatGPTClient {
	t.Helper()
	f := &fakeChatGPTClient{profiles: []chatgpt.Profile{{ClientID: "saved", Active: true}}, profile: chatgpt.Profile{ClientID: "saved", Email: "person@example.test", PlanEnabled: true}, models: []chatgpt.Model{{Slug: "account-model", DisplayName: "Account model"}}}
	previous, previousBrowser := newChatGPTClient, openChatGPTBrowser
	newChatGPTClient = func() (chatGPTClient, error) { return f, nil }
	openChatGPTBrowser = func(string) error { return nil }
	t.Cleanup(func() { newChatGPTClient = previous; openChatGPTBrowser = previousBrowser })
	return f
}

func TestChatGPTLoginConfiguresProviderWithoutSecrets(t *testing.T) {
	freshOCRHome(t)
	f := installFakeChatGPT(t)
	if err := runChatGPTLogin(context.Background(), "", false, ""); err != nil {
		t.Fatal(err)
	}
	if f.loginID != "saved" {
		t.Fatal("active registration must be reused")
	}
	path, err := defaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loadOrCreateConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	entry := cfg.Providers[chatgpt.ProviderName]
	if cfg.Provider != chatgpt.ProviderName || entry.Model != "account-model" || entry.APIKey != "" || len(entry.Models) != 1 {
		t.Fatalf("invalid config: %+v", entry)
	}
	if err := runChatGPTLogin(context.Background(), "", true, "account-model"); err != nil {
		t.Fatal(err)
	}
	if f.loginID != "" {
		t.Fatal("new-account must start a fresh registration")
	}
	if err := runChatGPTLogin(context.Background(), "explicit", false, "account-model"); err != nil {
		t.Fatal(err)
	}
	if f.loginID != "explicit" {
		t.Fatal("selected registration was ignored")
	}
	if err := runChatGPTLogin(context.Background(), "", false, "missing-model"); err == nil {
		t.Fatal("unavailable model accepted")
	}
	f.profile.PlanEnabled = false
	if err := runChatGPTLogin(context.Background(), "", false, ""); err == nil {
		t.Fatal("identity-only grant accepted")
	}
	f.profile.PlanEnabled = true
	f.loginErr = errors.New("declined")
	if err := runChatGPTLogin(context.Background(), "", false, ""); err == nil {
		t.Fatal("login failure ignored")
	}
}

func TestChatGPTUtilityCommands(t *testing.T) {
	f := installFakeChatGPT(t)
	for _, name := range []string{"status", "models", "logout", "account"} {
		cmd, _, err := llmCmd.Find([]string{name})
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetContext(context.Background())
		t.Cleanup(func() { cmd.SetOut(nil) })
		args := []string{chatgpt.ProviderName}
		if name == "account" {
			args = []string{"saved"}
		}
		if err := cmd.Args(cmd, args); err != nil {
			t.Fatal(err)
		}
		if err := cmd.RunE(cmd, args); err != nil {
			t.Fatal(err)
		}
		if output.Len() == 0 {
			t.Fatal("command did not display its result")
		}
		if name != "account" && cmd.Args(cmd, []string{"another-provider"}) == nil {
			t.Fatal("unsupported provider accepted")
		}
		f.err = errors.New("disconnected")
		if err := cmd.RunE(cmd, args); err == nil {
			t.Fatal("client failure ignored")
		}
		f.err = nil
	}
	if f.selected != "saved" {
		t.Fatal("account selection ignored")
	}
	f.profiles = nil
	cmd, _, _ := llmCmd.Find([]string{"status"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	defer cmd.SetOut(nil)
	if err := cmd.RunE(cmd, []string{chatgpt.ProviderName}); err != nil || !strings.Contains(out.String(), "not connected") {
		t.Fatalf("empty status: %s %v", out.String(), err)
	}
}

func TestChatGPTModelSelectionAndKeyRequirement(t *testing.T) {
	if _, err := chatGPTModel(nil, "", ""); err == nil {
		t.Fatal("empty catalog accepted")
	}
	catalog := []chatgpt.Model{{Slug: "first"}, {Slug: "configured"}}
	if got, err := chatGPTModel(catalog, "", "configured"); err != nil || got != "configured" {
		t.Fatalf("re-login must keep the configured model: %q %v", got, err)
	}
	if got, err := chatGPTModel(catalog, "", "withdrawn"); err != nil || got != "first" {
		t.Fatalf("a withdrawn model must fall back to the first listed: %q %v", got, err)
	}
	p, _ := llm.LookupProvider(chatgpt.ProviderName)
	if err := checkAPIKeyRequirement(p.Name, "", "", p, true); err != nil {
		t.Fatal(err)
	}
	m := newProviderTUI(&Config{Providers: map[string]ProviderEntry{chatgpt.ProviderName: {Model: "account-model", Models: []string{"account-model"}}}}, "")
	for i, p := range m.providers {
		if p.Name == chatgpt.ProviderName {
			m.officialIdx = i
		}
	}
	m.activeTab = tabOfficial
	m.step = stepModel
	m.prepareModelSelection(chatgpt.ProviderName, "account-model")
	result, _ := m.handleEnter()
	if !result.(providerTUIModel).confirmed {
		t.Fatal("OAuth provider prompted for an API key")
	}
	m = newProviderTUI(&Config{}, "")
	for i, p := range m.providers {
		if p.OAuth {
			m.officialIdx = i
		}
	}
	m.activeTab = tabOfficial
	result, _ = m.handleEnter()
	if !result.(providerTUIModel).confirmed || result.(providerTUIModel).result().model != "" {
		t.Fatal("first-time OAuth selection must launch login")
	}
}

func TestOAuthSelectionDropsPreviousProviderModel(t *testing.T) {
	m := newProviderTUI(&Config{Provider: "openai", Providers: map[string]ProviderEntry{"openai": {Model: "my-custom-model"}}}, "")
	m.activeTab = tabOfficial
	selectOfficial := func(name string) {
		for i, p := range m.providers {
			if p.Name == name {
				m.officialIdx = i
			}
		}
	}
	selectOfficial("openai")
	next, _ := m.handleEnter()
	m = next.(providerTUIModel)
	m.modelIdx = 0
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(providerTUIModel)
	selectOfficial(chatgpt.ProviderName)
	next, _ = m.handleEnter()
	if got := next.(providerTUIModel).result().model; got != "" {
		t.Fatalf("OAuth selection must launch login, got stale model %q", got)
	}
}
