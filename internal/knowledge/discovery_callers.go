// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"context"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/tamnd/gopapy/v2/parser2"
	"github.com/tamnd/gopapy/v2/symbols2"
)

type discoveryCaller struct {
	Function    string `json:"function"`
	Callee      string `json:"callee"`
	CalleePath  string `json:"callee_path"`
	CallLine    int    `json:"call_line"`
	ResultLine  int    `json:"result_line,omitempty"`
	Role        string `json:"role"`
	first, last int
}

type focusedFunction struct {
	path, name  string
	first, last int
}

const maxCallerQuote = 800

// Search the same admitted, proximity-ranked tree used for ordinary sampling.
// Git narrows the paths by spelling. A parsed call and a resolved local import
// must agree before a file gets one of the current-source slots.
func (t Tree) discoveryCallers(ctx context.Context, opts DiscoverOptions, ranked []discoveryBlob, files map[string]discoveryBlob, result *DiscoveryResult) (map[string][]discoverySource, map[string]bool, error) {
	callers := map[string][]discoverySource{}
	inspected := map[string]bool{}
	if len(opts.FocusLines) == 0 {
		return callers, inspected, nil
	}
	t.AllowPath = opts.allowed
	r := newRechecker(ctx, t)
	var targets []focusedFunction
	linked := map[focusedFunction]bool{}
	for _, b := range ranked {
		focus := opts.FocusLines[b.path]
		if len(focus) == 0 || path.Ext(b.path) != ".py" || discoveryKind(b.path) != "code" || b.size > maxDiscussionDefinitionBytes {
			continue
		}
		if len(r.files) >= 4 || len(targets) >= 4 {
			result.limit("sampled up to four focused Python functions for callers")
			break
		}
		file := r.discussionDefinitionFile(b.path)
		if !file.parsed {
			continue
		}
		module, err := parser2.ParseFile(b.path, file.file.text)
		if err != nil || pythonWildcardImport(module) {
			continue
		}
		root := symbols2.Build(module).Root
		for _, d := range file.definitions {
			if len(targets) == 4 {
				break
			}
			if !slices.ContainsFunc(focus[:min(len(focus), 32)], func(line int) bool { return d.start <= line && line <= d.end }) {
				continue
			}
			if r.focusedPythonDefinition(b.path, d, root) {
				targets = append(targets, focusedFunction{b.path, d.name, d.start, d.end})
			}
		}
	}
	if len(targets) != 0 {
		index := discussionNameIndex{reader: r, paths: callerSearchPaths(ranked, targets, opts, result), hits: map[string][]string{}}
		hits := make([][]string, len(targets))
		rounds, pairs := 0, 0
		for i, target := range targets {
			hits[i] = index.matchingPaths(target.name, false)
			rounds = max(rounds, len(hits[i]))
		}
		for round := range rounds {
			for i, target := range targets {
				if round >= len(hits[i]) {
					continue
				}
				p := hits[i][round]
				if _, inspected := r.files[p]; !inspected && len(r.files) >= 16 || pairs == 4 {
					result.limit("bounded inspection of accepted caller candidates")
					continue
				}
				if evidence := r.pythonCallerSources(p, target, "", func(p string) bool { return files[p].oid != "" }); len(evidence) != 0 && len(callers[p])+len(evidence) <= 4 {
					// Four excerpts of at most 800 bytes share the usual 3200-byte file budget.
					callers[p] = append(callers[p], evidence...)
					linked[target] = true
					pairs++
				}
			}
		}
	}
	for p, file := range r.files {
		if file.err != nil {
			return nil, nil, file.err
		}
		inspected[p] = true
	}
	// A generic window can stop between a calculation and its return.
	// Keep complete linked declarations when together they cover the focus.
	byPath := map[string][]focusedFunction{}
	for _, target := range targets {
		if linked[target] {
			byPath[target.path] = append(byPath[target.path], target)
		}
	}
	for p, declarations := range byPath {
		if slices.ContainsFunc(opts.FocusLines[p], func(line int) bool {
			return !slices.ContainsFunc(declarations, func(d focusedFunction) bool { return d.first <= line && line <= d.last })
		}) {
			continue
		}
		lines := strings.Split(r.files[p].text, "\n")
		var excerpts []discoverySource
		size := 0
		for _, d := range declarations {
			text := strings.TrimRight(strings.Join(lines[d.first-1:d.last], "\n"), "\n")
			size += len(text)
			excerpts = append(excerpts, discoverySource{Kind: "code", Path: p, Revision: t.Commit, StartLine: d.first, Text: text})
		}
		if size <= 3200 {
			callers[p] = excerpts
		}
	}
	return callers, inspected, ctx.Err()
}

// Each focused module gets a turn before a large, unrelated subtree can
// exhaust the shared Git path, file-count, or byte budget.
func callerSearchPaths(ranked []discoveryBlob, targets []focusedFunction, opts DiscoverOptions, result *DiscoveryResult) []string {
	var eligible []discoveryBlob
	for _, b := range ranked {
		if path.Ext(b.path) == ".py" && discoveryKind(b.path) == "code" && b.size <= maxDiscussionDefinitionBytes && len(opts.FocusLines[b.path]) == 0 {
			eligible = append(eligible, b)
		}
	}
	directories := map[string]bool{}
	var queues [][]discoveryBlob
	for _, target := range targets {
		dir := path.Dir(target.path)
		if directories[dir] {
			continue
		}
		directories[dir] = true
		var siblings, rest []discoveryBlob
		for _, b := range eligible {
			if path.Dir(b.path) == dir {
				siblings = append(siblings, b)
			} else {
				rest = append(rest, b)
			}
		}
		queues = append(queues, append(siblings, rankDiscoveryFiles(rest, nil, []string{target.path}, 1)...))
	}
	var paths []string
	seen := map[string]bool{}
	size, pathBytes := 0, 0
	for {
		before := len(paths)
		for i, queue := range queues {
			for len(queue) != 0 && seen[queue[0].path] {
				queue = queue[1:]
			}
			queues[i] = queue
			if len(queue) == 0 {
				continue
			}
			b := queue[0]
			size, pathBytes = size+b.size, pathBytes+len(b.path)+3
			if len(paths) == maxDiscussionIndexFiles || size > maxDiscussionIndexBytes || pathBytes > 12<<10 {
				result.limit("sampled nearby modules for accepted callers")
				return paths
			}
			seen[b.path] = true
			paths = append(paths, b.path)
			queues[i] = queue[1:]
		}
		if len(paths) == before {
			return paths
		}
	}
}

func pythonWildcardImport(module *parser2.Module) bool {
	return slices.ContainsFunc(module.Body, func(stmt parser2.Stmt) bool {
		imp, ok := stmt.(*parser2.ImportFrom)
		return ok && slices.ContainsFunc(imp.Names, func(a *parser2.Alias) bool { return a.Name == "*" })
	})
}

func (r *rechecker) focusedPythonDefinition(p string, d discussionDefinition, root *symbols2.Scope) bool {
	binding := root.Symbols[d.name]
	if d.unquotable || d.kind != "def" && d.kind != "async def" || strings.Contains(d.name, ".") ||
		binding == nil || len(binding.BindSites) != 1 || slices.ContainsFunc(root.Children, func(s *symbols2.Scope) bool {
		return s.Name == d.name && slices.ContainsFunc(s.Children, func(n *symbols2.Scope) bool {
			return n.Kind == symbols2.ScopeFunction || n.Kind == symbols2.ScopeClass
		})
	}) {
		return false
	}
	_, _, unique := r.findDiscussionDefinition([]string{p}, d.name, false)
	return unique
}

type callerUse struct {
	source discoverySource
	rank   int
}

type callerFlow struct {
	call discoverySource
	uses []callerUse
}

func (u callerUse) preferredTo(other callerUse) bool {
	if other.source.Caller == nil {
		return true
	}
	a, b := u.source.Caller, other.source.Caller
	return u.rank > other.rank || u.rank == other.rank && a.ResultLine-a.CallLine < b.ResultLine-b.CallLine
}

func (r *rechecker) pythonCallerSources(p string, target focusedFunction, caller string, admitted func(string) bool) []discoverySource {
	var best []discoverySource
	var chosen callerUse
	for _, flow := range r.pythonCallerFlows(p, target, caller, admitted) {
		if len(best) == 0 {
			best = []discoverySource{flow.call}
		}
		for _, u := range flow.uses {
			if u.preferredTo(chosen) {
				best, chosen = []discoverySource{flow.call, u.source}, u
			}
		}
	}
	return best
}

func (r *rechecker) pythonCallerFlows(p string, target focusedFunction, caller string, admitted func(string) bool) []callerFlow {
	file := r.discussionDefinitionFile(p)
	if !file.parsed {
		return nil
	}
	module, err := parser2.ParseFile(p, file.file.text)
	if err != nil || pythonWildcardImport(module) {
		return nil
	}
	moduleName := strings.TrimSuffix(path.Base(target.path), ".py")
	if moduleName == "__init__" {
		moduleName = path.Base(path.Dir(target.path))
	}
	names := map[string]bool{}
	for _, stmt := range module.Body {
		imp, ok := stmt.(*parser2.ImportFrom)
		if !ok {
			continue
		}
		for _, alias := range imp.Names {
			local := alias.Asname
			if local == "" {
				local = alias.Name
			}
			if alias.Name == target.name {
				names[local] = true
			}
			if alias.Name == moduleName {
				names[local+"."+target.name] = true
			}
		}
	}
	for name := range names {
		if !r.importsFocusedFunction(p, name, target, admitted) {
			delete(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	root := symbols2.Build(module).Root
	lines := strings.Split(file.file.text, "\n")
	var flows []callerFlow
	for _, stmt := range module.Body {
		var name string
		var body []parser2.Stmt
		var pos parser2.Pos
		switch d := stmt.(type) {
		case *parser2.FunctionDef:
			name, body, pos = d.Name, d.Body, d.P
		case *parser2.AsyncFunctionDef:
			name, body, pos = d.Name, d.Body, d.P
		default:
			continue
		}
		if caller != "" && caller != name || root.Symbols[name] == nil || len(root.Symbols[name].BindSites) != 1 {
			continue
		}
		scope := slices.IndexFunc(root.Children, func(s *symbols2.Scope) bool { return s.Kind == symbols2.ScopeFunction && s.Pos == pos })
		if scope < 0 {
			continue
		}
		for _, statement := range body {
			call, local, value := pythonResultCall(statement)
			if local == nil || !names[call] || pythonModuleBindingAtUse(root, local.Id, func(pos parser2.Pos) bool { return pos == local.P }) == nil {
				continue
			}
			line := pythonNodeLine(statement)
			link := discoveryCaller{Function: name, Callee: target.name, CalleePath: target.path, CallLine: local.P.Line, Role: "call", first: target.first, last: target.last}
			source := discoverySource{Kind: "code", Path: p, Revision: r.tree.Commit, Caller: &link}
			at := callerExcerpt(source, lines, line, local.P.Line)
			if at.Text == "" {
				continue
			}
			flows = append(flows, callerFlow{call: at})
			flow := &flows[len(flows)-1]
			if value == nil {
				continue
			}
			binding := root.Children[scope].Symbols[value.Id]
			if binding == nil || binding.Has(symbols2.FlagGlobal|symbols2.FlagNonlocal) || len(binding.BindSites) != 1 || binding.BindSites[0] != value.P {
				continue
			}
			for _, use := range binding.UseSites {
				if use.Line <= local.P.Line || use.Line > len(lines) {
					continue
				}
				// Prefer the result itself to a property read on that result.
				raw := lines[use.Line-1]
				if use.Col < 0 || !strings.HasPrefix(raw[min(use.Col, len(raw)):], value.Id) ||
					strings.HasPrefix(strings.TrimLeft(raw[use.Col+len(value.Id):], " \t"), ".") {
					continue
				}
				begin, _ := slices.BinarySearchFunc(body, use.Line, func(s parser2.Stmt, n int) int { return pythonNodeLine(s) - n })
				if begin == len(body) || pythonNodeLine(body[begin]) != use.Line {
					begin--
				}
				if begin < 0 {
					continue
				}
				rank, end := pythonResultUse(body[begin], use)
				destination := link
				destination.Role, destination.ResultLine = "result_use", use.Line
				source.Caller = &destination
				useSite := callerExcerpt(source, lines, pythonNodeLine(body[begin]), end)
				if useSite.Text == "" || useSite.StartLine > use.Line {
					useSite = callerExcerpt(source, lines, use.Line, use.Line)
				}
				if useSite.Text != "" {
					flow.uses = append(flow.uses, callerUse{source: useSite, rank: rank})
				}
			}
		}
	}
	return flows
}

// A guard, a return, or an assignment shows how the caller acts on the
// result. A bare expression may only observe it before that decision.
func pythonResultUse(stmt parser2.Stmt, use parser2.Pos) (priority, end int) {
	var test parser2.Expr
	var body []parser2.Stmt
	switch s := stmt.(type) {
	case *parser2.If:
		test, body = s.Test, s.Body
	case *parser2.While:
		test, body = s.Test, s.Body
	case *parser2.Return, *parser2.Assign, *parser2.AnnAssign:
		return 1, use.Line
	default:
		return 0, use.Line
	}
	if len(body) != 0 {
		first := pythonNodeLine(body[0])
		name, direct := test.(*parser2.Name)
		if use.Line < first || direct && name.P == use {
			return 2, max(use.Line, first)
		}
	}
	return 0, use.Line
}

func pythonResultCall(stmt parser2.Stmt) (string, *parser2.Name, *parser2.Name) {
	var expr, target parser2.Expr
	switch s := stmt.(type) {
	case *parser2.Assign:
		if len(s.Targets) != 1 {
			return "", nil, nil
		}
		expr, target = s.Value, s.Targets[0]
	case *parser2.AnnAssign:
		expr, target = s.Value, s.Target
	case *parser2.Return:
		expr = s.Value
	}
	name, local := pythonInvokedName(expr)
	if local == nil {
		return "", nil, nil
	}
	value, _ := target.(*parser2.Name)
	return name, local, value
}

func pythonInvokedName(expr parser2.Expr) (string, *parser2.Name) {
	if await, ok := expr.(*parser2.Await); ok {
		expr = await.Value
	}
	call, ok := expr.(*parser2.Call)
	if !ok {
		return "", nil
	}
	switch f := call.Func.(type) {
	case *parser2.Name:
		return f.Id, f
	case *parser2.Attribute:
		if base, ok := f.Value.(*parser2.Name); ok {
			return base.Id + "." + f.Attr, base
		}
	}
	return "", nil
}

func (r *rechecker) importsFocusedFunction(caller, spelling string, target focusedFunction, admitted func(string) bool) bool {
	imports := r.discussionImportTargets(caller, spelling)
	if len(imports) != 1 {
		return false
	}
	imp := imports[0]
	if imp.symbol == target.name {
		return slices.Contains(imp.paths, target.path) && !r.alternativeImportPath(imp.paths, target.path)
	}
	// "from .pkg import module" can resolve an attribute of pkg instead
	// of the submodule. Inspect its admitted initializer before binding.
	member, function, qualified := strings.Cut(imp.symbol, ".")
	if !qualified || function != target.name {
		return false
	}
	for _, init := range imp.paths {
		if path.Base(init) != "__init__.py" || !admitted(init) || r.alternativeImportPath(imp.paths, init) {
			continue
		}
		base := path.Join(path.Dir(init), member)
		alternatives := []string{base + ".py", path.Join(base, "__init__.py")}
		if !slices.Contains(alternatives, target.path) || r.alternativeImportPath(alternatives, target.path) {
			continue
		}
		pkg := r.file(init)
		if pkg.err != nil || len(pkg.data) > maxDiscussionDefinitionBytes || !utf8.Valid(pkg.data) ||
			bytes.IndexByte(pkg.data, 0) >= 0 || discoveryCredential.Match(pkg.data) {
			return false
		}
		module, err := parser2.ParseFile(init, pkg.text)
		if err != nil || pythonWildcardImport(module) {
			return false
		}
		bindings := symbols2.Build(module).Root.Symbols
		return bindings[member] == nil && bindings["__getattr__"] == nil
	}
	return false
}

// Include the enclosing statement when the complete lines fit together.
func callerExcerpt(source discoverySource, lines []string, start, end int) discoverySource {
	if start < 1 || end < start || end > len(lines) || len(lines[end-1]) > maxCallerQuote {
		return discoverySource{}
	}
	if end-start > 8 || len(strings.Join(lines[start-1:end], "\n")) > maxCallerQuote {
		start = end
	}
	source.StartLine, source.Text = start, strings.Join(lines[start-1:end], "\n")
	return source
}

// Only reorder quotes actually chosen by the extractor for a demonstrated
// caller/callee pair. These are the two current quotes shown during review.
func (r discoveryRule) preferCallerCitations(f *Fact, sources map[string]discoverySource) {
	if f.Category != CategoryBusinessRule && f.Category != CategoryDomainConstraint {
		return
	}
	for _, role := range []string{"result_use", "call"} {
		for _, q := range r.Citations {
			caller := sources[q.Source].Caller
			if caller == nil || caller.Role != role {
				continue
			}
			callee := slices.IndexFunc(f.Citations, func(c Citation) bool {
				return c.live() && c.Path == caller.CalleePath && c.StartLine >= caller.first && c.EndLine <= caller.last
			})
			c, err := q.cite(sources)
			line := caller.CallLine
			if role == "result_use" {
				line = caller.ResultLine
			}
			if callee < 0 || err != nil || line < c.StartLine || line > c.EndLine {
				continue
			}
			consumer := slices.IndexFunc(f.Citations, func(x Citation) bool {
				return x.Kind == "code" && x.Path == c.Path && x.StartLine == c.StartLine && x.EndLine == c.EndLine && x.Quote == c.Quote
			})
			if consumer < 0 {
				continue
			}
			first, second := f.Citations[callee], f.Citations[consumer]
			rest := slices.DeleteFunc(slices.Clone(f.Citations), func(x Citation) bool { return x == first || x == second })
			second.Symbol = caller.Function
			f.AcceptedCall = &AcceptedCall{CallerPath: second.Path, Caller: caller.Function, CalleePath: first.Path, Callee: caller.Callee}
			f.Citations = append([]Citation{first, second}, rest...)
			return
		}
	}
}
