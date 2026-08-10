// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/config/template"
)

// initIdentityRepo builds a workspace with two reviewable files, one the
// extension rules drop, and one large enough for the token filter to drop.
func initIdentityRepo(t *testing.T) string {
	t.Helper()
	dir := initPreviewRepo(t)
	for name, content := range map[string]string{
		"main.go":   "package main\n\nfunc main() {}\n",
		"keep.go":   "package main\n\nfunc keep() {}\n",
		"notes.xyz": "not a reviewable extension\n",
		"huge.go":   "package main\n\n// " + strings.Repeat("filler ", 4000) + "\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func identityArgs(dir string, maxTokens int) Args {
	return Args{RepoDir: dir, Template: template.Template{MaxTokens: maxTokens}}
}

// TestResolveIdentityMatchesRunPath is the assertion the whole resume check rests
// on: the digest resolved before a run must equal the digest that run records.
//
// source_artifact_sha256 is computed from the sealed selected set, which is what
// survives filterDiffs and filterLargeDiffs — not every parsed diff. A pre-flight
// that skipped either pass would produce a value no run ever writes, and every
// resume comparison would then fail against a parent manifest for a reason that
// has nothing to do with the input having changed. The subtests below prove both
// filters really do move the digest, so this equality is load-bearing rather than
// coincidental.
func TestResolveIdentityMatchesRunPath(t *testing.T) {
	dir := initIdentityRepo(t)
	args := identityArgs(dir, 4000)

	// Replay the review path in Run's order and take the digest
	// applyInputIdentity would hand the manifest builder.
	run := &Agent{args: args}
	if err := run.loadDiffs(context.Background()); err != nil {
		t.Fatalf("loadDiffs: %v", err)
	}
	rawDigest := run.sourceArtifactSHA256()
	rawCount := len(run.diffs)

	run.diffs = run.filterDiffs(run.diffs)
	afterExtDigest := run.sourceArtifactSHA256()
	afterExtCount := len(run.diffs)

	run.diffs = run.filterLargeDiffs(run.diffs)
	want := run.sourceArtifactSHA256()

	if afterExtCount >= rawCount || len(run.diffs) >= afterExtCount {
		t.Fatalf("fixture must exercise both filters, got raw=%d ext=%d large=%d",
			rawCount, afterExtCount, len(run.diffs))
	}
	if rawDigest == afterExtDigest || afterExtDigest == want {
		t.Fatal("both filters must move the digest, otherwise this test proves nothing")
	}

	sealed, err := ResolveIdentity(context.Background(), args)
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	got := sealed.Identity
	if got.SourceArtifactSHA256 != want {
		t.Errorf("source_artifact_sha256 drifted between the two entry points:\n pre-flight = %s\n run path   = %s",
			got.SourceArtifactSHA256, want)
	}
	if got.Mode == "" {
		t.Error("mode is mandatory in the manifest and must never resolve empty")
	}
	if len(got.SourceArtifactSHA256) != 64 || len(got.RuleConfigSHA256) != 64 {
		t.Errorf("digests must be 64 hex chars, got %d and %d",
			len(got.SourceArtifactSHA256), len(got.RuleConfigSHA256))
	}
}

func TestResolveIdentityTracksConfigChanges(t *testing.T) {
	dir := initIdentityRepo(t)

	baseSealed, err := ResolveIdentity(context.Background(), identityArgs(dir, 4000))
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	base := baseSealed.Identity

	t.Run("is deterministic", func(t *testing.T) {
		again, err := ResolveIdentity(context.Background(), identityArgs(dir, 4000))
		if err != nil {
			t.Fatalf("ResolveIdentity: %v", err)
		}
		if again.Identity != base {
			t.Errorf("identity is not deterministic:\n%+v\n%+v", again.Identity, base)
		}
	})

	t.Run("an exclude that drops a file moves both digests", func(t *testing.T) {
		args := identityArgs(dir, 4000)
		args.FileFilter = &rules.FileFilter{Exclude: []string{"keep.go"}}
		sealed, err := ResolveIdentity(context.Background(), args)
		if err != nil {
			t.Fatalf("ResolveIdentity: %v", err)
		}
		got := sealed.Identity
		if got.RuleConfigSHA256 == base.RuleConfigSHA256 {
			t.Error("rule_config_sha256 must move when the file filter changes")
		}
		if got.SourceArtifactSHA256 == base.SourceArtifactSHA256 {
			t.Error("source_artifact_sha256 must move when a selected file disappears")
		}
	})

	t.Run("an exclude that matches nothing moves only the rule digest", func(t *testing.T) {
		// This is why the resume check compares source_artifact before
		// rule_config: a filter change that really dropped files is reported as
		// the input change it caused, and only a filter change with no effect on
		// the selected set falls through to the unattributable rule digest.
		args := identityArgs(dir, 4000)
		args.FileFilter = &rules.FileFilter{Exclude: []string{"no/such/path/**"}}
		sealed, err := ResolveIdentity(context.Background(), args)
		if err != nil {
			t.Fatalf("ResolveIdentity: %v", err)
		}
		got := sealed.Identity
		if got.SourceArtifactSHA256 != base.SourceArtifactSHA256 {
			t.Error("the selected set did not change, so source_artifact_sha256 must not either")
		}
		if got.RuleConfigSHA256 == base.RuleConfigSHA256 {
			t.Error("rule_config_sha256 must still move")
		}
	})

	t.Run("max tokens moves the input identity", func(t *testing.T) {
		// max_tokens feeds filterLargeDiffs, so raising it admits files the parent
		// run had dropped. The input identity changes even though no file did, and
		// the resume is rejected as an input change.
		sealed, err := ResolveIdentity(context.Background(), identityArgs(dir, 4_000_000))
		if err != nil {
			t.Fatalf("ResolveIdentity: %v", err)
		}
		got := sealed.Identity
		if got.SourceArtifactSHA256 == base.SourceArtifactSHA256 {
			t.Error("a higher token ceiling admits huge.go and must move source_artifact_sha256")
		}
		if got.RuleConfigSHA256 != base.RuleConfigSHA256 {
			t.Error("max_tokens is not part of the rule configuration")
		}
	})
}
