// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"path"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/tamnd/gopapy/v2/parser2"
)

type reviewField struct {
	file, owner, name string
}

var (
	reviewCodeIdentifier = regexp.MustCompile(`[\p{L}_][\p{L}\p{N}_]*`)
	reviewReceiverMember = regexp.MustCompile(`\b(self|cls)\.([A-Za-z_]\w*)`)
)

// A subclass override connects two changed files only through a changed read
// of that exact field in its imported base. Keep any resulting member names
// on the same receiver and in the same hunk as that read.
func reviewLandedIdentifiers(changes []model.Diff, landings map[string]bool) map[string]map[string]bool {
	byPath := map[string]map[string]bool{}
	overrides := map[reviewField][]string{}
	var uses []receiverHunk
	for _, change := range changes {
		p := change.NewPath
		wantLocal := landings[p] || change.IsRenamed && landings[change.OldPath]
		if !validPath(p) || !wantLocal && path.Ext(p) != ".py" || len(change.Diff) > maxDiscussionDefinitionBytes {
			continue
		}
		lexer := lexers.Match(p)
		if lexer == nil {
			continue
		}
		names := map[string]bool{}
		byPath[p] = names
		if change.IsRenamed && validPath(change.OldPath) {
			byPath[change.OldPath] = names
		}
		var head []string
		var definitions []discussionDefinition
		var module *parser2.Module
		if len(change.NewFileContent) <= maxDiscussionDefinitionBytes && change.NewFileContent != "" {
			head = reviewCodeLines(lexer, change.NewFileContent)
			switch path.Ext(p) {
			case ".py":
				if parsed, err := parser2.ParseFile(p, change.NewFileContent); err == nil {
					module = parsed
					definitions = parsedPythonDiscussionDefinitions("", change.NewFileContent, module)
				}
			case ".go":
				definitions, _ = goDiscussionDefinitions(p, []byte(change.NewFileContent))
			case ".js", ".mjs", ".cjs":
				definitions, _ = javascriptDiscussionDefinitions("", change.NewFileContent)
			}
		}
		scopes := make([]*discussionDefinition, len(head)+1)
		symbols := map[string]int{}
		for i := range definitions {
			d := &definitions[i]
			if d.unquotable || d.start < 1 || d.end >= len(scopes) {
				continue
			}
			switch d.kind {
			case "class", "func", "method", "def", "async def", "function", "function*":
				symbols[shortDeclarationName(d.name)]++
				for n := d.start; n <= d.end; n++ {
					scopes[n] = d
				}
			}
		}
		edited := map[int]bool{}
		for _, h := range diff.ParseHunks(change.Diff) {
			var current, deleted []string
			for _, line := range h.Lines {
				if line.Type == diff.HunkDeleted {
					deleted = append(deleted, line.Content)
					continue
				}
				if len(current) >= h.NewCount {
					continue
				}
				n := h.NewStart + len(current)
				if line.Type == diff.HunkAdded && n > 0 && n <= len(head) && strings.TrimSpace(head[n-1]) != "" {
					edited[n] = true
					if scope := scopes[n]; wantLocal && scope != nil {
						name := shortDeclarationName(scope.name)
						if symbols[name] == 1 {
							names[name] = true
						}
					}
				}
				current = append(current, line.Content)
			}
			if h.NewStart > 0 && h.NewStart-1+len(current) <= len(head) {
				current = head[h.NewStart-1 : h.NewStart-1+len(current)]
			} else {
				current = reviewCodeLines(lexer, strings.Join(current, "\n"))
			}
			if wantLocal {
				for _, lines := range [][]string{current, reviewCodeLines(lexer, strings.Join(deleted, "\n"))} {
					for _, code := range lines {
						if len(names) >= 512 {
							break
						}
						for _, id := range reviewCodeIdentifier.FindAllString(code, 512) {
							names[id] = true
							if len(names) == 512 {
								break
							}
						}
					}
				}
			}
			if module != nil {
				uses = append(uses, reviewHunkFieldUses(p, h.NewStart, current, scopes, edited)...)
			}
		}
		if wantLocal && module != nil {
			reviewClassOverrides(p, module, definitions, edited, overrides)
		}
	}
	for _, h := range uses {
		for field := range h.reads {
			for _, p := range overrides[reviewField{h.file, h.owner, field}] {
				for member := range h.members {
					byPath[p][member] = true
				}
			}
		}
	}
	return byPath
}

func reviewCodeLines(lexer chroma.Lexer, source string) []string {
	if source == "" {
		return nil
	}
	tokens, err := lexer.Tokenise(nil, source)
	if err != nil {
		return nil
	}
	var code strings.Builder
	for tok := tokens(); tok != chroma.EOF; tok = tokens() {
		if tok.Type.InSubCategory(chroma.LiteralString) {
			// A string-only assignment edit is code even when it contains no identifiers.
			code.WriteString(strings.Map(func(r rune) rune {
				if r == '\n' {
					return r
				}
				return '0'
			}, tok.Value))
		} else if tok.Type.InCategory(chroma.Comment) {
			code.WriteByte(' ')
			code.WriteString(strings.Repeat("\n", strings.Count(tok.Value, "\n")))
		} else {
			code.WriteString(tok.Value)
		}
	}
	return strings.Split(code.String(), "\n")
}

func reviewLandingTokens(p, quote string) []string {
	lexer := lexers.Match(p)
	if lexer == nil {
		return nil
	}
	return discussionSymbolToken.FindAllString(strings.Join(reviewCodeLines(lexer, quote), "\n"), -1)
}

type receiverHunk struct {
	file, owner    string
	members, reads map[string]bool
}

func shortDeclarationName(name string) string { return name[strings.LastIndexByte(name, '.')+1:] }

func reviewHunkFieldUses(file string, first int, lines []string, scopes []*discussionDefinition, edited map[int]bool) []receiverHunk {
	type scopeReceiver struct {
		scope    *discussionDefinition
		receiver string
	}
	receivers := map[scopeReceiver]*receiverHunk{}
	for i, code := range lines {
		n := first + i
		if n < 1 || n >= len(scopes) {
			continue
		}
		scope := scopes[n]
		if scope == nil || scope.kind != "def" && scope.kind != "async def" {
			continue
		}
		owner, method, ok := strings.Cut(scope.name, ".")
		if !ok || strings.Contains(method, ".") {
			continue
		}
		for _, loc := range reviewReceiverMember.FindAllStringSubmatchIndex(code, -1) {
			if loc[0] > 0 && code[loc[0]-1] == '.' {
				continue
			}
			receiver, field := code[loc[2]:loc[3]], code[loc[4]:loc[5]]
			key := scopeReceiver{scope, receiver}
			h := receivers[key]
			if h == nil {
				h = &receiverHunk{file: file, owner: owner, members: map[string]bool{}, reads: map[string]bool{}}
				receivers[key] = h
			}
			h.members[field] = true
			tail := strings.TrimSpace(code[loc[1]:])
			if edited[n] && !(strings.HasPrefix(tail, "=") && !strings.HasPrefix(tail, "==")) {
				h.reads[field] = true
			}
		}
	}
	var out []receiverHunk
	for _, h := range receivers {
		if len(h.reads) > 0 {
			out = append(out, *h)
		}
	}
	return out
}

func reviewClassOverrides(p string, module *parser2.Module, definitions []discussionDefinition, edited map[int]bool, overrides map[reviewField][]string) {
	imports := map[string]reviewField{}
	for _, stmt := range module.Body {
		imp, ok := stmt.(*parser2.ImportFrom)
		if !ok || imp.Module == "" {
			continue
		}
		mod := strings.ReplaceAll(imp.Module, ".", "/")
		if imp.Level > 0 {
			dir := path.Dir(p)
			for n := 1; n < imp.Level; n++ {
				dir = path.Dir(dir)
			}
			mod = path.Join(dir, mod)
		}
		for _, alias := range imp.Names {
			binding := alias.Asname
			if binding == "" {
				binding = alias.Name
			}
			imports[binding] = reviewField{file: mod + ".py", owner: alias.Name}
		}
	}
	bases := map[string][]reviewField{}
	for _, stmt := range module.Body {
		class, ok := stmt.(*parser2.ClassDef)
		if !ok {
			continue
		}
		for _, base := range class.Bases {
			if name, ok := base.(*parser2.Name); ok {
				if imported, ok := imports[name.Id]; ok {
					bases[class.Name] = append(bases[class.Name], imported)
				}
			}
		}
	}
	for _, d := range definitions {
		owner, field, ok := strings.Cut(d.name, ".")
		if d.kind != "assign" || d.unquotable || !ok || strings.Contains(field, ".") || len(bases[owner]) == 0 {
			continue
		}
		for n := d.start; n <= d.end; n++ {
			if !edited[n] {
				continue
			}
			for _, key := range bases[owner] {
				key.name = field
				overrides[key] = append(overrides[key], p)
			}
			break
		}
	}
}
