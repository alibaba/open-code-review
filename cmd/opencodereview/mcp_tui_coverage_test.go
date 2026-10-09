// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
)

type mcpTUICoverageErrorReader struct {
	err error
}

func (r mcpTUICoverageErrorReader) Read([]byte) (int, error) {
	return 0, r.err
}

type mcpTUICoverageControlJSON struct{}

func (mcpTUICoverageControlJSON) MarshalJSON() ([]byte, error) {
	return []byte{34, 'a', 0xc2, 0x85, 'b', 34}, nil
}

func TestMCPManagementTextModelStateCoverage(t *testing.T) {
	model := newMCPManagementTextModel("Server name", "fallback")
	if command := model.Init(); command != nil {
		t.Fatalf("Init() command = %v, want nil", command)
	}
	if model.input.EchoMode == textinput.EchoPassword {
		t.Fatal("ordinary management input unexpectedly masks its value")
	}
	for _, label := range []string{"Environment entries", "HTTP Header values"} {
		secretModel := newMCPManagementTextModel(label, "")
		if secretModel.input.EchoMode != textinput.EchoPassword || secretModel.input.EchoCharacter != '*' {
			t.Errorf("%q input is not password-masked", label)
		}
	}

	view := model.View().Content
	for _, want := range []string{"MCP setup", "Server name", "Enter confirms", "Esc"} {
		if !strings.Contains(view, want) {
			t.Errorf("text view %q does not contain %q", view, want)
		}
	}

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = updated.(mcpManagementTextModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	model = updated.(mcpManagementTextModel)
	if got := model.input.Value(); got != "x" {
		t.Fatalf("text input value = %q, want x", got)
	}

	model.input.SetValue("  submitted value  ")
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	submitted := updated.(mcpManagementTextModel)
	if command == nil || !submitted.submitted || submitted.cancelled || submitted.value != "submitted value" {
		t.Fatalf("submitted text model = %+v, command nil = %v", submitted, command == nil)
	}

	model = newMCPManagementTextModel("Server name", "fallback")
	model.input.SetValue("   ")
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	fallback := updated.(mcpManagementTextModel)
	if command == nil || !fallback.submitted || fallback.value != "fallback" {
		t.Fatalf("fallback text model = %+v, command nil = %v", fallback, command == nil)
	}

	model = newMCPManagementTextModel("Server name", "fallback")
	model.input.SetValue("  CaNcEl  ")
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cancelWord := updated.(mcpManagementTextModel)
	if command == nil || !cancelWord.cancelled || cancelWord.submitted {
		t.Fatalf("cancel-word text model = %+v, command nil = %v", cancelWord, command == nil)
	}

	for _, test := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{name: "escape", key: tea.KeyPressMsg{Code: tea.KeyEscape}},
		{name: "control-c", key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
		{name: "control-d", key: tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}},
	} {
		t.Run(test.name, func(t *testing.T) {
			updated, command := newMCPManagementTextModel("Server", "").Update(test.key)
			got := updated.(mcpManagementTextModel)
			if command == nil || !got.cancelled || got.submitted {
				t.Fatalf("cancel result = %+v, command nil = %v", got, command == nil)
			}
		})
	}
}

func TestMCPManagementConfirmModelStateCoverage(t *testing.T) {
	model := newMCPManagementConfirmModel("Connect to the server?")
	if command := model.Init(); command != nil {
		t.Fatalf("Init() command = %v, want nil", command)
	}
	if model.selected != 0 {
		t.Fatalf("default selection = %d, want No", model.selected)
	}
	view := model.View().Content
	for _, want := range []string{"MCP confirmation", "Connect to the server?", "No", "Yes", "Default: No"} {
		if !strings.Contains(view, want) {
			t.Errorf("confirmation view %q does not contain %q", view, want)
		}
	}

	updated, command := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if command != nil || updated.(mcpManagementConfirmModel).selected != 0 {
		t.Fatal("non-key message changed the confirmation model")
	}

	for _, key := range []tea.KeyPressMsg{
		{Code: tea.KeyRight},
		{Code: 'l', Text: "l"},
		{Code: tea.KeyDown},
		{Code: 'j', Text: "j"},
	} {
		updated, _ = model.Update(key)
		if got := updated.(mcpManagementConfirmModel).selected; got != 1 {
			t.Errorf("right/down key %q selected %d, want Yes", key.String(), got)
		}
	}
	model.selected = 1
	for _, key := range []tea.KeyPressMsg{
		{Code: tea.KeyLeft},
		{Code: 'h', Text: "h"},
		{Code: tea.KeyUp},
		{Code: 'k', Text: "k"},
	} {
		updated, _ = model.Update(key)
		if got := updated.(mcpManagementConfirmModel).selected; got != 0 {
			t.Errorf("left/up key %q selected %d, want No", key.String(), got)
		}
	}

	for _, test := range []struct {
		name     string
		key      tea.KeyPressMsg
		selected int
	}{
		{name: "lower-y", key: tea.KeyPressMsg{Code: 'y', Text: "y"}, selected: 1},
		{name: "upper-y", key: tea.KeyPressMsg{Code: 'Y', Text: "Y"}, selected: 1},
		{name: "lower-n", key: tea.KeyPressMsg{Code: 'n', Text: "n"}, selected: 0},
		{name: "upper-n", key: tea.KeyPressMsg{Code: 'N', Text: "N"}, selected: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			updated, command := newMCPManagementConfirmModel("Confirm?").Update(test.key)
			got := updated.(mcpManagementConfirmModel)
			if command == nil || !got.submitted || got.cancelled || got.selected != test.selected {
				t.Fatalf("confirmation result = %+v, command nil = %v", got, command == nil)
			}
		})
	}

	model = newMCPManagementConfirmModel("Confirm?")
	model.selected = 1
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	entered := updated.(mcpManagementConfirmModel)
	if command == nil || !entered.submitted || entered.cancelled || entered.selected != 1 {
		t.Fatalf("Enter result = %+v, command nil = %v", entered, command == nil)
	}

	for _, test := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{name: "escape", key: tea.KeyPressMsg{Code: tea.KeyEscape}},
		{name: "control-c", key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
		{name: "control-d", key: tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}},
		{name: "q", key: tea.KeyPressMsg{Code: 'q', Text: "q"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			updated, command := newMCPManagementConfirmModel("Confirm?").Update(test.key)
			got := updated.(mcpManagementConfirmModel)
			if command == nil || !got.cancelled || got.submitted {
				t.Fatalf("cancel result = %+v, command nil = %v", got, command == nil)
			}
		})
	}
}

func TestMCPManagementBubbleTeaRunnersAndBoundaryCoverage(t *testing.T) {
	t.Run("text submitted", func(t *testing.T) {
		var output bytes.Buffer
		value, err := runMCPManagementTextPrompt(nil, "Server", "fallback", strings.NewReader("  chosen  \r"), &output)
		if err != nil || value != "chosen" {
			t.Fatalf("text prompt = %q, %v", value, err)
		}
		if output.Len() == 0 {
			t.Fatal("text prompt produced no terminal output")
		}
	})

	t.Run("text fallback", func(t *testing.T) {
		value, err := runMCPManagementTextPrompt(context.Background(), "Server", "fallback", strings.NewReader("\r"), io.Discard)
		if err != nil || value != "fallback" {
			t.Fatalf("fallback prompt = %q, %v", value, err)
		}
	})

	t.Run("text cancel word", func(t *testing.T) {
		value, err := runMCPManagementTextPrompt(context.Background(), "Server", "fallback", strings.NewReader("cancel\r"), io.Discard)
		if value != "" || !errors.Is(err, errMCPPromptCancelled) {
			t.Fatalf("cancel prompt = %q, %v", value, err)
		}
	})

	t.Run("confirm yes", func(t *testing.T) {
		var output bytes.Buffer
		confirmed, err := runMCPManagementConfirmPrompt(nil, "Connect?", strings.NewReader("y"), &output)
		if err != nil || !confirmed {
			t.Fatalf("yes confirmation = %v, %v", confirmed, err)
		}
		if output.Len() == 0 {
			t.Fatal("confirmation prompt produced no terminal output")
		}
	})

	t.Run("confirm default no", func(t *testing.T) {
		confirmed, err := runMCPManagementConfirmPrompt(context.Background(), "Connect?", strings.NewReader("\r"), io.Discard)
		if err != nil || confirmed {
			t.Fatalf("default confirmation = %v, %v", confirmed, err)
		}
	})

	t.Run("confirm cancellation", func(t *testing.T) {
		confirmed, err := runMCPManagementConfirmPrompt(context.Background(), "Connect?", strings.NewReader("q"), io.Discard)
		if err != nil || confirmed {
			t.Fatalf("cancelled confirmation = %v, %v", confirmed, err)
		}
	})

	t.Run("input termination fails closed", func(t *testing.T) {
		readErr := errors.New("terminal read failed")
		if _, err := runMCPManagementTextPrompt(context.Background(), "Server", "", mcpTUICoverageErrorReader{err: readErr}, io.Discard); !errors.Is(err, errMCPPromptCancelled) {
			t.Fatalf("text input error = %v", err)
		}
		if confirmed, err := runMCPManagementConfirmPrompt(context.Background(), "Connect?", mcpTUICoverageErrorReader{err: readErr}, io.Discard); err != nil || confirmed {
			t.Fatalf("confirmation input error = %v, confirmed = %v", err, confirmed)
		}
	})

	t.Run("pre-cancelled contexts", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := runMCPManagementTextPrompt(ctx, "Server", "", strings.NewReader(""), io.Discard); !errors.Is(err, context.Canceled) {
			t.Fatalf("text prompt error = %v, want context.Canceled", err)
		}
		if _, err := runMCPManagementConfirmPrompt(ctx, "Connect?", strings.NewReader(""), io.Discard); !errors.Is(err, context.Canceled) {
			t.Fatalf("confirmation error = %v, want context.Canceled", err)
		}
	})

	file, err := os.CreateTemp(t.TempDir(), "mcp-tui-io-*")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if (*mcpPrompter)(nil).usesBubbleTea() {
		t.Fatal("nil prompter reported Bubble Tea support")
	}
	if (&mcpPrompter{in: strings.NewReader(""), out: io.Discard}).usesBubbleTea() {
		t.Fatal("reader/writer fallback reported Bubble Tea support")
	}
	if (&mcpPrompter{in: file, out: io.Discard}).usesBubbleTea() {
		t.Fatal("mixed file/writer prompt reported Bubble Tea support")
	}
	if !(&mcpPrompter{in: file, out: file}).usesBubbleTea() {
		t.Fatal("file-backed terminal prompt did not report Bubble Tea support")
	}
}

func TestMCPApprovalModelStateAndViewCoverage(t *testing.T) {
	invocation := ocrmcp.Invocation{
		Grant: ocrmcp.ToolGrant{
			ID:                   ocrmcp.ToolID{Server: "server\x00 name", Name: "tool\nname"},
			ModelAlias:           "alias\x1b[31m",
			DefinitionSHA256:     strings.Repeat("a", 64),
			UntrustedDescription: "  hostile\n description\x1b[31m  ",
		},
		Arguments: map[string]any{"query": "visible", "token": "token-canary"},
	}
	model := newMCPApprovalModel(invocation)
	if command := model.Init(); command != nil {
		t.Fatalf("Init() command = %v, want nil", command)
	}
	if model.selected != 2 {
		t.Fatalf("default selection = %d, want Deny once", model.selected)
	}
	view := model.View().Content
	for _, want := range []string{
		"MCP tool approval required", "server name", "tool name", "alias [31m",
		"Untrusted server description", "hostile description [31m", "aaaaaaaaaaaa...",
		"visible", "[redacted]", "Allow once", "Allow this review", "Deny once", "Deny this review",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("approval view %q does not contain %q", view, want)
		}
	}
	if strings.Contains(view, "token-canary") || strings.ContainsRune(view, '\x00') || strings.ContainsRune(view, '\n') && strings.Contains(view, "tool\nname") {
		t.Errorf("approval view retained unsafe input: %q", view)
	}

	short := newMCPApprovalModel(ocrmcp.Invocation{Grant: ocrmcp.ToolGrant{
		ID: ocrmcp.ToolID{Server: "server", Name: "tool"}, DefinitionSHA256: "short",
	}}).View().Content
	if strings.Contains(short, "Untrusted server description") || strings.Contains(short, "Definition:") {
		t.Errorf("short/empty optional fields unexpectedly rendered: %q", short)
	}

	updated, command := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = updated.(mcpApprovalModel)
	if command != nil || model.width != 120 {
		t.Fatalf("window update = width %d, command nil %v", model.width, command == nil)
	}
	updated, command = model.Update(struct{}{})
	if command != nil || updated.(mcpApprovalModel).selected != model.selected {
		t.Fatal("unrelated message changed the approval model")
	}

	model.selected = 0
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyUp}, {Code: 'k', Text: "k"}} {
		updated, _ = model.Update(key)
		if got := updated.(mcpApprovalModel).selected; got != 0 {
			t.Errorf("upper-bound key %q selected %d", key.String(), got)
		}
	}
	model.selected = 2
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyUp}, {Code: 'k', Text: "k"}} {
		updated, _ = model.Update(key)
		model = updated.(mcpApprovalModel)
	}
	if model.selected != 0 {
		t.Fatalf("up keys selected %d, want 0", model.selected)
	}
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: 'j', Text: "j"}} {
		updated, _ = model.Update(key)
		model = updated.(mcpApprovalModel)
	}
	if model.selected != 2 {
		t.Fatalf("down keys selected %d, want 2", model.selected)
	}
	model.selected = len(mcpApprovalChoices) - 1
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: 'j', Text: "j"}} {
		updated, _ = model.Update(key)
		if got := updated.(mcpApprovalModel).selected; got != len(mcpApprovalChoices)-1 {
			t.Errorf("lower-bound key %q selected %d", key.String(), got)
		}
	}

	for index, decision := range []ocrmcp.Decision{
		ocrmcp.DecisionAllowOnce,
		ocrmcp.DecisionAllowReview,
		ocrmcp.DecisionDenyOnce,
		ocrmcp.DecisionDenyReview,
	} {
		key := tea.KeyPressMsg{Code: rune('1' + index), Text: string(rune('1' + index))}
		updated, command = newMCPApprovalModel(invocation).Update(key)
		got := updated.(mcpApprovalModel)
		if command == nil || got.selected != index || got.decision != decision {
			t.Errorf("digit %d result = %+v, command nil = %v", index+1, got, command == nil)
		}
	}

	model = newMCPApprovalModel(invocation)
	model.selected = 3
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := updated.(mcpApprovalModel); command == nil || got.decision != ocrmcp.DecisionDenyReview {
		t.Fatalf("Enter result = %+v, command nil = %v", got, command == nil)
	}

	for _, test := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{name: "escape", key: tea.KeyPressMsg{Code: tea.KeyEscape}},
		{name: "control-c", key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
		{name: "q", key: tea.KeyPressMsg{Code: 'q', Text: "q"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			updated, command := newMCPApprovalModel(invocation).Update(test.key)
			got := updated.(mcpApprovalModel)
			if command == nil || got.decision != ocrmcp.DecisionDenyOnce {
				t.Fatalf("cancel result = %+v, command nil = %v", got, command == nil)
			}
		})
	}
}

func TestMCPApprovalFormattingAndRedactionCoverage(t *testing.T) {
	arguments := map[string]any{
		"visible": "plain",
		"api-key": "api-canary",
		"nested": []any{
			map[string]any{"password": "password-canary", "safe": "kept"},
			"list-value",
		},
	}
	formatted := formatMCPApprovalArguments(arguments)
	for _, secret := range []string{"api-canary", "password-canary"} {
		if strings.Contains(formatted, secret) {
			t.Errorf("formatted arguments leaked %q: %s", secret, formatted)
		}
	}
	for _, want := range []string{"plain", "kept", "list-value", "[redacted]"} {
		if !strings.Contains(formatted, want) {
			t.Errorf("formatted arguments %q do not contain %q", formatted, want)
		}
	}

	wantRedacted := []any{map[string]any{"safe": "value", "secret": "[redacted]"}, 7.0}
	gotRedacted := redactMCPApprovalValue([]any{map[string]any{"safe": "value", "secret": "hidden"}, 7.0}, "")
	if !reflect.DeepEqual(gotRedacted, wantRedacted) {
		t.Fatalf("redacted nested value = %#v, want %#v", gotRedacted, wantRedacted)
	}
	if got := redactMCPApprovalValue("hidden", "AUTHORIZATION"); got != "[redacted]" {
		t.Fatalf("sensitive root value = %#v", got)
	}
	if got := redactMCPApprovalValue(42, "ordinary"); got != 42 {
		t.Fatalf("ordinary primitive = %#v", got)
	}

	for _, key := range []string{
		"token", "client-secret", "password", "Authorization", "session.cookie", "header value",
		"ENV_VAR", "api_key", "apikey", "private-key", "privatekey", "credential-id",
	} {
		if !mcpApprovalSensitiveKey(key) {
			t.Errorf("sensitive key %q was not detected", key)
		}
	}
	for _, key := range []string{"", "query", "monkey", "setting"} {
		if mcpApprovalSensitiveKey(key) {
			t.Errorf("ordinary key %q was marked sensitive", key)
		}
	}

	if got := formatMCPApprovalArguments(map[string]any{"bad": make(chan int)}); !strings.Contains(got, "could not be rendered") {
		t.Fatalf("marshal failure message = %q", got)
	}
	if got := formatMCPApprovalArguments(map[string]any{"visible": mcpTUICoverageControlJSON{}}); strings.ContainsRune(got, '\u0085') || !strings.Contains(got, "a b") {
		t.Fatalf("control-bearing JSON was not sanitized: %q", got)
	}
	truncated := formatMCPApprovalArguments(map[string]any{"visible": strings.Repeat("x", maxMCPApprovalArgumentsBytes+512)})
	if !strings.HasSuffix(truncated, "\n... (truncated)") || len(truncated) <= maxMCPApprovalArgumentsBytes {
		t.Fatalf("large arguments were not visibly truncated: length=%d suffix=%q", len(truncated), truncated[len(truncated)-32:])
	}

	if got := sanitizeMCPApprovalText(" \x00 first\n\t second\x7f ", 100); got != "first second" {
		t.Fatalf("sanitized approval text = %q", got)
	}
	if got := sanitizeMCPApprovalText("你好世界", 2); got != "你好..." { // allow-non-english: fixture verifies rune-safe truncation
		t.Fatalf("truncated Unicode approval text = %q", got)
	}
	if got := sanitizeMCPApprovalText("short", 20); got != "short" {
		t.Fatalf("short approval text = %q", got)
	}
}

func TestMCPApprovalRunnerAndNonInteractiveBoundaryCoverage(t *testing.T) {
	t.Run("allow once", func(t *testing.T) {
		var output bytes.Buffer
		decision, err := runMCPApprovalBubbleTea(context.Background(), ocrmcp.Invocation{}, strings.NewReader("1"), &output)
		if err != nil || decision != ocrmcp.DecisionAllowOnce {
			t.Fatalf("approval result = %q, %v", decision, err)
		}
		if output.Len() == 0 {
			t.Fatal("approval prompt produced no terminal output")
		}
	})

	t.Run("default deny", func(t *testing.T) {
		decision, err := runMCPApprovalBubbleTea(context.Background(), ocrmcp.Invocation{}, strings.NewReader("\r"), io.Discard)
		if err != nil || decision != ocrmcp.DecisionDenyOnce {
			t.Fatalf("default approval = %q, %v", decision, err)
		}
	})

	t.Run("explicit cancellation", func(t *testing.T) {
		decision, err := runMCPApprovalBubbleTea(context.Background(), ocrmcp.Invocation{}, strings.NewReader("q"), io.Discard)
		if err != nil || decision != ocrmcp.DecisionDenyOnce {
			t.Fatalf("cancelled approval = %q, %v", decision, err)
		}
	})

	t.Run("input termination fails closed", func(t *testing.T) {
		readErr := errors.New("terminal read failed")
		decision, err := runMCPApprovalBubbleTea(context.Background(), ocrmcp.Invocation{}, mcpTUICoverageErrorReader{err: readErr}, io.Discard)
		if decision != ocrmcp.DecisionDenyOnce || !errors.Is(err, context.Canceled) {
			t.Fatalf("approval input error = %q, %v", decision, err)
		}
	})

	t.Run("pre-cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		decision, err := runMCPApprovalBubbleTea(ctx, ocrmcp.Invocation{}, strings.NewReader("1"), io.Discard)
		if decision != ocrmcp.DecisionDenyOnce || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled approval = %q, %v", decision, err)
		}
	})

	previousInteractive := mcpInteractiveTerminal
	previousRunner := runMCPApprovalPrompt
	t.Cleanup(func() {
		mcpInteractiveTerminal = previousInteractive
		runMCPApprovalPrompt = previousRunner
	})

	runnerCalls := 0
	runMCPApprovalPrompt = func(context.Context, ocrmcp.Invocation, io.Reader, io.Writer) (ocrmcp.Decision, error) {
		runnerCalls++
		return ocrmcp.DecisionAllowReview, nil
	}
	mcpInteractiveTerminal = func() bool { return false }
	decision, err := newMCPRuntimeApprovalPrompter().PromptApproval(context.Background(), ocrmcp.Invocation{})
	if decision != ocrmcp.DecisionDenyOnce || !errors.Is(err, ocrmcp.ErrInteractionUnavailable) || runnerCalls != 0 {
		t.Fatalf("non-interactive approval = %q, %v, runner calls %d", decision, err, runnerCalls)
	}

	mcpInteractiveTerminal = func() bool { return true }
	decision, err = newMCPRuntimeApprovalPrompter().PromptApproval(context.Background(), ocrmcp.Invocation{})
	if decision != ocrmcp.DecisionAllowReview || err != nil || runnerCalls != 1 {
		t.Fatalf("interactive approval = %q, %v, runner calls %d", decision, err, runnerCalls)
	}
}
