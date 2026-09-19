// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
)

type beforeConfigConfirmation struct {
	reader io.Reader
	before func()
}

func TestConfigRevisionLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	a, _ := loadOrCreateConfig(path)
	b, _ := loadOrCreateConfig(path)
	a.Language = "English"
	if err := saveConfig(path, a); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(path, b); !errors.Is(err, errConfigConflict) {
		t.Fatal("concurrent creation accepted", err)
	}
	stale, _ := LoadAppConfig(path)
	clone, _ := cloneAppConfig(stale)
	a.Model = "new-model"
	if err := saveConfig(path, a); err != nil {
		t.Fatal("repeat save failed", err)
	}
	before, _ := os.ReadFile(path)
	if err := saveConfig(path, clone); !errors.Is(err, errConfigConflict) {
		t.Fatal("clone lost revision", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("conflict modified disk")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(path, a); !errors.Is(err, errConfigConflict) {
		t.Fatal("deleted config recreated by stale writer", err)
	}
	missing, _ := loadOrCreateConfig(path)
	if err := saveConfig(path, missing); err != nil {
		t.Fatal(err)
	}
}

func TestConfigConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := saveConfig(path, &Config{}); err != nil {
		t.Fatal(err)
	}
	var drafts []*Config
	for range 8 {
		cfg, _ := loadOrCreateConfig(path)
		drafts = append(drafts, cfg)
	}
	start := make(chan struct{})
	results := make(chan error, len(drafts))
	var workers sync.WaitGroup
	for i, cfg := range drafts {
		workers.Add(1)
		go func() { defer workers.Done(); <-start; cfg.MaxTokens = i + 1; results <- saveConfig(path, cfg) }()
	}
	close(start)
	workers.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, errConfigBusy) && !errors.Is(err, errConfigConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("got %d successful writers for one revision", winners)
	}
}

func TestConfigLockAcrossProcesses(t *testing.T) {
	if os.Getenv("_OCR_CONFIG_LOCK_TEST") == "1" {
		path := os.Args[len(os.Args)-1]
		if err := saveConfig(path, &Config{}); !errors.Is(err, errConfigBusy) {
			t.Fatal("child escaped lock", err)
		}
		return
	}
	path := filepath.Join(t.TempDir(), "config.json")
	unlock, err := lockConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConfigLockAcrossProcesses$", "--", path)
	child.Env = append(os.Environ(), "_OCR_CONFIG_LOCK_TEST=1")
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("child: %v %s", err, output)
	}
}

func (r *beforeConfigConfirmation) Read(p []byte) (int, error) {
	if r.before != nil {
		f := r.before
		r.before = nil
		f()
	}
	return r.reader.Read(p)
}

func TestPermissionSavePreservesConcurrentDisable(t *testing.T) {
	setupMCPTestHome(t, &Config{MCP: &ocrmcp.MCPConfig{Version: 1}, MCPServers: map[string]MCPServerConfig{
		"docs": {Command: "never-start", Enabled: boolPointer(true), Tools: []string{"search"}, ToolPermissions: map[string]ocrmcp.Permission{"search": ocrmcp.PermissionAllow}, ToolDefinitionSHA256: map[string]string{"search": mcpTestFingerprint}},
	}})
	setMCPTestInteractive(t, true)
	cmd, _, _ := newMCPTestCommand("")
	cmd.SetIn(&beforeConfigConfirmation{reader: strings.NewReader("y\n"), before: func() {
		other, _, _ := newMCPTestCommand("")
		if err := runMCPEnable(other, "docs", false, true); err != nil {
			t.Fatal(err)
		}
	}})
	if err := runMCPPermissions(cmd, "", mcpPermissionsOptions{timeoutSeconds: 120}); err == nil {
		t.Fatal("stale draft accepted")
	}
	if *loadMCPTestConfig(t).MCPServers["docs"].Enabled {
		t.Fatal("concurrent disable was lost")
	}
}
