// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/model"
)

const linkedReplyPrompt = ` For the listed inline review replies, compare only the named candidates against the original finding, actual reply, and current implementation. A reply can agree, ask a question or defer work. Use only a specific explanation that you can verify on today's code. A question can suggest a factual counterexample; trace it yourself. Check the triggering inputs, guards and actual effect of the proposed change. An earlier author preference has no authority to withdraw a valid finding. Preserve distinct failures on the same line and independently useful alternatives.
For such a check use outcome "refuted", the linked record id, candidate, path, the current candidate line and its exact reviewed quote. The original and submitted line numbers describe earlier revisions. Give "prior_claim" and "reply_quote" as exact pertinent excerpts of the original comment and this record's own reply. Consider clarifications later in that discussion. In "refuted_text" quote the complete current finding only if all of it is wrong. For a mixed finding, select the exact unsupported clause and its obsolete lead-in or conjunction. Read the entire surviving sentence after deletion. Keep its independent recommendation grammatical: if a surviving word at the join needs inflection or capitalization, give its exact "join_word" and the corrected "join_form". Use "also_refuted" only for other exact passages in the current candidate body repeating the same error. Its published predecessor can contain an additional Markdown example: quote that only as historical evidence. Everything else stays verbatim. Use "disputed_suggestion" only when the current candidate has a separate suggestion_code implementing that wrong edit, copied in full. Give "trigger" and "effect" as distinct verbatim excerpts of refuted_text identifying its condition and consequence, or requested edit and its subject. Quote the current source line(s) that demonstrate the contradiction in source_path and source_quote, and explain why in reason. A source quote can span consecutive lines. Omit an assessment whose premise is uncertain.`

type reviewReplyCandidate struct {
	ID         string `json:"candidate"`
	Start      int    `json:"current_start_line"`
	End        int    `json:"current_end_line"`
	Suggestion string `json:"suggestion_code,omitempty"`
}

type reviewReplyTarget struct {
	ID         string                 `json:"id"`
	Candidates []reviewReplyCandidate `json:"candidates,omitempty"`
	Source     knowledge.Citation     `json:"current_source"`
	Submitted  *knowledge.Citation    `json:"reply_time_source,omitempty"`
	Hunks      []reviewedReplyHunk    `json:"changes_since_reply,omitempty"`
	Operation  *knowledge.Citation    `json:"current_operation,omitempty"`
}

func isReviewReply(f knowledge.Fact) bool { return f.Origin == "review-reply" }

func (a *Agent) reviewBusinessContext(snapshot knowledge.Snapshot, changes []model.Diff) businessContext {
	if !slices.ContainsFunc(snapshot, isReviewReply) {
		return selectBusinessContext(snapshot, changes)
	}
	drafts, _ := a.businessDrafts(changes)
	if a.args.SkipFilter {
		drafts = nil
	}
	var relevant knowledge.Snapshot
	replies := map[string]reviewReplyTarget{}
	for _, f := range snapshot {
		if isReviewReply(f) {
			target, ok := locateReviewReply(f, changes, drafts)
			if !ok {
				continue
			}
			replies[f.ID] = target
		}
		relevant = append(relevant, f)
	}
	b := selectBusinessContext(relevant, changes)
	for _, f := range b.rules {
		if target, ok := replies[f.ID]; ok {
			b.replies = append(b.replies, target)
		}
	}
	if len(b.replies) != 0 {
		data, _ := json.Marshal(b.replies)
		b.text += "\nInline replies, candidate locations and current source for rechecking their reasoning:\n" + string(data)
	}
	return b
}

func locateReviewReply(f knowledge.Fact, changes []model.Diff, drafts []model.LlmComment) (reviewReplyTarget, bool) {
	if f.ReviewReply == nil || f.ContextStatus() != knowledge.StatusHistorical || len(f.Citations) == 0 {
		return reviewReplyTarget{}, false
	}
	anchor := f.Citations[0]
	i := slices.IndexFunc(changes, func(d model.Diff) bool { return !d.IsDeleted && d.NewPath == anchor.CurrentPath })
	if i < 0 {
		return reviewReplyTarget{}, false
	}
	change := changes[i]
	lines := strings.Split(strings.ReplaceAll(change.NewFileContent, "\r\n", "\n"), "\n")
	currentSource := func(c knowledge.Citation) (knowledge.Citation, bool) {
		start, end := c.CurrentStartLine, c.CurrentEndLine
		if start <= 0 || end < start || end > len(lines) || c.CurrentPath != change.NewPath {
			return knowledge.Citation{}, false
		}
		quote := strings.Join(lines[start-1:end], "\n")
		if strings.TrimSpace(quote) == "" || strings.TrimSpace(quote) != strings.TrimSpace(strings.ReplaceAll(c.Quote, "\r\n", "\n")) {
			return knowledge.Citation{}, false
		}
		return knowledge.Citation{Kind: c.Kind, Path: c.CurrentPath, StartLine: start, EndLine: end, Quote: quote}, true
	}
	source, ok := currentSource(anchor)
	if !ok {
		return reviewReplyTarget{}, false
	}
	target := reviewReplyTarget{ID: f.ID, Source: source}
	for i, cm := range drafts {
		if cm.Path != change.NewPath {
			continue
		}
		start, end := cm.StartLine, findingEndLine(cm)
		if start == 0 {
			start = uniqueBusinessQuoteLine(change.NewFileContent, cm.ExistingCode)
			end = start + strings.Count(strings.TrimSpace(cm.ExistingCode), "\n")
		}
		if start > 0 && start <= source.EndLine && source.StartLine <= end {
			target.Candidates = append(target.Candidates, reviewReplyCandidate{fmt.Sprintf("c-%d", i), max(start, source.StartLine), min(end, source.EndLine), cm.SuggestionCode})
		}
	}
	if changed := f.ReviewReply.Change; changed != nil && f.Revision != "" && changed.Submitted.Revision == f.Revision {
		head := changed.Current
		head.CurrentPath, head.CurrentStartLine, head.CurrentEndLine = head.Path, head.StartLine, head.EndLine
		if _, ok := currentSource(head); ok && head.StartLine <= source.StartLine && source.EndLine <= head.EndLine {
			target.Hunks = reviewedReplyChanges(change, changed)
			if len(target.Hunks) > 0 {
				target.Submitted, target.Source, target.Operation = &changed.Submitted, changed.Current, &source
			}
		}
	}
	if len(target.Candidates) == 0 && target.Submitted == nil {
		return reviewReplyTarget{}, false
	}
	// A guard or value that establishes the actual input can be outside the diff.
	if target.Submitted == nil {
		routine := knowledge.EnclosingReviewFunctions(change.NewPath, change.NewFileContent, source.StartLine)[0]
		if routine.EndLine >= source.EndLine && len(routine.Quote) <= 8<<10 {
			target.Source = routine
		} else {
			for _, c := range f.Citations[1:] {
				if context, ok := currentSource(c); ok && context.StartLine <= source.StartLine && context.EndLine >= source.EndLine && len(context.Quote) <= 8<<10 {
					target.Source = context
					break
				}
			}
		}
	}
	// The hunk may show supporting code just beyond the anchor, including a
	// declaration comment or assertions following a test's first operation.
	for _, h := range diff.ParseHunks(change.Diff) {
		if h.NewStart > source.EndLine || h.NewStart+h.NewCount <= source.StartLine {
			continue
		}
		at := h.NewStart
		first, last := target.Source.StartLine, target.Source.EndLine
		consistent := true
		for _, l := range h.Lines {
			if at >= h.NewStart+h.NewCount {
				break
			}
			if l.Type == diff.HunkDeleted {
				continue
			}
			if at >= max(1, source.StartLine-12) && at <= min(len(lines), source.EndLine+12) {
				if strings.TrimSuffix(l.Content, "\r") != lines[at-1] {
					consistent = false
					break
				}
				first, last = min(first, at), max(last, at)
			}
			at++
		}
		if !consistent || first == target.Source.StartLine && last == target.Source.EndLine {
			continue
		}
		quote := strings.Join(lines[first-1:last], "\n")
		if len(quote) <= 8<<10 {
			target.Source.StartLine, target.Source.EndLine, target.Source.Quote = first, last, quote
		}
	}
	return target, true
}

// A historical line number can be accurate at submission and wrong at head.
// Resolve the actual quote only inside the mapped candidate. When it also
// matches another current line explicitly named by the assessor, keep it there.
func (b businessContext) checkLocation(c *businessCheck, lines []changedLine) int {
	location := c.location(lines)
	quote := strings.TrimSpace(c.Quote)
	if location != 0 || c.Outcome != "refuted" || quote == "" || len(quote) > 1200 {
		return location
	}
	for _, target := range b.replies {
		if target.ID != c.ID || target.Source.Path != c.Path {
			continue
		}
		for _, cm := range target.Candidates {
			if cm.ID != c.Candidate {
				continue
			}
			if strings.ContainsRune(quote, '\n') {
				// Some assessments quote the complete selected block. Bind its
				// full text to the current candidate before using the first line.
				offset := uniqueBusinessQuoteLine(target.Source.Quote, quote)
				if offset == 0 {
					return 0
				}
				block := strings.Split(strings.ReplaceAll(quote, "\r\n", "\n"), "\n")
				start := target.Source.StartLine + offset - 1
				end := start + len(block) - 1
				at := slices.IndexFunc(lines, func(l changedLine) bool { return l.number == start })
				if start < cm.Start || end > cm.End || c.Line != 0 && (c.Line < start || c.Line > end) || at < 0 || len(block) > len(lines)-at {
					return 0
				}
				for i, s := range block {
					if l := lines[at+i]; l.number != start+i || l.text != strings.TrimSpace(s) {
						return 0
					}
				}
				c.Quote = block[0]
				return start
			}
			var matched int
			for _, line := range lines {
				if line.number >= cm.Start && line.number <= cm.End && line.text == quote {
					if matched != 0 && line.number != matched {
						return 0
					}
					matched = line.number
				}
			}
			return matched
		}
	}
	return 0
}

func (b businessContext) replyAmendment(f knowledge.Fact, c *businessCheck, drafts []model.LlmComment, changes []model.Diff) (int, model.LlmComment, bool) {
	i, ok := refutedCandidate(*c, c.Line, drafts)
	if !ok || c.Outcome != "refuted" || c.Reference != "" || c.SourceID != "" && c.SourceID != f.ID {
		return 0, model.LlmComment{}, false
	}
	index := slices.IndexFunc(b.replies, func(t reviewReplyTarget) bool {
		return t.ID == f.ID && t.Source.Path == c.Path && slices.ContainsFunc(t.Candidates, func(candidate reviewReplyCandidate) bool {
			return candidate.ID == c.Candidate && candidate.Start <= c.Line && c.Line <= candidate.End
		})
	})
	if index < 0 || !replyExcerpt(f.Finding, c.PriorClaim) || !replyExcerpt(f.Reason, c.ReplyQuote) || !c.bindRefutationScenario(c.RefutedText) {
		return 0, model.LlmComment{}, false
	}
	source := b.replies[index].Source
	current := citedBusinessLine([]knowledge.Citation{source}, nil, *c)
	if !current && !(c.Inherited != "" && strings.Contains(c.PriorClaim, c.Inherited) &&
		strings.Contains(c.RefutedText, c.Inherited) && showsInheritedExpression(*c, changes)) {
		return 0, model.LlmComment{}, false
	}
	if !c.projectReplyPassages(f.Finding, drafts[i]) {
		return 0, model.LlmComment{}, false
	}
	amended, ok := c.amendReplyFinding(drafts[i])
	return i, amended, ok
}

// A published review can contain examples omitted from today's candidate.
// Neither assessment can edit such a passage, even when it quotes it correctly.
func (c *businessCheck) projectReplyPassages(prior string, draft model.LlmComment) bool {
	if len(c.AlsoRefuted) > 4 {
		return false
	}
	onlyInPrior := func(p string) bool {
		return replyExcerpt(prior, p) && strings.Contains(prior, p) && !strings.Contains(draft.Content, strings.TrimSpace(p))
	}
	var current []string
	for _, passage := range c.AlsoRefuted {
		if strings.Contains(draft.Content, passage) {
			current = append(current, passage)
		} else if !onlyInPrior(passage) {
			return false
		}
	}
	c.AlsoRefuted = current
	if c.DisputedSuggestion != "" && c.DisputedSuggestion != draft.SuggestionCode {
		if draft.SuggestionCode != "" || !onlyInPrior(c.DisputedSuggestion) {
			return false
		}
		c.DisputedSuggestion = ""
	}
	return true
}

func replyExcerpt(s, excerpt string) bool {
	excerpt = strings.TrimSpace(excerpt)
	return excerpt != "" && (len(excerpt) >= 8 || excerpt == strings.TrimSpace(s)) && strings.Contains(s, excerpt)
}

func (c businessCheck) disputedPassages() []string {
	return append([]string{c.RefutedText}, c.AlsoRefuted...)
}

func (c businessCheck) amendReplyFinding(original model.LlmComment) (model.LlmComment, bool) {
	if len(c.AlsoRefuted) > 4 || c.DisputedSuggestion != "" && original.SuggestionCode != c.DisputedSuggestion {
		return model.LlmComment{}, false
	}
	content := original.Content
	for i, passage := range c.disputedPassages() {
		if !replyExcerpt(original.Content, passage) || strings.Count(original.Content, passage) != 1 {
			return model.LlmComment{}, false
		}
		before, after, found := strings.Cut(content, passage)
		if !found {
			return model.LlmComment{}, false
		}
		if i == 0 && (c.JoinWord != "" || c.JoinForm != "") {
			if before, after, found = inflectReplyJoin(before, after, c.JoinWord, c.JoinForm); !found {
				return model.LlmComment{}, false
			}
		}
		if strings.HasSuffix(before, " ") {
			after = strings.TrimLeft(after, " ")
		}
		content = before + after
	}
	original.Content = strings.TrimSpace(content)
	if c.DisputedSuggestion != "" {
		original.SuggestionCode = ""
	}
	if original.Content == "" && original.SuggestionCode != "" ||
		original.Content != "" && !businessIdentifier.MatchString(original.Content) {
		return model.LlmComment{}, false
	}
	return original, true
}

// Only the surviving word immediately beside a removal can change form. Keep
// its lexical root, and never rewrite a quoted code token or a different word.
func inflectReplyJoin(before, after, word, form string) (string, string, bool) {
	letters := func(s string) bool {
		return s != "" && !strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) })
	}
	if !letters(word) || !letters(form) || len(word) > 64 || len(form) > 64 {
		return "", "", false
	}
	w, f := strings.ToLower(word), strings.ToLower(form)
	sameForm := w == f
	for _, suffix := range []string{"s", "es", "ed", "ing"} {
		root := strings.TrimSuffix(f, suffix)
		if root != f && len(root) >= 3 && (root == w || root+"e" == w) {
			sameForm = true
		}
	}
	if !sameForm {
		return "", "", false
	}
	boundary := func(r rune) bool {
		return r == utf8.RuneError || unicode.IsSpace(r) || strings.ContainsRune(",;:.!?", r)
	}
	right := strings.TrimLeft(after, " \t")
	if rest, ok := strings.CutPrefix(right, word); ok {
		if next, _ := utf8.DecodeRuneInString(rest); boundary(next) {
			return before, after[:len(after)-len(right)] + form + rest, true
		}
	}
	left := strings.TrimRight(before, " \t")
	if rest, ok := strings.CutSuffix(left, word); ok {
		if prev, _ := utf8.DecodeLastRuneInString(rest); boundary(prev) {
			return rest + form + before[len(left):], after, true
		}
	}
	return "", "", false
}

// Two assessors can select different adjoining punctuation. Apply only a
// passage that the first assessment also disputed, preserving every other byte.
func (c businessCheck) corroboratesReply(first businessCheck) bool {
	if first.ID != c.ID || first.Candidate != c.Candidate || c.DisputedSuggestion != "" && c.DisputedSuggestion != first.DisputedSuggestion {
		return false
	}
	for _, part := range c.disputedPassages() {
		if !slices.ContainsFunc(first.disputedPassages(), func(proposed string) bool { return strings.Contains(proposed, part) }) {
			return false
		}
	}
	return true
}
