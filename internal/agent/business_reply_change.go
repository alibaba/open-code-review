// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"fmt"
	"strings"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/model"
)

const changedReplyPrompt = ` Separately investigate records with changes_since_reply, even if there are no provisional findings. Compare the original inline claim and the specific reply at its submitted revision with the same operation in today's code. Trace an input for which the reply's factual explanation was correct. Did an actual change in the reviewed patch invalidate it, making the same original issue newly actionable? An acknowledgment, a future plan, different wording or a different defect does not establish this. Check all supported branches before calling an operation redundant. Leave an issue already diagnosed by an ordinary finding alone.
If established, return a check with outcome "reopened" and no candidate, the record "id", reviewed "path", "line" and exact current "quote" locating the affected operation or its causal edit in the patch. Include "prior_claim" and "reply_quote" verbatim from that record, a concrete "trigger", "submitted_quote" and current "source_quote" copying complete source lines on each side of the same change, plus "source_path". For an added or removed statement, cite the other revision's operative context. Use "change_effect":"failure" for a newly reachable wrong result or "change_effect":"redundancy" for an operation whose removal now preserves the result for every admitted input. In "reason", write a fresh review comment in the requested language about the actual current input, effect and appropriate fix. Give a severity. Omit unresolved, pre-existing and unrelated issues.`

const changedReplyEffectPrompt = `Independently check each proposal with "historical_reply". Its submitted function is the code at reply publication; the indexed current function is the code under review. The original comment and its reply are claims to check. Use the named input verbatim. Trace the original operation on both revisions, including its guards and observable result. Verify the reply's specific premise held at publication and that the supplied changed lines, also in the reviewed patch, invalidate that premise and establish this same original issue today. Reject a mere preference, a newly introduced different issue, or a claim unsupported by these functions.
Return {"checks":[]} with one entry per proposal. Supply id, verdict (confirmed, refuted or unknown), input, reason, actual "before" and "after" values, a complete exact source line or lines for "submitted_quote" and "current_quote" grounding the changed premise, and "output_path", "output_line", "output_quote" citing a complete current line. For "failure", measure the same invariant in both revisions, such as how many required items went unprocessed if workload sizes differ. Give "expected": the reply-time before must satisfy it and the current after must violate it. For "redundancy", cite the disputed operation in output_quote. Also compute "before_without" and "after_without" for the same input with just that operation removed or simplified. Use identical units and formatting. The original operation must make a difference at reply time and none today. Explain in "all_inputs" why the actual current code enforces that equivalence across every admitted input, including alternate branches. Omit a confirmation if any branch still needs the operation.`

type reviewedReplyHunk struct {
	knowledge.ReviewReplyHunk
	patch string
}

// Intersect historical differences with the actual base-to-head edits. A
// comment beside a previously changed statement does not reintroduce it.
func reviewedReplyChanges(d model.Diff, change *knowledge.ReviewReplyChange) []reviewedReplyHunk {
	var relevant []reviewedReplyHunk
	for _, h := range diff.ParseHunks(d.Diff) {
		old, current := h.OldStart, h.NewStart
		if h.OldCount == 0 {
			old++
		}
		if h.NewCount == 0 {
			current++
		}
		for i := 0; i < len(h.Lines); {
			if h.Lines[i].Type == diff.HunkContext {
				old++
				current++
				i++
				continue
			}
			startOld, start := old, current
			var deleted, added []string
			for ; i < len(h.Lines) && h.Lines[i].Type != diff.HunkContext; i++ {
				line := h.Lines[i]
				if line.Type == diff.HunkDeleted {
					deleted = append(deleted, line.Content)
					old++
				} else {
					added = append(added, line.Content)
					current++
				}
			}
			if start > change.Current.EndLine+1 || current < change.Current.StartLine {
				continue
			}
			left, right := strings.Join(deleted, "\n"), strings.Join(added, "\n")
			codeChanged := knowledge.ReviewReplyCodeChanged(d.NewPath, left, right)
			for _, historical := range change.Hunks {
				// A standalone Python statement has no enclosing guard to parse.
				// Reuse the function-level proof only for this exact pair of edits.
				if !codeChanged && !(historical.LayoutChanged && left == historical.Submitted.Quote && right == historical.Current.Quote) {
					continue
				}
				if checked, ok := intersectReplyHunk(d.NewPath, historical, start, added, left); ok {
					// Keep the reviewed base distinct from the earlier submission.
					var patch strings.Builder
					from, to := startOld, start
					if len(deleted) == 0 {
						from--
					}
					if len(added) == 0 {
						to--
					}
					fmt.Fprintf(&patch, "@@ -%d,%d +%d,%d @@\n", from, len(deleted), to, len(added))
					for _, line := range deleted {
						fmt.Fprintln(&patch, "-"+line)
					}
					for _, line := range added {
						fmt.Fprintln(&patch, "+"+line)
					}
					relevant = append(relevant, reviewedReplyHunk{checked, strings.TrimSuffix(patch.String(), "\n")})
				}
			}
		}
	}
	return relevant
}

func intersectReplyHunk(path string, h knowledge.ReviewReplyHunk, start int, added []string, deleted string) (knowledge.ReviewReplyHunk, bool) {
	if h.Current.Quote == "" {
		return h, uniqueBusinessQuoteLine(deleted, h.Submitted.Quote) > 0
	}
	first, last := max(h.Current.StartLine, start), min(h.Current.EndLine, start+len(added)-1)
	if first > last {
		return h, false
	}
	quote := strings.Join(added[first-start:last-start+1], "\n")
	historical := strings.Split(h.Current.Quote, "\n")
	offset := first - h.Current.StartLine
	if end := last - h.Current.StartLine + 1; end > len(historical) || strings.TrimSpace(quote) != strings.TrimSpace(strings.Join(historical[offset:end], "\n")) ||
		!knowledge.ReviewReplyCodeChanged(path, "", quote) {
		return h, false
	}
	h.Current.StartLine, h.Current.EndLine, h.Current.Quote = first, last, quote
	return h, true
}

type reopenedReply struct {
	Finding            string                    `json:"original_claim"`
	Reply              string                    `json:"reply"`
	LaterNotes         string                    `json:"later_notes,omitempty"`
	Kind               string                    `json:"change_effect"`
	Input              string                    `json:"input"`
	Operation          knowledge.Citation        `json:"operation"`
	Change             knowledge.ReviewReplyHunk `json:"changed_lines"`
	submitted, current knowledge.Citation
	patch              string
}

func (r *reopenedReply) provesChange(submitted, current string) bool {
	matches := func(source, changed knowledge.Citation, quote string) bool {
		if len(quote) > 1600 {
			return false
		}
		offset := uniqueBusinessQuoteLine(source.Quote, quote)
		if offset == 0 {
			return false
		}
		start := source.StartLine + offset - 1
		end := start + strings.Count(strings.TrimSpace(quote), "\n")
		return changed.Quote == "" || start <= changed.EndLine && changed.StartLine <= end
	}
	return matches(r.submitted, r.Change.Submitted, submitted) && matches(r.current, r.Change.Current, current)
}

func (b businessContext) reopenReply(f knowledge.Fact, c businessCheck) *reopenedReply {
	if c.Candidate != "" || c.SourcePath != c.Path || c.Reference != "" || strings.TrimSpace(c.Trigger) == "" ||
		c.ChangeEffect != "failure" && c.ChangeEffect != "redundancy" || !replyExcerpt(f.Finding, c.PriorClaim) || !replyExcerpt(f.Reason, c.ReplyQuote) {
		return nil
	}
	for _, target := range b.replies {
		if target.ID != f.ID || target.Source.Path != c.Path || target.Submitted == nil || target.Operation == nil {
			continue
		}
		for _, h := range target.Hunks {
			if (c.Line < target.Operation.StartLine || c.Line > target.Operation.EndLine) &&
				(c.Line < h.Current.StartLine || c.Line > h.Current.EndLine) {
				continue
			}
			r := &reopenedReply{Finding: f.Finding, Reply: f.Reason, LaterNotes: f.ReviewReply.LaterNotes, Kind: c.ChangeEffect, Input: c.Trigger,
				Operation: *target.Operation, Change: h.ReviewReplyHunk, patch: h.patch, submitted: *target.Submitted, current: f.ReviewReply.Change.Current}
			if r.provesChange(c.SubmittedQuote, c.SourceQuote) {
				return r
			}
		}
	}
	return nil
}
