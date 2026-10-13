// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Decision is an authorization outcome. Review-scoped decisions are cached by
// exact ToolID for the lifetime of one ApprovalBroker.
type Decision string

const (
	DecisionAllowOnce   Decision = "allow_once"
	DecisionAllowReview Decision = "allow_review"
	DecisionDenyOnce    Decision = "deny_once"
	DecisionDenyReview  Decision = "deny_review"
)

var (
	// ErrPermissionDenied reports an explicit policy or user denial.
	ErrPermissionDenied = errors.New("MCP tool invocation denied")
	// ErrInteractionUnavailable reports that an ask policy could not prompt.
	ErrInteractionUnavailable = errors.New("MCP approval requires an interactive terminal")
	// ErrApprovalTimedOut reports an unanswered approval prompt.
	ErrApprovalTimedOut = errors.New("MCP approval timed out")
)

// Invocation is the non-secret runtime request passed to an Authorizer. The
// arguments originate with the model and are provided for local user review;
// callers must not persist or log them without redaction.
type Invocation struct {
	Grant     ToolGrant
	Arguments map[string]any
}

// Authorizer independently decides whether one already-visible tool invocation
// may cross the MCP tools/call boundary.
type Authorizer interface {
	Authorize(context.Context, Invocation) (Decision, error)
}

// Prompter obtains one local decision. Terminal implementation and rendering
// live outside the MCP core and are injectable for deterministic tests.
type Prompter interface {
	PromptApproval(context.Context, Invocation) (Decision, error)
}

// PromptFunc adapts a function to Prompter.
type PromptFunc func(context.Context, Invocation) (Decision, error)

func (f PromptFunc) PromptApproval(ctx context.Context, invocation Invocation) (Decision, error) {
	return f(ctx, invocation)
}

// ApprovalBroker is a concurrency-safe, per-review authorizer. Interactive
// prompts are serialized so concurrent review workers never compete for stdin.
type ApprovalBroker struct {
	interactive bool
	timeout     time.Duration
	prompt      Prompter
	invalid     error
	gate        chan struct{}

	mu              sync.RWMutex
	reviewDecisions map[ToolID]Decision
}

// NewRuntimeAuthorizer creates a per-review authorization broker. Invalid
// timeout values make every invocation fail closed.
func NewRuntimeAuthorizer(interactive bool, timeout time.Duration, prompt Prompter) *ApprovalBroker {
	broker := &ApprovalBroker{
		interactive:     interactive,
		timeout:         timeout,
		prompt:          prompt,
		gate:            make(chan struct{}, 1),
		reviewDecisions: make(map[ToolID]Decision),
	}
	broker.gate <- struct{}{}
	if timeout < minApprovalTimeout || timeout > maxApprovalTimeout {
		broker.invalid = fmt.Errorf("MCP approval timeout must be between 1 and 600 seconds")
	}
	return broker
}

// Authorize implements Authorizer. Auto-approval applies only to the exact
// grant supplied by the already-resolved visible tool binding.
func (b *ApprovalBroker) Authorize(ctx context.Context, invocation Invocation) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return DecisionDenyOnce, err
	}
	if b == nil {
		return DecisionDenyOnce, ErrInteractionUnavailable
	}
	if b.invalid != nil {
		return DecisionDenyOnce, b.invalid
	}
	if err := validateToolGrant(invocation.Grant); err != nil {
		return DecisionDenyReview, fmt.Errorf("%w: invalid MCP tool grant: %v", ErrPermissionDenied, err)
	}
	switch invocation.Grant.Permission {
	case PermissionAllow:
		return DecisionAllowOnce, nil
	case PermissionDeny, PermissionInherit, "":
		return DecisionDenyReview, ErrPermissionDenied
	case PermissionAsk:
		// Continue below.
	default:
		return DecisionDenyReview, fmt.Errorf("%w: invalid permission %q", ErrPermissionDenied, invocation.Grant.Permission)
	}

	if decision, ok := b.cachedDecision(invocation.Grant.ID); ok {
		return decisionResult(decision)
	}
	if !b.interactive || b.prompt == nil {
		return DecisionDenyOnce, ErrInteractionUnavailable
	}

	approvalCtx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	select {
	case <-approvalCtx.Done():
		return DecisionDenyOnce, approvalContextError(approvalCtx)
	case <-b.gate:
	}

	// Another waiter may have stored a review-scoped decision while this request
	// was queued.
	if decision, ok := b.cachedDecision(invocation.Grant.ID); ok {
		b.gate <- struct{}{}
		return decisionResult(decision)
	}
	type promptResult struct {
		decision Decision
		err      error
	}
	resultCh := make(chan promptResult, 1)
	go func() {
		var result promptResult
		defer func() {
			if recovered := recover(); recovered != nil {
				result = promptResult{decision: DecisionDenyOnce, err: fmt.Errorf("MCP approval prompt panicked: %v", recovered)}
			}
			resultCh <- result
		}()
		result.decision, result.err = b.prompt.PromptApproval(approvalCtx, cloneInvocation(invocation))
	}()
	var result promptResult
	select {
	case <-approvalCtx.Done():
		// Keep the prompt gate closed until an input implementation that ignored
		// cancellation eventually returns. This prevents a stale terminal reader
		// from racing a new prompt. The buffered result channel avoids blocking it.
		go func() {
			<-resultCh
			b.gate <- struct{}{}
		}()
		return DecisionDenyOnce, approvalContextError(approvalCtx)
	case result = <-resultCh:
		// Keep the prompt gate until a review-scoped answer has been cached.
		// Otherwise a queued invocation can observe the empty cache and open a
		// duplicate prompt in the small window between receipt and storage.
	}
	decision, err := result.decision, result.err
	if err != nil {
		b.gate <- struct{}{}
		if approvalCtx.Err() != nil {
			return DecisionDenyOnce, approvalContextError(approvalCtx)
		}
		return DecisionDenyOnce, err
	}
	if approvalCtx.Err() != nil {
		b.gate <- struct{}{}
		return DecisionDenyOnce, approvalContextError(approvalCtx)
	}
	switch decision {
	case DecisionAllowOnce:
		b.gate <- struct{}{}
		return decision, nil
	case DecisionAllowReview:
		b.storeDecision(invocation.Grant.ID, decision)
		b.gate <- struct{}{}
		return decision, nil
	case DecisionDenyReview:
		b.storeDecision(invocation.Grant.ID, decision)
		b.gate <- struct{}{}
		return decision, ErrPermissionDenied
	case DecisionDenyOnce, "":
		b.gate <- struct{}{}
		return DecisionDenyOnce, ErrPermissionDenied
	default:
		b.gate <- struct{}{}
		return DecisionDenyOnce, fmt.Errorf("%w: invalid approval decision %q", ErrPermissionDenied, decision)
	}
}

func validateToolGrant(grant ToolGrant) error {
	if err := validateServerName(grant.ID.Server); err != nil {
		return errors.New("invalid server identity")
	}
	if err := validateToolName(grant.ID.Name); err != nil {
		return errors.New("invalid tool identity")
	}
	if grant.ModelAlias != ModelAlias(grant.ID) {
		return errors.New("model alias does not match the exact tool identity")
	}
	if len(grant.DefinitionSHA256) != 64 || !isLowerHex(grant.DefinitionSHA256) {
		return errors.New("definition fingerprint is invalid")
	}
	switch grant.Permission {
	case PermissionAsk, PermissionAllow, PermissionDeny:
		return nil
	default:
		return errors.New("resolved permission is invalid")
	}
}

func approvalContextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrApprovalTimedOut
	}
	return ctx.Err()
}

func (b *ApprovalBroker) cachedDecision(id ToolID) (Decision, bool) {
	b.mu.RLock()
	decision, ok := b.reviewDecisions[id]
	b.mu.RUnlock()
	return decision, ok
}

func (b *ApprovalBroker) storeDecision(id ToolID, decision Decision) {
	b.mu.Lock()
	b.reviewDecisions[id] = decision
	b.mu.Unlock()
}

func decisionResult(decision Decision) (Decision, error) {
	if decision == DecisionAllowReview {
		return decision, nil
	}
	return decision, ErrPermissionDenied
}

func cloneInvocation(invocation Invocation) Invocation {
	result := invocation
	result.Arguments = make(map[string]any, len(invocation.Arguments))
	for key, value := range invocation.Arguments {
		result.Arguments[key] = cloneArgumentValue(value)
	}
	return result
}

func cloneArgumentValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, child := range item {
			result[key] = cloneArgumentValue(child)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for i, child := range item {
			result[i] = cloneArgumentValue(child)
		}
		return result
	case []byte:
		return append([]byte(nil), item...)
	default:
		return value
	}
}
