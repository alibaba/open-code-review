// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"slices"
	"strings"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/tamnd/gopapy/v2/parser2"
	"github.com/tamnd/gopapy/v2/symbols2"
)

// ImportedReviewCode contains the reviewed operation and a current local
// module it explicitly calls. The caller and target obey the same file policy
// and revision as the selected knowledge.
type ImportedReviewCode struct {
	Caller             Citation          `json:"caller"`
	Implementation     Citation          `json:"implementation"`
	BaseCaller         *Citation         `json:"base_caller,omitempty"`
	BaseImplementation *Citation         `json:"base_implementation,omitempty"`
	BaseImports        map[string]string `json:"base_called_imports,omitempty"`
	Imports            map[string]string `json:"called_imports"`
	baseCall           string
	baseDefinition     Citation
}

func (t Tree) ReviewImport(ctx context.Context, base string, change model.Diff, line int, source string) (ImportedReviewCode, bool) {
	var result ImportedReviewCode
	caller, reviewed := change.NewPath, change.NewFileContent
	if t.Git == nil || !validOID(base) || caller == source || !discussionDefinitionExt(caller) || !discussionDefinitionExt(source) {
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
	var currentDefinition string
	for _, name := range reviewImportedCallNames(body, entry.file.text, line) {
		if len(r.discussionImportTargets(caller, name)) != 1 {
			continue
		}
		binding, def, ok := r.scopedDiscussionDefinition([]string{caller, source}, caller, name)
		if ok && binding.Bound && binding.Path == source {
			result.Imports[name] = def.name
			currentDefinition = binding.Key
		}
	}
	if len(result.Imports) == 0 {
		return result, false
	}
	result.Caller = body
	result.Implementation = reviewImportedModule(source, target.file.text)
	if !r.atRevision(base).readImportBase(&result, entry, change, line, currentDefinition, !t.Workspace && t.Commit == base) {
		return ImportedReviewCode{}, false
	}
	return result, true
}

func reviewImportedModule(p, content string) Citation {
	text := strings.TrimSuffix(content, "\n")
	return Citation{Kind: "code", Path: p, StartLine: 1, EndLine: 1 + strings.Count(text, "\n"), Quote: text}
}

func reviewImportedCallNames(body Citation, file string, line int) []string {
	switch path.Ext(body.Path) {
	case ".js", ".mjs", ".cjs":
		return reviewJavaScriptCalls(body, line)
	default:
		return reviewCallNames(body.Path, file, line)
	}
}

func reviewCallerIdentity(file discussionDefinitionFile, body Citation) string {
	for _, d := range file.definitions {
		if !d.unquotable && d.start == body.StartLine && d.end >= body.EndLine &&
			(d.kind == "func" || d.kind == "method" || d.kind == "def" || d.kind == "async def" || strings.HasPrefix(d.kind, "function")) {
			return d.kind + " " + d.name
		}
	}
	return ""
}

func (r *rechecker) readImportBase(result *ImportedReviewCode, head discussionDefinitionFile, change model.Diff, line int, currentDefinition string, sameTree bool) bool {
	if change.IsNew {
		return false
	}
	oldPath := change.OldPath
	if oldPath == "" && !change.IsRenamed {
		oldPath = change.NewPath
	}
	entry := r.discussionDefinitionFile(oldPath)
	if !entry.parsed {
		return false
	}
	oldLines := strings.Split(entry.file.text, "\n")
	oldLine := line
	var before, after importLineBlock
	if !sameTree || oldPath != change.NewPath {
		before, after = oldImportBlocks(change.Diff, line)
		if !before.matches(oldLines) || !after.matches(strings.Split(head.file.text, "\n")) {
			return false
		}
		oldLine = before.start
	}
	baseCaller := EnclosingReviewFunctions(oldPath, entry.file.text, oldLine)[0]
	if identity := reviewCallerIdentity(entry, baseCaller); baseCaller.Path == "" || identity == "" || identity != reviewCallerIdentity(head, result.Caller) {
		return false
	}
	pairedReturn := len(before.lines) > 1 || len(after.lines) > 1
	if pairedReturn {
		oldLine = before.uniqueReturn(baseCaller)
		if oldLine == 0 || after.uniqueReturn(result.Caller) != line {
			return false
		}
	}
	baseCaller.Revision = r.tree.Commit
	result.baseCall = strings.TrimSpace(oldLines[oldLine-1])
	binding, definition, resolved := r.reviewBaseImport(oldPath, reviewImportedCallNames(baseCaller, entry.file.text, oldLine))
	if !resolved {
		return false
	}
	if binding.Path == "" {
		// A direct return can establish the old result without an imported implementation.
		if !strings.HasPrefix(result.baseCall, "return ") || strings.ContainsAny(result.baseCall, "()") || !reviewExecutableLine(baseCaller, oldLine) {
			return false
		}
		result.BaseCaller = &baseCaller
		return true
	}
	file := r.discussionDefinitionFile(binding.Path)
	if len(file.file.data) > maxReviewFunctionBytes {
		return false
	}
	result.baseDefinition = EnclosingReviewFunctions(binding.Path, file.file.text, definition.start)[0]
	old := reviewImportedModule(binding.Path, file.file.text)
	changedModule := old.Path != result.Implementation.Path || old.Quote != result.Implementation.Quote
	if changedModule {
		old.Revision = r.tree.Commit
		result.BaseImplementation = &old
	}
	if changedModule || pairedReturn || len(result.Imports) != 1 || binding.Key != currentDefinition {
		result.BaseCaller = &baseCaller
		result.BaseImports = map[string]string{binding.Mention: definition.name}
	}
	return true
}

func (r *rechecker) reviewBaseImport(caller string, calls []string) (discussionNamedBinding, discussionDefinition, bool) {
	var binding discussionNamedBinding
	var definition discussionDefinition
	for _, name := range calls {
		targets := r.discussionImportTargets(caller, name)
		if len(targets) != 1 {
			return discussionNamedBinding{}, discussionDefinition{}, false
		}
		target := targets[0]
		paths := target.paths
		if !target.goPackage {
			paths = slices.DeleteFunc(slices.Clone(paths), func(p string) bool { return !r.discussionDefinitionFile(p).parsed })
		}
		b, d, ok := r.scopedDiscussionDefinition(paths, caller, name)
		if !ok || !b.Bound || binding.Path != "" && (binding.Path != b.Path || binding.Key != b.Key) {
			return discussionNamedBinding{}, discussionDefinition{}, false
		}
		binding, definition = b, d
	}
	return binding, definition, true
}

type importLineBlock struct {
	start int
	lines []string
}

func (b importLineBlock) matches(source []string) bool {
	if b.start < 1 || b.start > len(source) || len(b.lines) == 0 || len(b.lines) > len(source)-b.start+1 {
		return false
	}
	for i, text := range b.lines {
		if strings.TrimSuffix(text, "\r") != source[b.start-1+i] {
			return false
		}
	}
	return true
}

func oldImportBlocks(patch string, line int) (importLineBlock, importLineBlock) {
	for _, h := range diff.ParseHunks(patch) {
		if line < h.NewStart || line-h.NewStart >= h.NewCount {
			continue
		}
		old, current := h.OldStart, h.NewStart
		for i := 0; i < len(h.Lines); {
			if l := h.Lines[i]; l.Type == diff.HunkContext {
				if current == line {
					return importLineBlock{old, []string{l.Content}}, importLineBlock{current, []string{l.Content}}
				}
				old++
				current++
				i++
				continue
			}
			before, after := importLineBlock{start: old}, importLineBlock{start: current}
			for ; i < len(h.Lines) && h.Lines[i].Type != diff.HunkContext; i++ {
				if l := h.Lines[i]; l.Type == diff.HunkDeleted {
					before.lines = append(before.lines, l.Content)
					old++
				} else {
					after.lines = append(after.lines, l.Content)
					current++
				}
			}
			if line >= after.start && line < current {
				return before, after
			}
		}
	}
	return importLineBlock{}, importLineBlock{}
}

func (b importLineBlock) uniqueReturn(caller Citation) int {
	end := b.start + len(b.lines) - 1
	if b.start < caller.StartLine || end > caller.EndLine {
		return 0
	}
	found := 0
	record := func(relative int) bool {
		line := caller.StartLine + relative - 1
		if line < b.start || line > end {
			return false
		}
		if found != 0 {
			found = -1
			return true
		}
		found = line
		return false
	}
	switch path.Ext(caller.Path) {
	case ".go":
		positions, f := goReviewCallable(caller)
		if f == nil {
			return 0
		}
		ast.Inspect(f.Body, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.FuncLit:
				return false
			case *ast.ReturnStmt:
				return !record(positions.PositionFor(n.Pos(), false).Line - 1)
			}
			return found >= 0
		})
	case ".py":
		walkPythonReviewStatements(caller.Quote, func(s parser2.Stmt, line int) bool {
			_, ok := s.(*parser2.Return)
			return ok && record(line)
		})
	default:
		body, bodyLine := javascriptReviewBody(caller.Quote)
		depth, _, ok := javascriptNesting(body)
		if len(body) == 0 || !ok {
			return 0
		}
		lines := token.NewFileSet().AddFile(caller.Path, -1, len(body))
		lines.SetLinesForContent(body)
		for _, loc := range discussionSymbolToken.FindAllIndex(body, -1) {
			if !bytes.Equal(body[loc[0]:loc[1]], []byte("return")) {
				continue
			}
			line := bodyLine + lines.Line(lines.Pos(loc[0])) - 1
			actual := caller.StartLine + line - 1
			// A nested JavaScript return may belong to a callback.
			if actual >= b.start && actual <= end && (depth[loc[0]] != 0 || record(line)) {
				return 0
			}
		}
	}
	if found > 0 {
		text := strings.TrimSpace(b.lines[found-b.start])
		if site := discussionSymbolToken.FindStringIndex(text); site != nil && site[0] == 0 && text[:site[1]] == "return" {
			return found
		}
	}
	return 0
}

// CitesBaseQuote confines the old premise to the actual call or its definition.
func (c ImportedReviewCode) CitesBaseQuote(quote string) bool {
	quote = strings.TrimSpace(quote)
	if quote == "" || strings.ContainsAny(quote, "\r\n") {
		return false
	}
	if quote == c.baseCall {
		return true
	}
	span := c.baseDefinition
	line := 0
	for i, source := range strings.Split(span.Quote, "\n") {
		if strings.TrimSpace(source) == quote {
			if line != 0 {
				return false
			}
			line = span.StartLine + i
		}
	}
	return line != 0 && reviewExecutableLine(span, line)
}

func reviewExecutableLine(code Citation, line int) bool {
	line -= code.StartLine - 1
	switch path.Ext(code.Path) {
	case ".go":
		pos, f := goReviewCallable(code)
		if f == nil {
			return false
		}
		found := false
		ast.Inspect(f.Body, func(n ast.Node) bool {
			switch n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.EmptyStmt, *ast.BlockStmt:
			case ast.Stmt:
				found = found || pos.PositionFor(n.Pos(), false).Line == line+1
			}
			return !found
		})
		return found
	case ".py":
		return walkPythonReviewStatements(code.Quote, func(_ parser2.Stmt, at int) bool { return at == line })
	default:
		return javascriptExecutableReviewLine(code.Quote, line)
	}
}

func goReviewCallable(code Citation) (*token.FileSet, *ast.FuncDecl) {
	positions := token.NewFileSet()
	module, err := parser.ParseFile(positions, code.Path, "package evidence\n"+code.Quote, parser.SkipObjectResolution)
	if err == nil && len(module.Decls) == 1 {
		if f, ok := module.Decls[0].(*ast.FuncDecl); ok && f.Body != nil {
			return positions, f
		}
	}
	return nil, nil
}

func walkPythonReviewStatements(text string, visit func(parser2.Stmt, int) bool) bool {
	indented := strings.HasPrefix(text, " ") || strings.HasPrefix(text, "\t")
	shift := 0
	if indented {
		text, shift = "if True:\n"+text, 1
	}
	module, err := parser2.ParseFile("base.py", text+"\n")
	if err != nil || len(module.Body) != 1 {
		return false
	}
	stmts := module.Body
	if indented {
		suite, ok := stmts[0].(*parser2.If)
		if !ok || len(suite.Body) != 1 {
			return false
		}
		stmts = suite.Body
	}
	switch f := stmts[0].(type) {
	case *parser2.FunctionDef:
		stmts = f.Body
	case *parser2.AsyncFunctionDef:
		stmts = f.Body
	default:
		return false
	}
	for stmts = slices.Clone(stmts); len(stmts) != 0; {
		s := stmts[0]
		stmts = stmts[1:]
		switch n := s.(type) {
		case *parser2.FunctionDef, *parser2.AsyncFunctionDef, *parser2.ClassDef:
			continue
		case *parser2.ExprStmt:
			if _, literal := n.Value.(*parser2.Constant); literal {
				continue
			}
		case *parser2.If:
			stmts = append(stmts, n.Body...)
			stmts = append(stmts, n.Orelse...)
		case *parser2.While:
			stmts = append(stmts, n.Body...)
			stmts = append(stmts, n.Orelse...)
		case *parser2.For:
			stmts = append(stmts, n.Body...)
			stmts = append(stmts, n.Orelse...)
		case *parser2.AsyncFor:
			stmts = append(stmts, n.Body...)
			stmts = append(stmts, n.Orelse...)
		case *parser2.With:
			stmts = append(stmts, n.Body...)
		case *parser2.AsyncWith:
			stmts = append(stmts, n.Body...)
		case *parser2.Try:
			stmts = append(stmts, n.Body...)
			stmts = append(stmts, n.Orelse...)
			stmts = append(stmts, n.Finalbody...)
			for _, handler := range n.Handlers {
				stmts = append(stmts, handler.Body...)
			}
		case *parser2.Match:
			for _, c := range n.Cases {
				stmts = append(stmts, c.Body...)
			}
		}
		if visit(s, pythonNodeLine(s)-shift) {
			return true
		}
	}
	return false
}

func javascriptExecutableReviewLine(text string, line int) bool {
	body, first := javascriptReviewBody(text)
	lines := bytes.Split(body, []byte{'\n'})
	relative := line - first
	return relative >= 0 && relative < len(lines) && slices.ContainsFunc(discussionSymbolToken.FindAll(lines[relative], -1), func(symbol []byte) bool {
		return !bytes.Equal(symbol, []byte{'_'})
	})
}

func javascriptReviewBody(text string) ([]byte, int) {
	code, ok := maskJavaScript(text)
	if !ok {
		return nil, 0
	}
	depth, closes, ok := javascriptNesting(code)
	if !ok {
		return nil, 0
	}
	start, kind := 0, "method"
	if m := javascriptDeclaration.FindSubmatchIndex(code); m != nil && depth[m[2]] == 0 {
		start, kind = m[7], string(code[m[4]:m[5]])
	} else if m := javascriptMethod.FindSubmatchIndex(code); m != nil {
		start = m[1] - 1
	}
	if start == 0 {
		return nil, 0
	}
	open, end, ok := javascriptDefinitionEnd(code, depth, closes, start, kind)
	if !ok || code[open] != '{' {
		return nil, 0
	}
	return code[open+1 : end-1], 1 + bytes.Count(code[:open], []byte{'\n'})
}

// Read calls at their physical source location and rule out local shadows.
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
	module, err := parser2.ParseFile(p, text)
	if err != nil {
		return nil
	}
	bindings := symbols2.Build(module).Root
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
		base, _, _ := strings.Cut(name, ".")
		b := pythonModuleBindingAtUse(bindings, base, func(site parser2.Pos) bool { return site.Line == line && site.Col == loc[0] })
		if b == nil || !b.Has(symbols2.FlagImport) {
			continue
		}
		found = append(found, name)
	}
	return found
}

func reviewJavaScriptCalls(body Citation, line int) []string {
	code, ok := maskJavaScript(body.Quote)
	if !ok || bytes.IndexByte(code, '\\') >= 0 {
		return nil
	}
	_, closes, ok := javascriptNesting(code)
	if !ok {
		return nil
	}
	positions := token.NewFileSet().AddFile("review.js", -1, len(code))
	positions.SetLinesForContent(code)
	var calls []string
	uncertain := map[string]bool{}
	for _, loc := range discussionSymbolToken.FindAllIndex(code, -1) {
		name := string(code[loc[0]:loc[1]])
		base, _, _ := strings.Cut(name, ".")
		prefix := bytes.TrimRight(code[:loc[0]], " \t\r\n\v\f")
		if n := len(prefix); n > 0 && (prefix[n-1] == '.' || prefix[n-1] == '#') {
			continue
		}
		open := nextJavaScriptToken(code, loc[1])
		close, bracketed := closes[open]
		// A non-call use can bind a parameter, local or destructured name.
		if !bracketed || code[open] != '(' {
			uncertain[base] = true
			continue
		}
		after := nextJavaScriptToken(code, close+1)
		if after < len(code) && (code[after] == '{' || bytes.HasPrefix(code[after:], []byte("=>"))) {
			uncertain[base] = true
			continue
		}
		callLine := positions.Line(positions.Pos(loc[0]))
		if callLine+body.StartLine-1 == line && positions.Line(positions.Pos(open)) == callLine {
			calls = append(calls, name)
		}
	}
	return slices.DeleteFunc(calls, func(name string) bool {
		base, _, _ := strings.Cut(name, ".")
		return uncertain[base]
	})
}
