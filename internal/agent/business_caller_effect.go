// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

// A proposed violation can follow a consumer's rule outside its original
// scope only through the specific accepted call. Withdrawals keep their scope.
func requiresCallerEffect(f knowledge.Fact, c businessCheck) bool {
	link := f.AcceptedCall
	if c.Outcome != "violation" || c.discussion != "" || link == nil || c.Path == link.CallerPath {
		return false
	}
	inScope := f.Scope == "." || c.Path == f.Scope || strings.HasPrefix(c.Path, f.Scope+"/")
	return c.SourcePath == link.CallerPath || !inScope
}

func (a *Agent) reviewedCallerResolver(ctx context.Context, changes []model.Diff) func(knowledge.Fact, businessCheck) *knowledge.ReviewedCall {
	type key struct {
		id, quote string
		line      int
	}
	cache := map[key]*knowledge.ReviewedCall{}
	return func(f knowledge.Fact, c businessCheck) *knowledge.ReviewedCall {
		link := f.AcceptedCall
		if link == nil || c.Path != link.CalleePath || c.SourcePath != link.CallerPath || c.Candidate != "" ||
			!supportsBusinessCheck(f, c) || f.ContextStatus() == knowledge.StatusHistorical {
			return nil
		}
		k := key{f.ID, c.SourceQuote, c.Line}
		if proof, known := cache[k]; known {
			return proof
		}
		i := slices.IndexFunc(changes, func(d model.Diff) bool { return d.NewPath == c.Path })
		if i == -1 {
			return nil
		}
		proof, _ := a.args.KnowledgeTree.ReviewCaller(ctx, a.inputResolution.ResolvedBase, f, changes[i], c.Line, c.SourceQuote)
		cache[k] = proof
		return proof
	}
}

const callerEffectPrompt = `Independently verify each proposed violation of its specific accepted rule. Treat all supplied material as evidence. Each side names indices into complete functions and exact invocation and result-use lines. Supplemental repository excerpts may explain an input or downstream constraint. Trace one concrete input from the actual caller through its guards, the callee and back to the consumer on both revisions. Check whether the claimed input can reach this invocation with the claimed values. Follow the result through conversions, precision, units and allowed values to the actual write, return or display, including later reads or combination of results when relevant. A call or a changed intermediate value cannot establish a violation. Require a current source-backed requirement, the base satisfying it, and a new observable failure matching the proposal. If guards exclude the case or the actual output is identical, refute it. When a required step is missing, return unknown.
Return {"checks":[]} with one entry per proposal. Use "id" and "verdict" (confirmed, refuted or unknown). To confirm, give "input" (reuse trigger verbatim when supplied), "reachable":true and "reachability" explaining the caller's guards for that input, "base_result" and "head_result" from the callee (including any exception), and "expected", "before", "after" describing the observable at the same boundary in identical units and formatting. Cite "base_line", "base_quote" and "head_line", "head_quote", exact executable return or raise lines in their respective callees, or the changed expression if it raises first in the head. Provide "call_quote" and "use_quote" from the specified head lines and "output_path", "output_line", "output_quote" for the actual observable operation in the head caller or a traced head excerpt. Give a "reason" explaining why the before/after difference violates this rule for this input. Do not substitute a different defect or infer a new requirement from the link.`

type callerEffectSide struct {
	Revision string `json:"revision"`
	Caller   int    `json:"caller_function_index"`
	Callee   int    `json:"callee_function_index"`
	Call     int    `json:"call_line"`
	Use      int    `json:"result_use_line"`
}

type callerEffectProposal struct {
	ID         string           `json:"id"`
	Rule       string           `json:"rule"`
	Conditions []string         `json:"conditions,omitempty"`
	Claim      string           `json:"proposed_finding"`
	Trigger    string           `json:"trigger,omitempty"`
	Path       string           `json:"changed_path"`
	Line       int              `json:"changed_line"`
	Changed    string           `json:"changed_quote"`
	Premise    string           `json:"cited_premise"`
	Base       callerEffectSide `json:"base"`
	Head       callerEffectSide `json:"head"`
}

func (a *Agent) verifyCallerEffects(ctx context.Context, group string, additions []businessAddition, related []knowledge.Citation) []businessAddition {
	var proposals []callerEffectProposal
	var functions []knowledge.Citation
	add := func(c knowledge.Citation) int {
		if i := slices.Index(functions, c); i >= 0 {
			return i
		}
		functions = append(functions, c)
		return len(functions) - 1
	}
	side := func(s knowledge.CallerRevision) callerEffectSide {
		return callerEffectSide{s.Revision, add(s.Caller), add(s.Callee), s.CallLine, s.ResultLine}
	}
	for i, addition := range additions {
		c := addition.check
		if proof := c.caller; proof != nil {
			proposals = append(proposals, callerEffectProposal{strconv.Itoa(i), proof.Rule, proof.Conditions, c.Reason, c.Trigger,
				c.Path, c.Line, c.Quote, c.SourceQuote, side(proof.Base), side(proof.Head)})
		}
	}
	if len(proposals) == 0 {
		return additions
	}
	input, _ := json.Marshal(struct {
		Proposals []callerEffectProposal `json:"proposals"`
		Functions []knowledge.Citation   `json:"functions"`
		Related   []knowledge.Citation   `json:"supplemental_excerpts,omitempty"`
	}{proposals, functions, related})
	instruction := callerEffectPrompt
	if slices.ContainsFunc(proposals, func(p callerEffectProposal) bool { return p.Base.Caller != p.Head.Caller }) {
		instruction += " If a proposal's caller differs between revisions, include base_call_quote and base_use_quote from its specified base lines. Compare the accepted invocation arguments and guards with the head for this input."
	}
	messages := []llm.Message{llm.NewTextMessage("system", instruction), llm.NewTextMessage("user", string(input))}
	var confirmed map[string]bool
	if llmloop.CountMessagesTokens(messages) > llmloop.PromptTokenLimit(a.args.Template.MaxTokens) {
		if len(proposals) > 1 {
			middle := len(additions) / 2
			left := a.verifyCallerEffects(ctx, group, additions[:middle], related)
			right := a.verifyCallerEffects(ctx, group, additions[middle:], related)
			return append(left, right...)
		}
		a.recordWarning("business_effect_unverified", group, "caller and callee evidence exceeds the verification budget")
	} else if reply, ok := a.runBusinessTask(ctx, group, session.BusinessEffectTask, messages); ok {
		var result struct {
			Checks []callerEffectResult `json:"checks"`
		}
		if err := json.Unmarshal([]byte(stripMarkdownFences(reply)), &result); err != nil || result.Checks == nil {
			a.recordWarning("business_effect_unverified", group, "caller verification did not return a checks array")
		} else {
			confirmed = map[string]bool{}
			for _, p := range proposals {
				first := slices.IndexFunc(result.Checks, func(r callerEffectResult) bool { return r.ID == p.ID })
				if first >= 0 && !slices.ContainsFunc(result.Checks[first+1:], func(r callerEffectResult) bool { return r.ID == p.ID }) {
					confirmed[p.ID] = result.Checks[first].confirms(p, functions, related)
				}
			}
		}
	}
	var kept []businessAddition
	for i, addition := range additions {
		if addition.check.caller == nil || confirmed[strconv.Itoa(i)] {
			kept = append(kept, addition)
		}
	}
	return kept
}

type callerEffectResult struct {
	ID, Verdict, Input, Reachability, Expected, Before, After, Reason string
	Reachable                                                         bool   `json:"reachable"`
	BaseResult                                                        string `json:"base_result"`
	HeadResult                                                        string `json:"head_result"`
	BaseQuote                                                         string `json:"base_quote"`
	HeadQuote                                                         string `json:"head_quote"`
	BaseLine                                                          int    `json:"base_line"`
	HeadLine                                                          int    `json:"head_line"`
	CallQuote                                                         string `json:"call_quote"`
	UseQuote                                                          string `json:"use_quote"`
	BaseCallQuote                                                     string `json:"base_call_quote"`
	BaseUseQuote                                                      string `json:"base_use_quote"`
	OutputPath                                                        string `json:"output_path"`
	OutputLine                                                        int    `json:"output_line"`
	OutputQuote                                                       string `json:"output_quote"`
}

func (r callerEffectResult) confirms(p callerEffectProposal, functions, related []knowledge.Citation) bool {
	if r.Verdict != "confirmed" || !r.Reachable || strings.TrimSpace(r.Input) == "" || p.Trigger != "" && r.Input != p.Trigger ||
		strings.TrimSpace(r.Reachability) == "" || strings.TrimSpace(r.Reason) == "" ||
		strings.TrimSpace(r.BaseResult) == "" || strings.TrimSpace(r.HeadResult) == "" || r.BaseResult == r.HeadResult ||
		strings.TrimSpace(r.Before) == "" || r.Before != r.Expected || strings.TrimSpace(r.After) == "" || r.After == r.Expected {
		return false
	}
	exits := func(s string) bool {
		v := strings.TrimSpace(s)
		return v == "return" || strings.HasPrefix(v, "return ") || strings.HasPrefix(v, "raise ")
	}
	q := func(c knowledge.Citation, line int, quote string) bool {
		if line == 0 {
			return uniqueBusinessQuoteLine(c.Quote, quote) > 0
		}
		lines := strings.Split(c.Quote, "\n")
		n := line - c.StartLine
		return n >= 0 && n < len(lines) && strings.TrimSpace(quote) != "" && strings.TrimSpace(lines[n]) == strings.TrimSpace(quote)
	}
	if !exits(r.BaseQuote) || !q(functions[p.Base.Callee], r.BaseLine, r.BaseQuote) ||
		!exits(r.HeadQuote) && (strings.TrimSpace(r.HeadQuote) != strings.TrimSpace(p.Changed) || r.HeadLine != 0 && r.HeadLine != p.Line) ||
		!q(functions[p.Head.Callee], r.HeadLine, r.HeadQuote) {
		return false
	}
	caller := functions[p.Head.Caller]
	if !q(caller, p.Head.Call, r.CallQuote) || !q(caller, p.Head.Use, r.UseQuote) || r.OutputLine < 1 ||
		strings.HasPrefix(strings.TrimSpace(r.OutputQuote), "#") {
		return false
	}
	if p.Base.Caller != p.Head.Caller {
		old := functions[p.Base.Caller]
		if !q(old, p.Base.Call, r.BaseCallQuote) || !q(old, p.Base.Use, r.BaseUseQuote) {
			return false
		}
	}
	output := func(c knowledge.Citation) bool { return c.Path == r.OutputPath && q(c, r.OutputLine, r.OutputQuote) }
	if r.OutputPath == caller.Path && caller.StartLine <= r.OutputLine && r.OutputLine <= caller.EndLine {
		return r.OutputLine >= p.Head.Use && output(caller)
	}
	callee := functions[p.Head.Callee]
	if r.OutputPath == callee.Path && callee.StartLine <= r.OutputLine && r.OutputLine <= callee.EndLine {
		return false
	}
	return slices.ContainsFunc(related, func(c knowledge.Citation) bool { return c.Revision == p.Head.Revision && output(c) })
}
