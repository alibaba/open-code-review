// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectInspectionDoesNotAuthorizeOrRelaxIntegrity(t *testing.T) {
	ctx := context.Background()
	p, source := testProject(t, NewStore(t.TempDir()))
	if view, err := p.Inspect(ctx); err != nil || view.Present || view.Authorized {
		t.Fatalf("empty = %+v, %v", view, err)
	}
	m := testManifest("example", "1")
	m.MCPServers = map[string]MCPServer{"tools": {Command: "must-not-execute"}}
	writeTestJSON(t, filepath.Join(source, ManifestFilename), m)
	if _, err := p.Add(ctx, source, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Inspect(ctx); err == nil {
		t.Fatal("missing snapshot was accepted")
	}
	if _, err := p.Sync(ctx, true); err != nil {
		t.Fatal(err)
	}
	for _, trust := range []bool{false, true, false} {
		if err := p.Trust(ctx, !trust); err != nil {
			t.Fatal(err)
		}
		view, err := p.Inspect(ctx)
		if err != nil || !view.Present || view.Authorized != trust || len(view.Plugins) != 1 {
			t.Fatalf("inspection = %+v, %v", view, err)
		}
		_, _, loadErr := p.Load(ctx)
		if !trust {
			requireError(t, loadErr, "PLUGIN_EXEC_NOT_AUTHORIZED")
		} else if loadErr != nil {
			t.Fatal(loadErr)
		}
	}
	view, _ := p.Inspect(ctx)
	if err := os.WriteFile(filepath.Join(view.Plugins[0].Dir, "rules.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := p.Inspect(ctx)
	requireError(t, err, "ARTIFACT_MISMATCH")
	if err := os.WriteFile(filepath.Join(p.root, ".opencodereview", projectLockFile), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = p.Inspect(ctx)
	requireError(t, err, "LOCK_MISMATCH")
}
