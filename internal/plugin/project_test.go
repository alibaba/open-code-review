// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func testProject(t *testing.T, store *Store) (*Project, string) {
	t.Helper()
	repo := t.TempDir()
	source := filepath.Join(repo, "plugins", "example")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, filepath.Join(source, ManifestFilename), testManifest("example", "1"))
	writeTestJSON(t, filepath.Join(source, "rules.json"), map[string]any{"default_rule": "Review carefully"})
	p, err := store.Project(repo)
	if err != nil {
		t.Fatal(err)
	}
	return p, source
}

func TestProjectFrozenLocalLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	p, source := testProject(t, store)
	if items, present, err := p.Load(ctx); err != nil || present || len(items) != 0 {
		t.Fatalf("undeclared project = %v, %v, %v", items, present, err)
	}
	lock, err := p.Add(ctx, source, "")
	if err != nil {
		t.Fatal(err)
	}
	entry := lock.Platforms[projectPlatform()][0]
	if entry.Source != "./plugins/example" || entry.ArchiveSHA256 != "" || entry.Directory != "." {
		t.Fatalf("local lock = %+v", entry)
	}
	_, _, err = p.Load(ctx)
	requireError(t, err, "ARTIFACT_MISSING")
	items, err := p.Sync(ctx, true)
	if err != nil || len(items) != 1 {
		t.Fatalf("local frozen sync = %v, %v", items, err)
	}
	if _, present, err := p.Load(ctx); !present || err != nil {
		t.Fatalf("data-only preflight = %v, %v", present, err)
	}
	// A frozen run uses the installed snapshot, not the mutable source tree.
	writeTestJSON(t, filepath.Join(source, ManifestFilename), testManifest("example", "2"))
	if loaded, _, err := p.Load(ctx); err != nil || loaded[0].Manifest.Version != "1" {
		t.Fatalf("source edits changed frozen load: %v, %v", loaded, err)
	}
	otherStore := NewStore(t.TempDir())
	other, _ := otherStore.Project(p.root)
	_, err = other.Sync(ctx, true)
	requireError(t, err, "ARTIFACT_MISMATCH")
	if _, err := p.Lock(ctx); err != nil {
		t.Fatal(err)
	}
	items, err = p.Sync(ctx, true)
	if err != nil || items[0].Manifest.Version != "2" {
		t.Fatalf("explicit relock = %v, %v", items, err)
	}
	writeTestJSON(t, filepath.Join(items[0].Dir, "rules.json"), map[string]string{"default_rule": "Changed installed bytes"})
	_, _, err = p.Load(ctx)
	requireError(t, err, "ARTIFACT_MISMATCH")
	_, err = p.Sync(ctx, true)
	requireError(t, err, "ARTIFACT_MISMATCH")
	if global, err := store.List(); err != nil || len(global) != 0 {
		t.Fatalf("project changed global plugins: %v, %v", global, err)
	}
}

func TestProjectExecutionTrustIsLocalAndBoundToLock(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	p, source := testProject(t, store)
	m := testManifest("example", "1")
	m.MCPServers = map[string]MCPServer{"tool": {Command: "must-never-execute", Args: []string{"private-value"}}}
	writeTestJSON(t, filepath.Join(source, ManifestFilename), m)
	if _, err := p.Add(ctx, source, ""); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(p.root, ".opencodereview", projectLockFile))
	if strings.Contains(string(data), "private-value") || strings.Contains(string(data), store.root) {
		t.Fatal("lock includes private execution values or machine paths")
	}
	if err := p.Trust(ctx, false); err == nil {
		t.Fatal("authorized a package before installation")
	}
	if _, err := p.Sync(ctx, true); err != nil {
		t.Fatal(err)
	}
	_, _, err := p.Load(ctx)
	requireError(t, err, "PLUGIN_EXEC_NOT_AUTHORIZED")
	if err := p.Trust(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.Load(ctx); err != nil {
		t.Fatal(err)
	}
	other, otherSource := testProject(t, store)
	writeTestJSON(t, filepath.Join(otherSource, ManifestFilename), m)
	if _, err := other.Add(ctx, otherSource, ""); err != nil {
		t.Fatal(err)
	}
	otherItems, err := other.Sync(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	items, _, err := p.Load(ctx)
	if err != nil || items[0].Dir == otherItems[0].Dir {
		t.Fatalf("different projects share an installed snapshot: %v", err)
	}
	_, _, err = other.Load(ctx)
	requireError(t, err, "PLUGIN_EXEC_NOT_AUTHORIZED")
	if err := p.Trust(ctx, true); err != nil {
		t.Fatal(err)
	}
	_, _, err = p.Load(ctx)
	requireError(t, err, "PLUGIN_EXEC_NOT_AUTHORIZED")
	if err := p.Trust(ctx, false); err != nil {
		t.Fatal(err)
	}
	m.Version = "2"
	writeTestJSON(t, filepath.Join(source, ManifestFilename), m)
	if _, err := p.Lock(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Sync(ctx, true); err != nil {
		t.Fatal(err)
	}
	_, _, err = p.Load(ctx)
	requireError(t, err, "PLUGIN_EXEC_NOT_AUTHORIZED")
}

func TestProjectFingerprintVerifiesContentAndTrust(t *testing.T) {
	ctx := context.Background()
	p, source := testProject(t, NewStore(t.TempDir()))
	_, err := p.Fingerprint(ctx)
	requireError(t, err, "LOCK_MISMATCH")
	m := testManifest("example", "1")
	m.MCPServers = map[string]MCPServer{"tool": {Command: "must-never-execute"}}
	writeTestJSON(t, filepath.Join(source, ManifestFilename), m)
	if _, err := p.Add(ctx, source, ""); err != nil {
		t.Fatal(err)
	}
	_, err = p.Fingerprint(ctx)
	requireError(t, err, "ARTIFACT_MISSING")
	items, err := p.Sync(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Fingerprint(ctx)
	requireError(t, err, "PLUGIN_EXEC_NOT_AUTHORIZED")
	if err := p.Trust(ctx, false); err != nil {
		t.Fatal(err)
	}
	before, err := p.Fingerprint(ctx)
	if err != nil || len(before) != 64 {
		t.Fatalf("verified fingerprint = %q, %v", before, err)
	}
	other, err := NewStore(t.TempDir()).Project(p.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Sync(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := other.Trust(ctx, false); err != nil {
		t.Fatal(err)
	}
	if got, err := other.Fingerprint(ctx); err != nil || got != before {
		t.Fatalf("fingerprint depends on private store path: %q, %v", got, err)
	}
	writeTestJSON(t, filepath.Join(items[0].Dir, "rules.json"), map[string]string{"default_rule": "Tampered snapshot"})
	_, err = p.Fingerprint(ctx)
	requireError(t, err, "ARTIFACT_MISMATCH")
	writeTestJSON(t, filepath.Join(source, "rules.json"), map[string]string{"default_rule": "Approved new content"})
	if _, err := other.Lock(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Sync(ctx, true); err != nil {
		t.Fatal(err)
	}
	_, err = other.Fingerprint(ctx)
	requireError(t, err, "PLUGIN_EXEC_NOT_AUTHORIZED")
	if err := other.Trust(ctx, false); err != nil {
		t.Fatal(err)
	}
	if after, err := other.Fingerprint(ctx); err != nil || after == before {
		t.Fatalf("changed package retained fingerprint: %q, %v", after, err)
	}
}

func TestProjectFrozenArchiveUsesLockedBytes(t *testing.T) {
	ctx := context.Background()
	archive := testZIP(t, "example", "1")
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/package.zip" {
			w.Write(archive)
			return
		}
		t.Errorf("unexpected artifact request: %s", r.URL)
		w.WriteHeader(500)
	}))
	defer server.Close()
	store := NewStore(t.TempDir())
	store.client = server.Client()
	p, _ := testProject(t, store)
	lock, err := p.Add(ctx, server.URL+"/package.zip", "")
	if err != nil {
		t.Fatal(err)
	}
	if entry := lock.Platforms[projectPlatform()][0]; entry.Source != server.URL+"/package.zip" || entry.ArchiveSHA256 != zipDigest(archive) {
		t.Fatalf("archive was not locked: %+v", entry)
	}
	before := requests.Load()
	_, err = p.Sync(ctx, true)
	requireError(t, err, "ARTIFACT_MISSING")
	if requests.Load() != before {
		t.Fatal("offline sync contacted network")
	}
	if _, err := p.Sync(ctx, false); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != before+1 {
		t.Fatal("frozen sync did not fetch exactly the locked archive")
	}
	if _, err := p.Sync(ctx, true); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != before+1 {
		t.Fatal("cached sync contacted network")
	}
}

func TestProjectGitHubLocksConcreteArtifacts(t *testing.T) {
	ctx := context.Background()
	revision := strings.Repeat("c", 40)
	manifest, _ := json.Marshal(testManifest("example", "1"))
	archive := tarEntries(t, map[string]string{"repository-commit/sub/" + ManifestFilename: string(manifest), "repository-commit/sub/rules.json": "{}"})
	resolving := true
	var downloads []string
	store := githubStore(t, func(r *http.Request) (int, []byte) {
		if resolving && r.URL.Host == "api.github.com" {
			return 200, []byte(`{"sha":"` + revision + `"}`)
		}
		if !resolving {
			downloads = append(downloads, r.URL.String())
		}
		if r.URL.String() == "https://codeload.github.com/owner/repository/tar.gz/"+revision {
			return 200, archive
		}
		t.Errorf("unexpected source lookup: %s", r.URL)
		return 500, nil
	})
	p, _ := testProject(t, store)
	lock, err := p.Add(ctx, "https://github.com/owner/repository", "")
	if err != nil {
		t.Fatal(err)
	}
	entry := lock.Platforms[projectPlatform()][0]
	if entry.Revision != revision || entry.Directory != "repository-commit/sub" {
		t.Fatalf("locked artifact = %+v", entry)
	}
	resolving = false
	if _, err := p.Sync(ctx, false); err != nil {
		t.Fatal(err)
	}
	if len(downloads) != 1 || downloads[0] != entry.Source {
		t.Fatalf("frozen downloads = %v", downloads)
	}
}

func TestProjectFrozenArchiveRejectsChangedDownload(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			ctx := context.Background()
			original, changed := testZIP(t, "example", "1"), testZIP(t, "example", "2")
			if format == "tar.gz" {
				original, changed = testTARGZ(t, "example", "1"), testTARGZ(t, "example", "2")
			}
			var serveChanged atomic.Bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if serveChanged.Load() {
					w.Write(changed)
				} else {
					w.Write(original)
				}
			}))
			defer server.Close()
			store := NewStore(t.TempDir())
			store.client = server.Client()
			project, _ := testProject(t, store)
			if _, err := project.Add(ctx, server.URL+"/plugin."+format, ""); err != nil {
				t.Fatal(err)
			}
			serveChanged.Store(true)
			_, err := project.Sync(ctx, false)
			requireError(t, err, "ARTIFACT_MISMATCH")
			serveChanged.Store(false)
			installed, err := project.Sync(ctx, false)
			if err != nil || len(installed) != 1 || installed[0].Manifest.Version != "1" {
				t.Fatalf("retry with locked bytes = %+v, %v", installed, err)
			}
		})
	}
}

func TestProjectRejectsDeclarationAndLockDrift(t *testing.T) {
	for _, edit := range []string{"declaration", "missing-declaration", "missing-lock", "schema", "platform", "count", "id", "digest", "source", "directory"} {
		t.Run(edit, func(t *testing.T) {
			ctx := context.Background()
			p, source := testProject(t, NewStore(t.TempDir()))
			lock, err := p.Add(ctx, source, "")
			if err != nil {
				t.Fatal(err)
			}
			entry := lock.Platforms[projectPlatform()][0]
			switch edit {
			case "declaration":
				writeTestJSON(t, filepath.Join(p.root, ".opencodereview", declarationFile), ProjectDeclaration{SchemaVersion: 1, Plugins: []ProjectDependency{}})
			case "missing-declaration":
				os.Remove(filepath.Join(p.root, ".opencodereview", declarationFile))
			case "missing-lock":
				os.Remove(filepath.Join(p.root, ".opencodereview", projectLockFile))
			case "schema":
				lock.SchemaVersion = 2
			case "platform":
				delete(lock.Platforms, projectPlatform())
			case "count":
				lock.Platforms[projectPlatform()] = nil
			case "id":
				entry.ID = "other"
			case "digest":
				entry.ContentSHA256 = "invalid"
			case "source":
				entry.Source = "./../outside"
			case "directory":
				entry.Directory = "../outside"
			}
			if edit != "declaration" && !strings.HasPrefix(edit, "missing-") {
				if entries := lock.Platforms[projectPlatform()]; len(entries) > 0 {
					entries[0] = entry
				}
				if err := p.writeJSON(projectLockFile, lock); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := p.Load(ctx); err == nil {
				t.Fatal("accepted invalid project configuration")
			}
			if _, err := p.Sync(ctx, false); err == nil {
				t.Fatal("sync accepted invalid project configuration")
			}
		})
	}
}

func TestProjectSourceAndEmptyDeclaration(t *testing.T) {
	p, source := testProject(t, NewStore(t.TempDir()))
	ctx := context.Background()
	if _, err := p.Add(ctx, testPackage(t, "outside", "1"), ""); err == nil {
		t.Fatal("accepted nonportable external directory")
	}
	for _, source := range []string{"../outside", "./../outside", "file:///tmp/package", "https://example.com/p?token=secret", "https://user:password@example.com/p"} {
		if err := validateProjectSource(source); err == nil {
			t.Fatalf("accepted source %q", source)
		}
	}
	if _, err := p.Lock(ctx); err == nil {
		t.Fatal("locked an absent declaration")
	}
	if _, err := p.Add(ctx, source, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Add(ctx, source, ""); err == nil {
		t.Fatal("accepted duplicate project plugin")
	}
	if err := p.writeJSON(declarationFile, ProjectDeclaration{SchemaVersion: 1, Plugins: []ProjectDependency{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Lock(ctx); err != nil {
		t.Fatal(err)
	}
	if items, present, err := p.Load(ctx); err != nil || !present || len(items) != 0 {
		t.Fatalf("empty project declaration = %v, %v, %v", items, present, err)
	}
}

func TestProjectLocalSourceThroughLinkedWorkspace(t *testing.T) {
	store := NewStore(t.TempDir())
	p, _ := testProject(t, store)
	alias := filepath.Join(t.TempDir(), "workspace")
	packageDirectoryLink(t, p.root, alias)
	linked, err := store.Project(alias)
	if err != nil {
		t.Fatal(err)
	}
	if linked.root != p.root {
		t.Fatalf("linked project root = %q, want %q", linked.root, p.root)
	}
	ctx := context.Background()
	lock, err := linked.Add(ctx, filepath.Join(alias, "plugins", "example"), "")
	if err != nil {
		t.Fatal(err)
	}
	if entry := lock.Platforms[projectPlatform()][0]; entry.Source != "./plugins/example" {
		t.Fatalf("linked source was not portable: %+v", entry)
	}
	if _, err := linked.Lock(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := linked.Sync(ctx, true); err != nil {
		t.Fatal(err)
	}
	outside := testPackage(t, "outside", "1")
	insideLink := filepath.Join(p.root, "plugins", "outside")
	packageDirectoryLink(t, outside, insideLink)
	if _, err := linked.sourcePath("./plugins/outside"); err == nil {
		t.Fatal("accepted a local source link outside the project")
	}
}

func TestProjectLockCleansEachResolvedPackage(t *testing.T) {
	scratch := t.TempDir()
	archives := map[string][]byte{
		"/first.zip":  testZIP(t, "first", "1"),
		"/second.zip": testZIP(t, "second", "1"),
	}
	requests := 0
	store := githubStore(t, func(r *http.Request) (int, []byte) {
		requests++
		directories, err := filepath.Glob(filepath.Join(scratch, "ocr-plugin-*"))
		if err != nil || len(directories) != 1 {
			t.Errorf("previous extraction retained during %s: %v, %v", r.URL.Path, directories, err)
		}
		return 200, archives[r.URL.Path]
	})
	p, _ := testProject(t, store)
	for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(variable, scratch)
	}
	declaration := ProjectDeclaration{SchemaVersion: 1, Plugins: []ProjectDependency{
		{ID: "first", Source: "https://example.com/first.zip"},
		{ID: "second", Source: "https://example.com/second.zip"},
	}}
	if err := p.writeJSON(declarationFile, declaration); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("downloads = %d, want 2", requests)
	}
	directories, err := os.ReadDir(scratch)
	if err != nil || len(directories) != 0 {
		t.Fatalf("extractions retained after lock: %v, %v", directories, err)
	}
}
