// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"slices"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/tamnd/gopapy/v2/parser2"
)

// Both function bodies come from the same operation that survived the push.
// Quote only hunks that change executable text in that function. A matching
// change is a reason to investigate again, including when no draft exists.
func changedReplyFunction(path, submitted, head, before, after string, oldLine, newLine int, hunks []diff.Hunk) *ReviewReplyChange {
	old := EnclosingReviewFunctions(path, before, oldLine)[0]
	current := EnclosingReviewFunctions(path, after, newLine)[0]
	if old.Path == "" || current.Path == "" || old.Quote == current.Quote || len(old.Quote) > 8<<10 || len(current.Quote) > 8<<10 {
		return nil
	}
	old.Revision, current.Revision = submitted, head
	lexer := lexers.Match(path)
	if lexer == nil {
		return nil
	}
	oldCode, newCode := replyFunctionLines(lexer, old.Quote), replyFunctionLines(lexer, current.Quote)
	if oldCode == nil || newCode == nil {
		return nil
	}
	sameTokens := strings.Join(oldCode, "") == strings.Join(newCode, "")
	// In Python a statement can move in or out of a guard with an
	// indentation change alone. Ignore reformatting that preserves its AST.
	if sameTokens && !replyLayoutChanged(path, old.Quote, current.Quote) {
		return nil
	}
	change := &ReviewReplyChange{Submitted: old, Current: current}
	for _, h := range hunks {
		left, previous, inOld := replyHunkSide(old, oldCode, h.OldStart, h.OldCount)
		right, next, inNew := replyHunkSide(current, newCode, h.NewStart, h.NewCount)
		if !inOld || !inNew {
			continue
		}
		layout := previous == next
		if layout {
			if previous == "" || left.Quote == right.Quote || !strings.HasSuffix(path, ".py") {
				continue
			}
			// Revert just this hunk in the full current function. Its indentation
			// is significant only if that single edit changes the syntax tree.
			lines := strings.Split(current.Quote, "\n")
			restored := slices.Concat(lines[:right.StartLine-current.StartLine], strings.Split(left.Quote, "\n"), lines[right.EndLine-current.StartLine+1:])
			if !pythonReplyStructureChanged(strings.Join(restored, "\n"), current.Quote) {
				continue
			}
		}
		change.Hunks = append(change.Hunks, ReviewReplyHunk{Submitted: left, Current: right, LayoutChanged: layout})
		if len(change.Hunks) > 6 {
			return nil
		}
	}
	if len(change.Hunks) == 0 {
		return nil
	}
	return change
}

// ReviewReplyCodeChanged tests a block from the actual review diff before
// admitting it as evidence that an earlier explanation stopped applying.
func ReviewReplyCodeChanged(path, before, after string) bool {
	lexer := lexers.Match(path)
	if lexer == nil || before == after {
		return false
	}
	old, current := replyFunctionLines(lexer, before), replyFunctionLines(lexer, after)
	if before != "" && old == nil || after != "" && current == nil {
		return false
	}
	return strings.Join(old, "") != strings.Join(current, "") || replyLayoutChanged(path, before, after)
}

func replyLayoutChanged(path, before, after string) bool {
	return strings.HasSuffix(path, ".py") && pythonReplyStructureChanged(before, after)
}

func pythonReplyStructureChanged(before, after string) bool {
	parse := func(source string) *parser2.Module {
		if strings.HasPrefix(source, " ") || strings.HasPrefix(source, "\t") {
			source = "if True:\n" + source
		}
		module, err := parser2.ParseFile("reply.py", source+"\n")
		if err != nil {
			return nil
		}
		return module
	}
	old, current := parse(before), parse(after)
	return old != nil && current != nil && parser2.DumpModule(old) != parser2.DumpModule(current)
}

// Preserve literal values, including spaces inside a string, while dropping
// comments and formatting. Lex the entire function so multiline tokens
// retain their language-specific meaning at the edges of each change.
func replyFunctionLines(lexer chroma.Lexer, source string) []string {
	iter, err := lexer.Tokenise(nil, source)
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range chroma.SplitTokensIntoLines(iter.Tokens()) {
		var code strings.Builder
		for _, tok := range line {
			if tok.Type.InCategory(chroma.Comment) || tok.Type == chroma.LiteralStringDoc {
				continue
			}
			value := strings.TrimSuffix(tok.Value, "\n")
			if !tok.Type.InSubCategory(chroma.LiteralString) {
				value = strings.TrimSpace(value)
			}
			if value != "" {
				code.WriteString(value)
				code.WriteByte(0)
			}
		}
		lines = append(lines, code.String())
	}
	return lines
}

func replyHunkSide(function Citation, code []string, start, count int) (Citation, string, bool) {
	if count == 0 {
		// For an empty side Git numbers the line just before the change.
		return Citation{}, "", start >= function.StartLine-1 && start <= function.EndLine
	}
	first := max(start, function.StartLine)
	last := min(start+count-1, function.EndLine)
	if first > last || last-function.StartLine >= len(code) {
		return Citation{}, "", false
	}
	lines := strings.Split(function.Quote, "\n")
	if last-function.StartLine >= len(lines) {
		return Citation{}, "", false
	}
	offset, limit := first-function.StartLine, last-function.StartLine+1
	quote := strings.Join(lines[offset:limit], "\n")
	if len(quote) > maxCitationQuote {
		return Citation{}, "", false
	}
	part := Citation{Kind: function.Kind, Path: function.Path, Revision: function.Revision, StartLine: first, EndLine: last, Quote: quote}
	return part, strings.Join(code[offset:limit], ""), true
}
