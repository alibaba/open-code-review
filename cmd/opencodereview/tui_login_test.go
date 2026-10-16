// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTUILoginProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executable, repo := filepath.Join(t.TempDir(), "ocr.exe"), t.TempDir()
	cmd := tuiLoginProcess(ctx, executable, repo)
	want := []string{executable, "auth", "login", "--provider", "chatgpt"}
	if !reflect.DeepEqual(cmd.Args, want) || cmd.Dir != repo {
		t.Fatalf("login command = %v in %q", cmd.Args, cmd.Dir)
	}
	if err := cmd.Run(); !errors.Is(err, context.Canceled) {
		t.Fatalf("login lost caller cancellation: %v", err)
	}
}

func TestTUILoginResult(t *testing.T) {
	store := commandChatGPTAccount(t)
	if result := tuiLoginResult(nil); result.err != nil || !result.planEnabled {
		t.Fatalf("plan-enabled result = %+v", result)
	}
	if err := store.Select(context.Background(), "oaiapp_two"); err != nil {
		t.Fatal(err)
	}
	if result := tuiLoginResult(nil); result.err != nil || result.planEnabled {
		t.Fatalf("plan-disabled result = %+v", result)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "accounts.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if result := tuiLoginResult(context.Canceled); !errors.Is(result.err, context.Canceled) {
		t.Fatalf("cancellation replaced by store error: %v", result.err)
	}
	if result := tuiLoginResult(nil); result.err == nil {
		t.Fatal("unreadable authentication status reported success")
	}
}

func TestTUILoginPreservesSelectionAndReportsStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result tuiLoginDoneMsg
		want   string
	}{
		{"enabled", tuiLoginDoneMsg{planEnabled: true}, "/provider to configure ChatGPT"},
		{"disabled", tuiLoginDoneMsg{}, "plan usage is disabled"},
		{"canceled", tuiLoginDoneMsg{err: context.Canceled}, "context canceled"},
		{"failed", tuiLoginDoneMsg{err: errors.New("authorization declined")}, "authorization declined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testReviewTUI()
			m.opts.provider, m.opts.model = "selected-provider", "selected-model"
			m.opts.from, m.opts.to, m.opts.resume = "main", "feature", "saved-session"
			m.opts.background = "Review the selected change"
			m.screen = tuiResults
			before := m.opts
			calls := 0
			m.services.login = func() tea.Cmd {
				calls++
				return func() tea.Msg { return tc.result }
			}
			next, cmd := m.command("/login")
			m = next.(reviewTUIModel)
			if calls != 1 || !m.busy || cmd == nil {
				t.Fatal("login command did not suspend the workspace")
			}
			next, _ = m.Update(cmd())
			m = next.(reviewTUIModel)
			if m.busy || m.screen != tuiResults || !reflect.DeepEqual(m.opts, before) {
				t.Fatal("login return changed review options or the current view")
			}
			if m.isError != (tc.result.err != nil) || !strings.Contains(m.notice, tc.want) {
				t.Fatalf("login status = %q, error = %v", m.notice, m.isError)
			}
		})
	}
}

func TestTUILoginPaletteAndArguments(t *testing.T) {
	m := testReviewTUI()
	m.input.SetValue("/login")
	choices := m.commandChoices()
	if len(choices) != 1 || choices[0].name != "/login" || !choices[0].immediate {
		t.Fatalf("login palette entry = %v", choices)
	}
	m.services.login = func() tea.Cmd {
		t.Fatal("invalid login arguments started sign-in")
		return nil
	}
	next, cmd := m.command("/login --enable-plan")
	m = next.(reviewTUIModel)
	if cmd != nil || m.busy || !m.isError {
		t.Fatal("login accepted unsupported arguments")
	}
	m.busy = true
	if _, cmd := m.command("/login"); cmd != nil {
		t.Fatal("login interrupted an active review")
	}
}
