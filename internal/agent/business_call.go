// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"go/ast"
	"go/parser"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/gopapy/v2/parser2"
)

var opensBusinessCall = regexp.MustCompile(`[\p{L}_][\p{L}\p{N}_]*\($`)

// A changed call may put its arguments on the next lines. Only extend a
// witness to the end of that same parsed call in one contiguous diff hunk.
func usesCitedMember(c businessCheck, lines []changedLine) bool {
	used := map[string]bool{}
	for _, m := range businessMember.FindAllStringSubmatch(c.SourceQuote, -1) {
		used[m[1]] = true
	}
	for _, m := range businessMember.FindAllStringSubmatch(c.Quote, -1) {
		if used[m[1]] {
			return true
		}
	}
	ext := strings.ToLower(path.Ext(c.Path))
	if len(used) == 0 || (ext != ".go" && ext != ".py" && ext != ".pyi") || !opensBusinessCall.MatchString(strings.TrimSpace(c.Quote)) {
		return false
	}
	start := slices.IndexFunc(lines, func(l changedLine) bool { return l.number == c.Line })
	if start < 0 {
		return false
	}
	var call strings.Builder
	call.WriteString("witness(")
	for n := start + 1; n < len(lines) && n <= start+24; n++ {
		if lines[n].number != lines[n-1].number+1 || call.Len()+len(lines[n].text) > 4096 {
			break
		}
		call.WriteByte('\n')
		call.WriteString(lines[n].text)
		if !strings.ContainsRune(lines[n].text, ')') {
			continue
		}
		if ext == ".go" {
			if expr, err := parser.ParseExpr(call.String()); err == nil {
				parsed, ok := expr.(*ast.CallExpr)
				if !ok {
					return false
				}
				found := false
				ast.Inspect(parsed, func(node ast.Node) bool {
					if _, deferred := node.(*ast.FuncLit); deferred {
						return false
					}
					if sel, ok := node.(*ast.SelectorExpr); ok && used[sel.Sel.Name] {
						found = true
					}
					return !found
				})
				return found
			}
		} else if expr, err := parser2.ParseExpression(call.String()); err == nil {
			parsed, ok := expr.(*parser2.Call)
			return ok && pythonUsesMember(reflect.ValueOf(parsed), used)
		}
	}
	return false
}

// parser2 has no generic visitor. Inspect only AST attribute nodes, including
// those nested inside containers, comparisons and keyword arguments.
func pythonUsesMember(v reflect.Value, used map[string]bool) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() || !v.CanInterface() {
			return false
		}
		switch n := v.Interface().(type) {
		case *parser2.Attribute:
			if n != nil && used[n.Attr] {
				return true
			}
		case *parser2.Lambda, *parser2.GeneratorExp:
			return false
		}
		return pythonUsesMember(v.Elem(), used)
	case reflect.Struct:
		for i := range v.NumField() {
			if pythonUsesMember(v.Field(i), used) {
				return true
			}
		}
	case reflect.Slice:
		for i := range v.Len() {
			if pythonUsesMember(v.Index(i), used) {
				return true
			}
		}
	}
	return false
}
