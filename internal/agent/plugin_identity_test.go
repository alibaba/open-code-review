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
	"github.com/alibaba/open-code-review/internal/plugin"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestResumeAfterRulePluginReinstall(t *testing.T) {
	repo, home, source := initIdentityRepo(t), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	ctx := context.Background()
	for name, content := range map[string]string{
		plugin.ManifestFilename: `{"schema_version":1,"id":"identity","name":"Identity","version":"1","rules":["rules.json"]}`,
		"rules.json":            `{"rules":[{"path":"**/*.go","rule":"Check error handling."}]}`,
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
		sealed, err := ResolveIdentity(ctx, args)
		if err != nil {
			t.Fatal(err)
		}
		return sealed.Identity
	}
	before := identity()
	parent := &session.ResumeState{SessionID: "rule-plugin", Closed: true, Manifest: &session.RunManifest{
		SchemaVersion: session.ManifestSchemaVersion, Operation: session.OperationReview,
		Input:      session.ManifestInput{Mode: before.Mode, SourceArtifactSHA256: before.SourceArtifactSHA256},
		Repository: session.ManifestRepository{IdentitySHA256: before.RepositorySHA256},
		Execution:  session.ManifestExecution{RuleConfigSHA256: before.RuleConfigSHA256},
		Coverage:   session.Coverage{Selected: []session.CoverageItem{{ItemID: "selected"}}},
	}}
	if err := store.Remove("identity"); err != nil {
		t.Fatal(err)
	}
	reinstalled, err := store.Install(ctx, source, true)
	if err != nil {
		t.Fatal(err)
	}
	if installed.Dir == reinstalled.Dir || installed.ContentSHA256 != reinstalled.ContentSHA256 {
		t.Fatal("reinstall did not change only the snapshot location")
	}
	if err := parent.ValidateResume(session.ResumeRequest{Identity: identity()}); err != nil {
		t.Fatalf("identical reinstall invalidated reuse: %v", err)
	}
	if err := os.WriteFile(filepath.Join(reinstalled.Dir, "rules.json"), []byte(`{"rules":[{"path":"**/*.go","rule":"Check transactions."}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	after := identity()
	if before.SourceArtifactSHA256 != after.SourceArtifactSHA256 {
		t.Fatal("plugin changed source identity")
	}
	if err := parent.ValidateResume(session.ResumeRequest{Identity: after}); err == nil || !strings.Contains(err.Error(), "plugin configuration or package contents") {
		t.Fatalf("changed rules reused checkpoints: %v", err)
	}
}
