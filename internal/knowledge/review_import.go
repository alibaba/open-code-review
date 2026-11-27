// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strings"

	"github.com/tamnd/gopapy/v2/parser2"
	"github.com/tamnd/gopapy/v2/symbols2"
)

// ImportedReviewCode contains the reviewed operation and a current local
// module it explicitly calls. The caller and target obey the same file policy
// and revision as the selected knowledge.
type ImportedReviewCode struct {
	Caller         Citation          `json:"caller"`
	Implementation Citation          `json:"implementation"`
	Imports        map[string]string `json:"called_imports"`
}

func (t Tree) ReviewImport(ctx context.Context, caller, reviewed string, line int, source string) (ImportedReviewCode, bool) {
	var result ImportedReviewCode
	if t.Git == nil || caller == source || !discussionDefinitionExt(caller) || !discussionDefinitionExt(source) {
		return result, false
	}
	runner, err := t.Git.LocalForRepository(ctx, t.Repo)
	if err != nil {
		return result, false
	}
	t.Git = runner
	r := newRechecker(ctx, t)
	entry := r.discussionDefinitionFile(caller)
	if !entry.parsed || strings.ReplaceAll(reviewed, "\r\n", "\n") != strings.ReplaceAll(entry.file.text, "\r\n", "\n") {
		return result, false
	}
	body := EnclosingReviewFunctions(caller, reviewed, line)[0]
	if body.Quote == "" {
		return result, false
	}
	target := r.discussionDefinitionFile(source)
	if !target.parsed || len(target.file.data) > maxReviewFunctionBytes {
		return result, false
	}
	result.Imports = map[string]string{}
	for _, name := range reviewCallNames(caller, entry.file.text, line) {
		if len(r.discussionImportTargets(caller, name)) != 1 {
			continue
		}
		binding, def, ok := r.scopedDiscussionDefinition([]string{caller, source}, caller, name)
		if ok && binding.Bound && binding.Path == source {
			result.Imports[name] = def.name
		}
	}
	if len(result.Imports) == 0 {
		return result, false
	}
	text := strings.TrimSuffix(strings.ReplaceAll(target.file.text, "\r\n", "\n"), "\n")
	result.Caller = body
	result.Implementation = Citation{Kind: "code", Path: source, StartLine: 1, EndLine: 1 + strings.Count(text, "\n"), Quote: text}
	return result, true
}

// Read the call at its physical source location. Go and Python also expose
// local bindings that can shadow an imported name.
func reviewCallNames(p, text string, line int) []string {
	var found []string
	switch path.Ext(p) {
	case ".go":
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, p, text, 0)
		if err != nil {
			return nil
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || fset.PositionFor(call.Fun.Pos(), false).Line != line {
				return true
			}
			if qualified, ok := call.Fun.(*ast.SelectorExpr); ok {
				if recv, ok := qualified.X.(*ast.Ident); ok && recv.Obj == nil {
					found = append(found, recv.Name+"."+qualified.Sel.Name)
				}
			}
			return true
		})
		return found
	}
	var bindings *symbols2.Scope
	if path.Ext(p) == ".py" {
		module, err := parser2.ParseFile(p, text)
		if err != nil {
			return nil
		}
		bindings = symbols2.Build(module).Root
	} else {
		code, ok := maskJavaScript(text)
		if !ok {
			return nil
		}
		text = string(code)
	}
	lines := strings.Split(text, "\n")
	if line < 1 || line > len(lines) {
		return nil
	}
	changed := lines[line-1]
	for _, loc := range discussionSymbolToken.FindAllStringIndex(changed, -1) {
		if !strings.HasPrefix(strings.TrimLeft(changed[loc[1]:], " \t"), "(") {
			continue
		}
		name := changed[loc[0]:loc[1]]
		if bindings != nil {
			base, _, _ := strings.Cut(name, ".")
			b := pythonModuleBindingAtUse(bindings, base, func(site parser2.Pos) bool { return site.Line == line && site.Col == loc[0] })
			if b == nil || !b.Has(symbols2.FlagImport) {
				continue
			}
		}
		found = append(found, name)
	}
	return found
}
