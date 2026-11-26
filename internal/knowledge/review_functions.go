// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strings"
	"unicode/utf8"
)

const maxReviewFunctionBytes = 16 << 10

// EnclosingReviewFunctions uses the complete, already admitted reviewed file.
// Results align with lines, with a zero value for an unavailable full span.
func EnclosingReviewFunctions(p, content string, lines ...int) []Citation {
	out := make([]Citation, len(lines))
	if len(lines) == 0 || !validPath(p) || privateDiscoveryPath(p) || len(content) > maxEvidenceBytes ||
		!utf8.ValidString(content) || strings.IndexByte(content, 0) >= 0 ||
		discoveryCredential.MatchString(p) || discoveryCredential.MatchString(content) {
		return out
	}
	text := strings.ReplaceAll(content, "\r\n", "\n")
	if strings.ContainsRune(text, '\r') {
		return out
	}
	sourceLines := strings.Split(content, "\n")
	var definitions []discussionDefinition
	switch path.Ext(p) {
	case ".go":
		definitions = goReviewFunctions(p, content, sourceLines)
	case ".py":
		definitions, _ = pythonDiscussionDefinitions("", text)
	case ".js", ".mjs", ".cjs":
		definitions = javascriptReviewFunctions(text)
	}
	for _, d := range definitions {
		if d.unquotable || (d.kind != "func" && d.kind != "def" && d.kind != "async def") ||
			d.start < 1 || d.end < d.start || d.end > len(sourceLines) {
			continue
		}
		end := d.end
		for end > d.start && strings.TrimSpace(sourceLines[end-1]) == "" {
			end--
		}
		var citation Citation
		for i, line := range lines {
			if line < d.start || line > end {
				continue
			}
			if citation.Path == "" {
				quote := strings.TrimSuffix(strings.Join(sourceLines[d.start-1:end], "\n"), "\r")
				if len(quote) > maxReviewFunctionBytes {
					break
				}
				citation = Citation{Kind: discoveryKind(p), Path: p, StartLine: d.start, EndLine: end, Quote: quote}
			}
			out[i] = citation
		}
	}
	return out
}

func goReviewFunctions(p, content string, lines []string) []discussionDefinition {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, p, content, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	var definitions []discussionDefinition
	for _, node := range file.Decls {
		function, ok := node.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		start := positions.PositionFor(function.Pos(), false)
		end := positions.PositionFor(function.End(), false)
		// A whole-line citation cannot isolate two statements on a shared edge.
		if strings.TrimSpace(lines[start.Line-1][:start.Column-1]) != "" ||
			strings.TrimSpace(lines[end.Line-1][end.Column-1:]) != "" {
			continue
		}
		definitions = append(definitions, discussionDefinition{kind: "func", start: start.Line, end: end.Line})
	}
	return definitions
}

func javascriptReviewFunctions(content string) []discussionDefinition {
	code, ok := maskJavaScript(content)
	if !ok {
		return nil
	}
	depth, closes, ok := javascriptNesting(code)
	if !ok {
		return nil
	}
	positions := token.NewFileSet().AddFile("review.js", -1, len(code))
	positions.SetLinesForContent(code)
	var definitions []discussionDefinition
	add := func(start, end int) {
		before := bytes.LastIndexByte(code[:start], '\n') + 1
		after, _, _ := bytes.Cut(code[end:], []byte{'\n'})
		if len(bytes.TrimSpace(code[before:start])) != 0 || len(bytes.TrimSpace(after)) != 0 {
			return
		}
		definitions = append(definitions, discussionDefinition{kind: "func",
			start: positions.Line(positions.Pos(start)), end: positions.Line(positions.Pos(end - 1))})
	}
	for _, declaration := range javascriptDeclaration.FindAllSubmatchIndex(code, -1) {
		if depth[declaration[2]] != 0 {
			continue
		}
		kind := string(code[declaration[4]:declaration[5]])
		if kind != "class" && !strings.HasPrefix(kind, "function") {
			continue
		}
		open, end, ok := javascriptDefinitionEnd(code, depth, closes, declaration[7], kind)
		if !ok {
			continue
		}
		if kind != "class" {
			add(declaration[2], end)
			continue
		}
		for _, method := range javascriptMethod.FindAllSubmatchIndex(code[open:end], -1) {
			start := open + method[0]
			if depth[start] != 1 {
				continue
			}
			_, end, ok := javascriptDefinitionEnd(code, depth, closes, open+method[1]-1, "method")
			if ok {
				add(start, end)
			}
		}
	}
	return definitions
}
