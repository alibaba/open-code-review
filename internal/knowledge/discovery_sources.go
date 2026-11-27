// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	allowedext "github.com/alibaba/open-code-review/internal/config/allowlist"
	"github.com/alibaba/open-code-review/internal/diff"
)

const (
	discoveryFiles    = 16
	discoveryCommits  = 8
	discoveryComments = 12
)

func (o DiscoverOptions) protected(p string) bool {
	return !validPath(p) || o.exports[p] || privateDiscoveryPath(p) || discoveryCredential.MatchString(p) ||
		(o.allowPath != nil && !o.allowPath(p)) || (o.FileFilter != nil && o.FileFilter.IsUserExcluded(p))
}

func (o DiscoverOptions) allowed(p string) bool {
	if o.protected(p) || !inDiscoveryScope(p, o.Scope) || diff.ProviderDirPrefix(p) != "" {
		return false
	}
	if allowedext.IsExcludedPath(p) {
		ordinary, test := discoveryTestPath(p)
		if !test || allowedext.IsExcludedPath(ordinary) {
			return false
		}
	}
	if o.FileFilter != nil && o.FileFilter.IsUserIncluded(p) {
		return true
	}
	if allowedext.IsAllowedExt(path.Ext(p)) {
		return true
	}
	switch strings.ToLower(path.Base(p)) {
	case "dockerfile", "makefile", "jenkinsfile", "procfile", ".editorconfig":
		return true
	}
	return false
}

func privateDiscoveryPath(p string) bool {
	p = "/" + strings.ToLower(p)
	switch path.Ext(p) {
	case ".pem", ".key", ".p12", ".pfx", ".jks", ".keystore":
		return true
	}
	return strings.HasSuffix(p, "/.aws/credentials") || strings.HasSuffix(p, "/.docker/config.json") || strings.HasSuffix(p, "/.kube/config")
}

func discoveryKind(p string) string {
	if _, test := discoveryTestPath(p); test {
		return "test"
	}
	return discoveryFileKind(p)
}

func discoveryFileKind(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".json", ".jsonc", ".json5", ".yaml", ".yml", ".toml", ".ini", ".xml", ".properties", ".hcl", ".tfvars", ".editorconfig", ".env":
		return "config"
	case ".md", ".rst", ".adoc", ".txt":
		return "documentation"
	}
	return "code"
}

// Remove test naming conventions before rechecking the ordinary path filters.
// A generated test or a test below vendor, fixtures or testdata stays excluded.
func discoveryTestPath(p string) (string, bool) {
	ext := path.Ext(p)
	if !allowedext.IsAllowedExt(ext) || discoveryFileKind(p) != "code" {
		return p, false
	}
	parts := strings.Split(p, "/")
	last := len(parts) - 1
	test := false
	lang := strings.ToLower(ext)
	suffixes := []string{"_test", "_tests", "_unittest", "_spec"}
	specDir := false
	switch lang {
	case ".go":
		suffixes = []string{"_test"}
	case ".js", ".jsx", ".ts", ".tsx", ".mts", ".cts", ".mjs", ".cjs", ".ets":
		suffixes, specDir = []string{".test", ".spec"}, true
	case ".java", ".kt", ".kts", ".scala", ".groovy", ".cs", ".fs", ".fsi", ".fsx", ".swift", ".elm":
		suffixes = []string{"Test", "Tests", "_test"}
	case ".hs", ".lhs":
		suffixes = []string{"Spec", "_test"}
	case ".rb":
		specDir = true
	}
	for i, part := range parts[:last] {
		switch dir := strings.ToLower(part); {
		case dir == "test", dir == "tests", dir == "__tests__", dir == "unittests", dir == "spec" && specDir:
			parts[i], test = "src", true
		}
	}
	stem := strings.TrimSuffix(parts[last], ext)
	if lang == ".py" || lang == ".pyi" || lang == ".nu" {
		for _, prefix := range []string{"test_", "test-"} {
			if strings.HasPrefix(stem, prefix) {
				stem, test = strings.TrimPrefix(stem, prefix), true
				break
			}
		}
	}
	for _, suffix := range suffixes {
		if strings.HasSuffix(stem, suffix) {
			stem, test = strings.TrimSuffix(stem, suffix), true
			break
		}
	}
	if !test {
		return p, false
	}
	parts[last] = stem + ext
	return strings.Join(parts, "/"), true
}

type discoveryBlob struct {
	path string
	oid  string
	size int
}

type discoveryCommit struct {
	oid   string
	paths []string
	// A message can discuss any file in its changeset.
	withholdMessage bool
}

var errDiscoveryLimit = errors.New("discovery read limit reached")

func (t Tree) discoveryGit(ctx context.Context, limit int, args ...string) ([]byte, bool, error) {
	var out []byte
	err := t.Git.Stream(ctx, t.Repo, func(r io.Reader) error {
		var err error
		out, err = io.ReadAll(io.LimitReader(r, int64(limit+1)))
		if len(out) > limit {
			return errDiscoveryLimit
		}
		return err
	}, args...)
	if errors.Is(err, errDiscoveryLimit) {
		return out[:limit], true, nil
	}
	return out, false, err
}

func (t Tree) discoveryHistory(ctx context.Context, opts DiscoverOptions, result *DiscoveryResult) ([]discoveryCommit, error) {
	args := []string{"--literal-pathspecs", "rev-list", "--max-count=" + strconv.Itoa(discoveryCommits+1), t.Commit, "--"}
	if opts.Scope != "" {
		args = append(args, opts.Scope)
	}
	out, err := t.Git.Output(ctx, t.Repo, args...)
	if err != nil {
		return nil, fmt.Errorf("list discovery commits: %w", err)
	}
	oids := strings.Fields(string(out))
	if len(oids) > discoveryCommits {
		result.limit("sampled the latest 8 commits in scope")
		oids = oids[:discoveryCommits]
	}
	var commits []discoveryCommit
	for _, oid := range oids {
		if !validOID(oid) {
			return nil, errors.New("invalid commit in Git history")
		}
		raw, clipped, err := t.discoveryGit(ctx, 64<<10, "diff-tree", "--no-commit-id", "--root", "--first-parent", "-m", "-r", "--no-renames", "--name-only", "-z", oid)
		if err != nil {
			return nil, fmt.Errorf("list files in discovery commit: %w", err)
		}
		if clipped {
			result.limit("sampled up to 64 KiB of changed paths per commit")
		}
		c := discoveryCommit{oid: oid, withholdMessage: clipped}
		paths := bytes.Split(raw, []byte{0})
		for _, name := range paths[:len(paths)-1] {
			p := string(name)
			if opts.protected(p) {
				c.withholdMessage = true
			}
			if opts.allowed(p) {
				c.paths = append(c.paths, p)
			}
		}
		if len(c.paths) > 0 {
			commits = append(commits, c)
		}
	}
	return commits, nil
}

func (t Tree) discoveryTree(ctx context.Context, rev string, paths []string, opts DiscoverOptions, result *DiscoveryResult) ([]discoveryBlob, error) {
	args := append([]string{"--literal-pathspecs", "ls-tree", "-rlz", rev, "--"}, paths...)
	out, clipped, err := t.discoveryGit(ctx, 2<<20, args...)
	if err != nil {
		return nil, fmt.Errorf("list discovery files: %w", err)
	}
	if clipped {
		result.limit("sampled up to 2 MiB of Git tree entries")
	}
	var blobs []discoveryBlob
	entries := bytes.Split(out, []byte{0})
	for _, entry := range entries[:len(entries)-1] {
		header, name, ok := bytes.Cut(entry, []byte{'\t'})
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 4 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" || !validOID(fields[2]) || !opts.allowed(string(name)) {
			continue
		}
		size, err := strconv.Atoi(fields[3])
		if err != nil || size < 0 {
			return nil, errors.New("invalid Git tree object size")
		}
		if size > maxEvidenceBytes {
			result.limit("omitted source files larger than 1 MiB")
			continue
		}
		blobs = append(blobs, discoveryBlob{path: string(name), oid: fields[2], size: size})
	}
	return blobs, nil
}

func (t Tree) discoverySources(ctx context.Context, opts DiscoverOptions, result *DiscoveryResult) ([]discoverySource, error) {
	if validPath(opts.Scope) && slices.ContainsFunc(opts.Discussions, func(d Discussion) bool { return d.Path == "" }) {
		entry, err := t.Git.Output(ctx, t.Repo, "--literal-pathspecs", "ls-tree", "-z", t.Commit, "--", opts.Scope)
		if err != nil {
			return nil, err
		}
		header, _, _ := bytes.Cut(entry, []byte{'\t'})
		if !bytes.Contains(header, []byte(" tree ")) {
			// An absent path may retire an earlier exact file binding.
			opts.discussionFile = opts.Scope
		}
	}
	reader := newRechecker(ctx, t)
	discussions, cutoff, err := t.discoveryDiscussions(ctx, opts, result, reader)
	if err != nil {
		return nil, err
	}
	result.DiscussionSources, err = t.directDiscussionSources(reader, opts, discussions, result)
	if err != nil {
		return nil, err
	}
	opts.Discussions = latestDiscussionInputs(discussions, cutoff)
	if len(result.discussionNotices) != 0 {
		latest := map[string]time.Time{}
		for _, edit := range result.discussionNotices {
			if edit.discussionNotice == noticeExpiry {
				continue
			}
			when, _ := time.Parse(time.RFC3339Nano, edit.ObservedAt)
			if when.After(latest[edit.ID]) {
				latest[edit.ID] = when
			}
		}
		opts.Discussions = slices.DeleteFunc(opts.Discussions, func(d Discussion) bool {
			when, ok := latest[discussionSourceID(d.ID)]
			return ok && !discussionTime(d).After(when)
		})
	}
	commits, err := t.discoveryHistory(ctx, opts, result)
	if err != nil {
		return nil, err
	}
	// Resolve requested files separately so a truncated recursive tree listing
	// cannot displace the files a user is refreshing or a reviewer discussed.
	var priority []string
	prioritize := func(p string) {
		if opts.allowed(p) && !slices.Contains(priority, p) {
			if len(priority) < 64 {
				priority = append(priority, p)
			} else {
				result.limit("sampled up to 64 preferred paths")
			}
		}
	}
	for _, p := range slices.Sorted(maps.Keys(opts.FocusLines)) {
		prioritize(p)
	}
	for _, p := range opts.Paths {
		prioritize(p)
	}
	for _, d := range opts.Discussions {
		prioritize(d.Path)
	}
	explicit := len(priority)
	for _, c := range commits {
		for _, p := range c.paths {
			prioritize(p)
		}
	}
	var scope []string
	if opts.Scope != "" {
		scope = []string{opts.Scope}
	}
	blobs, err := t.discoveryTree(ctx, t.Commit, scope, opts, result)
	if err != nil {
		return nil, err
	}
	files := map[string]discoveryBlob{}
	for _, b := range blobs {
		files[b.path] = b
	}
	var missing []string
	for _, p := range priority {
		if _, ok := files[p]; !ok {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		preferred, err := t.discoveryTree(ctx, t.Commit, missing, opts, result)
		if err != nil {
			return nil, err
		}
		for _, b := range preferred {
			if _, ok := files[b.path]; !ok {
				files[b.path] = b
				blobs = append(blobs, b)
			}
		}
	}
	ranked := rankDiscoveryFiles(blobs, files, priority, explicit)
	callSources, inspected, err := t.discoveryCallers(ctx, opts, ranked, files, result)
	if err != nil {
		return nil, err
	}
	if len(callSources) > 0 {
		targets := map[string]bool{}
		for _, excerpts := range callSources {
			for _, excerpt := range excerpts {
				if link := excerpt.Caller; link != nil {
					targets[link.CalleePath] = true
				}
			}
		}
		priority := func(p string) int {
			switch {
			case targets[p]:
				return 0
			case len(callSources[p]) != 0:
				return 1
			default:
				return 2
			}
		}
		slices.SortStableFunc(ranked, func(a, b discoveryBlob) int { return priority(a.path) - priority(b.path) })
	}
	var sources []discoverySource
	selected := map[string]string{}
	for _, b := range ranked {
		if !inspected[b.path] && len(inspected) >= 32 {
			result.limit("inspected up to 32 candidate blobs")
			continue
		}
		if len(selected) == discoveryFiles {
			result.limit("sampled up to 16 current files")
			break
		}
		inspected[b.path] = true
		excerpts := callSources[b.path]
		if len(excerpts) == 0 {
			source := discoverySource{Kind: discoveryKind(b.path), Path: b.path, Revision: t.Commit}
			excerpts, err = t.discoveryBlobSources(ctx, b, source, 3200, result, opts.FocusLines[b.path])
			if err != nil {
				return nil, err
			}
		}
		if len(excerpts) > 0 {
			sources = append(sources, excerpts...)
			selected[b.path] = b.oid
		}
	}
	if len(selected) == 0 {
		return sources, nil
	}
	historical, err := t.discoveryPastSources(ctx, opts, commits, selected, result)
	if err != nil {
		return nil, err
	}
	sources = append(sources, historical...)
	comments := 0
	for _, d := range opts.Discussions {
		if comments == discoveryComments {
			result.limit("sampled up to 12 discussion comments")
			break
		}
		s := discoverySource{Kind: "discussion", Path: d.Path, Revision: d.Revision, Reference: d.ID, URL: d.URL, ObservedAt: d.UpdatedAt, ExpiresAt: d.ExpiresAt, Landing: d.landing}
		excerpts := discoveryExcerpts([]byte(d.Body), s, 1600, result, nil)
		if len(excerpts) > 0 {
			comments++
			sources = append(sources, excerpts...)
		}
	}
	return sources, nil
}

func rankDiscoveryFiles(blobs []discoveryBlob, files map[string]discoveryBlob, priority []string, explicit int) []discoveryBlob {
	var ranked, code, config []discoveryBlob
	seen := map[string]bool{}
	add := func(b discoveryBlob, preferred bool) {
		if b.oid == "" || seen[b.path] {
			return
		}
		seen[b.path] = true
		switch {
		case preferred:
			ranked = append(ranked, b)
		case discoveryKind(b.path) == "config":
			config = append(config, b)
		default:
			code = append(code, b)
		}
	}
	for i, p := range priority {
		add(files[p], i < explicit)
	}
	for _, b := range blobs {
		add(b, false)
	}
	// An unchanged sibling often holds the policy a changed caller consumes.
	directories := map[string]int{}
	companions := map[string]bool{}
	for _, p := range priority[:explicit] {
		plain, _ := discoveryTestPath(p)
		companions[plain] = true
		for d := path.Dir(p); d != "."; d = path.Dir(d) {
			directories[d] = strings.Count(d, "/") + 1
		}
	}
	proximity := func(b discoveryBlob) int {
		if p, test := discoveryTestPath(b.path); test && companions[p] {
			return 1 + len(directories)
		}
		for d := path.Dir(b.path); d != "."; d = path.Dir(d) {
			if score := directories[d]; score > 0 {
				return score
			}
		}
		return 0
	}
	nearFirst := func(a, b discoveryBlob) int { return proximity(b) - proximity(a) }
	slices.SortStableFunc(code, nearFirst)
	slices.SortStableFunc(config, nearFirst)
	// Keep configuration represented alongside implementation in larger trees.
	for len(code)+len(config) > 0 {
		n := min(3, len(code))
		ranked = append(ranked, code[:n]...)
		code = code[n:]
		if len(config) > 0 {
			ranked = append(ranked, config[0])
			config = config[1:]
		}
	}
	return ranked
}

func (t Tree) discoveryBlobSources(ctx context.Context, b discoveryBlob, s discoverySource, budget int, result *DiscoveryResult, focus []int) ([]discoverySource, error) {
	data, err := t.Git.Output(ctx, t.Repo, "cat-file", "blob", b.oid)
	if err != nil {
		return nil, fmt.Errorf("read discovery blob: %w", err)
	}
	if len(data) != b.size {
		return nil, errors.New("Git blob size differs from tree entry")
	}
	return discoveryExcerpts(data, s, budget, result, focus), nil
}

func (t Tree) discoveryPastSources(ctx context.Context, opts DiscoverOptions, commits []discoveryCommit, selected map[string]string, result *DiscoveryResult) ([]discoverySource, error) {
	var sources []discoverySource
	historyBlobs := map[string]bool{}
	for _, c := range commits {
		var paths []string
		for _, p := range c.paths {
			if selected[p] != "" && !slices.Contains(paths, p) {
				paths = append(paths, p)
			}
		}
		if len(paths) == 0 {
			continue
		}
		s := discoverySource{Kind: "history", Revision: c.oid, Reference: c.oid}
		if !c.withholdMessage {
			msg, clipped, err := t.discoveryGit(ctx, 32<<10, "show", "--no-patch", "--no-notes", "--no-show-signature", "--format=format:%B", c.oid)
			if err != nil {
				return nil, fmt.Errorf("read discovery commit message: %w", err)
			}
			if clipped {
				result.limit("omitted commit messages larger than 32 KiB")
			} else {
				sources = append(sources, discoveryExcerpts(msg, s, 1200, result, nil)...)
			}
		} else {
			result.limit("withheld commit messages covering excluded or uninspected paths")
		}
		if c.oid == t.Commit {
			continue
		}
		if len(historyBlobs) >= 4 {
			result.limit("sampled up to 4 different historical file versions")
			continue
		}
		previous, err := t.discoveryTree(ctx, c.oid, paths, opts, result)
		if err != nil {
			return nil, err
		}
		for _, b := range previous {
			key := b.path + "\x00" + b.oid
			if head := selected[b.path]; head == "" || head == b.oid || historyBlobs[key] {
				continue
			}
			if len(historyBlobs) >= 4 {
				result.limit("sampled up to 4 different historical file versions")
				break
			}
			s.Path = b.path
			excerpts, err := t.discoveryBlobSources(ctx, b, s, 1600, result, nil)
			if err != nil {
				return nil, err
			}
			sources = append(sources, excerpts...)
			historyBlobs[key] = true
		}
	}
	return sources, nil
}

const credentialKey = `(?:api[_-]?key|auth[_-]?token|access[_-]?token|client[_-]?secret|password|passwd)`

var discoveryCredential = regexp.MustCompile(`(?im)-----BEGIN [A-Z ]*PRIVATE KEY-----|\b(?:github_pat_|gh[pousr]_|glpat-|sk-(?:proj-|live_)?)[a-z0-9_-]{16,}|\b(?:AKIA|ASIA)[A-Z0-9]{16}\b|\bbearer[ \t]+[a-z0-9._~+/=-]{12,}|://[^\s/@:]+:[^\s/@]+@|` +
	credentialKey + `["']?\s*[:=]\s*["'][^"'\s$]{4,}["']|^[ \t]*(?:export[ \t]+)?(?:[a-z0-9_.-]+[_.-])?(?:` + credentialKey + `|token|secret)` +
	`[ \t]*[:=][ \t]*[a-z0-9_+./:@!?%=-][a-z0-9_+./:@!?%$=-]*[ \t]*(?:[#;][^\r\n]*)?\r?$`)

var decisionLine = regexp.MustCompile(`(?i)\b(if|else|unless|switch|case|when|require|assert|check|constraint|limit|minimum|maximum|enum|default|reject|throw|raise)\b`)

func discoveryExcerpts(data []byte, source discoverySource, budget int, result *DiscoveryResult, focus []int) []discoverySource {
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil
	}
	if discoveryCredential.Match(data) {
		result.limit("omitted sources containing credential-shaped text")
		return nil
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if !source.current() {
		source.digest = Digest([]byte(text))
	}
	text = strings.TrimSuffix(text, "\n")
	if len(text) <= budget {
		source.StartLine, source.Text = 1, text
		return []discoverySource{source}
	}
	result.limit("sampled excerpts of longer sources")
	if len(focus) > 32 {
		result.limit("sampled up to 32 focus lines per file")
	}
	lines := strings.Split(text, "\n")
	type window struct{ start, end, score int }
	var windows []window
	for i := 0; i < len(lines); {
		start, used, score := i, 0, 0
		for i < len(lines) && i-start < 60 && used+len(lines[i])+1 <= budget/2 {
			used += len(lines[i]) + 1
			if decisionLine.MatchString(lines[i]) {
				score++
			}
			i++
		}
		if start == i {
			i++ // A line alone exceeds the window. Never quote a partial line.
		} else if strings.TrimSpace(strings.Join(lines[start:i], "\n")) != "" {
			for _, n := range focus[:min(32, len(focus))] {
				if n > start && n <= i {
					score += 100
				}
			}
			windows = append(windows, window{start, i, score})
		}
	}
	slices.SortStableFunc(windows, func(a, b window) int { return b.score - a.score })
	windows = windows[:min(2, len(windows))]
	slices.SortFunc(windows, func(a, b window) int { return a.start - b.start })
	var excerpts []discoverySource
	for _, w := range windows {
		source.StartLine, source.Text = w.start+1, strings.Join(lines[w.start:w.end], "\n")
		excerpts = append(excerpts, source)
	}
	return excerpts
}

// HistoricalDiscussionCutoff returns the selected commit time for an older
// revision. An unset time means a live sync of the currently checked-out HEAD.
func (t Tree) HistoricalDiscussionCutoff(ctx context.Context) (time.Time, error) {
	head, err := t.Git.Output(ctx, t.Repo, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(string(head)) == t.Commit {
		return time.Time{}, err
	}
	raw, err := t.Git.Output(ctx, t.Repo, "show", "-s", "--no-notes", "--no-show-signature", "--format=%cI", t.Commit)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, strings.TrimSpace(string(raw)))
}

func (t Tree) discoveryDiscussions(ctx context.Context, opts DiscoverOptions, result *DiscoveryResult, r *rechecker) ([]Discussion, time.Time, error) {
	if err := validateDiscussions(opts.Discussions); err != nil || len(opts.Discussions) == 0 {
		return nil, time.Time{}, err
	}
	cutoff := opts.AsOf
	if cutoff.IsZero() {
		var err error
		cutoff, err = t.HistoricalDiscussionCutoff(ctx)
		if err != nil {
			return nil, time.Time{}, err
		}
	}
	strict := !cutoff.IsZero()
	if !strict {
		cutoff = t.Now
		if cutoff.IsZero() {
			cutoff = time.Now()
		}
	}
	var eligible []Discussion
	originals := landingFiles{}
	for _, d := range opts.Discussions {
		if discoveryCredential.MatchString(d.ID) || discoveryCredential.MatchString(d.URL) {
			result.limit("omitted sources containing credential-shaped text")
			continue
		}
		if d.UpdatedAt == "" && strict {
			result.limit("withheld discussions without a last-edit timestamp at the historical cutoff")
			continue
		}
		if discussionTime(d).After(cutoff) {
			result.limit("withheld discussions edited after the cutoff")
			continue
		}
		if d.Revision != "" && !r.ancestor(d.Revision) {
			var landed bool
			if d.remote != nil && opts.allowed(d.Path) && discoveryKind(d.Path) != "documentation" && !discoveryCredential.MatchString(d.Body) {
				d, landed = r.landDiscussion(d, cutoff, originals, result)
			}
			if !landed {
				result.limit("withheld discussions outside the accepted Git history")
				continue
			}
		}
		if d.Path != "" && !inDiscoveryScope(d.Path, opts.Scope) {
			// An edit that moved out of the requested scope can still retire
			// its earlier edition inside that scope.
			result.withholdEdition(t.Commit, d, Digest([]byte(opts.Scope)))
			continue
		}
		p := d.Path
		if p == "" {
			p = opts.discussionFile
		}
		if p != "" && (!opts.allowed(p) || discoveryKind(p) == "documentation") || discoveryCredential.MatchString(d.Body) {
			result.withholdFilteredEdition(t.Commit, d, opts.discussionFile)
			continue
		}
		if d.Revision != "" && d.Revision != t.Commit && opts.allowed(p) && discoveryKind(p) != "documentation" {
			prior, current := r.atRevision(d.Revision).file(p), r.file(p)
			for _, file := range []checkedFile{prior, current} {
				if file.err != nil && !errors.Is(file.err, errIneligibleFile) {
					return nil, time.Time{}, file.err
				}
			}
			if !sameDiscussionFile(p, prior, current) {
				result.limit("withheld discussions whose original implementation could not be located")
				result.withholdFilteredEdition(t.Commit, d, opts.discussionFile)
				continue
			}
		}
		result.recordDiscussionExpiry(t.Commit, d, opts.Scope)
		eligible = append(eligible, d)
	}
	// Prefer the latest available correction when a long conversation is sampled.
	slices.SortStableFunc(eligible, func(a, b Discussion) int {
		return discussionTime(b).Compare(discussionTime(a))
	})
	return eligible, cutoff, nil
}

func latestDiscussionInputs(eligible []Discussion, cutoff time.Time) []Discussion {
	positions := map[string]int{}
	var active []Discussion
	for _, d := range eligible {
		if i, ok := positions[d.ID]; ok {
			if d.landing != nil && active[i].landing == nil && discussionTime(d).Equal(discussionTime(active[i])) {
				active[i] = d
			}
			continue
		}
		positions[d.ID] = len(active)
		active = append(active, d)
	}
	conflicts := map[string]bool{}
	for _, d := range eligible {
		best := active[positions[d.ID]]
		if discussionTime(d).Equal(discussionTime(best)) && (d.Body != best.Body || d.Path != best.Path || d.URL != best.URL ||
			d.ExpiresAt != best.ExpiresAt || !matchesDiscussionOrigin(best.Revision, best.landing.value(), d.Revision, d.landing)) {
			conflicts[d.ID] = true
		}
	}
	return slices.DeleteFunc(active, func(d Discussion) bool {
		expires, _ := time.Parse(time.RFC3339Nano, d.ExpiresAt)
		return conflicts[d.ID] || d.ExpiresAt != "" && !cutoff.Before(expires)
	})
}

func discussionTime(d Discussion) time.Time {
	stamp := d.UpdatedAt
	if stamp == "" {
		stamp = d.CreatedAt
	}
	when, _ := time.Parse(time.RFC3339, stamp) // validateDiscussions already checked nonempty timestamps.
	return when
}
