// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const maxCitationQuote = 2000

type citationFile struct {
	data   []byte
	text   string
	digest string
	goDoc  *goEvidence
}

type lineAnchor struct {
	start, end int
	digest     string
}

type goEvidence struct {
	whole   string
	anchors map[string]lineAnchor
}

func newCitationFile(p string, data []byte) citationFile {
	f := citationFile{data: data, text: strings.ReplaceAll(string(data), "\r\n", "\n"), digest: Digest(data)}
	if strings.HasSuffix(p, ".go") {
		f.goDoc = parseGoEvidence(p, data)
	}
	return f
}

func parseGoEvidence(p string, data []byte) *goEvidence {
	files := token.NewFileSet()
	f, err := parser.ParseFile(files, p, data, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	var imports []string
	cgo := false
	for _, i := range f.Imports {
		imports = append(imports, i.Name.String()+" "+i.Path.Value)
		cgo = cgo || i.Path.Value == `"C"`
	}
	slices.Sort(imports)
	// Compare declarations only within the same package and filename build variant.
	_, variant, _ := strings.Cut(strings.TrimSuffix(path.Base(p), ".go"), "_")
	base := []string{f.Name.Name, variant, strings.Join(imports, "\n")}
	var comments []string
	for _, g := range f.Comments {
		for _, c := range g.List {
			comments = append(comments, c.Text)
			if strings.HasPrefix(c.Text, "//go:") || strings.HasPrefix(c.Text, "// +build") ||
				strings.HasPrefix(c.Text, "//line ") || strings.HasPrefix(c.Text, "/*line ") ||
				cgo {
				base = append(base, c.Text)
			}
		}
	}
	whole, ok := formattedGo(f)
	if !ok {
		return nil
	}
	doc := &goEvidence{
		whole:   Digest([]byte(whole + "\x00" + strings.Join(comments, "\x00"))),
		anchors: make(map[string]lineAnchor),
	}
	for _, decl := range f.Decls {
		name := declarationName(decl)
		if name == "" {
			continue
		}
		if _, duplicate := doc.anchors[name]; duplicate {
			doc.anchors[name] = lineAnchor{}
			continue
		}
		parsed, ok := formattedGo(decl)
		if !ok {
			continue
		}
		parts := append(slices.Clone(base), parsed)
		for _, group := range f.Comments {
			if group.Pos() >= decl.Pos() && group.End() <= decl.End() {
				for _, c := range group.List {
					parts = append(parts, c.Text)
				}
			}
		}
		start := files.PositionFor(decl.Pos(), false).Line
		end := files.PositionFor(decl.End()-1, false).Line
		doc.anchors[name] = lineAnchor{start, end, Digest([]byte(strings.Join(parts, "\x00")))}
	}
	return doc
}

func formattedGo(n ast.Node) (string, bool) {
	// An empty FileSet prevents source positions from influencing whitespace.
	var out bytes.Buffer
	err := format.Node(&out, token.NewFileSet(), n)
	return out.String(), err == nil
}

func declarationName(d ast.Decl) string {
	switch n := d.(type) {
	case *ast.FuncDecl:
		if n.Recv == nil || len(n.Recv.List) == 0 {
			return "func " + n.Name.Name
		}
		return "method " + receiverName(n.Recv.List[0].Type) + "." + n.Name.Name
	case *ast.GenDecl:
		var names []string
		for _, spec := range n.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				names = append(names, s.Name.Name)
			case *ast.ValueSpec:
				for _, id := range s.Names {
					names = append(names, id.Name)
				}
			}
		}
		if len(names) > 0 {
			return n.Tok.String() + " " + strings.Join(names, ",")
		}
	}
	return ""
}

func receiverName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverName(t.X)
	case *ast.IndexExpr:
		return receiverName(t.X)
	case *ast.IndexListExpr:
		return receiverName(t.X)
	}
	return ""
}

func (f citationFile) cite(p, revision string, start, end int) (Citation, error) {
	if !utf8.Valid(f.data) || bytes.ContainsRune(f.data, 0) {
		return Citation{}, fmt.Errorf("citation requires a text file")
	}
	lines := strings.Split(strings.TrimSuffix(f.text, "\n"), "\n")
	if start < 1 || end < start || end > len(lines) {
		return Citation{}, fmt.Errorf("citation lines %d-%d are outside %s", start, end, p)
	}
	quote := strings.Join(lines[start-1:end], "\n")
	if strings.TrimSpace(quote) == "" || len(quote) > maxCitationQuote {
		return Citation{}, fmt.Errorf("citation must contain between 1 and %d bytes of text", maxCitationQuote)
	}
	c := Citation{Kind: discoveryKind(p), Path: p, Revision: revision, StartLine: start, EndLine: end, Quote: quote}
	if f.goDoc != nil {
		var nameMatch string
		var digestMatch string
		for name, anchor := range f.goDoc.anchors {
			if anchor.digest != "" && start >= anchor.start && end <= anchor.end {
				if nameMatch != "" {
					nameMatch = ""
					break
				}
				nameMatch, digestMatch = name, anchor.digest
			}
		}
		if nameMatch != "" {
			c.Anchor, c.Digest = nameMatch, digestMatch
			return c, nil
		}
	}
	c.Digest = Digest([]byte(quote))
	c.ContextDigest = f.contextDigest()
	return c, nil
}

func (f citationFile) contextDigest() string {
	if f.goDoc != nil {
		return f.goDoc.whole
	}
	return f.digest
}

func (t Tree) Cite(ctx context.Context, p string, start, end int) (Citation, error) {
	data, err := t.read(ctx, p)
	if err != nil {
		return Citation{}, err
	}
	return newCitationFile(p, data).cite(p, t.Commit, start, end)
}

func checkSupplemental(c Citation) error {
	switch c.Kind {
	case "history":
		if !validOID(c.Revision) || c.Reference != c.Revision {
			return fmt.Errorf("history citation requires an immutable commit reference")
		}
	case "discussion":
		if c.Reference == "" || c.Revision != "" && !validOID(c.Revision) {
			return fmt.Errorf("discussion citation requires a stable reference")
		}
	default:
		return fmt.Errorf("unknown citation kind %q", c.Kind)
	}
	if c.URL != "" {
		u, err := url.Parse(c.URL)
		if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return fmt.Errorf("invalid citation URL")
		}
	}
	if c.ObservedAt != "" {
		if _, err := time.Parse(time.RFC3339, c.ObservedAt); err != nil {
			return fmt.Errorf("invalid citation observation time")
		}
	}
	if !validDigest(c.Digest) || c.StartLine < 1 || c.EndLine < c.StartLine ||
		strings.Count(c.Quote, "\n") != c.EndLine-c.StartLine || len(c.Quote) > maxCitationQuote || strings.TrimSpace(c.Quote) == "" {
		return fmt.Errorf("invalid supplemental citation range, excerpt, or source digest")
	}
	return nil
}

func (f citationFile) match(c Citation) (lineAnchor, bool) {
	if c.Anchor != "" {
		if f.goDoc == nil {
			return lineAnchor{}, false
		}
		a, found := f.goDoc.anchors[c.Anchor]
		return a, found
	}
	if c.ContextDigest == "" || c.ContextDigest != f.contextDigest() ||
		Digest([]byte(c.Quote)) != c.Digest || c.Quote == "" {
		return lineAnchor{}, false
	}
	lines := strings.Split(strings.TrimSuffix(f.text, "\n"), "\n")
	width := c.EndLine - c.StartLine + 1
	isMatch := func(first int) bool {
		return first >= 0 && first+width <= len(lines) && strings.Join(lines[first:first+width], "\n") == c.Quote
	}
	if width < 1 || width > len(lines) {
		return lineAnchor{}, false
	}
	if isMatch(c.StartLine - 1) {
		return lineAnchor{c.StartLine, c.EndLine, c.Digest}, true
	}
	match := -1
	for i := 0; i+width <= len(lines); i++ {
		if isMatch(i) {
			if match >= 0 {
				return lineAnchor{}, false
			}
			match = i
		}
	}
	if match >= 0 {
		return lineAnchor{match + 1, match + width, c.Digest}, true
	}
	return lineAnchor{}, false
}

func (c Citation) atCurrent(p string, a lineAnchor) Citation {
	c.CurrentPath = p
	c.CurrentStartLine, c.CurrentEndLine = a.start, a.end
	return c
}
