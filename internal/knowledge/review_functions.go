// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/tamnd/gopapy/v2/parser2"
	"github.com/tamnd/gopapy/v2/symbols2"
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
		definitions = goReviewFunctions(p, text, sourceLines)
	case ".py":
		definitions, _ = pythonDiscussionDefinitions("", text)
	case ".js", ".mjs", ".cjs":
		definitions = javascriptReviewFunctions(text)
	}
	for _, d := range definitions {
		if d.unquotable || (d.kind != "func" && d.kind != "def" && d.kind != "async def") {
			continue
		}
		var citation Citation
		for i, line := range lines {
			if line < d.start || line > d.end {
				continue
			}
			if citation.Path == "" {
				citation = reviewSourceSpan(p, sourceLines, d.start, d.end)
				if citation.Path == "" {
					break
				}
			}
			if line <= citation.EndLine {
				out[i] = citation
			}
		}
	}
	return out
}

// ReviewEffectEvidence pairs the changed function with the complete scope of
// the quoted premise. A module declaration may supply an input to the function.
func ReviewEffectEvidence(p, content string, changed, start, end int) (function, premise Citation, declaration bool) {
	pair := EnclosingReviewFunctions(p, content, changed, start)
	function = pair[0]
	if function.Path == "" || start < 1 || end < start {
		return function, Citation{}, false
	}
	if pair[1].Path != "" && pair[1].EndLine >= end {
		return function, pair[1], false
	}
	lines := strings.Split(content, "\n")
	text := strings.ReplaceAll(content, "\r\n", "\n")
	switch path.Ext(p) {
	case ".go":
		premise = goReviewConstant(p, text, lines, function.StartLine, start, end)
	case ".py":
		premise = pythonReviewAssignment(p, text, lines, function, start, end)
	}
	if premise.EndLine < end {
		return function, Citation{}, false
	}
	return function, premise, true
}

func reviewSourceSpan(p string, lines []string, start, end int) Citation {
	if start < 1 || end < start || end > len(lines) {
		return Citation{}
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	quote := strings.TrimSuffix(strings.Join(lines[start-1:end], "\n"), "\r")
	if len(quote) > maxReviewFunctionBytes {
		return Citation{}
	}
	return Citation{Kind: discoveryKind(p), Path: p, StartLine: start, EndLine: end, Quote: quote}
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
		if start, end := goReviewLines(positions, lines, function); start != 0 {
			definitions = append(definitions, discussionDefinition{kind: "func", start: start, end: end})
		}
	}
	return definitions
}

func goReviewLines(positions *token.FileSet, lines []string, node ast.Node) (int, int) {
	start := positions.PositionFor(node.Pos(), false)
	end := positions.PositionFor(node.End(), false)
	// A whole-line citation cannot isolate two statements on a shared edge.
	rest := strings.TrimSpace(lines[end.Line-1][end.Column-1:])
	if strings.TrimSpace(lines[start.Line-1][:start.Column-1]) != "" || rest != "" && !strings.HasPrefix(rest, "//") {
		return 0, 0
	}
	return start.Line, end.Line
}

func goReviewConstant(p, content string, lines []string, caller, start, end int) Citation {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, p, content, parser.DeclarationErrors)
	if err != nil {
		return Citation{}
	}
	references := map[*ast.ValueSpec]bool{}
	for _, decl := range file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Body == nil || positions.PositionFor(function.Pos(), false).Line != caller {
			continue
		}
		ast.PreorderStack(function.Body, nil, func(node ast.Node, parents []ast.Node) bool {
			if id, ok := node.(*ast.Ident); ok && id.Obj != nil && id.Obj.Kind == ast.Con {
				if pair, ok := parents[len(parents)-1].(*ast.KeyValueExpr); ok && pair.Key == id {
					literal, ok := parents[len(parents)-2].(*ast.CompositeLit)
					if !ok || !goReviewIndexedLiteral(literal.Type) {
						return true
					}
				}
				if spec, ok := id.Obj.Decl.(*ast.ValueSpec); ok {
					references[spec] = true
				}
			}
			return true
		})
		break
	}
	for _, node := range file.Decls {
		decl, ok := node.(*ast.GenDecl)
		if !ok || decl.Tok != token.CONST {
			continue
		}
		first, last := goReviewLines(positions, lines, decl)
		if first == 0 || start < first || end > last {
			continue
		}
		for _, raw := range decl.Specs {
			spec := raw.(*ast.ValueSpec)
			if references[spec] && positions.PositionFor(spec.Pos(), false).Line <= end &&
				positions.PositionFor(spec.End()-1, false).Line >= start {
				// A grouped constant can inherit both its expression and iota.
				return reviewSourceSpan(p, lines, first, last)
			}
		}
	}
	return Citation{}
}

func goReviewIndexedLiteral(expr ast.Expr) bool {
	seen := map[*ast.Object]bool{}
	for {
		switch t := ast.Unparen(expr).(type) {
		case *ast.ArrayType, *ast.MapType:
			return true
		case *ast.Ident:
			if t.Obj == nil || seen[t.Obj] {
				return false
			}
			spec, ok := t.Obj.Decl.(*ast.TypeSpec)
			if !ok {
				return false
			}
			seen[t.Obj], expr = true, spec.Type
		default:
			return false
		}
	}
}

func pythonReviewAssignment(p, text string, lines []string, function Citation, start, end int) Citation {
	module, err := parser2.ParseFile("evidence.py", text)
	if err != nil {
		return Citation{}
	}
	for i, stmt := range module.Body {
		var name *parser2.Name
		switch s := stmt.(type) {
		case *parser2.Assign:
			if len(s.Targets) == 1 {
				name, _ = s.Targets[0].(*parser2.Name)
			}
		case *parser2.AnnAssign:
			if s.Simple && s.Value != nil {
				name, _ = s.Target.(*parser2.Name)
			}
		}
		if name == nil {
			continue
		}
		first, last := pythonStatementStart(stmt, lines), len(lines)
		if i+1 < len(module.Body) {
			last = pythonStatementStart(module.Body[i+1], lines) - 1
		}
		if first < 1 || start < first || end > last || i > 0 && pythonStatementStart(module.Body[i-1], lines) == first {
			continue
		}
		binding := pythonModuleBindingAtUse(symbols2.Build(module).Root, name.Id, func(pos parser2.Pos) bool {
			return pos.Line >= function.StartLine && pos.Line <= function.EndLine
		})
		if binding != nil && binding.BindSites[0] == name.P {
			return reviewSourceSpan(p, lines, first, pythonDeclarationEnd(lines, first, last))
		}
	}
	return Citation{}
}

func pythonModuleBindingAtUse(root *symbols2.Scope, name string, matchesUse func(parser2.Pos) bool) *symbols2.Binding {
	module := root.Symbols[name]
	if module == nil || len(module.BindSites) != 1 {
		return nil
	}
	used := false
	for scopes := []*symbols2.Scope{root}; len(scopes) != 0; {
		scope := scopes[len(scopes)-1]
		scopes = append(scopes[:len(scopes)-1], scope.Children...)
		binding := scope.Symbols[name]
		if binding == nil {
			continue
		}
		if scope != root && binding.Has(symbols2.FlagGlobal) && binding.Has(symbols2.FlagBound) {
			return nil
		}
		if !used && slices.ContainsFunc(binding.UseSites, matchesUse) {
			resolved, _, owner := scope.Resolve(name)
			used = owner == root || resolved != nil && resolved.Has(symbols2.FlagGlobal)
		}
	}
	if used {
		return module
	}
	return nil
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
