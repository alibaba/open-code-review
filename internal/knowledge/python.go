// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/tamnd/gopapy/v2/parser2"
)

var pythonName = regexp.MustCompile(`[\p{L}_][\p{L}\p{N}_]*`)

type pythonDeclaration struct {
	name       string
	start, end int
	body       string
}

type pythonWitnesses struct {
	fileDigest string
	shared     string
	functions  map[string]pythonDeclaration
}

// parser2 exposes the source position as P on every concrete AST node, while
// its position accessor on the Stmt and Expr interfaces is package-private.
func pythonNodeLine(node any) int {
	v := reflect.Indirect(reflect.ValueOf(node))
	if v.Kind() != reflect.Struct {
		return 0
	}
	field := v.FieldByName("P")
	if !field.IsValid() || !field.CanInterface() {
		return 0
	}
	pos, _ := field.Interface().(parser2.Pos)
	return pos.Line
}

func pythonStatementStart(stmt parser2.Stmt, lines []string) int {
	var decorators []parser2.Expr
	switch s := stmt.(type) {
	case *parser2.FunctionDef:
		decorators = s.DecoratorList
	case *parser2.AsyncFunctionDef:
		decorators = s.DecoratorList
	case *parser2.ClassDef:
		decorators = s.DecoratorList
	}
	if len(decorators) > 0 {
		// The expression position can follow the @ in a multiline decorator.
		for n := min(pythonNodeLine(decorators[0]), len(lines)); n > 0; n-- {
			if strings.HasPrefix(strings.TrimLeft(lines[n-1], " \t"), "@") {
				return n
			}
		}
	}
	return pythonNodeLine(stmt)
}

func pythonDeclarationEnd(lines []string, start, end int) int {
	header := lines[start-1]
	indent := header[:len(header)-len(strings.TrimLeft(header, " \t"))]
	var possible []int
	for n := end; n > start; n-- {
		line := lines[n-1]
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		if !strings.HasPrefix(text, "#") {
			break
		}
		rest, hasIndent := strings.CutPrefix(line, indent)
		if !hasIndent || !strings.HasPrefix(rest, " ") && !strings.HasPrefix(rest, "\t") {
			possible = append(possible, n)
		}
	}
	// A detached comment may document the next sibling. Lines starting with
	// '#' can also be part of a final multiline string. Once the prefix parses
	// as a complete declaration, every remaining such line is a real comment.
	slices.Reverse(possible)
	cut := sort.Search(len(possible), func(i int) bool {
		src := strings.Join(lines[start-1:possible[i]-1], "\n") + "\n"
		if indent != "" {
			src = "if True:\n" + src
		}
		_, err := parser2.ParseFile("evidence.py", src)
		return err == nil
	})
	if cut < len(possible) {
		return possible[cut] - 1
	}
	return end
}

// Compare the complete cited function, including its guards and decorators.
// Keep module and class setup, and any sibling referenced elsewhere in the
// file. The bodies of isolated sibling functions do not establish this rule.
func pythonWitnessesFor(ctx context.Context, data []byte) *pythonWitnesses {
	src := string(data)
	module, err := parser2.ParseFile("evidence.py", src)
	if err != nil || ctx.Err() != nil {
		return nil
	}
	lines := strings.Split(src, "\n")
	uses := map[string]int{}
	for _, word := range pythonName.FindAllString(src, -1) {
		uses[word]++
	}
	dynamic := uses["globals"]+uses["locals"]+uses["getattr"]+uses["vars"]+uses["__dict__"]+uses["eval"]+uses["exec"] != 0
	out := &pythonWitnesses{fileDigest: Digest(data), functions: map[string]pythonDeclaration{}}
	var shared strings.Builder
	for _, line := range lines {
		if strings.Contains(line, "#") {
			shared.WriteString(line)
			shared.WriteByte(0)
		}
	}
	var collect func([]parser2.Stmt, string, int)
	collect = func(stmts []parser2.Stmt, prefix string, suiteEnd int) {
		for i, stmt := range stmts {
			// A following statement in the same suite bounds this declaration.
			// Physical lines inside a string or a joined expression can start in column one.
			end := suiteEnd
			if i+1 < len(stmts) {
				end = pythonStatementStart(stmts[i+1], lines) - 1
			}
			var name string
			var start int
			var body *[]parser2.Stmt
			var decorated bool
			switch s := stmt.(type) {
			case *parser2.ClassDef:
				collect(s.Body, prefix+s.Name+".", end)
				continue
			case *parser2.FunctionDef:
				name, start, body, decorated = s.Name, s.P.Line, &s.Body, len(s.DecoratorList) > 0
			case *parser2.AsyncFunctionDef:
				name, start, body, decorated = s.Name, s.P.Line, &s.Body, len(s.DecoratorList) > 0
			default:
				continue
			}
			if start < 1 || start > len(lines) {
				continue
			}
			end = pythonDeclarationEnd(lines, start, end)
			qualified := prefix + name
			full := parser2.DumpModule(&parser2.Module{Body: []parser2.Stmt{stmt}})
			d := pythonDeclaration{qualified, start, end, Digest([]byte(full))}
			if _, duplicate := out.functions[qualified]; duplicate {
				d = pythonDeclaration{}
			}
			out.functions[qualified] = d
			specialMethod := strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__")
			if !dynamic && uses[name] == 1 && !decorated && !specialMethod {
				*body = nil
			}
		}
	}
	collect(module.Body, "", len(lines))
	shared.WriteString(parser2.DumpModule(module))
	out.shared = Digest([]byte(shared.String()))
	return out
}

func (r *rechecker) pythonFile(revision, path string, current *checkedFile) *pythonWitnesses {
	key := [2]string{revision, path}
	if parsed, ok := r.python[key]; ok {
		return parsed
	}
	var data []byte
	if current != nil {
		data = current.data
	} else {
		recorded := r.tree
		recorded.Commit, recorded.Workspace = revision, false
		var err error
		data, err = recorded.read(r.ctx, path)
		if err != nil {
			r.python[key] = nil
			return nil
		}
	}
	parsed := pythonWitnessesFor(r.ctx, data)
	r.python[key] = parsed
	return parsed
}

// Existing Python records carry the original whole-file digest and the
// quoted lines. Recover precise positions from that immutable observation.
func (r *rechecker) relocatedPython(c Citation, current checkedFile) (lineAnchor, bool) {
	if c.Anchor != "" || !strings.HasSuffix(c.Path, ".py") || c.StartLine < 1 ||
		c.EndLine < c.StartLine || c.Quote == "" || Digest([]byte(c.Quote)) != c.Digest {
		return lineAnchor{}, false
	}
	before := r.pythonFile(c.Revision, c.Path, nil)
	if before == nil || before.fileDigest != c.ContextDigest {
		return lineAnchor{}, false
	}
	after := r.pythonFile("", c.Path, &current)
	if after == nil || before.shared != after.shared {
		return lineAnchor{}, false
	}
	for _, old := range before.functions {
		if old.name == "" || c.StartLine < old.start || c.EndLine > old.end {
			continue
		}
		now, ok := after.functions[old.name]
		if !ok || old.body != now.body {
			break
		}
		offset := now.start - old.start
		start, end := c.StartLine+offset, c.EndLine+offset
		lines := strings.Split(current.text, "\n")
		if start >= now.start && end <= now.end && strings.Join(lines[start-1:end], "\n") == c.Quote {
			return lineAnchor{start, end, c.Digest}, true
		}
		width := c.EndLine - c.StartLine + 1
		matched := 0
		for n := now.start; n+width-1 <= now.end; n++ {
			if strings.Join(lines[n-1:n-1+width], "\n") == c.Quote {
				if matched != 0 {
					return lineAnchor{}, false
				}
				matched = n
			}
		}
		if matched != 0 {
			return lineAnchor{matched, matched + width - 1, c.Digest}, true
		}
	}
	return lineAnchor{}, false
}
