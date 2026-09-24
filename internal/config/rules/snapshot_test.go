// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestSnapshotProjectRulesAndDocuments(t *testing.T) {
	setTestHome(t, t.TempDir())
	dir, ref := initRepo(t, map[string]string{
		".opencodereview/rule.json": `{"exclude":["blocked.go"],"rules":[{"path":"**/*.go","rule":"docs/review.md"}]}`,
		"docs/review.md":            "frozen rule\n",
	})
	writeFile(t, dir, ".opencodereview/rule.json", `{"exclude":["live.go"],"rules":[{"path":"**/*.go","rule":"live rule"}]}`)
	writeFile(t, dir, "docs/review.md", "live document")
	r, filter, err := NewResolver(dir, "", ResolverOptions{Ref: ref, ProjectRef: ref})
	if err != nil {
		t.Fatal(err)
	}
	if r.Resolve("src/main.go") != "frozen rule" || filter == nil || len(filter.Exclude) != 1 || filter.Exclude[0] != "blocked.go" {
		t.Fatalf("rules escaped snapshot: %q, %+v", r.Resolve("src/main.go"), filter)
	}
	custom := dir + "/custom.json"
	writeFile(t, dir, "custom.json", `{"rules":[{"path":"**/*.go","rule":"explicit override"}]}`)
	r, _, err = NewResolver(dir, custom, ResolverOptions{Ref: ref, ProjectRef: ref})
	if err != nil || r.Resolve("src/main.go") != "explicit override" {
		t.Fatalf("explicit rule override lost: %v", err)
	}
}

func TestSnapshotMissingProjectRuleDoesNotReadDisk(t *testing.T) {
	setTestHome(t, t.TempDir())
	dir, ref := initRepo(t, map[string]string{"a.go": "package a\n"})
	writeFile(t, dir, ".opencodereview/rule.json", "invalid live JSON")
	_, filter, err := NewResolver(dir, "", ResolverOptions{ProjectRef: ref, Runner: gitcmd.New(1)})
	if err != nil || filter != nil {
		t.Fatalf("read unstaged project configuration: %v, %+v", err, filter)
	}
}

func TestSnapshotInvalidRulesFailClearly(t *testing.T) {
	setTestHome(t, t.TempDir())
	for _, tc := range []struct {
		name, config string
		files        map[string]string
	}{
		{"invalid JSON", "{", nil},
		{"missing document", `{"rules":[{"path":"**","rule":"missing.md"}]}`, nil},
		{"outside document", `{"rules":[{"path":"**","rule":"../outside.md"}]}`, nil},
		{"absolute document", `{"rules":[{"path":"**","rule":"/outside.md"}]}`, nil},
		{"directory document", `{"rules":[{"path":"**","rule":"dir.md"}]}`, map[string]string{"dir.md/a": "x"}},
		{"oversized document", `{"rules":[{"path":"**","rule":"large.md"}]}`, map[string]string{"large.md": strings.Repeat("x", 512*1024+1)}},
		{"oversized config", `{"rules":[]}` + strings.Repeat(" ", 512*1024), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{".opencodereview/rule.json": tc.config}
			for k, v := range tc.files {
				files[k] = v
			}
			dir, ref := initRepo(t, files)
			if _, _, err := NewResolver(dir, "", ResolverOptions{ProjectRef: ref}); err == nil {
				t.Fatal("invalid snapshot rules were accepted")
			}
		})
	}
	dir, _ := initRepo(t, map[string]string{"a.md": "rule"})
	if _, _, err := readSnapshotRule(context.Background(), dir, "missing-ref", "a.md", gitcmd.New(1)); err == nil || !strings.Contains(err.Error(), "missing-ref") {
		t.Fatalf("invalid snapshot must report the Git failure: %v", err)
	}
}

func TestSnapshotRejectsSymlinkRuleDocument(t *testing.T) {
	setTestHome(t, t.TempDir())
	dir, _ := initRepo(t, map[string]string{
		".opencodereview/rule.json": `{"rules":[{"path":"**","rule":"link.md"}]}`,
		"link.md":                   "target.md",
		"target.md":                 "do not follow this rule document",
	})
	runner := gitcmd.New(1)
	ctx := context.Background()
	git := func(args ...string) string {
		t.Helper()
		out, stderr, err := runner.RunSplit(ctx, dir, args...)
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, stderr)
		}
		return strings.TrimSpace(out)
	}
	// Set the index mode directly so the fixture works without OS symlink rights.
	blob := git("rev-parse", "HEAD:link.md")
	git("update-index", "--cacheinfo", "120000,"+blob+",link.md")
	ref := git("write-tree")
	if _, _, err := NewResolver(dir, "", ResolverOptions{ProjectRef: ref, Runner: runner}); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("symlink rule document was not rejected: %v", err)
	}
}
