// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/plugin"
	"github.com/alibaba/open-code-review/internal/session"
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

func TestResumeAfterIdenticalPluginUpdate(t *testing.T) {
	for _, executable := range []bool{false, true} {
		name := "rules"
		if executable {
			name = "mcp"
		}
		t.Run(name, func(t *testing.T) {
			repo := initIdentityRepo(t)
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			ctx := context.Background()
			manifest := plugin.Manifest{SchemaVersion: 1, ID: "identity", Name: "Identity", Version: "1", Rules: []string{"rules.json"}}
			if executable {
				manifest.MCPServers = map[string]plugin.MCPServer{"tool": {Command: "node", Args: []string{"${plugin_dir}/tool.js"}}}
			}
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			source := t.TempDir()
			for name, content := range map[string]string{
				plugin.ManifestFilename: string(data),
				"rules.json":            `{"rules":[{"path":"**/*.go","rule":"Check error handling."}]}`,
				"tool.js":               "process.exit(0);\n",
			} {
				if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			store, err := plugin.DefaultStore()
			if err != nil {
				t.Fatal(err)
			}
			installed, err := store.Install(ctx, source, true)
			if err != nil {
				t.Fatal(err)
			}
			identity := func() session.RunIdentity {
				t.Helper()
				args := identityArgs(repo, 4000)
				args.SystemRule, args.FileFilter, err = rules.NewResolver(repo, "", rules.ResolverOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(args.SystemRule.Resolve("main.go"), "Check error handling.") {
					t.Fatal("identity fixture did not load its rule plugin")
				}
				sealed, err := ResolveIdentity(ctx, args)
				if err != nil {
					t.Fatal(err)
				}
				return sealed.Identity
			}
			before := identity()
			parent := &session.ResumeState{SessionID: "plugin-update", Closed: true, Manifest: &session.RunManifest{
				SchemaVersion: session.ManifestSchemaVersion, Operation: session.OperationReview,
				Input:      session.ManifestInput{Mode: before.Mode, SourceArtifactSHA256: before.SourceArtifactSHA256},
				Repository: session.ManifestRepository{IdentitySHA256: before.RepositorySHA256},
				Execution:  session.ManifestExecution{RuleConfigSHA256: before.RuleConfigSHA256},
				Coverage:   session.Coverage{Selected: []session.CoverageItem{{ItemID: "selected"}}},
			}}
			updated, err := store.Update(ctx, "identity")
			if err != nil {
				t.Fatal(err)
			}
			if installed.Dir == updated.Dir || installed.ContentSHA256 != updated.ContentSHA256 {
				t.Fatal("update must change only the snapshot path")
			}
			if err := parent.ValidateResume(session.ResumeRequest{Identity: identity()}); (err != nil) != executable {
				t.Fatalf("resume after identical %s update: %v", name, err)
			}
			if err := os.WriteFile(filepath.Join(updated.Dir, "tool.js"), []byte("process.exit(1);\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := parent.ValidateResume(session.ResumeRequest{Identity: identity()}); err == nil {
				t.Fatal("changed package contents must reject resume")
			}
		})
	}
}

func TestResumeRejectsChangedPluginScript(t *testing.T) {
	repo := initIdentityRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	ctx := context.Background()
	source := t.TempDir()
	for name, content := range map[string]string{
		plugin.ManifestFilename: `{"schema_version":1,"id":"identity","name":"Identity","version":"1","mcp_servers":{"tool":{"command":"node","args":["${plugin_dir}/tool.js"]}}}`,
		"tool.js":               "process.exit(0);\n",
	} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	store, err := plugin.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	installed, err := store.Install(ctx, source, true)
	if err != nil {
		t.Fatal(err)
	}
	identity := func() session.RunIdentity {
		t.Helper()
		args := identityArgs(repo, 4000)
		var err error
		args.SystemRule, args.FileFilter, err = rules.NewResolver(repo, "", rules.ResolverOptions{})
		if err != nil {
			t.Fatal(err)
		}
		loaded := args.SystemRule.(interface{ EnabledPlugins() []plugin.Installed }).EnabledPlugins()
		if len(loaded) != 1 || loaded[0].Manifest.ID != "identity" {
			t.Fatalf("resume fixture did not load its plugin: %+v", loaded)
		}
		sealed, err := ResolveIdentity(ctx, args)
		if err != nil {
			t.Fatal(err)
		}
		return sealed.Identity
	}
	before := identity()
	parent := &session.ResumeState{SessionID: "plugin-script", Closed: true, Manifest: &session.RunManifest{
		SchemaVersion: session.ManifestSchemaVersion, Operation: session.OperationReview,
		Input:      session.ManifestInput{Mode: before.Mode, SourceArtifactSHA256: before.SourceArtifactSHA256},
		Repository: session.ManifestRepository{IdentitySHA256: before.RepositorySHA256},
		Execution:  session.ManifestExecution{RuleConfigSHA256: before.RuleConfigSHA256},
		Coverage:   session.Coverage{Selected: []session.CoverageItem{{ItemID: "selected"}}},
	}}
	if err := parent.ValidateResume(session.ResumeRequest{Identity: identity()}); err != nil {
		t.Fatalf("unchanged plugin cannot resume: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installed.Dir, "tool.js"), []byte("process.exit(1);\n"), 0600); err != nil {
		t.Fatal(err)
	}
	after := identity()
	if after.SourceArtifactSHA256 != before.SourceArtifactSHA256 {
		t.Fatal("plugin script changed the reviewed source identity")
	}
	if err := parent.ValidateResume(session.ResumeRequest{Identity: after}); err == nil || !strings.Contains(err.Error(), "plugin configuration or package contents") {
		t.Fatalf("changed script reused the old review: %v", err)
	}
}

// TestResolveIdentityMatchesRunPath is the assertion the whole resume check rests
// on: the digest resolved before a run must equal the digest that run records.
//
// source_artifact_sha256 is computed from the sealed selected set, which is what
// selectFiles retains — not every parsed diff. A pre-flight that skipped either
// of its gates, static or size, would produce a value no run ever writes, and
// every resume comparison would then fail against a parent manifest for a reason
// that has nothing to do with the input having changed. The checks below prove
// both gates really do move the digest, so this equality is load-bearing rather
// than coincidental.
func TestResolveIdentityMatchesRunPath(t *testing.T) {
	dir := initIdentityRepo(t)
	args := identityArgs(dir, 4000)

	// Replay the review path in Run's order and take the digest
	// applyInputIdentity would hand the manifest builder.
	run := &Agent{args: args}
	if err := run.loadDiffs(context.Background()); err != nil {
		t.Fatalf("loadDiffs: %v", err)
	}
	raw := run.diffs
	rawDigest := run.sourceArtifactSHA256()

	// A zero ceiling disables the size gate, isolating the static gates' share.
	run.args.Template.MaxTokens = 0
	run.diffs, _ = summarizeSelection(run.selectFiles(raw))
	afterExtDigest := run.sourceArtifactSHA256()
	afterExtCount := len(run.diffs)

	run.args.Template.MaxTokens = args.Template.MaxTokens
	run.diffs, _ = summarizeSelection(run.selectFiles(raw))
	want := run.sourceArtifactSHA256()

	if afterExtCount >= len(raw) || len(run.diffs) >= afterExtCount {
		t.Fatalf("fixture must exercise both gates, got raw=%d ext=%d large=%d",
			len(raw), afterExtCount, len(run.diffs))
	}
	if rawDigest == afterExtDigest || afterExtDigest == want {
		t.Fatal("both gates must move the digest, otherwise this test proves nothing")
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
		// max_tokens feeds the size gate, so raising it admits files the parent
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

func TestResolveInputBeforeDiffCommit(t *testing.T) {
	dir := sealRepo(t)
	gitIn(t, dir, "remote", "add", "origin", "https://example.com/org/repo.git")
	sealed, err := ResolveIdentity(context.Background(), Args{RepoDir: dir, Commit: "HEAD"})
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	if sealed.Resolution.ResolvedHead == "" {
		t.Fatalf("commit resolve must freeze its head, got %+v", sealed.Resolution)
	}
	if sealed.Identity.RepositorySHA256 == "" {
		t.Fatal("resolved identity must include the repository identity")
	}
}

func TestResolveInputBeforeDiffRejectsInvalidRefs(t *testing.T) {
	dir := sealRepo(t)
	for _, tc := range []struct {
		name string
		args Args
	}{
		{"commit", Args{RepoDir: dir, Commit: "missing"}},
		{"range from", Args{RepoDir: dir, From: "missing", To: "feature"}},
		{"range to", Args{RepoDir: dir, From: "main", To: "missing"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ResolveIdentity(context.Background(), tc.args); err == nil {
				t.Fatal("unresolvable ref must fail before loading the diff")
			}
		})
	}
}
