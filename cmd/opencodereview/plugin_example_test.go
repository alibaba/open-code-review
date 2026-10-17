// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/plugin"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestSinglePassExampleRecoversFromInvalidRequests(t *testing.T) {
	python, err := exec.LookPath("python")
	if err != nil {
		python, err = exec.LookPath("python3")
	}
	if err != nil {
		t.Skip("Python 3 is required to exercise the bundled review example")
	}
	requests := []struct {
		input string
		kind  string
	}{
		{`{`, "error"},
		{`[]`, "error"},
		{`"text"`, "error"},
		{`null`, "error"},
		{`{"version":1,"stage":"review"}`, "error"},
		{`{"version":1,"stage":"review","method":"begin"}`, "error"},
		{`{"version":1,"stage":"review","method":"begin","severities":["medium"],"categories":null}`, "error"},
		{`{"version":1,"stage":"review","method":"submit","arguments":null}`, "rejected"},
		{`{"version":1,"stage":"review","method":"submit","arguments":[]}`, "rejected"},
		{`{"version":1,"stage":"review","method":"begin","severities":["medium"],"categories":["bug"]}`, "task"},
		{`{"version":1,"stage":"review","method":"submit","arguments":{"complete":true,"summary":"No findings.","comments":[]}}`, "complete"},
	}
	var input bytes.Buffer
	for _, request := range requests {
		input.WriteString(request.input + "\n")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, filepath.Join("..", "..", "examples", "ocr-plugins", "single-pass", "review.py"))
	cmd.Stdin = &input
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("example exited before completing requests: %v: %s", err, stderr.String())
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	for i, request := range requests {
		var response plugin.ReviewResponse
		if err := decoder.Decode(&response); err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
		if response.Version != 1 {
			t.Fatalf("request %d: wrong protocol version: %+v", i+1, response)
		}
		switch request.kind {
		case "error":
			if response.Error == "" {
				t.Fatalf("request %d: missing protocol error", i+1)
			}
		case "rejected":
			if response.Accepted == nil || *response.Accepted || response.Feedback == "" {
				t.Fatalf("request %d: missing rejection: %+v", i+1, response)
			}
		case "task":
			if response.Error != "" || response.Task == nil || response.Task.Instructions == "" {
				t.Fatalf("valid begin after invalid requests failed: %+v", response)
			}
		case "complete":
			if response.Error != "" || response.Accepted == nil || !*response.Accepted || response.Complete == nil || !*response.Complete {
				t.Fatalf("valid submit after invalid requests failed: %+v", response)
			}
		}
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("unexpected extra output: %v", err)
	}
}

func TestNativePluginExamplePackagesThroughRuntime(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node.js is required to exercise the bundled MCP example")
	}
	freshOCRHome(t)
	packageDir, err := filepath.Abs(filepath.Join("..", "..", "examples", "ocr-plugins"))
	if err != nil {
		t.Fatal(err)
	}
	pluginCommandOK(t, "install", filepath.Join(packageDir, "review-checklist"), "--enable")
	pluginCommandOK(t, "install", filepath.Join(packageDir, "project-notes"))
	repo := t.TempDir()
	resolver, _, err := rules.NewResolver(repo, "", rules.ResolverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := resolver.Resolve("main.go"); !strings.Contains(got, "resource ownership mistakes") || strings.Contains(got, "project_review_notes") {
		t.Fatalf("enabled/disabled example rules not respected: %s", got)
	}
	pluginCommandOK(t, "enable", "project-notes")
	resolver, _, err = rules.NewResolver(repo, "", rules.ResolverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	clients, err := initRuntimeMCPClients(ctx, nil, resolver, reg, repo, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeReviewMCPClients(clients) })
	if len(clients) != 1 {
		t.Fatalf("example plugin did not launch exactly one tool server: %d", len(clients))
	}
	provider, ok := reg.Get("project_review_notes")
	if !ok {
		t.Fatal("example MCP tool was not discovered")
	}
	if out, err := provider.Execute(ctx, map[string]any{}); err != nil || !strings.Contains(out, "No REVIEW_NOTES.md") {
		t.Fatalf("missing notes: %q, %v", out, err)
	}
	path := filepath.Join(repo, "REVIEW_NOTES.md")
	if err := os.WriteFile(path, []byte("Check that retry failures preserve the original cause.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := provider.Execute(ctx, map[string]any{}); err != nil || out != "Check that retry failures preserve the original cause.\n" {
		t.Fatalf("plugin did not read reviewed repository: %q, %v", out, err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 65537)), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := provider.Execute(ctx, map[string]any{}); err != nil || !strings.Contains(out, "exceeds 64 KiB") {
		t.Fatalf("example lost bounded file read: %q, %v", out, err)
	}
	pluginCommandOK(t, "disable", "project-notes")
	nextResolver, _, err := rules.NewResolver(repo, "", rules.ResolverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	nextClients, err := initRuntimeMCPClients(ctx, nil, nextResolver, tool.NewRegistry(), repo, "test")
	defer closeReviewMCPClients(nextClients)
	if err != nil || len(nextClients) != 0 || strings.Contains(nextResolver.Resolve("main.go"), "project_review_notes") {
		t.Fatalf("disabled example still contributes: clients=%d, error=%v", len(nextClients), err)
	}
}
