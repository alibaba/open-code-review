// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestApprovalBrokerPolicyDecisions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		permission  Permission
		interactive bool
		want        Decision
		wantErr     error
	}{
		{name: "allow without prompt", permission: PermissionAllow, want: DecisionAllowOnce},
		{name: "deny without prompt", permission: PermissionDeny, want: DecisionDenyReview, wantErr: ErrPermissionDenied},
		{name: "inherit fails closed", permission: PermissionInherit, want: DecisionDenyReview, wantErr: ErrPermissionDenied},
		{name: "empty fails closed", permission: "", want: DecisionDenyReview, wantErr: ErrPermissionDenied},
		{name: "invalid fails closed", permission: "unexpected", want: DecisionDenyReview, wantErr: ErrPermissionDenied},
		{name: "ask cannot prompt in CI", permission: PermissionAsk, want: DecisionDenyOnce, wantErr: ErrInteractionUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var prompts atomic.Int32
			broker := NewRuntimeAuthorizer(tt.interactive, time.Second, PromptFunc(func(context.Context, Invocation) (Decision, error) {
				prompts.Add(1)
				return DecisionAllowOnce, nil
			}))
			got, err := broker.Authorize(context.Background(), testInvocation(tt.permission, "tool"))
			if got != tt.want {
				t.Fatalf("Authorize() decision = %q, want %q", got, tt.want)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Authorize() error = %v, want errors.Is(_, %v)", err, tt.wantErr)
			}
			if prompts.Load() != 0 {
				t.Fatalf("prompt called %d times, want 0", prompts.Load())
			}
		})
	}
}

func TestApprovalBrokerPromptDecisions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prompt  Decision
		want    Decision
		wantErr error
	}{
		{name: "allow once", prompt: DecisionAllowOnce, want: DecisionAllowOnce},
		{name: "allow review", prompt: DecisionAllowReview, want: DecisionAllowReview},
		{name: "deny once", prompt: DecisionDenyOnce, want: DecisionDenyOnce, wantErr: ErrPermissionDenied},
		{name: "empty means deny once", prompt: "", want: DecisionDenyOnce, wantErr: ErrPermissionDenied},
		{name: "deny review", prompt: DecisionDenyReview, want: DecisionDenyReview, wantErr: ErrPermissionDenied},
		{name: "invalid fails closed", prompt: "maybe", want: DecisionDenyOnce, wantErr: ErrPermissionDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			broker := NewRuntimeAuthorizer(true, time.Second, PromptFunc(func(context.Context, Invocation) (Decision, error) {
				return tt.prompt, nil
			}))
			got, err := broker.Authorize(context.Background(), testInvocation(PermissionAsk, "tool"))
			if got != tt.want {
				t.Fatalf("Authorize() decision = %q, want %q", got, tt.want)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Authorize() error = %v, want errors.Is(_, %v)", err, tt.wantErr)
			}
		})
	}
}

func TestApprovalBrokerReviewDecisionIsScopedAndCached(t *testing.T) {
	t.Parallel()

	var prompts atomic.Int32
	broker := NewRuntimeAuthorizer(true, time.Second, PromptFunc(func(_ context.Context, invocation Invocation) (Decision, error) {
		prompts.Add(1)
		if invocation.Grant.ID.Name == "allowed" {
			return DecisionAllowReview, nil
		}
		return DecisionDenyReview, nil
	}))

	for range 2 {
		decision, err := broker.Authorize(context.Background(), testInvocation(PermissionAsk, "allowed"))
		if err != nil || decision != DecisionAllowReview {
			t.Fatalf("cached allow = (%q, %v), want (%q, nil)", decision, err, DecisionAllowReview)
		}
	}
	for range 2 {
		decision, err := broker.Authorize(context.Background(), testInvocation(PermissionAsk, "denied"))
		if !errors.Is(err, ErrPermissionDenied) || decision != DecisionDenyReview {
			t.Fatalf("cached deny = (%q, %v), want (%q, permission denied)", decision, err, DecisionDenyReview)
		}
	}
	if got := prompts.Load(); got != 2 {
		t.Fatalf("prompt called %d times, want once for each exact ToolID", got)
	}
}

func TestApprovalBrokerSerializesConcurrentPrompts(t *testing.T) {
	t.Parallel()

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var prompts atomic.Int32
	broker := NewRuntimeAuthorizer(true, 2*time.Second, PromptFunc(func(context.Context, Invocation) (Decision, error) {
		if prompts.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		return DecisionAllowReview, nil
	}))

	invocation := testInvocation(PermissionAsk, "same")
	results := make(chan error, 2)
	go func() {
		_, err := broker.Authorize(context.Background(), invocation)
		results <- err
	}()
	<-firstStarted
	go func() {
		_, err := broker.Authorize(context.Background(), invocation)
		results <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if got := prompts.Load(); got != 1 {
		t.Fatalf("%d prompts active before release, want 1", got)
	}
	close(releaseFirst)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("Authorize() error = %v", err)
		}
	}
	if got := prompts.Load(); got != 1 {
		t.Fatalf("prompt called %d times, want cached concurrent decision", got)
	}
}

func TestApprovalBrokerCancellationAndTimeoutFailClosed(t *testing.T) {
	t.Parallel()

	t.Run("canceled before policy", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		broker := NewRuntimeAuthorizer(true, time.Second, PromptFunc(func(context.Context, Invocation) (Decision, error) {
			t.Fatal("prompt must not run")
			return DecisionAllowOnce, nil
		}))
		decision, err := broker.Authorize(ctx, testInvocation(PermissionAllow, "tool"))
		if decision != DecisionDenyOnce || !errors.Is(err, context.Canceled) {
			t.Fatalf("Authorize() = (%q, %v), want deny_once/context canceled", decision, err)
		}
	})

	t.Run("timeout keeps stale prompt isolated", func(t *testing.T) {
		promptStarted := make(chan struct{})
		releasePrompt := make(chan struct{})
		var prompts atomic.Int32
		broker := newTestApprovalBroker(15*time.Millisecond, PromptFunc(func(context.Context, Invocation) (Decision, error) {
			if prompts.Add(1) == 1 {
				close(promptStarted)
				<-releasePrompt
			}
			return DecisionAllowOnce, nil
		}))
		decision, err := broker.Authorize(context.Background(), testInvocation(PermissionAsk, "tool"))
		if decision != DecisionDenyOnce || !errors.Is(err, ErrApprovalTimedOut) {
			t.Fatalf("Authorize() = (%q, %v), want deny_once/timeout", decision, err)
		}
		<-promptStarted

		secondDone := make(chan error, 1)
		go func() {
			_, secondErr := broker.Authorize(context.Background(), testInvocation(PermissionAsk, "other"))
			secondDone <- secondErr
		}()
		select {
		case err := <-secondDone:
			if !errors.Is(err, ErrApprovalTimedOut) {
				t.Fatalf("queued approval error = %v, want timeout", err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("queued approval did not honor its timeout")
		}
		close(releasePrompt)
		if got := prompts.Load(); got != 1 {
			t.Fatalf("prompt called %d times, want stale prompt only", got)
		}
	})
}

func TestApprovalBrokerPromptErrorPanicAndArgumentIsolation(t *testing.T) {
	t.Parallel()

	t.Run("prompt error", func(t *testing.T) {
		promptErr := errors.New("terminal failed")
		broker := NewRuntimeAuthorizer(true, time.Second, PromptFunc(func(context.Context, Invocation) (Decision, error) {
			return DecisionAllowOnce, promptErr
		}))
		decision, err := broker.Authorize(context.Background(), testInvocation(PermissionAsk, "tool"))
		if decision != DecisionDenyOnce || !errors.Is(err, promptErr) {
			t.Fatalf("Authorize() = (%q, %v), want deny_once/prompt error", decision, err)
		}
	})

	t.Run("prompt panic", func(t *testing.T) {
		broker := NewRuntimeAuthorizer(true, time.Second, PromptFunc(func(context.Context, Invocation) (Decision, error) {
			panic("boom")
		}))
		decision, err := broker.Authorize(context.Background(), testInvocation(PermissionAsk, "tool"))
		if decision != DecisionDenyOnce || err == nil {
			t.Fatalf("Authorize() = (%q, %v), want fail-closed panic error", decision, err)
		}
	})

	t.Run("prompt receives a deep copy", func(t *testing.T) {
		arguments := map[string]any{"nested": map[string]any{"secret": "original"}}
		broker := NewRuntimeAuthorizer(true, time.Second, PromptFunc(func(_ context.Context, invocation Invocation) (Decision, error) {
			invocation.Arguments["nested"].(map[string]any)["secret"] = "changed"
			return DecisionDenyOnce, nil
		}))
		_, _ = broker.Authorize(context.Background(), Invocation{
			Grant: testInvocation(PermissionAsk, "tool").Grant, Arguments: arguments,
		})
		if got := arguments["nested"].(map[string]any)["secret"]; got != "original" {
			t.Fatalf("prompt mutated caller arguments: %v", got)
		}
	})
}

func TestApprovalBrokerInvalidConstructionAndNilReceiver(t *testing.T) {
	t.Parallel()

	invalid := NewRuntimeAuthorizer(true, time.Millisecond, PromptFunc(func(context.Context, Invocation) (Decision, error) {
		t.Fatal("prompt must not run")
		return DecisionAllowOnce, nil
	}))
	if decision, err := invalid.Authorize(context.Background(), testInvocation(PermissionAllow, "tool")); decision != DecisionDenyOnce || err == nil {
		t.Fatalf("invalid broker Authorize() = (%q, %v), want fail closed", decision, err)
	}

	var nilBroker *ApprovalBroker
	if decision, err := nilBroker.Authorize(context.Background(), testInvocation(PermissionAsk, "tool")); decision != DecisionDenyOnce || !errors.Is(err, ErrInteractionUnavailable) {
		t.Fatalf("nil broker Authorize() = (%q, %v), want interaction unavailable", decision, err)
	}
}

func TestApprovalBrokerRejectsMalformedAllowGrant(t *testing.T) {
	t.Parallel()

	valid := testInvocation(PermissionAllow, "tool")
	tests := []struct {
		name   string
		mutate func(*ToolGrant)
	}{
		{name: "empty server", mutate: func(grant *ToolGrant) { grant.ID.Server = "" }},
		{name: "empty tool", mutate: func(grant *ToolGrant) { grant.ID.Name = "" }},
		{name: "wrong alias", mutate: func(grant *ToolGrant) { grant.ModelAlias += "-other" }},
		{name: "missing fingerprint", mutate: func(grant *ToolGrant) { grant.DefinitionSHA256 = "" }},
		{name: "uppercase fingerprint", mutate: func(grant *ToolGrant) { grant.DefinitionSHA256 = strings.Repeat("A", 64) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			invocation := valid
			tt.mutate(&invocation.Grant)
			broker := NewRuntimeAuthorizer(false, time.Second, nil)
			decision, err := broker.Authorize(context.Background(), invocation)
			if decision != DecisionDenyReview || !errors.Is(err, ErrPermissionDenied) {
				t.Fatalf("Authorize() = (%q, %v), want malformed grant denied", decision, err)
			}
		})
	}
}

func testInvocation(permission Permission, name string) Invocation {
	id := ToolID{Server: "server", Name: name}
	return Invocation{Grant: ToolGrant{
		ID: id, ModelAlias: ModelAlias(id), Permission: permission,
		DefinitionSHA256: strings.Repeat("a", 64),
	}}
}

func newTestApprovalBroker(timeout time.Duration, prompt Prompter) *ApprovalBroker {
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return &ApprovalBroker{
		interactive:     true,
		timeout:         timeout,
		prompt:          prompt,
		gate:            gate,
		reviewDecisions: make(map[ToolID]Decision),
	}
}
