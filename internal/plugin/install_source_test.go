// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPreparedInstallHoldsInspectedBytesAndPrivateMetadata(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "store")
	store := NewStore(root)
	source := testPackage(t, "example", "1")
	prepared, err := store.PrepareInstall(ctx, source, "")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preparation changed store: %v", err)
	}
	if prepared.Manifest.ID != "example" || prepared.Source != source || len(prepared.SHA256) != 64 {
		t.Fatalf("prepared metadata = %+v", prepared)
	}
	// The source and the public preview can both change without changing the
	// package that was already prepared and confirmed.
	writeTestJSON(t, filepath.Join(source, ManifestFilename), testManifest("example", "2"))
	prepared.Manifest.ID = "modified"
	prepared.Manifest.Rules[0] = "other.json"
	prepared.Source, prepared.SHA256, prepared.Revision = "modified", "modified", "modified"
	prepared.PluginID = "modified"
	installed, err := NewStore(root).InstallPrepared(ctx, prepared, true)
	if err != nil || installed.Manifest.ID != "example" || installed.Manifest.Version != "1" || installed.Manifest.Rules[0] != "rules.json" || installed.Source != source || len(installed.SHA256) != 64 || installed.Revision != "" || installed.PluginID != "" || !installed.Enabled {
		t.Fatalf("confirmed package = %+v, %v", installed, err)
	}
	if _, err := os.Stat(prepared.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed preparation retained files: %v", err)
	}
	_, err = store.InstallPrepared(ctx, prepared, false)
	requireError(t, err, "closed")
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedInstallCloseChangeAndStoreOwnership(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	source := testPackage(t, "example", "1")
	prepared, err := store.PrepareInstall(ctx, source, "")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	_, err = NewStore(t.TempDir()).InstallPrepared(ctx, prepared, false)
	requireError(t, err, "another store")
	_, err = store.InstallPrepared(ctx, nil, false)
	requireError(t, err, "required")
	writeTestJSON(t, filepath.Join(prepared.packageDir, "rules.json"), map[string]string{"default_rule": "Modified after preview"})
	_, err = store.InstallPrepared(ctx, prepared, false)
	requireError(t, err, "changed after inspection")
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = store.InstallPrepared(ctx, prepared, false)
	requireError(t, err, "closed")
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = store.PrepareInstall(ctx, source, "INVALID")
	requireError(t, err, "invalid plugin selection")
	_, err = store.PrepareInstall(ctx, source, "other")
	requireError(t, err, "does not match selected")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.PrepareInstall(cancelled, source, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled preparation = %v", err)
	}
}

func TestPreparedInstallCloseSerializesWithCommit(t *testing.T) {
	store := NewStore(t.TempDir())
	prepared, err := store.PrepareInstall(context.Background(), testPackage(t, "example", "1"), "")
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		if err := prepared.Close(); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer group.Done()
		if _, err := store.InstallPrepared(context.Background(), prepared, false); err != nil && !strings.Contains(err.Error(), "closed") {
			t.Error(err)
		}
	}()
	group.Wait()
	if _, err := os.Stat(prepared.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("racing cleanup retained preparation: %v", err)
	}
}

func TestSourceCannotContainTemporaryStagingDirectory(t *testing.T) {
	source := testPackage(t, "example", "1")
	store := NewStore(t.TempDir())
	stage := filepath.Join(source, "stage")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	requireError(t, store.prepare(context.Background(), source, "", stage), "cannot contain the staging directory")
	os.Remove(stage)
	t.Setenv("TMP", source)
	t.Setenv("TEMP", source)
	t.Setenv("TMPDIR", source)
	_, err := store.PrepareInstall(context.Background(), source, "")
	requireError(t, err, "cannot contain the staging directory")
	entries, err := os.ReadDir(source)
	if err != nil || len(entries) != 2 {
		t.Fatalf("rejected staging retained files: %v, %v", entries, err)
	}
}

func TestDirectHTTPSZIPPreviewCommitAndUpdate(t *testing.T) {
	ctx := context.Background()
	var version atomic.Int32
	version.Store(1)
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Write(testZIP(t, "example", fmt.Sprint(version.Load())))
	}))
	defer server.Close()
	store := NewStore(t.TempDir())
	store.client = server.Client()
	prepared, err := store.PrepareInstall(ctx, server.URL+"/download", "")
	if err != nil || prepared.PluginID != "example" || prepared.SHA256 == "" {
		t.Fatalf("direct preview = %v, %v", prepared, err)
	}
	defer prepared.Close()
	version.Store(2)
	installed, err := store.InstallPrepared(ctx, prepared, false)
	if err != nil || installed.Manifest.Version != "1" || requests.Load() != 1 {
		t.Fatalf("ZIP was fetched again after preview: %+v, %v, %d requests", installed, err, requests.Load())
	}
	updated, err := store.Update(ctx, "example")
	if err != nil || updated.Manifest.Version != "2" || updated.Source != installed.Source || updated.SHA256 == installed.SHA256 || updated.Enabled || updated.PluginID != "example" {
		t.Fatalf("direct update = %+v, %v", updated, err)
	}
}

func TestHTTPSRejectsIndexesAndUnknownFormats(t *testing.T) {
	for _, body := range []string{`{"schema_version":1,"name":"Market","plugins":[{"id":"example","source":"https://example.com/plugin.zip"}]}`, `{"broken":`, "<html>download page</html>"} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer server.Close()
			store := NewStore(t.TempDir())
			store.client = server.Client()
			_, err := store.PrepareInstall(context.Background(), server.URL+"/download", "")
			requireError(t, err, "JSON indexes are not supported")
			items, err := store.List()
			if err != nil || len(items) != 0 {
				t.Fatalf("rejected source changed registry: %v, %v", items, err)
			}
		})
	}
}

func TestDirectHTTPSZIPDiscoveryAndInvalidPackages(t *testing.T) {
	ctx := context.Background()
	first, _ := json.Marshal(testManifest("example", "1"))
	second, _ := json.Marshal(testManifest("another", "1"))
	archives := map[string][]byte{
		"/multi":       zipEntries(t, map[string]string{"wrapped/a/" + ManifestFilename: string(first), "wrapped/a/rules.json": "{}", "wrapped/b/" + ManifestFilename: string(second), "wrapped/b/rules.json": "{}"}),
		"/duplicate":   zipEntries(t, map[string]string{"a/" + ManifestFilename: string(first), "a/rules.json": "{}", "b/" + ManifestFilename: string(first), "b/rules.json": "{}"}),
		"/root":        testZIP(t, "example", "1"),
		"/bad-root":    zipEntries(t, map[string]string{ManifestFilename: "{"}),
		"/bad-nested":  zipEntries(t, map[string]string{"wrapped/" + ManifestFilename: "{"}),
		"/no-manifest": zipEntries(t, map[string]string{"README.md": "No plugin"}),
		"/bad-zip":     []byte("PKbroken"),
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if archive, exists := archives[request.URL.Path]; exists {
			writer.Write(archive)
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	store := NewStore(t.TempDir())
	store.client = server.Client()
	_, err := store.PrepareInstall(ctx, server.URL+"/multi", "")
	var selection *SelectionRequiredError
	if !errors.As(err, &selection) || len(selection.Candidates) != 2 {
		t.Fatalf("direct ZIP selection = %v", err)
	}
	prepared, err := store.PrepareInstall(ctx, server.URL+"/multi", "example")
	if err != nil || prepared.Manifest.ID != "example" || prepared.PluginID != "example" {
		t.Fatalf("selected direct ZIP = %v, %v", prepared, err)
	}
	prepared.Close()
	for _, test := range []struct{ path, selection, message string }{
		{"/duplicate", "", "duplicate plugin id"},
		{"/root", "another", "does not match selected"},
		{"/bad-root", "", "parse plugin manifest"},
		{"/bad-nested", "", "parse plugin manifest"},
		{"/no-manifest", "", "no plugins"},
		{"/bad-zip", "", "read plugin ZIP"},
		{"/missing", "", "HTTP 404"},
	} {
		_, err := store.PrepareInstall(ctx, server.URL+test.path, test.selection)
		requireError(t, err, test.message)
	}
	_, err = store.PrepareInstall(ctx, "https://github.com/owner", "")
	requireError(t, err, "owner and repository")
}

func TestHTTPSZIPPreservesNestedManifestData(t *testing.T) {
	manifest, err := json.Marshal(testManifest("example", "1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ name, content string }{
		{"malformed", "{"},
		{"duplicate", string(manifest)},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			archives := make(map[string][]byte)
			for _, prefix := range []string{"", "wrapped/"} {
				archives["/"+prefix+"plugin.zip"] = zipEntries(t, map[string]string{
					prefix + ManifestFilename:               string(manifest),
					prefix + "rules.json":                   "{}",
					prefix + "fixtures/" + ManifestFilename: fixture.content,
					prefix + "fixtures/rules.json":          "{}",
				})
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if archive, ok := archives[r.URL.Path]; ok {
					w.Write(archive)
				} else {
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			for _, path := range []string{"/plugin.zip", "/wrapped/plugin.zip"} {
				t.Run(path, func(t *testing.T) {
					store := NewStore(t.TempDir())
					store.client = server.Client()
					installed, err := store.Install(context.Background(), server.URL+path, false)
					if err != nil {
						t.Fatal(err)
					}
					data, err := os.ReadFile(filepath.Join(installed.Dir, "fixtures", ManifestFilename))
					if err != nil || string(data) != fixture.content {
						t.Fatalf("nested manifest data = %q, %v", data, err)
					}
				})
			}
		})
	}
}

type pluginRoundTripper func(*http.Request) (*http.Response, error)

func (f pluginRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func githubStore(t *testing.T, handler func(*http.Request) (int, []byte)) *Store {
	t.Helper()
	store := NewStore(t.TempDir())
	store.client = &http.Client{Transport: pluginRoundTripper(func(request *http.Request) (*http.Response, error) {
		status, data := handler(request)
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(data))), ContentLength: int64(len(data)), Header: make(http.Header), Request: request}, nil
	})}
	return store
}

func TestGitHubNestedPackageAndTreeSource(t *testing.T) {
	ctx := context.Background()
	manifest, _ := json.Marshal(testManifest("example", "1"))
	archive := tarEntries(t, map[string]string{"repository-commit/README.md": "Read me", "repository-commit/plugin/" + ManifestFilename: string(manifest), "repository-commit/plugin/rules.json": "{}"})
	revision := strings.Repeat("b", 40)
	for _, source := range []string{"https://github.com/owner/repository", "https://github.com/owner/repository.git/", "https://github.com/owner/repository/tree/v1/plugin"} {
		t.Run(source, func(t *testing.T) {
			store := githubStore(t, func(request *http.Request) (int, []byte) {
				if request.URL.Host == "api.github.com" {
					ref := "HEAD"
					if strings.Contains(source, "/tree/") {
						ref = "v1"
					}
					if request.URL.Path != "/repos/owner/repository/commits/"+ref {
						t.Errorf("revision request = %s", request.URL)
					}
					return http.StatusOK, []byte(`{"sha":"` + revision + `","extra":"allowed"}`)
				}
				if request.URL.String() != "https://codeload.github.com/owner/repository/tar.gz/"+revision {
					t.Errorf("archive request = %s", request.URL)
				}
				return http.StatusOK, archive
			})
			installed, err := store.Install(ctx, source, false)
			if err != nil || installed.Manifest.ID != "example" || installed.Source != source || installed.SHA256 != zipDigest(archive) || installed.Revision != revision || installed.PluginID != "example" {
				t.Fatalf("GitHub package install = %+v, %v", installed, err)
			}
			if _, err := os.Stat(filepath.Join(installed.Dir, "README.md")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("repository root was copied instead of plugin: %v", err)
			}
		})
	}
}

func TestGitHubDoesNotFollowPublishedCatalog(t *testing.T) {
	ctx := context.Background()
	revision := strings.Repeat("a", 40)
	manifest, _ := json.Marshal(testManifest("example", "source-version"))
	for _, hasPackage := range []bool{true, false} {
		t.Run(fmt.Sprint(hasPackage), func(t *testing.T) {
			files := map[string]string{"repository/marketplace.json": `{"schema_version":1,"name":"Published","plugins":[{"id":"example","source":"https://example.com/release.zip"}]}`}
			if hasPackage {
				files["repository/plugin/"+ManifestFilename] = string(manifest)
				files["repository/plugin/rules.json"] = "{}"
			}
			archive := tarEntries(t, files)
			store := githubStore(t, func(r *http.Request) (int, []byte) {
				if r.URL.Host == "api.github.com" {
					return 200, []byte(`{"sha":"` + revision + `"}`)
				}
				if r.URL.String() == "https://codeload.github.com/owner/repository/tar.gz/"+revision {
					return 200, archive
				}
				t.Errorf("followed removed index: %s", r.URL)
				return 500, nil
			})
			installed, err := store.Install(ctx, "https://github.com/owner/repository", false)
			if !hasPackage {
				requireError(t, err, "no plugins")
				return
			}
			if err != nil || installed.Manifest.Version != "source-version" || installed.Revision != revision {
				t.Fatalf("repository package = %+v, %v", installed, err)
			}
		})
	}
}

func TestGitHubMultiPackageSelectionAndSourceFailures(t *testing.T) {
	ctx := context.Background()
	first, _ := json.Marshal(testManifest("example", "1"))
	second, _ := json.Marshal(testManifest("another", "1"))
	archive := tarEntries(t, map[string]string{"repository-root/a/" + ManifestFilename: string(first), "repository-root/a/rules.json": "{}", "repository-root/b/" + ManifestFilename: string(second), "repository-root/b/rules.json": "{}"})
	store := githubStore(t, func(request *http.Request) (int, []byte) {
		if request.URL.Host == "api.github.com" {
			return http.StatusOK, []byte(`{"sha":"` + strings.Repeat("a", 40) + `"}`)
		}
		return http.StatusOK, archive
	})
	source := "https://github.com/owner/repository"
	_, err := store.PrepareInstall(ctx, source, "")
	var selection *SelectionRequiredError
	if !errors.As(err, &selection) || len(selection.Candidates) != 2 || selection.Candidates[0].ID != "another" {
		t.Fatalf("GitHub package selection = %v", err)
	}
	prepared, err := store.PrepareInstall(ctx, source, "example")
	if err != nil || prepared.Manifest.ID != "example" {
		t.Fatalf("selected GitHub package = %v, %v", prepared, err)
	}
	prepared.Close()
	_, err = store.PrepareInstall(ctx, source+"/tree/main/absent", "")
	requireError(t, err, "does not exist")
	for _, test := range []struct {
		status        int
		data, message string
	}{{http.StatusNotFound, `{}`, "HTTP 404"}, {http.StatusOK, `{`, "parse GitHub revision"}, {http.StatusOK, `{"sha":"short"}`, "invalid commit"}} {
		bad := githubStore(t, func(request *http.Request) (int, []byte) { return test.status, []byte(test.data) })
		_, err := bad.PrepareInstall(ctx, source, "")
		requireError(t, err, test.message)
	}
}

func TestGitHubSourceParsing(t *testing.T) {
	for _, test := range []struct {
		source            string
		repository, fails bool
	}{
		{"https://github.com/owner/repository", true, false},
		{"https://github.com/owner/repository/tree/feature%2Ftopic/plugin", true, false},
		{"https://github.com/owner/repository/releases/download/v1/plugin.zip", false, false},
		{"https://github.com/owner/repository/archive/refs/heads/main.zip", false, false},
		{"https://example.com/owner/repository", false, false},
		{"https://github.com/owner", true, true},
		{"https://github.com/owner/../", true, true},
		{"https://github.com/owner/repository/tree", true, true},
		{"https://github.com/owner/repository/blob/main/ocr-plugin.json", true, true},
		{"https://github.com/owner/repository/tree/main/../escape", true, true},
	} {
		_, repository, err := parseGitHubSource(test.source)
		if repository != test.repository || (err != nil) != test.fails {
			t.Errorf("parseGitHubSource(%q) = %v, %v", test.source, repository, err)
		}
	}
}

func TestGitHubRevisionFailureDoesNotDownloadOrUpdate(t *testing.T) {
	ctx := context.Background()
	revision := strings.Repeat("a", 40)
	manifest, _ := json.Marshal(testManifest("example", "1"))
	archive := tarEntries(t, map[string]string{"repo/" + ManifestFilename: string(manifest), "repo/rules.json": "{}"})
	for _, source := range []string{"https://github.com/owner/repository", "https://github.com/owner/repository/tree/main"} {
		failed := false
		downloads := 0
		store := githubStore(t, func(request *http.Request) (int, []byte) {
			if request.URL.Host == "api.github.com" {
				if failed {
					return http.StatusForbidden, []byte("rate limited")
				}
				return http.StatusOK, []byte(`{"sha":"` + revision + `"}`)
			}
			downloads++
			return http.StatusOK, archive
		})
		installed, err := store.Install(ctx, source, true)
		if err != nil {
			t.Fatal(err)
		}
		failed = true
		statePath := filepath.Join(store.root, "state.json")
		before, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.Update(ctx, "example")
		requireError(t, err, "resolve GitHub revision")
		after, err := os.ReadFile(statePath)
		if err != nil || string(before) != string(after) || downloads != 1 {
			t.Fatalf("failed resolution changed installation: %v, downloads=%d", err, downloads)
		}
		items, err := store.List()
		if err != nil || len(items) != 1 || items[0].Dir != installed.Dir {
			t.Fatalf("old installation = %v, %v", items, err)
		}
	}
}

func TestGitHubFullCommitDoesNotRequireAPI(t *testing.T) {
	revision := strings.Repeat("B", 40)
	manifest, _ := json.Marshal(testManifest("example", "1"))
	archive := tarEntries(t, map[string]string{"repo/" + ManifestFilename: string(manifest), "repo/rules.json": "{}"})
	store := githubStore(t, func(request *http.Request) (int, []byte) {
		if request.URL.String() != "https://codeload.github.com/owner/repository/tar.gz/"+strings.ToLower(revision) {
			t.Fatalf("unexpected request for pinned source: %s", request.URL)
		}
		return http.StatusOK, archive
	})
	item, err := store.Install(context.Background(), "https://github.com/owner/repository/tree/"+revision, false)
	if err != nil || item.Revision != strings.ToLower(revision) {
		t.Fatalf("pinned installation = %+v, %v", item, err)
	}
}

func TestExplicitPackageDigestsAndLegacyState(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	source := testPackage(t, "example", "1")
	local, err := store.Install(ctx, source, false)
	if err != nil || local.ArchiveSHA256 != "" || local.ContentSHA256 == "" || local.ContentDigestFormat != "ocr-package-tree-v1" || local.SHA256 != local.ContentSHA256 {
		t.Fatalf("local digests = %+v, %v", local, err)
	}
	digest, err := packageDigest(ctx, local.Dir)
	if err != nil || digest != local.ContentSHA256 {
		t.Fatalf("installed content digest = %s, %v", digest, err)
	}
	statePath := filepath.Join(store.root, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	record := legacy["installed"].(map[string]any)["example"].(map[string]any)
	delete(record, "archive_sha256")
	delete(record, "content_sha256")
	delete(record, "content_digest_format")
	writeTestJSON(t, statePath, legacy)
	list, err := store.List()
	if err != nil || len(list) != 1 || list[0].SHA256 != local.SHA256 || list[0].ContentSHA256 != "" {
		t.Fatalf("legacy record = %+v, %v", list, err)
	}
	updated, err := store.Update(ctx, "example")
	if err != nil || updated.ContentSHA256 != digest {
		t.Fatalf("updated legacy record = %+v, %v", updated, err)
	}
	if err := store.Remove("example"); err != nil {
		t.Fatal(err)
	}
	archive := testZIP(t, "example", "1")
	archivePath := filepath.Join(t.TempDir(), "plugin.zip")
	if err := os.WriteFile(archivePath, archive, 0600); err != nil {
		t.Fatal(err)
	}
	item, err := store.Install(ctx, archivePath, false)
	if err != nil || item.ArchiveSHA256 != archiveDigest(archive) || item.SHA256 != item.ArchiveSHA256 || item.ContentSHA256 == "" {
		t.Fatalf("archive digests = %+v, %v", item, err)
	}
	digest, err = packageDigest(ctx, item.Dir)
	if err != nil || digest != item.ContentSHA256 {
		t.Fatalf("archive content digest = %s, %v", digest, err)
	}
}
