// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/tool"
)

func TestRunPreview(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "x.go", "package x\n", "add x")
	cc, err := loadCommonContext(dir, "", 0, 0, true)
	if err != nil {
		t.Fatalf("loadCommonContext: %v", err)
	}
	silenceStdout(t, func() {
		if err := runPreview(cc, reviewOptions{commit: "HEAD"}); err != nil {
			t.Fatalf("runPreview error: %v", err)
		}
	})
}

func TestLoadReviewResumeState(t *testing.T) {
	dir := initTestGitRepo(t)

	t.Run("empty resume returns nil", func(t *testing.T) {
		state, err := loadReviewResumeState(dir, reviewOptions{})
		if err != nil || state != nil {
			t.Errorf("got state=%v err=%v, want nil,nil", state, err)
		}
	})

	t.Run("workspace resume rejected", func(t *testing.T) {
		_, err := loadReviewResumeState(dir, reviewOptions{resume: "sess-1"})
		if err == nil {
			t.Fatal("expected error for workspace-mode resume")
		}
	})

	t.Run("missing session load fails", func(t *testing.T) {
		_, err := loadReviewResumeState(dir, reviewOptions{resume: "does-not-exist", commit: "HEAD"})
		if err == nil {
			t.Fatal("expected error loading nonexistent resume session")
		}
	})
}

func TestInitMCPClients(t *testing.T) {
	ctx := context.Background()
	reg := tool.NewRegistry()

	t.Run("nil config", func(t *testing.T) {
		if got := initMCPClients(ctx, nil, reg, "/tmp", "v"); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("empty servers", func(t *testing.T) {
		if got := initMCPClients(ctx, &Config{}, reg, "/tmp", "v"); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("remote without url skipped", func(t *testing.T) {
		cfg := &Config{MCPServers: map[string]MCPServerConfig{
			"r": {Type: "remote"},
		}}
		if got := initMCPClients(ctx, cfg, reg, "/tmp", "v"); len(got) != 0 {
			t.Errorf("got %d clients, want 0", len(got))
		}
	})

	t.Run("stdio without command skipped", func(t *testing.T) {
		cfg := &Config{MCPServers: map[string]MCPServerConfig{
			"s": {Type: "stdio"},
		}}
		if got := initMCPClients(ctx, cfg, reg, "/tmp", "v"); len(got) != 0 {
			t.Errorf("got %d clients, want 0", len(got))
		}
	})
}
