// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/plugin"
)

func pluginURLArchive(t *testing.T, id, version string) []byte {
	t.Helper()
	manifest := plugin.Manifest{
		SchemaVersion: 1, ID: id, Name: "URL test plugin", Version: version,
		Rules: []string{"rules.json"},
		MCPServers: map[string]plugin.MCPServer{"notes": {
			Command: "must-not-execute-plugin", Args: []string{"private-argument"}, Env: []string{"TOKEN=private-token"},
		}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, content := range map[string][]byte{
		"ocr-plugin.json": data,
		"rules.json":      []byte(`{"rules":[{"path":"**/*.go","rule":"Check errors."}]}`),
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func usePluginURLServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	previous := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = previous })
	return server
}

func TestPluginURLCommandPreviewInstallAndUpdate(t *testing.T) {
	freshOCRHome(t)
	t.Setenv("PATH", "")
	var mu sync.Mutex
	archive := pluginURLArchive(t, "url-review", "1.0")
	server := usePluginURLServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write(archive)
	}))
	source := server.URL + "/plugin.zip?token=private-query"
	output := pluginCommandOK(t, "install", source, "--dry-run", "--enable", "--json")
	var preview pluginInstallPreview
	if err := json.Unmarshal([]byte(output), &preview); err != nil || len(preview.Plugins) != 1 || preview.Plugins[0].ID != "url-review" || len(preview.Plugins[0].SHA256) != 64 {
		t.Fatalf("preview = %s, error = %v", output, err)
	}
	if strings.Contains(output, "private-") {
		t.Fatalf("preview exposed credentials: %s", output)
	}
	if output := pluginCommandOK(t, "list", "--json"); strings.TrimSpace(output) != "[]" {
		t.Fatalf("preview installed a plugin: %s", output)
	}
	if output := pluginCommandOK(t, "install", source, "--dry-run"); !strings.Contains(output, "No plugin was installed") || !strings.Contains(output, "url-review") {
		t.Fatalf("missing preview guidance: %s", output)
	}
	var installed pluginInfo
	output = pluginCommandOK(t, "install", source, "--json")
	if err := json.Unmarshal([]byte(output), &installed); err != nil || installed.Enabled || installed.SHA256 != preview.Plugins[0].SHA256 || installed.Source != server.URL+"/plugin.zip" {
		t.Fatalf("install = %s, error = %v", output, err)
	}
	pluginCommandOK(t, "enable", "url-review")
	updated := pluginURLArchive(t, "url-review", "2.0")
	mu.Lock()
	archive = updated
	mu.Unlock()
	output = pluginCommandOK(t, "update", "url-review", "--json")
	if err := json.Unmarshal([]byte(output), &installed); err != nil || !installed.Enabled || installed.Version != "2.0" || installed.SHA256 == preview.Plugins[0].SHA256 {
		t.Fatalf("update = %s, error = %v", output, err)
	}
}

func TestPluginArchiveURLCommandSelectsPackage(t *testing.T) {
	freshOCRHome(t)
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, id := range []string{"first", "second"} {
		data := pluginURLArchive(t, id, "1")
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range reader.File {
			in, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			out, err := writer.Create(id + "/" + file.Name)
			if err != nil {
				in.Close()
				t.Fatal(err)
			}
			_, err = io.Copy(out, in)
			in.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	server := usePluginURLServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(archive.Bytes()) }))
	source := server.URL + "/packages.zip"
	var preview pluginInstallPreview
	output := pluginCommandOK(t, "install", source, "--dry-run", "--json")
	if err := json.Unmarshal([]byte(output), &preview); err != nil || len(preview.Plugins) != 2 {
		t.Fatalf("archive selection = %s, %v", output, err)
	}
	if _, err := runPluginCommand(context.Background(), []string{"install", source}); err == nil || !strings.Contains(err.Error(), "--plugin") {
		t.Fatalf("ambiguous install = %v", err)
	}
	if output := pluginCommandOK(t, "list", "--json"); strings.TrimSpace(output) != "[]" {
		t.Fatalf("preview installed packages: %s", output)
	}
	pluginCommandOK(t, "install", source, "--plugin", "second", "--enable")
	var installed []pluginInfo
	output = pluginCommandOK(t, "list", "--json")
	if err := json.Unmarshal([]byte(output), &installed); err != nil || len(installed) != 1 || installed[0].ID != "second" || !installed[0].Enabled {
		t.Fatalf("selected archive install = %s, %v", output, err)
	}
}

func TestPluginCLIRejectsRemovedMarketplaceInputs(t *testing.T) {
	freshOCRHome(t)
	if help := pluginCommandOK(t, "--help"); strings.Contains(strings.ToLower(help), "market") {
		t.Fatalf("help still advertises marketplace: %s", help)
	}
	server := usePluginURLServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"schema_version":1,"name":"Old market","plugins":[]}`))
	}))
	for _, args := range [][]string{{"market", "list"}, {"install", "example@team"}, {"install", server.URL + "/index.json"}} {
		if _, err := runPluginCommand(context.Background(), args); err == nil {
			t.Errorf("removed input accepted: %v", args)
		}
	}
	if output := pluginCommandOK(t, "list", "--json"); strings.TrimSpace(output) != "[]" {
		t.Fatalf("rejected input installed packages: %s", output)
	}
}
