// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"context"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/contract"
)

const (
	actionReview  = "review"
	actionScan    = "scan"
	actionClarify = "clarify"
	actionReject  = "reject"

	reviewWorkspace = "workspace"
	reviewRange     = "range"
	reviewCommit    = "commit"

	defaultParseTimeout = 15 * time.Second
)

// LLMClient performs one constrained tool call for natural-language parsing.
// Implementations live outside this package (phase 4 ships one in
// internal/llmresolve); tests inject a scripted fake.
type LLMClient interface {
	CallTool(ctx context.Context, req LLMRequest) (ToolCall, error)
}

// LLMRequest is a single tool-call request.
type LLMRequest struct {
	System string
	User   string
	Tool   ToolSchema
}

// ToolSchema is the JSON-Schema tool the model must call.
type ToolSchema struct {
	Name        string
	Description string
	Parameters  []byte
}

// ToolCall is one tool invocation returned by the model.
type ToolCall struct {
	Name      string
	Arguments []byte
}

// Parser converts prompt text into a Result. The LLM is used only for
// natural-language input; slash commands are always deterministic.
type Parser struct {
	llm     LLMClient
	repo    RepoContext
	timeout time.Duration
}

// NewParser builds a parser. Both dependencies may be nil: a nil LLM makes
// natural-language input a rejection, and a nil RepoContext skips ref checks.
func NewParser(llm LLMClient, repo RepoContext) *Parser {
	return &Parser{llm: llm, repo: repo, timeout: defaultParseTimeout}
}

// WithTimeout overrides the natural-language parse timeout.
func (p *Parser) WithTimeout(d time.Duration) *Parser {
	p.timeout = d
	return p
}

// Parse routes one prompt. A leading slash selects the deterministic parser;
// anything else goes to the LLM.
func (p *Parser) Parse(ctx context.Context, text string, st *State) (Result, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return RejectResult("empty request", "Send /review, /scan, or describe what to review."), nil
	}
	if strings.HasPrefix(trimmed, "/") {
		return p.parseSlash(ctx, trimmed, st)
	}
	return p.parseNatural(ctx, text, st)
}

// pendingSlots is the unvalidated working set shared by both parsers.
type pendingSlots struct {
	action     string
	reviewType string
	from       string
	to         string
	commit     string
	paths      []string
	extra      []string
}

func newSlots(action string) *pendingSlots {
	return &pendingSlots{action: action}
}

func slotsFromState(st *State) *pendingSlots {
	if st == nil {
		return nil
	}
	pend := st.Pending()
	if pend == nil || pend.Action == "" {
		return nil
	}
	return &pendingSlots{
		action:     pend.Action,
		reviewType: pend.ReviewType,
		from:       pend.From,
		to:         pend.To,
		commit:     pend.Commit,
		paths:      append([]string(nil), pend.Paths...),
		extra:      append([]string(nil), pend.Extra...),
	}
}

func (p *Parser) storePending(st *State, s *pendingSlots, missing []string) {
	if s == nil {
		return
	}
	st.setPending(&Pending{
		Action:     s.action,
		ReviewType: s.reviewType,
		From:       s.from,
		To:         s.to,
		Commit:     s.commit,
		Paths:      append([]string(nil), s.paths...),
		Extra:      append([]string(nil), s.extra...),
		Missing:    append([]string(nil), missing...),
	})
}

func (p *Parser) clearPending(st *State) {
	st.Clear()
}

// finalizeReview validates slots, builds argv, checks refs, and updates state.
func (p *Parser) finalizeReview(ctx context.Context, st *State, s *pendingSlots) (Result, error) {
	var ri *contract.ReviewIntent
	var missing []string

	switch {
	case s.reviewType == reviewCommit || s.commit != "":
		if s.commit == "" {
			missing = append(missing, "commit")
		} else {
			ri = &contract.ReviewIntent{Type: contract.ReviewTypeCommit, Commit: s.commit, Extra: s.extra}
		}
	case s.reviewType == reviewRange || s.from != "" || s.to != "":
		if s.from == "" {
			missing = append(missing, "from")
		}
		if s.to == "" {
			missing = append(missing, "to")
		}
		if len(missing) == 0 {
			ri = &contract.ReviewIntent{Type: contract.ReviewTypeRange, From: s.from, To: s.to, Extra: s.extra}
		}
	default:
		ri = &contract.ReviewIntent{Type: contract.ReviewTypeWorkspace, Extra: s.extra}
	}

	if len(missing) > 0 {
		if s.reviewType == "" {
			if missing[0] == "commit" {
				s.reviewType = reviewCommit
			} else {
				s.reviewType = reviewRange
			}
		}
		p.storePending(st, s, missing)
		return ClarifyResult(reviewClarifyQuestion(missing), missing...), nil
	}

	if _, err := contract.BuildReviewArgs(ri); err != nil {
		p.clearPending(st)
		return RejectResult(err.Error(), "Use /review or /scan with supported flags."), nil
	}
	if err := p.validateReviewRefs(ctx, ri); err != nil {
		p.storePending(st, s, []string{"ref"})
		return ClarifyResult("I could not resolve that ref: "+err.Error()+" Please check the branch or commit name.", "ref"), nil
	}
	p.clearPending(st)
	return IntentResult(ri, nil), nil
}

func (p *Parser) finalizeScan(ctx context.Context, st *State, s *pendingSlots) (Result, error) {
	si := &contract.ScanIntent{Paths: s.paths, Extra: s.extra}
	if _, err := contract.BuildScanArgs(si); err != nil {
		p.clearPending(st)
		return RejectResult(err.Error(), "Use /scan with relative paths inside the scan root."), nil
	}
	p.clearPending(st)
	return IntentResult(nil, si), nil
}

func (p *Parser) validateReviewRefs(ctx context.Context, ri *contract.ReviewIntent) error {
	if p.repo == nil {
		return nil
	}
	switch ri.Type {
	case contract.ReviewTypeRange:
		if err := p.repo.ResolveCommit(ctx, ri.From); err != nil {
			return err
		}
		return p.repo.ResolveCommit(ctx, ri.To)
	case contract.ReviewTypeCommit:
		return p.repo.ResolveCommit(ctx, ri.Commit)
	}
	return nil
}

func reviewClarifyQuestion(missing []string) string {
	if len(missing) >= 2 {
		return "Which two refs should I compare? Provide both --from and --to."
	}
	if missing[0] == "from" {
		return "Which base ref should I compare from? Add --from <ref>."
	}
	if missing[0] == "commit" {
		return "Which commit should I review? Add --commit <sha>."
	}
	return "Which head ref should I compare to? Add --to <ref>."
}
