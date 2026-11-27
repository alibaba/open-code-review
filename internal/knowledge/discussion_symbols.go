// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/tamnd/gopapy/v2/parser2"
)

const (
	maxDiscussionSymbols         = 8
	maxDiscussionDefinitionBytes = 128 << 10
	maxDiscussionIndexFiles      = 256
	maxDiscussionIndexBytes      = 32 << 20
	maxDiscussionMatchFiles      = 24
	discussionSymbolName         = `[\pL_$][\pL\pN_$]*`
)

type discussionNamedBinding struct {
	Mention string `json:"mention"`
	Path    string `json:"path"`
	Key     string `json:"declaration"`
	Bound   bool   `json:"bound,omitempty"`
}

type discussionBindings struct {
	Paths   []string                 `json:"paths"`
	Symbols []discussionNamedBinding `json:"symbols"`
	Search  *discussionSearchPolicy  `json:"search,omitempty"`
}

type discussionSearchPolicy struct {
	Scope      string   `json:"scope,omitempty"`
	Excludes   []string `json:"excludes,omitempty"`
	Authorized []string `json:"authorized,omitempty"`
	Restricted bool     `json:"restricted,omitempty"`
}

type discussionNameIndex struct {
	reader *rechecker
	paths  []string
	hits   map[string][]string
}

func discussionSearchableFile(p string, tests bool) bool {
	return discussionDefinitionExt(p) && (tests || discoveryKind(p) != "test")
}

func (r *rechecker) indexDiscussionNames(opts DiscoverOptions, tests bool) *discussionNameIndex {
	allow := opts.allowPath
	opts.allowPath = func(p string) bool {
		return discussionSearchableFile(p, tests) && (allow == nil || allow(p))
	}
	var scopes []string
	if opts.Scope != "" {
		scopes = append(scopes, opts.Scope)
	}
	var result DiscoveryResult
	tree := r.tree
	tree.Git = tree.Git.LocalOnly()
	blobs, err := tree.discoveryTree(r.ctx, tree.Commit, scopes, opts, &result)
	if err != nil || result.Truncated || len(blobs) > maxDiscussionIndexFiles {
		return nil
	}
	index := &discussionNameIndex{reader: r, hits: map[string][]string{}}
	size, pathBytes := 0, 0
	for _, blob := range blobs {
		size += blob.size
		pathBytes += len(blob.path) + 3
		if size > maxDiscussionIndexBytes || pathBytes > 12<<10 {
			return nil
		}
		index.paths = append(index.paths, blob.path)
	}
	return index
}

func (i *discussionNameIndex) matchingPaths(name string, literal bool) []string {
	if i == nil || len(i.paths) == 0 {
		return nil
	}
	// A string literal can use escape sequences, so spelling is not a safe
	// prefilter for a named mode. Check every eligible file in a small index.
	if literal {
		if len(i.paths) <= maxDiscussionMatchFiles {
			return i.paths
		}
		return nil
	}
	leaf := name[strings.LastIndexByte(name, '.')+1:]
	if hits, ok := i.hits[leaf]; ok {
		return hits
	}
	i.hits[leaf] = nil
	tree := i.reader.tree
	tree.Git = tree.Git.LocalOnly()
	args := []string{"--literal-pathspecs", "grep", "--no-textconv", "-I", "-l", "-z", "-F", "-e", leaf, tree.Commit, "--"}
	raw, clipped, err := tree.discoveryGit(i.reader.ctx, 16<<10, append(args, i.paths...)...)
	var exit *exec.ExitError
	if clipped || err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return nil
	}
	var hits []string
	for _, entry := range bytes.Split(bytes.TrimSuffix(raw, []byte{0}), []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		p, ok := strings.CutPrefix(string(entry), tree.Commit+":")
		if !ok || !slices.Contains(i.paths, p) || len(hits) == maxDiscussionMatchFiles {
			return nil
		}
		hits = append(hits, p)
	}
	i.hits[leaf] = hits
	return hits
}

type discussionPathChange struct {
	path  string
	added bool
}

func (r *rechecker) discussionChangedPaths(revision string, policy *discussionSearchPolicy, tests bool) ([]string, bool) {
	tree := r.tree
	if !tree.Workspace && tree.Commit == revision {
		return nil, true
	}
	if !validOID(revision) || !validOID(tree.Commit) {
		return nil, false
	}
	tree.Git = tree.Git.LocalOnly()
	opts := DiscoverOptions{Scope: policy.Scope, FileFilter: &rules.FileFilter{Exclude: policy.Excludes}, allowPath: tree.AllowPath}
	args := []string{"--literal-pathspecs", "diff", "--name-status", "-z", "--no-ext-diff", "--no-textconv", "--no-renames", "--diff-filter=AMT", revision}
	if !tree.Workspace {
		args = append(args, tree.Commit)
	}
	args = append(args, "--")
	var scopes []string
	if policy.Scope != "" {
		scopes = append(scopes, policy.Scope)
	}
	raw, clipped, err := tree.discoveryGit(r.ctx, 64<<10, append(args, scopes...)...)
	if clipped || err != nil {
		return nil, false
	}
	var changes []discussionPathChange
	fields := bytes.Split(raw, []byte{0})
	if len(fields)%2 != 1 {
		return nil, false
	}
	for n := 0; n < len(fields)-1; n += 2 {
		changes = append(changes, discussionPathChange{string(fields[n+1]), bytes.Equal(fields[n], []byte("A"))})
	}
	if tree.Workspace {
		cmd := []string{"--literal-pathspecs", "ls-files", "-z", "--others", "--exclude-standard", "--"}
		raw, clipped, err := tree.discoveryGit(r.ctx, 64<<10, append(cmd, scopes...)...)
		if clipped || err != nil {
			return nil, false
		}
		for _, p := range bytes.Split(raw, []byte{0}) {
			changes = append(changes, discussionPathChange{string(p), true})
		}
	}
	var paths []string
	for _, change := range changes {
		p := change.path
		if !discussionSearchableFile(p, tests) || !opts.allowed(p) {
			continue
		}
		if policy.Restricted && !slices.Contains(policy.Authorized, p) {
			if change.added {
				return nil, false // The original caller filter could not admit a new name.
			}
			continue
		}
		paths = append(paths, p)
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	return paths, len(paths) <= maxDiscussionMatchFiles
}

var (
	discussionSymbolToken = regexp.MustCompile(discussionSymbolName + `(?:\.` + discussionSymbolName + `)*`)
	discussionCodeName    = regexp.MustCompile(`[a-z][A-Z]|[A-Za-z0-9]_[A-Za-z0-9]|^[A-Z][A-Z0-9_]{2,}$`)
)

func discussionMentionedSymbols(citations []Citation) []string {
	var names []string
	for _, c := range citations {
		if c.Kind != "discussion" {
			continue
		}
		text := strings.TrimSuffix(c.Quote, " [excerpt]")
		for _, loc := range discussionSymbolToken.FindAllStringIndex(text, -1) {
			name := text[loc[0]:loc[1]]
			if len(name) > 160 || slices.Contains(names, name) {
				continue
			}
			var before, after byte
			if loc[0] > 0 {
				before = text[loc[0]-1]
			}
			if loc[1] < len(text) {
				after = text[loc[1]]
			}
			// Do not treat a component of a URL, path or hyphenated word as
			// an independent name. Quoted names and calls can be ordinary words.
			if strings.ContainsRune("/\\@-", rune(before)) || strings.ContainsRune("/\\@-", rune(after)) {
				continue
			}
			quoted := strings.ContainsRune("'\"\x60", rune(before)) && (before == after || after == '(')
			if quoted || after == '(' || strings.ContainsRune(name, '.') || discussionCodeName.MatchString(name) {
				names = append(names, name)
				if len(names) == maxDiscussionSymbols {
					return names
				}
			}
		}
	}
	return names
}

type discussionDefinition struct {
	name, kind, namespace, literal string
	start, end                     int
	unquotable                     bool
}

func (d discussionDefinition) key() string { return d.kind + " " + d.namespace + " " + d.name }

func (d discussionDefinition) matches(name string) bool {
	return d.matchesIdentifier(name) || d.literal != "" && name == d.literal
}

func (d discussionDefinition) matchesIdentifier(name string) bool {
	if name == d.name || name == d.namespace+"."+d.name {
		return true
	}
	owner, method, qualified := strings.Cut(d.name, ".")
	return qualified && owner != "" && method == name
}

type discussionDefinitionFile struct {
	file        checkedFile
	definitions []discussionDefinition
	parsed      bool
}

func discussionDefinitionExt(p string) bool {
	switch path.Ext(p) {
	case ".go", ".py", ".js", ".mjs", ".cjs":
		return true
	}
	return false
}

func (r *rechecker) discussionDefinitionFile(p string) discussionDefinitionFile {
	if r.discussionDefinitions == nil {
		r.discussionDefinitions = map[string]discussionDefinitionFile{}
	}
	if f, ok := r.discussionDefinitions[p]; ok {
		return f
	}
	f := r.loadDiscussionDefinitionFile(p)
	r.discussionDefinitions[p] = f
	return f
}

func (r *rechecker) loadDiscussionDefinitionFile(p string) discussionDefinitionFile {
	if !discussionDefinitionExt(p) || !(DiscoverOptions{allowPath: r.tree.AllowPath}).allowed(p) {
		return discussionDefinitionFile{}
	}
	f := r.file(p)
	if f.err != nil || len(f.data) == 0 || !utf8.Valid(f.data) || bytes.IndexByte(f.data, 0) >= 0 || discoveryCredential.Match(f.data) {
		return discussionDefinitionFile{}
	}
	out := discussionDefinitionFile{file: f}
	if len(f.data) > maxDiscussionDefinitionBytes {
		return out
	}
	module := strings.TrimSuffix(path.Base(p), path.Ext(p))
	switch path.Ext(p) {
	case ".go":
		out.definitions, out.parsed = goDiscussionDefinitions(p, f.data)
	case ".py":
		out.definitions, out.parsed = pythonDiscussionDefinitions(module, f.text)
	default:
		out.definitions, out.parsed = javascriptDiscussionDefinitions(module, f.text)
	}
	return out
}

func (r *rechecker) findDiscussionDefinition(paths []string, name string, goPackage bool) (discussionNamedBinding, discussionDefinition, bool) {
	var binding discussionNamedBinding
	var found discussionDefinition
	leaf := name[strings.LastIndexByte(name, '.')+1:]
	for _, p := range paths {
		if r.ctx.Err() != nil {
			return discussionNamedBinding{}, discussionDefinition{}, false
		}
		f := r.discussionDefinitionFile(p)
		if !f.parsed && (goPackage || discoveryIdentifier(f.file.text, leaf)) {
			return discussionNamedBinding{}, discussionDefinition{}, false
		}
		for _, d := range f.definitions {
			if goPackage && d.name != name || !goPackage && !d.matches(name) {
				continue
			}
			if binding.Path != "" || d.unquotable || slices.ContainsFunc(f.definitions, func(peer discussionDefinition) bool {
				nested := (peer.kind == "class" && peer.start < d.start && peer.end >= d.end) ||
					(d.kind == "class" && d.start < peer.start && d.end >= peer.end)
				return peer != d && !nested && (peer.start == d.start || peer.start == d.end || peer.end == d.start)
			}) {
				return discussionNamedBinding{}, discussionDefinition{}, false
			}
			binding, found = discussionNamedBinding{Mention: name, Path: p, Key: d.key()}, d
		}
	}
	return binding, found, binding.Path != ""
}

func (r *rechecker) scopedDiscussionDefinition(paths []string, scope, name string) (discussionNamedBinding, discussionDefinition, bool) {
	if imports := r.discussionImportTargets(scope, name); len(imports) != 0 {
		if len(imports) != 1 {
			return discussionNamedBinding{}, discussionDefinition{}, false
		}
		target := imports[0]
		search := target.paths
		if !target.goPackage {
			search = slices.DeleteFunc(slices.Clone(search), func(p string) bool { return !slices.Contains(paths, p) })
			if len(search) != 1 || r.alternativeImportPath(target.paths, search[0]) {
				return discussionNamedBinding{}, discussionDefinition{}, false
			}
		}
		if slices.ContainsFunc(r.discussionDefinitionFile(scope).definitions, func(d discussionDefinition) bool { return d.matchesIdentifier(name) }) {
			return discussionNamedBinding{}, discussionDefinition{}, false
		}
		// A Go import names a whole package. Unsampled siblings can establish
		// uniqueness, but only a definition in the admitted corpus can be cited.
		binding, d, ok := r.findDiscussionDefinition(search, target.symbol, target.goPackage)
		if !ok || !slices.Contains(paths, binding.Path) || d.name != target.symbol || target.namespace != "" && d.namespace != target.namespace {
			return discussionNamedBinding{}, discussionDefinition{}, false
		}
		binding.Mention, binding.Bound = name, true
		return binding, d, true
	}
	if slices.Contains(paths, scope) {
		binding, d, ok := r.findDiscussionDefinition([]string{scope}, name, false)
		if ok && d.matchesIdentifier(name) {
			binding.Bound = true
			return binding, d, true
		}
	}
	return r.findDiscussionDefinition(paths, name, false)
}

type discussionImportTarget struct {
	symbol    string
	namespace string
	paths     []string
	goPackage bool
}

func importedDiscussionName(name, alias, member string) string {
	if name == alias {
		return member
	}
	if tail, ok := strings.CutPrefix(name, alias+"."); ok {
		if member == "" {
			return tail
		}
		return member + "." + tail
	}
	return ""
}

var javascriptLocalImport = regexp.MustCompile(`(?m)^[ \t]*import[ \t]+(\{[^}]*\}|\*[ \t]+as[ \t]+` +
	discussionSymbolName + `)[ \t\r\n]+from[ \t]+(_)`)

func (r *rechecker) discussionImportTargets(scope, name string) []discussionImportTarget {
	file := r.discussionDefinitionFile(scope)
	if !file.parsed {
		return nil
	}
	text := file.file.text
	var targets []discussionImportTarget
	switch path.Ext(scope) {
	case ".go":
		qualifier, symbol, ok := strings.Cut(name, ".")
		if !ok {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), scope, text, parser.ImportsOnly|parser.SkipObjectResolution)
		if err != nil {
			return nil
		}
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			alias := path.Base(p)
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			if alias != qualifier {
				continue
			}
			target := discussionImportTarget{symbol: symbol, goPackage: true}
			if imp.Name == nil {
				target.namespace = alias
			}
			if mod, root := r.discussionGoModule(path.Dir(scope)); ast.IsExported(symbol) && !strings.ContainsRune(symbol, '.') &&
				mod != "" && (p == mod || strings.HasPrefix(p, mod+"/")) {
				dir := path.Join(root, strings.TrimPrefix(strings.TrimPrefix(p, mod), "/"))
				if dir == "." || validPath(dir) {
					if other, otherRoot := r.discussionGoModule(dir); other == mod && otherRoot == root {
						target.paths = r.discussionGoPackagePaths(dir)
					}
				}
			}
			targets = append(targets, target)
		}
	case ".py":
		module, err := parser2.ParseFile("evidence.py", text)
		if err != nil {
			return nil
		}
		for _, stmt := range module.Body {
			imp, ok := stmt.(*parser2.ImportFrom)
			if !ok {
				continue
			}
			dir := path.Dir(scope)
			level := imp.Level
			for level > 1 && dir != "." {
				dir = path.Dir(dir)
				level--
			}
			for _, alias := range imp.Names {
				binding := alias.Asname
				if binding == "" {
					binding = alias.Name
				}
				symbol := importedDiscussionName(name, binding, alias.Name)
				if symbol == "" {
					continue
				}
				target := discussionImportTarget{symbol: symbol}
				if level == 1 && imp.Module != "" {
					p := path.Join(dir, strings.ReplaceAll(imp.Module, ".", "/"))
					target.paths = []string{p + ".py", path.Join(p, "__init__.py")}
				}
				targets = append(targets, target)
			}
		}
	case ".js", ".mjs", ".cjs":
		code, ok := maskJavaScript(text)
		if !ok {
			return nil
		}
		depth, _, _ := javascriptNesting(code)
		for _, m := range javascriptLocalImport.FindAllSubmatchIndex(code, -1) {
			if depth[m[0]] != 0 {
				continue
			}
			selection := string(code[m[2]:m[3]])
			var symbols []string
			if fields := strings.Fields(selection); len(fields) == 3 && fields[0] == "*" {
				symbols = append(symbols, importedDiscussionName(name, fields[2], ""))
			} else {
				for entry := range strings.SplitSeq(strings.Trim(selection, "{}"), ",") {
					switch fields := strings.Fields(entry); len(fields) {
					case 1:
						symbols = append(symbols, importedDiscussionName(name, fields[0], fields[0]))
					case 3:
						if fields[1] == "as" {
							symbols = append(symbols, importedDiscussionName(name, fields[2], fields[0]))
						}
					}
				}
			}
			for _, symbol := range symbols {
				if symbol == "" {
					continue
				}
				target := discussionImportTarget{symbol: symbol}
				open := m[4]
				if quote := text[open]; quote == '\'' || quote == '"' {
					value, _, closed := strings.Cut(text[open+1:], string(quote))
					p := path.Join(path.Dir(scope), value)
					if closed && (strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../")) &&
						!strings.ContainsAny(value, "\\\r\n") && validPath(p) && discussionDefinitionExt(p) {
						target.paths = []string{p}
					}
				}
				targets = append(targets, target)
			}
		}
	}
	return targets
}

func (r *rechecker) discussionGoModule(dir string) (string, string) {
	for n := 0; n < 16; n++ {
		file := r.file(path.Join(dir, "go.mod"))
		if file.err == nil {
			if !discoveryCredential.Match(file.data) {
				for line := range strings.SplitSeq(file.text, "\n") {
					fields := strings.Fields(line)
					if len(fields) < 2 || fields[0] != "module" {
						continue
					}
					name := fields[1]
					if strings.HasPrefix(name, `"`) {
						name, _ = strconv.Unquote(name)
					}
					if name != "" && path.Clean(name) == name && !strings.HasPrefix(name, ".") {
						return name, dir
					}
				}
			}
			return "", ""
		}
		if dir == "." {
			break
		}
		dir = path.Dir(dir)
	}
	return "", ""
}

func (r *rechecker) discussionGoPackagePaths(dir string) []string {
	tree := r.tree
	tree.Git = tree.Git.LocalOnly()
	args := []string{"--literal-pathspecs", "ls-tree", "--name-only", "-z", tree.Commit, "--"}
	if tree.Workspace {
		// Ignored Go files can still supply declarations in a working package.
		args = []string{"--literal-pathspecs", "ls-files", "-z", "--cached", "--others", "--"}
	}
	if dir != "." {
		args = append(args, dir+"/")
	}
	raw, clipped, err := tree.discoveryGit(r.ctx, 16<<10, args...)
	if clipped || err != nil {
		return nil
	}
	var paths []string
	for p := range strings.SplitSeq(string(raw), "\x00") {
		base := path.Base(p)
		if path.Dir(p) == dir && path.Ext(base) == ".go" && !strings.HasSuffix(base, "_test.go") && !strings.HasPrefix(base, "_") && !strings.HasPrefix(base, ".") {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	if len(paths) > maxDiscussionMatchFiles {
		return nil
	}
	return paths
}

func (r *rechecker) alternativeImportPath(paths []string, selected string) bool {
	other := slices.DeleteFunc(slices.Clone(paths), func(p string) bool { return p == selected })
	if len(other) == 0 {
		return false
	}
	if r.tree.Workspace {
		root, err := os.OpenRoot(r.tree.Repo)
		if err != nil {
			return true
		}
		defer root.Close()
		for _, p := range other {
			if _, err := root.Lstat(p); !errors.Is(err, os.ErrNotExist) {
				return true
			}
		}
		return false
	}
	tree := r.tree
	tree.Git = tree.Git.LocalOnly()
	args := []string{"--literal-pathspecs", "ls-tree", "-lz", tree.Commit, "--"}
	raw, clipped, err := tree.discoveryGit(r.ctx, 8<<10, append(args, other...)...)
	return err != nil || clipped || len(raw) != 0
}

func (t Tree) bindDiscussionDefinitions(ctx context.Context, opts DiscoverOptions, facts []Fact, sources []discoverySource) {
	if len(facts) == 0 {
		return
	}
	var paths []string
	for _, s := range sources {
		if (s.Kind == "code" || s.Kind == "test") && s.Revision == t.Commit && discussionDefinitionExt(s.Path) && opts.allowed(s.Path) {
			paths = append(paths, s.Path)
		}
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	t.AllowPath = func(p string) bool { return !opts.protected(p) }
	r := newRechecker(ctx, t)
	type indexKey struct {
		revision string
		tests    bool
	}
	indexes := map[indexKey]*discussionNameIndex{}
	indexAt := func(reader *rechecker, tests bool) *discussionNameIndex {
		key := indexKey{reader.tree.Commit, tests}
		if index, exists := indexes[key]; exists {
			return index
		}
		if len(indexes) == 16 {
			return nil
		}
		index := reader.indexDiscussionNames(opts, tests)
		indexes[key] = index
		return index
	}
	for i, f := range facts {
		names := discussionMentionedSymbols(f.Citations)
		if len(names) == 0 {
			continue
		}
		candidates := paths
		if discussionDefinitionExt(f.Scope) && opts.allowed(f.Scope) && !slices.Contains(paths, f.Scope) {
			candidates = append(slices.Clone(paths), f.Scope)
		}
		// A production file's agreement cannot select a same-named test stub.
		tests := discoveryKind(f.Scope) == "test"
		if !tests {
			candidates = slices.DeleteFunc(slices.Clone(candidates), func(p string) bool { return discoveryKind(p) == "test" })
		}
		bindings := discussionBindings{Paths: candidates}
		origin := r
		if revision := f.Citations[0].Revision; revision != "" {
			origin = r.atRevision(revision)
		}
		for _, name := range names {
			current, d, ok := r.scopedDiscussionDefinition(candidates, f.Scope, name)
			if !ok {
				continue
			}
			if current.Bound {
				prior, _, ok := origin.scopedDiscussionDefinition(candidates, f.Scope, name)
				if !ok || prior != current {
					continue
				}
			} else {
				index := indexAt(r, tests)
				literal := !d.matchesIdentifier(name)
				unique, _, ok := r.findDiscussionDefinition(index.matchingPaths(name, literal), name, false)
				if !ok || unique != current {
					continue
				}
				if origin != r {
					priorIndex := indexAt(origin, tests)
					prior, _, ok := origin.findDiscussionDefinition(priorIndex.matchingPaths(name, literal), name, false)
					if !ok || prior != current {
						continue
					}
				}
				if bindings.Search == nil {
					bindings.Search = &discussionSearchPolicy{Scope: opts.Scope, Restricted: opts.allowPath != nil}
					if opts.FileFilter != nil {
						bindings.Search.Excludes = slices.Clone(opts.FileFilter.Exclude)
					}
					if bindings.Search.Restricted {
						bindings.Search.Authorized = index.paths
					}
				}
			}
			bindings.Symbols = append(bindings.Symbols, current)
		}
		if len(bindings.Symbols) != 0 {
			facts[i].DiscussionBindings = &bindings
		}
	}
}

func (r *rechecker) discussionNamedWitnesses(f Fact) []Citation {
	b := f.DiscussionBindings
	if b == nil || len(b.Paths) > discoveryFiles+1 || len(b.Symbols) > maxDiscussionSymbols {
		return nil
	}
	names := discussionMentionedSymbols(f.Citations)
	var result []Citation
	seen := map[[2]string]bool{}
	var additions []string
	checkedChanges, certain := false, false
	for _, accepted := range b.Symbols {
		if !slices.Contains(names, accepted.Mention) {
			continue
		}
		paths := b.Paths
		if !accepted.Bound {
			if !checkedChanges {
				if b.Search != nil && len(b.Search.Authorized) <= maxDiscussionIndexFiles {
					additions, certain = r.discussionChangedPaths(f.Revision, b.Search, discoveryKind(f.Scope) == "test")
				}
				checkedChanges = true
			}
			if !certain {
				continue
			}
			paths = append(slices.Clone(paths), additions...)
			slices.Sort(paths)
			paths = slices.Compact(paths)
		}
		current, d, ok := r.scopedDiscussionDefinition(paths, f.Scope, accepted.Mention)
		if !ok || accepted != current {
			continue
		}
		key := [2]string{current.Path, current.Key}
		if seen[key] {
			continue
		}
		file := r.discussionDefinitions[current.Path].file
		lines := strings.Split(strings.TrimSuffix(file.text, "\n"), "\n")
		end, size := d.start-1, 0
		for end < min(d.end, len(lines), d.start+40) && size+len(lines[end])+1 <= 1000 {
			size += len(lines[end]) + 1
			end++
		}
		if c, err := file.cite(current.Path, r.tree.Commit, d.start, end); err == nil {
			seen[key] = true
			c.Reference, c.Symbol = f.discussionRef(), current.Mention
			result = append(result, c.atCurrent(c.Path, lineAnchor{start: c.StartLine, end: c.EndLine}))
		}
	}
	return result
}

type discussionWitnessOrigin [3]string

func witnessOrigin(c Citation) discussionWitnessOrigin {
	return discussionWitnessOrigin{c.Reference, c.ObservedAt, c.Digest}
}

func (s Snapshot) discussionWitnessOrigins() map[discussionWitnessOrigin]Fact {
	origins := map[discussionWitnessOrigin]Fact{}
	for _, f := range s {
		if f.contextStatus != StatusDiscussionSource || !slices.ContainsFunc(f.ScopeWitnesses, func(c Citation) bool { return c.Symbol != "" }) {
			continue
		}
		for _, c := range f.Citations {
			if c.Kind == "discussion" {
				origins[witnessOrigin(c)] = f
			}
		}
	}
	return origins
}

func (f Fact) linkDiscussionWitnesses(origins map[discussionWitnessOrigin]Fact) Fact {
	switch f.contextStatus {
	case StatusCandidate, StatusCurrent, StatusTransition, StatusConflict:
	default:
		return f
	}
	if len(origins) == 0 {
		return f
	}
	f.ScopeWitnesses = slices.Clone(f.ScopeWitnesses)
	for _, c := range f.Citations {
		if c.Kind != "discussion" {
			continue
		}
		raw, ok := origins[witnessOrigin(c)]
		if !ok || (c.Path != "" && raw.Scope != c.Path) || (c.Path == "" && raw.Scope != f.Scope && raw.Scope != f.Source) {
			continue
		}
		for _, w := range raw.ScopeWitnesses {
			if w.Symbol != "" && w.Reference == c.Reference && !slices.Contains(f.ScopeWitnesses, w) {
				f.ScopeWitnesses = append(f.ScopeWitnesses, w)
			}
		}
	}
	return f
}

func (r *contextRecord) addDiscussionWitnesses(witnesses []Citation) {
	visible := func(w Citation) bool {
		for _, cs := range [][]Citation{r.Citations, r.BaseProvenance} {
			for _, source := range cs {
				if source.Kind == "discussion" && source.Reference == w.Reference &&
					slices.Contains(discussionMentionedSymbols([]Citation{source}), w.Symbol) {
					return true
				}
			}
		}
		return false
	}
	for _, named := range []bool{true, false} {
		for _, w := range witnesses {
			if (w.Symbol != "") != named || (named && !visible(w)) || (!named && r.Context != StatusDiscussionSource) {
				continue
			}
			r.ScopeWitnesses = append(r.ScopeWitnesses, compactCitation(w, 1000))
			if !r.fitsPrompt() {
				r.ScopeWitnesses = r.ScopeWitnesses[:len(r.ScopeWitnesses)-1]
			}
		}
	}
}

func goDiscussionDefinitions(p string, data []byte) ([]discussionDefinition, bool) {
	positions := token.NewFileSet()
	f, err := parser.ParseFile(positions, p, data, parser.SkipObjectResolution)
	if err != nil {
		return nil, false
	}
	var out []discussionDefinition
	add := func(kind, name, literal string, node ast.Node, unquotable bool) {
		begin := positions.PositionFor(node.Pos(), false).Line
		end := positions.PositionFor(node.End()-1, false).Line
		out = append(out, discussionDefinition{name: name, kind: kind, namespace: f.Name.Name, literal: literal,
			start: begin, end: end, unquotable: unquotable})
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if kind, name, ok := strings.Cut(declarationName(d), " "); ok {
				add(kind, name, "", d, d.Body == nil)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					add("type", s.Name.Name, "", s, false)
				case *ast.ValueSpec:
					var literal string
					if len(s.Names) == 1 && len(s.Values) == 1 {
						if v, ok := s.Values[0].(*ast.BasicLit); ok && v.Kind == token.STRING {
							literal, _ = strconv.Unquote(v.Value)
						}
					}
					for _, name := range s.Names {
						add(d.Tok.String(), name.Name, literal, s, len(s.Names) != 1)
					}
				}
			}
		}
	}
	return out, true
}

func pythonDiscussionAssignmentNames(names []string, target parser2.Expr) []string {
	var children []parser2.Expr
	switch t := target.(type) {
	case *parser2.Name:
		return append(names, t.Id)
	case *parser2.Tuple:
		children = t.Elts
	case *parser2.List:
		children = t.Elts
	case *parser2.Starred:
		return pythonDiscussionAssignmentNames(names, t.Value)
	}
	for _, child := range children {
		names = pythonDiscussionAssignmentNames(names, child)
	}
	return names
}

func pythonDiscussionDefinitions(module, text string) ([]discussionDefinition, bool) {
	m, err := parser2.ParseFile("evidence.py", text)
	if err != nil {
		return nil, false
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	var out []discussionDefinition
	var collect func([]parser2.Stmt, string, int)
	collect = func(stmts []parser2.Stmt, owner string, suiteEnd int) {
		for i, stmt := range stmts {
			start := pythonStatementStart(stmt, lines)
			end := suiteEnd
			if i+1 < len(stmts) {
				end = pythonStatementStart(stmts[i+1], lines) - 1
			}
			if start < 1 || start > len(lines) {
				continue
			}
			sharedLine := end < start || i > 0 && pythonStatementStart(stmts[i-1], lines) == start
			add := func(kind, name string, value parser2.Expr, unquotable bool) {
				var literal string
				if scalar, ok := value.(*parser2.Constant); ok {
					literal, _ = scalar.Value.(string)
				}
				bound := start
				if !sharedLine {
					bound = pythonDeclarationEnd(lines, start, end)
				}
				out = append(out, discussionDefinition{name: owner + name, kind: kind, namespace: module, literal: literal,
					start: start, end: bound, unquotable: unquotable || sharedLine})
			}
			switch s := stmt.(type) {
			case *parser2.FunctionDef:
				add("def", s.Name, nil, false)
			case *parser2.AsyncFunctionDef:
				add("async def", s.Name, nil, false)
			case *parser2.ClassDef:
				add("class", s.Name, nil, false)
				collect(s.Body, owner+s.Name+".", end)
			case *parser2.Assign:
				for _, target := range s.Targets {
					if name, ok := target.(*parser2.Name); ok {
						add("assign", name.Id, s.Value, len(s.Targets) != 1)
					} else {
						for _, name := range pythonDiscussionAssignmentNames(nil, target) {
							add("assign", name, nil, true)
						}
					}
				}
			case *parser2.AnnAssign:
				if name, ok := s.Target.(*parser2.Name); ok && s.Value != nil {
					add("assign", name.Id, s.Value, false)
				}
			}
		}
	}
	collect(m.Body, "", len(lines))
	return out, true
}

var javascriptDeclaration = regexp.MustCompile(`(?m)(?:^|[;}])[ \t]*((?:export[ \t]+(?:default[ \t]+)?)?(?:async[ \t]+)?(function(?:[ \t]*\*)?|class|const|let|var)[ \t]+(` + discussionSymbolName + `))`)
var javascriptMethod = regexp.MustCompile(`(?m)^[ \t]*(?:static[ \t]+)?(?:async[ \t]+)?(` + discussionSymbolName + `)[ \t]*\(`)
var javascriptAssignmentName = regexp.MustCompile(`^` + discussionSymbolName)
var javascriptDestructuring = regexp.MustCompile(`(?m)(?:^|[;}])[ \t]*(?:export[ \t]+)?(?:const|let|var)[ \t]+([\[{])`)

// Mask literal and comment text before recognizing static declarations. This
// intentionally handles only ordinary source, without evaluating modules.
func maskJavaScript(text string) ([]byte, bool) {
	code := []byte(text)
	previous, afterControl := -1, false
	var controlParens []bool
	for i := 0; i < len(text); {
		c := text[i]
		var end int
		literal := false
		switch {
		case strings.HasPrefix(text[i:], "//"):
			end = i + strings.IndexByte(text[i:], '\n')
			if end < i {
				end = len(text)
			}
		case strings.HasPrefix(text[i:], "/*"):
			n := strings.Index(text[i+2:], "*/")
			if n < 0 {
				return nil, false
			}
			end = i + n + 4
		case c == '/' && (afterControl || javascriptStartsRegexp(code, previous)):
			end = javascriptRegexpEnd(text, i)
			if end == 0 {
				return nil, false
			}
			literal = true
		case c == '\'' || c == '"' || c == '\x60':
			for j := i + 1; j < len(text); j++ {
				if text[j] == '\\' {
					j++
					continue
				}
				if (c == '\x60' && strings.HasPrefix(text[j:], "${")) || (c != '\x60' && text[j] == '\n') {
					return nil, false
				}
				if text[j] == c {
					end = j + 1
					break
				}
			}
			if end == 0 {
				return nil, false
			}
			literal = true
		default:
			if c == '/' && previous >= 0 && code[previous] == '}' {
				// A closing brace may end either a statement or an operand.
				if next := strings.IndexAny(text[i+1:], "/\r\n"); next >= 0 && text[i+1+next] == '/' {
					return nil, false
				}
			}
			if !strings.ContainsRune(" \t\r\n\v\f", rune(c)) {
				afterControl = false
				switch c {
				case '(':
					word := unqualifiedJavaScriptWord(code, previous)
					control := word == "if" || word == "for" || word == "while" || word == "with" || word == "switch" || word == "catch"
					controlParens = append(controlParens, control)
				case ')':
					if n := len(controlParens); n > 0 {
						afterControl = controlParens[n-1]
						controlParens = controlParens[:n-1]
					}
				}
				previous = i
			}
			i++
			continue
		}
		for j := i; j < end; j++ {
			if code[j] != '\n' {
				code[j] = ' '
			}
		}
		if literal {
			code[i] = '_'
			previous, afterControl = i, false
		}
		i = end
	}
	return code, true
}

func javascriptWordByte(c byte) bool {
	return c >= utf8.RuneSelf || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '$' || c == '\\'
}

func unqualifiedJavaScriptWord(code []byte, end int) string {
	start := end
	for start >= 0 && javascriptWordByte(code[start]) {
		start--
	}
	word := string(code[start+1 : end+1])
	for start >= 0 && strings.ContainsRune(" \t\r\n\v\f", rune(code[start])) {
		start--
	}
	if start >= 0 && (code[start] == '.' || code[start] == '#') {
		return ""
	}
	return word
}

func javascriptStartsRegexp(code []byte, previous int) bool {
	if previous < 0 || strings.ContainsRune("=([{,:;!?&|%^~*/<>", rune(code[previous])) {
		return true
	}
	if c := code[previous]; c == '+' || c == '-' {
		return previous == 0 || code[previous-1] != c
	}
	switch unqualifiedJavaScriptWord(code, previous) {
	case "return", "throw", "case", "typeof", "void", "delete", "in", "instanceof", "else", "do", "default":
		return true
	}
	return false
}

func javascriptRegexpEnd(text string, start int) int {
	lineBreak := func(s string) bool {
		return s[0] == '\n' || s[0] == '\r' || strings.HasPrefix(s, "\u2028") || strings.HasPrefix(s, "\u2029")
	}
	inClass, nestedClass := false, false
	for i := start + 1; i < len(text); i++ {
		if lineBreak(text[i:]) {
			return 0
		}
		switch text[i] {
		case '\\':
			i++
			if i >= len(text) || lineBreak(text[i:]) {
				return 0
			}
		case '[':
			nestedClass = nestedClass || inClass
			inClass = true
		case ']':
			inClass = false
		case '/':
			if inClass {
				continue
			}
			end := i + 1
			for ; end < len(text) && javascriptWordByte(text[end]); end++ {
				flag, seen := text[end], text[i+1:end]
				if !strings.ContainsRune("dgimsuvy", rune(flag)) || strings.IndexByte(seen, flag) >= 0 ||
					flag == 'v' && (nestedClass || strings.Contains(seen, "u")) || flag == 'u' && strings.Contains(seen, "v") {
					return 0
				}
			}
			return end
		}
	}
	return 0
}

func javascriptNesting(code []byte) ([]int, map[int]int, bool) {
	depth := make([]int, len(code))
	closes := map[int]int{}
	var openings []int
	for i, c := range code {
		depth[i] = len(openings)
		if strings.ContainsRune("([{", rune(c)) {
			openings = append(openings, i)
		} else if strings.ContainsRune(")]}", rune(c)) {
			if len(openings) == 0 || strings.IndexByte("([{", code[openings[len(openings)-1]]) != strings.IndexByte(")]}", c) {
				return nil, nil, false
			}
			last := openings[len(openings)-1]
			openings = openings[:len(openings)-1]
			closes[last] = i
		}
	}
	return depth, closes, len(openings) == 0
}

func nextJavaScriptToken(code []byte, at int) int {
	for at < len(code) && strings.ContainsRune(" \t\r\n", rune(code[at])) {
		at++
	}
	return at
}

func javascriptDefinitionEnd(code []byte, depth []int, closes map[int]int, at int, kind string) (int, int, bool) {
	at = nextJavaScriptToken(code, at)
	if at >= len(code) {
		return 0, 0, false
	}
	switch {
	case strings.HasPrefix(kind, "function") || kind == "method":
		if code[at] != '(' {
			return 0, 0, false
		}
		at = nextJavaScriptToken(code, closes[at]+1)
	case kind == "class":
		if strings.HasPrefix(string(code[at:]), "extends ") {
			for at < len(code) && !(depth[at] == 0 && code[at] == '{') {
				at++
			}
		}
	default:
		if code[at] != '=' {
			return 0, 0, false
		}
		start := nextJavaScriptToken(code, at+1)
		level := depth[at]
		for i := start; i < len(code); i++ {
			if depth[i] != level {
				continue
			}
			switch code[i] {
			case ',':
				return at, i + 1, i > start
			case ';':
				return at, i + 1, i > start
			case '\n':
				prior := strings.TrimSpace(string(code[start:i]))
				next := nextJavaScriptToken(code, i+1)
				if prior != "" && !strings.ContainsRune("=(,+-:?.|&>", rune(prior[len(prior)-1])) &&
					(next == len(code) || !strings.ContainsRune(".?([", rune(code[next]))) {
					return at, i, true
				}
			}
		}
		return at, len(code), start < len(code)
	}
	if at < len(code) && code[at] == '{' {
		return at, closes[at] + 1, true
	}
	return 0, 0, false
}

func javascriptDiscussionDefinitions(module, text string) ([]discussionDefinition, bool) {
	code, ok := maskJavaScript(text)
	if !ok {
		return nil, false
	}
	depth, closes, ok := javascriptNesting(code)
	if !ok {
		return nil, false
	}
	line := func(at int) int { return 1 + strings.Count(text[:at], "\n") }
	var out []discussionDefinition
	for _, match := range javascriptDestructuring.FindAllSubmatchIndex(code, -1) {
		open := match[2]
		if depth[open] != 0 {
			continue
		}
		// Grouped patterns can contain aliases and defaults. Their names
		// can veto a lookup without providing a standalone source witness.
		for _, possible := range discussionSymbolToken.FindAllString(string(code[open+1:closes[open]]), -1) {
			out = append(out, discussionDefinition{name: possible, kind: "destructuring", namespace: module,
				start: line(open), end: line(closes[open]), unquotable: true})
		}
	}
	for _, m := range javascriptDeclaration.FindAllSubmatchIndex(code, -1) {
		if depth[m[2]] != 0 {
			continue
		}
		kind, name := string(code[m[4]:m[5]]), string(code[m[6]:m[7]])
		start, end, ok := javascriptDefinitionEnd(code, depth, closes, m[7], kind)
		if !ok {
			continue
		}
		first := bytes.LastIndexByte(code[:m[2]], '\n') + 1
		rest, _, _ := bytes.Cut(code[end:], []byte{'\n'})
		sharedLine := len(bytes.TrimSpace(code[first:m[2]])) != 0 || len(bytes.TrimSpace(rest)) != 0
		if code[start] == '=' {
			grouped := code[end-1] == ','
			out = append(out, discussionDefinition{name: name, kind: kind, namespace: module,
				literal: javascriptLiteralValue(text[start+1 : end]), start: line(m[2]), end: line(end - 1), unquotable: grouped || sharedLine})
			for grouped {
				head := nextJavaScriptToken(code, end)
				id := javascriptAssignmentName.FindIndex(code[head:])
				if id == nil {
					return nil, false
				}
				start, end, ok = javascriptDefinitionEnd(code, depth, closes, head+id[1], kind)
				if !ok {
					return nil, false
				}
				out = append(out, discussionDefinition{name: string(code[head : head+id[1]]), kind: kind, namespace: module,
					literal: javascriptLiteralValue(text[start+1 : end]), start: line(head), end: line(end - 1), unquotable: true})
				grouped = code[end-1] == ','
			}
			continue
		}
		out = append(out, discussionDefinition{name: name, kind: kind, namespace: module, start: line(m[2]), end: line(end - 1), unquotable: sharedLine})
		if kind != "class" {
			continue
		}
		for _, member := range javascriptMethod.FindAllSubmatchIndex(code[start:end], -1) {
			if depth[start+member[0]] != 1 {
				continue
			}
			_, end, ok := javascriptDefinitionEnd(code, depth, closes, start+member[1]-1, "method")
			if ok {
				method := name + "." + string(code[start+member[2]:start+member[3]])
				out = append(out, discussionDefinition{name: method, kind: "method", namespace: module, start: line(start + member[0]), end: line(end - 1)})
			}
		}
	}
	return out, true
}

func javascriptLiteralValue(text string) string {
	value := strings.TrimSpace(strings.TrimRight(text, ";,"))
	if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') && value[0] == value[len(value)-1] &&
		!strings.ContainsAny(value[1:len(value)-1], "'\"\\\r\n") {
		return value[1 : len(value)-1]
	}
	return ""
}
