// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"path"
	"slices"
	"strings"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/tamnd/gopapy/v2/parser2"
	"github.com/tamnd/gopapy/v2/symbols2"
)

// AcceptedCall identifies the two declarations behind a discovery citation pair.
type AcceptedCall struct {
	CallerPath string `json:"caller_path"`
	Caller     string `json:"caller"`
	CalleePath string `json:"callee_path"`
	Callee     string `json:"callee"`
	// A transitioned snapshot can show the stable invocation while retaining
	// the accepted use that originally authorized this link.
	baseWitness Citation
}

// DirectCalleeAssertion is a checked test predicate that needs an outcome check
// before a finding can rely on it to establish the rule.
type DirectCalleeAssertion struct {
	Source         Citation                `json:"source"`
	Callee         string                  `json:"callee"`
	Rule           string                  `json:"rule"`
	Conditions     []string                `json:"conditions,omitempty"`
	Predicates     []DirectCalleePredicate `json:"predicates"`
	implementation DirectCalleeRevision
}

type noneComparison string

const (
	isNone    noneComparison = "is None"
	isNotNone noneComparison = "is not None"
)

// DirectCalleePredicate names one complete assertion in the cited source.
type DirectCalleePredicate struct {
	StartLine      int            `json:"start_line"`
	EndLine        int            `json:"end_line"`
	Quote          string         `json:"quote"`
	NoneComparison noneComparison `json:"none_comparison,omitempty"`
}

func (a DirectCalleeAssertion) MatchedPredicate(quote string) (DirectCalleePredicate, bool) {
	quote = strings.TrimSpace(strings.ReplaceAll(quote, "\r\n", "\n"))
	if countCallerQuotes(a.Source.Quote, quote, 0) == 1 {
		for _, p := range a.Predicates {
			if strings.TrimSpace(p.Quote) == quote {
				return p, true
			}
		}
	}
	return DirectCalleePredicate{}, false
}

// Distinguishes rejects an inconsistent result for a direct None comparison.
// Other predicates depend on the independent evaluation of their expression.
func (p DirectCalleePredicate) Distinguishes(before, after string) bool {
	beforeNone, afterNone := strings.TrimSpace(before) == "None", strings.TrimSpace(after) == "None"
	switch p.NoneComparison {
	case isNone:
		return beforeNone && !afterNone
	case isNotNone:
		return !beforeNone && afterNone
	default:
		return p.NoneComparison == ""
	}
}

func (f Fact) DirectAssertion() *DirectCalleeAssertion { return f.directAssertion }

// WithDirectCalleeEvidence leads with a current witness that bears on the
// callee without going through the recorded caller.
func (t Tree) WithDirectCalleeEvidence(ctx context.Context, f Fact, content string) (Fact, bool) {
	link := f.AcceptedCall
	if link == nil || link.Callee == "" || !f.scopeMatches(link.CalleePath) {
		return Fact{}, false
	}
	r := newRechecker(ctx, t)
	implementation, ok := r.callerTarget(link.CalleePath, link.Callee, 0)
	if !ok || r.discussionDefinitionFile(link.CalleePath).file.text != strings.ReplaceAll(content, "\r\n", "\n") {
		return Fact{}, false
	}
	var witness Citation
	var assertion *DirectCalleeAssertion
	f.eachCitation(func(c Citation) {
		if witness.CurrentPath != "" || !c.live() || c.Quote == "" || c.CurrentPath == "" || c.CurrentStartLine < 1 ||
			c.CurrentEndLine < c.CurrentStartLine || c.Path == link.CallerPath || c.CurrentPath == link.CallerPath {
			return
		}
		if c.CurrentPath == implementation.path && c.CurrentStartLine <= implementation.last && c.CurrentEndLine >= implementation.first ||
			c.Path == implementation.path && c.CurrentPath != implementation.path {
			return
		}
		if test, ok := r.independentPythonWitness(c, implementation); ok {
			witness = c
			assertion = test
		}
	})
	if witness.CurrentPath == "" {
		return Fact{}, false
	}
	// The bounded prompt displays only two live citations per fact.
	f.Citations = append([]Citation{witness}, slices.DeleteFunc(slices.Clone(f.Citations), func(c Citation) bool { return c == witness })...)
	if assertion != nil {
		assertion.Rule = f.Claim
		for _, condition := range f.Conditions {
			assertion.Conditions = append(assertion.Conditions, condition.Description)
		}
		if f.contextStatus == StatusTransition {
			assertion.Rule, assertion.Conditions = f.baseClaim, f.baseConditions
		}
	}
	f.directAssertion = assertion
	return f, true
}

// DirectCalleeRevision contains a resolved function and scalar declarations it
// reads from the same module at one revision.
type DirectCalleeRevision struct {
	Function       Citation
	ModuleLiterals []Citation
}

// ReviewDirectCallee reads the same resolved function at the accepted revision.
func (t Tree) ReviewDirectCallee(ctx context.Context, base string, assertion DirectCalleeAssertion, change model.Diff, line int) (before, after DirectCalleeRevision) {
	current := assertion.implementation.Function
	if t.Git == nil || !validOID(base) || change.IsNew || change.IsDeleted || change.IsRenamed ||
		current.Path != change.NewPath || line < current.StartLine || line > current.EndLine {
		return DirectCalleeRevision{}, DirectCalleeRevision{}
	}
	lines := strings.Split(strings.ReplaceAll(change.NewFileContent, "\r\n", "\n"), "\n")
	if reviewSourceSpan(current.Path, lines, current.StartLine, current.EndLine) != current {
		return DirectCalleeRevision{}, DirectCalleeRevision{}
	}
	accepted := newRechecker(ctx, t).atRevision(base)
	old, ok := accepted.callerTarget(current.Path, assertion.Callee, 0)
	if !ok {
		return DirectCalleeRevision{}, DirectCalleeRevision{}
	}
	before = accepted.directCalleeRevision(old)
	if before.Function.Quote == "" || before.Function.Quote == current.Quote {
		return DirectCalleeRevision{}, DirectCalleeRevision{}
	}
	return before, assertion.implementation
}

func (r *rechecker) focusedFunctionSource(target focusedFunction) Citation {
	return reviewSourceSpan(target.path, strings.Split(r.discussionDefinitionFile(target.path).file.text, "\n"), target.first, target.last)
}

func (r *rechecker) directCalleeRevision(target focusedFunction) DirectCalleeRevision {
	revision := DirectCalleeRevision{Function: r.focusedFunctionSource(target)}
	if revision.Function.Quote == "" {
		return revision
	}
	source := r.discussionDefinitionFile(target.path)
	module, err := parser2.ParseFile(target.path, source.file.text)
	if err != nil {
		return revision
	}
	root := symbols2.Build(module).Root
	lines := strings.Split(source.file.text, "\n")
	for _, stmt := range module.Body {
		id := pythonLiteralAssignmentName(stmt)
		if id == nil {
			continue
		}
		binding := pythonBindingInCallee(root, id.Id, target)
		if binding == nil || binding.BindSites[0] != id.P {
			continue
		}
		line := pythonNodeLine(stmt)
		i := slices.IndexFunc(source.definitions, func(d discussionDefinition) bool {
			return !d.unquotable && d.kind == "assign" && d.name == id.Id && d.start == line
		})
		if i >= 0 {
			d := source.definitions[i]
			if declaration := reviewSourceSpan(target.path, lines, d.start, d.end); declaration.Path != "" {
				revision.ModuleLiterals = append(revision.ModuleLiterals, declaration)
			}
		}
	}
	return revision
}

func (r *rechecker) independentPythonWitness(c Citation, implementation focusedFunction) (*DirectCalleeAssertion, bool) {
	if (c.Kind != "test" && c.Kind != "code") || path.Ext(c.CurrentPath) != ".py" {
		return nil, false
	}
	source := r.discussionDefinitionFile(c.CurrentPath)
	lines := strings.Split(source.file.text, "\n")
	if !source.parsed || c.CurrentEndLine > len(lines) || strings.Join(lines[c.CurrentStartLine-1:c.CurrentEndLine], "\n") != c.Quote {
		return nil, false
	}
	module, err := parser2.ParseFile(c.CurrentPath, source.file.text)
	if err != nil || pythonWildcardImport(module) {
		return nil, false
	}
	root := symbols2.Build(module).Root
	if predicates := r.calleePredicates(c, implementation, source.definitions, lines, root); len(predicates) != 0 {
		revision := r.directCalleeRevision(implementation)
		if revision.Function.Quote != "" {
			return &DirectCalleeAssertion{Source: Citation{Kind: c.Kind, Path: c.CurrentPath, StartLine: c.CurrentStartLine,
				EndLine: c.CurrentEndLine, Quote: c.Quote}, Callee: implementation.name, Predicates: predicates, implementation: revision}, true
		}
	}
	if c.Kind != "code" {
		return nil, false
	}
	name := pythonCitedLiteral(c, module.Body, root)
	if name == "" || !slices.ContainsFunc(source.definitions, func(d discussionDefinition) bool {
		return !d.unquotable && d.kind == "assign" && d.name == name && d.start == c.CurrentStartLine && d.end >= c.CurrentEndLine
	}) {
		return nil, false
	}
	if c.CurrentPath == implementation.path {
		binding := pythonBindingInCallee(root, name, implementation)
		return nil, binding != nil && binding.BindSites[0].Line == c.CurrentStartLine
	}
	return nil, r.calleeUsesImportedLiteral(c, name, implementation)
}

func (r *rechecker) calleePredicates(c Citation, implementation focusedFunction, definitions []discussionDefinition, lines []string, root *symbols2.Scope) []DirectCalleePredicate {
	var predicates []DirectCalleePredicate
	for _, d := range definitions {
		if d.unquotable || d.kind != "def" && d.kind != "async def" || d.end < c.CurrentStartLine || d.start > c.CurrentEndLine {
			continue
		}
		walkPythonReviewStatements(strings.Join(lines[d.start-1:d.end], "\n"), func(stmt parser2.Stmt, line int) bool {
			assertion, ok := stmt.(*parser2.Assert)
			start := d.start + line - 1
			if !ok || start < c.CurrentStartLine || start > c.CurrentEndLine {
				return false
			}
			operands := []parser2.Expr{assertion.Test}
			if compare, ok := assertion.Test.(*parser2.Compare); ok {
				operands = append([]parser2.Expr{compare.Left}, compare.Comparators...)
			}
			var affected *parser2.Call
			for len(operands) != 0 {
				call := pythonAssertionCall(operands[0])
				operands = operands[1:]
				if call == nil {
					continue
				}
				name, id := pythonInvokedName(call)
				if id == nil {
					return false
				}
				at := start + id.P.Line - assertion.P.Line
				if at < c.CurrentStartLine || at > c.CurrentEndLine {
					return false
				}
				binding := pythonModuleBindingAtUse(root, id.Id, func(p parser2.Pos) bool { return p.Line == at && p.Col == id.P.Col })
				if binding == nil {
					return false
				}
				var matches bool
				if c.CurrentPath == implementation.path {
					matches = name == implementation.name && binding.BindSites[0].Line >= implementation.first && binding.BindSites[0].Line <= implementation.last
				} else if binding.Has(symbols2.FlagImport) {
					target, def, ok := r.scopedDiscussionDefinition([]string{implementation.path}, c.CurrentPath, name)
					matches = ok && target.Bound && target.Path == implementation.path && def.name == implementation.name && def.start == implementation.first
				}
				if matches {
					affected = call
					continue
				}
				// The verifier only has implementations present in the verified citation.
				if !r.citedPythonWrapper(c, name, binding, definitions, root) {
					return false
				}
				operands = append(operands, call.Args...)
				for _, arg := range call.Keywords {
					operands = append(operands, arg.Value)
				}
			}
			if affected != nil {
				if quote, end := pythonAssertionSpan(assertion, lines, start, min(d.end, c.CurrentEndLine)); end != 0 {
					predicates = append(predicates, DirectCalleePredicate{StartLine: start, EndLine: end, Quote: quote,
						NoneComparison: pythonNoneComparison(assertion.Test, affected)})
				}
			}
			return false
		})
	}
	return predicates
}

func (r *rechecker) citedPythonWrapper(c Citation, name string, binding *symbols2.Binding, definitions []discussionDefinition, root *symbols2.Scope) bool {
	if binding.Has(symbols2.FlagImport) {
		return false
	}
	return slices.ContainsFunc(definitions, func(d discussionDefinition) bool {
		if d.name != name || d.start != binding.BindSites[0].Line || d.start < c.CurrentStartLine ||
			!r.focusedPythonDefinition(c.CurrentPath, d, root) {
			return false
		}
		body := r.focusedFunctionSource(focusedFunction{c.CurrentPath, name, d.start, d.end})
		return body.Quote != "" && body.EndLine <= c.CurrentEndLine && closedPythonWrapper(body)
	})
}

// A wrapper can establish a predicate when the excerpt contains its entire
// transformation. A single scalar return needs no other declarations.
func closedPythonWrapper(source Citation) bool {
	module, err := parser2.ParseFile(source.Path, source.Quote+"\n")
	if err != nil || len(module.Body) != 1 {
		return false
	}
	fn, ok := module.Body[0].(*parser2.FunctionDef)
	if !ok || len(fn.Body) != 1 || len(fn.DecoratorList) != 0 || len(fn.TypeParams) != 0 || fn.Returns != nil {
		return false
	}
	args := fn.Args
	if args == nil || args.Vararg != nil || args.Kwarg != nil || len(args.Defaults) != 0 || len(args.KwOnly) != 0 {
		return false
	}
	parameters := map[string]bool{}
	for _, arg := range slices.Concat(args.PosOnly, args.Args) {
		if arg.Annotation != nil {
			return false
		}
		parameters[arg.Name] = true
	}
	result, ok := fn.Body[0].(*parser2.Return)
	return ok && closedPythonScalar(result.Value, parameters)
}

func closedPythonScalar(expr parser2.Expr, parameters map[string]bool) bool {
	switch x := expr.(type) {
	case *parser2.Constant:
		return true
	case *parser2.Name:
		return parameters[x.Id]
	case *parser2.UnaryOp:
		return closedPythonScalar(x.Operand, parameters)
	case *parser2.BinOp:
		return closedPythonScalar(x.Left, parameters) && closedPythonScalar(x.Right, parameters)
	case *parser2.IfExp:
		return closedPythonScalar(x.Test, parameters) && closedPythonScalar(x.Body, parameters) && closedPythonScalar(x.OrElse, parameters)
	case *parser2.Compare:
		return closedPythonScalar(x.Left, parameters) && !slices.ContainsFunc(x.Comparators, func(e parser2.Expr) bool {
			return !closedPythonScalar(e, parameters)
		})
	default:
		return false
	}
}

func pythonAssertionSpan(assertion *parser2.Assert, lines []string, start, last int) (string, int) {
	expected := parser2.DumpModule(&parser2.Module{Body: []parser2.Stmt{assertion}})
	indented := strings.HasPrefix(lines[start-1], " ") || strings.HasPrefix(lines[start-1], "\t")
	for end := start; end <= last; end++ {
		quote := strings.Join(lines[start-1:end], "\n")
		text := quote + "\n"
		if indented {
			text = "async def predicate():\n" + text
		}
		fragment, err := parser2.ParseFile("assertion.py", text)
		if err != nil || len(fragment.Body) != 1 {
			continue
		}
		if indented {
			wrapper, ok := fragment.Body[0].(*parser2.AsyncFunctionDef)
			if !ok || len(wrapper.Body) != 1 {
				continue
			}
			fragment = &parser2.Module{Body: wrapper.Body}
		}
		if _, ok := fragment.Body[0].(*parser2.Assert); ok && parser2.DumpModule(fragment) == expected {
			return quote, end
		}
	}
	return "", 0
}

func pythonAssertionCall(expr parser2.Expr) *parser2.Call {
	if await, ok := expr.(*parser2.Await); ok {
		expr = await.Value
	}
	call, _ := expr.(*parser2.Call)
	return call
}

func pythonNoneComparison(test parser2.Expr, call *parser2.Call) noneComparison {
	compare, ok := test.(*parser2.Compare)
	if !ok || len(compare.Ops) != 1 || compare.Ops[0] != "Is" && compare.Ops[0] != "IsNot" {
		return ""
	}
	left, leftConstant := compare.Left.(*parser2.Constant)
	right, rightConstant := compare.Comparators[0].(*parser2.Constant)
	if leftConstant && left.Kind == "None" && pythonAssertionCall(compare.Comparators[0]) == call ||
		rightConstant && right.Kind == "None" && pythonAssertionCall(compare.Left) == call {
		if compare.Ops[0] == "Is" {
			return isNone
		}
		return isNotNone
	}
	return ""
}

func pythonCitedLiteral(c Citation, stmts []parser2.Stmt, root *symbols2.Scope) string {
	for _, stmt := range stmts {
		if pythonNodeLine(stmt) != c.CurrentStartLine {
			continue
		}
		id := pythonLiteralAssignmentName(stmt)
		if id != nil && root.Symbols[id.Id] != nil && len(root.Symbols[id.Id].BindSites) == 1 {
			return id.Id
		}
	}
	return ""
}

func pythonLiteralAssignmentName(stmt parser2.Stmt) *parser2.Name {
	var target, value parser2.Expr
	switch s := stmt.(type) {
	case *parser2.Assign:
		if len(s.Targets) == 1 {
			target, value = s.Targets[0], s.Value
		}
	case *parser2.AnnAssign:
		target, value = s.Target, s.Value
	}
	if _, literal := value.(*parser2.Constant); !literal {
		return nil
	}
	name, _ := target.(*parser2.Name)
	return name
}

func pythonBindingInCallee(root *symbols2.Scope, name string, implementation focusedFunction) *symbols2.Binding {
	for _, scope := range root.Children {
		if scope.Kind == symbols2.ScopeFunction && scope.Name == implementation.name &&
			scope.Pos.Line >= implementation.first && scope.Pos.Line <= implementation.last {
			if binding := scope.Symbols[name]; binding != nil {
				return pythonModuleBindingAtUse(root, name, func(p parser2.Pos) bool { return slices.Contains(binding.UseSites, p) })
			}
		}
	}
	return nil
}

func (r *rechecker) calleeUsesImportedLiteral(c Citation, name string, implementation focusedFunction) bool {
	text := r.discussionDefinitionFile(implementation.path).file.text
	module, err := parser2.ParseFile(implementation.path, text)
	if err != nil || pythonWildcardImport(module) {
		return false
	}
	root := symbols2.Build(module).Root
	for _, stmt := range module.Body {
		imp, ok := stmt.(*parser2.ImportFrom)
		if !ok {
			continue
		}
		for _, alias := range imp.Names {
			if alias.Name != name {
				continue
			}
			local := alias.Asname
			if local == "" {
				local = name
			}
			binding := pythonBindingInCallee(root, local, implementation)
			if binding == nil || !binding.Has(symbols2.FlagImport) {
				continue
			}
			target, def, ok := r.scopedDiscussionDefinition([]string{c.CurrentPath}, implementation.path, local)
			if ok && target.Bound && target.Path == c.CurrentPath && def.kind == "assign" && def.start == c.CurrentStartLine {
				return true
			}
		}
	}
	return false
}

// Earlier discoveries stamped the caller symbol on their second citation.
// Recover the callee from the first citation in its immutable source, so old
// observations retain their existing ID, feedback and discovery receipt.
func (s Snapshot) restoreAcceptedCalls(ctx context.Context, before Tree, checked []Checked) {
	if !validOID(before.Commit) || before.Git == nil {
		return
	}
	original := map[string]Fact{}
	for _, c := range checked {
		f := c.Fact
		if f.AcceptedCall == nil && f.Origin == "discovery" && (f.Category == CategoryBusinessRule || f.Category == CategoryDomainConstraint) &&
			len(f.Citations) >= 2 && f.Citations[0].Kind == "code" && f.Citations[1].Kind == "code" &&
			f.Citations[1].Symbol != "" && f.Citations[1].Reference == "" && f.Citations[0].Path != f.Citations[1].Path {
			original[f.ID] = f
		}
	}
	if len(original) == 0 {
		return
	}
	r := newRechecker(ctx, before)
	for i := range s {
		f, ok := original[s[i].ID]
		if !ok || !r.ancestor(f.Revision) {
			continue
		}
		impl, use := f.Citations[0], f.Citations[1]
		origin := r.atRevision(f.Revision)
		focus, ok := origin.callerTarget(impl.Path, "", impl.StartLine)
		if !ok || impl.EndLine > focus.last || impl.Revision != f.Revision || use.Revision != f.Revision {
			continue
		}
		link := AcceptedCall{CallerPath: use.Path, Caller: use.Symbol, CalleePath: impl.Path, Callee: focus.name}
		if _, ok := origin.callWitness(link, focus, callerWitness{quote: use.Quote}); ok {
			s[i].AcceptedCall = &link
		}
	}
}

type CallerRevision struct {
	Revision   string   `json:"revision"`
	Caller     Citation `json:"caller"`
	Callee     Citation `json:"callee"`
	CallLine   int      `json:"call_line"`
	ResultLine int      `json:"result_line"`
	invocation discoverySource
}

type ReviewedCall struct {
	Rule       string
	Conditions []string
	Base, Head CallerRevision
}

// When the cited result use was rewritten, the accepted and reviewed code
// can still provide the same, independently resolved invocation as a current
// witness. Keep the old claim historical and expose the actual call location.
func (r *rechecker) retainCallerInvocation(f *Fact, prior Fact, base *rechecker) {
	link := prior.AcceptedCall
	if link == nil || slices.ContainsFunc(f.Citations, func(c Citation) bool {
		return c.Kind == "code" && c.Path == link.CallerPath && c.Symbol == link.Caller
	}) {
		return
	}
	old, ok := base.callerTarget(link.CalleePath, link.Callee, 0)
	if !ok {
		return
	}
	head, ok := r.callerTarget(link.CalleePath, link.Callee, 0)
	if !ok {
		return
	}
	for _, c := range prior.Citations {
		if c.Kind != "code" || c.Path != link.CallerPath || c.Symbol != link.Caller || c.Reference != "" {
			continue
		}
		accepted, ok := base.callWitness(*link, old, callerWitness{quote: c.Quote, anchors: []Citation{c}})
		if !ok || countCallerQuotes(accepted.Caller.Quote, accepted.invocation.Text, 0) != 1 {
			continue
		}
		current, ok := r.callWitness(*link, head, callerWitness{quote: accepted.invocation.Text, callOnly: true})
		if !ok || accepted.invocation.Text != current.invocation.Text {
			continue
		}
		n := strings.Count(accepted.invocation.Text, "\n")
		line, currentLine := accepted.invocation.StartLine, current.invocation.StartLine
		quote, err := base.file(link.CallerPath).cite(link.CallerPath, base.tree.Commit, line, line+n)
		if err != nil || quote.Quote != accepted.invocation.Text {
			continue
		}
		quote.Symbol = link.Caller
		quote = quote.atCurrent(link.CallerPath, lineAnchor{start: currentLine, end: currentLine + n})
		recovered := *link
		recovered.baseWitness = c
		f.AcceptedCall = &recovered
		f.Citations = slices.DeleteFunc(f.Citations, func(c Citation) bool {
			return c.Path == quote.Path && c.Quote == quote.Quote && c.CurrentStartLine == currentLine
		})
		f.Citations = append([]Citation{quote}, f.Citations...)
		return
	}
}

// ReviewCaller reads both reviewed versions, following the recorded import
// to the named callee and its actual result use in the named caller.
func (t Tree) ReviewCaller(ctx context.Context, base string, f Fact, change model.Diff, line int, sourceQuote string) (*ReviewedCall, bool) {
	link := f.AcceptedCall
	if link == nil || f.Origin != "discovery" || !validOID(base) || t.Git == nil || line < 1 || change.IsNew || change.IsDeleted ||
		change.IsRenamed || change.NewPath != link.CalleePath || change.NewFileContent == "" {
		return nil, false
	}
	// A currently cited complete line must lie inside the original consumer's
	// stamped excerpt. A different citation from the same file cannot replace it.
	var anchors []Citation
	for _, c := range f.Citations {
		if c.Kind == "code" && c.Path == link.CallerPath && (c.CurrentPath == "" || c.CurrentPath == c.Path) &&
			(c.Symbol == link.Caller || c.Symbol == "") && c.Reference == "" && quoteIncludesLine(c.Quote, sourceQuote, 0) {
			anchors = append(anchors, c)
		}
	}
	if len(anchors) == 0 {
		return nil, false
	}
	runner, err := t.Git.LocalForRepository(ctx, t.Repo)
	if err != nil {
		return nil, false
	}
	t.Git = runner
	head := newRechecker(ctx, t)
	file := head.discussionDefinitionFile(link.CalleePath)
	if !file.parsed || strings.ReplaceAll(change.NewFileContent, "\r\n", "\n") != file.file.text {
		return nil, false
	}
	code := &ReviewedCall{Rule: f.Claim}
	for _, c := range f.Conditions {
		code.Conditions = append(code.Conditions, c.Description)
	}
	if f.contextStatus == StatusTransition {
		code.Rule, code.Conditions = f.baseClaim, f.baseConditions
	}
	accepted := head.atRevision(base)
	if code.Rule == "" || !accepted.ancestor(f.Revision) {
		return nil, false
	}
	target, ok := head.callerTarget(link.CalleePath, link.Callee, line)
	if !ok {
		return nil, false
	}
	if code.Head, ok = head.callWitness(*link, target, callerWitness{quote: sourceQuote, anchors: anchors}); !ok {
		return nil, false
	}
	old, ok := accepted.callerTarget(link.CalleePath, link.Callee, 0)
	if !ok {
		return nil, false
	}
	if w := link.baseWitness; w.Quote != "" {
		code.Base, ok = accepted.callWitness(*link, old, callerWitness{quote: w.Quote, anchors: []Citation{w}})
		if !ok || code.Base.invocation.Text != code.Head.invocation.Text {
			return nil, false
		}
	} else if code.Base, ok = accepted.callWitness(*link, old, callerWitness{quote: sourceQuote}); !ok {
		// The use can change while the call stays identifiable in both revisions.
		code.Base, ok = accepted.callWitness(*link, old, callerWitness{quote: code.Head.invocation.Text, callOnly: true})
	}
	if !ok {
		return nil, false
	}
	return code, true
}

func (r *rechecker) callerTarget(p, name string, line int) (focusedFunction, bool) {
	file := r.discussionDefinitionFile(p)
	if path.Ext(p) != ".py" || !file.parsed {
		return focusedFunction{}, false
	}
	module, err := parser2.ParseFile(p, file.file.text)
	if err != nil || pythonWildcardImport(module) {
		return focusedFunction{}, false
	}
	root := symbols2.Build(module).Root
	for _, d := range file.definitions {
		if (name == "" || d.name == name) && (line == 0 || d.start <= line && line <= d.end) && r.focusedPythonDefinition(p, d, root) {
			return focusedFunction{p, d.name, d.start, d.end}, true
		}
	}
	return focusedFunction{}, false
}

type callerWitness struct {
	quote    string
	anchors  []Citation
	callOnly bool
}

func (w callerWitness) matches(s discoverySource) bool {
	line := s.Caller.ResultLine
	call := line == 0
	if call {
		line = s.Caller.CallLine
	} else if w.callOnly {
		return false
	}
	if !quoteIncludesLine(s.Text, w.quote, line-s.StartLine+1) {
		return false
	}
	return len(w.anchors) == 0 || slices.ContainsFunc(w.anchors, func(c Citation) bool {
		first, last := c.CurrentStartLine, c.CurrentEndLine
		if first == 0 {
			first, last = c.StartLine, c.EndLine
		}
		return (call || c.Symbol == s.Caller.Function) && first <= line && line <= last
	})
}

// A stored citation can select any verified use. Ranking chooses a downstream
// excerpt only when the invocation itself is the witness.
func (r *rechecker) callWitness(link AcceptedCall, target focusedFunction, w callerWitness) (CallerRevision, bool) {
	flows := r.pythonCallerFlows(link.CallerPath, target, link.Caller, (DiscoverOptions{allowPath: r.tree.AllowPath}).allowed)
	var chosen discoverySource
	var destination int
	for _, flow := range flows {
		atCall := w.matches(flow.call)
		var best callerUse
		var citedLine int
		for _, u := range flow.uses {
			if w.matches(u.source) {
				line := u.source.Caller.ResultLine
				if citedLine != 0 && citedLine != line {
					return CallerRevision{}, false
				}
				citedLine = line
			}
			if atCall && u.preferredTo(best) {
				best = u
			}
		}
		if !atCall && citedLine == 0 {
			continue
		}
		if chosen.Caller != nil {
			return CallerRevision{}, false
		}
		chosen, destination = flow.call, citedLine
		if destination == 0 && best.source.Caller != nil {
			destination = best.source.Caller.ResultLine
		}
		if destination == 0 && strings.HasPrefix(strings.TrimSpace(chosen.Text), "return ") {
			destination = chosen.Caller.CallLine
		}
	}
	if chosen.Caller == nil || destination == 0 {
		return CallerRevision{}, false
	}
	invocation := chosen.Caller.CallLine
	file := r.discussionDefinitionFile(link.CallerPath)
	body := EnclosingReviewFunctions(link.CallerPath, file.file.text, invocation)[0]
	impl := r.discussionDefinitionFile(target.path)
	callee := reviewSourceSpan(target.path, strings.Split(impl.file.text, "\n"), target.first, target.last)
	if body.Quote == "" || callee.Quote == "" || destination < body.StartLine || destination > body.EndLine ||
		w.callOnly && countCallerQuotes(body.Quote, w.quote, 0) != 1 {
		return CallerRevision{}, false
	}
	revision := r.tree.Commit
	if r.tree.Workspace {
		revision = "workspace"
	}
	return CallerRevision{Revision: revision, Caller: body, Callee: callee, CallLine: invocation, ResultLine: destination, invocation: chosen}, true
}

// Match complete lines, optionally requiring that the quote covers a specific
// one-based line in the supplied excerpt.
func quoteIncludesLine(text, quote string, line int) bool {
	return countCallerQuotes(text, quote, line) > 0
}

func countCallerQuotes(text, quote string, line int) int {
	quote = strings.TrimSpace(strings.ReplaceAll(quote, "\r\n", "\n"))
	if quote == "" {
		return 0
	}
	lines := strings.Split(text, "\n")
	count := strings.Count(quote, "\n") + 1
	matches := 0
	for i := 0; i+count <= len(lines); i++ {
		if (line == 0 || i < line && line <= i+count) && strings.TrimSpace(strings.Join(lines[i:i+count], "\n")) == quote {
			matches++
		}
	}
	return matches
}
