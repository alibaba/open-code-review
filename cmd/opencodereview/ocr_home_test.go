// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/viewer"
)

func TestOCRHomeSharedStorage(t *testing.T) {
	home := t.TempDir()
	storage := filepath.Join(t.TempDir(), "ocr data")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("OCR_HOME", storage)
	repo := t.TempDir()

	for _, entry := range [][2]string{
		{"llm.url", "https://example.com/v1/chat/completions"},
		{"llm.auth_token", "test-token"},
		{"llm.model", "test-model"},
	} {
		if err := runConfigSet(entry[0], entry[1]); err != nil {
			t.Fatal(err)
		}
	}
	configPath, err := defaultConfigPath()
	if err != nil || configPath != filepath.Join(storage, "config.json") {
		t.Fatalf("config path = %q, %v", configPath, err)
	}
	ep, err := llm.ResolveEndpoint(configPath)
	if err != nil || ep.Model != "test-model" || ep.Token != "test-token" {
		t.Fatalf("endpoint = %+v, %v", ep, err)
	}
	if got := telemetry.HomeConfigPath(); got != configPath {
		t.Fatalf("telemetry config path = %q, want %q", got, configPath)
	}
	auth, err := chatgptauth.DefaultStore()
	if err != nil || auth.Dir != filepath.Join(storage, "auth", "chatgpt") {
		t.Fatalf("auth store = %+v, %v", auth, err)
	}

	if err := os.WriteFile(filepath.Join(storage, "rule.json"), []byte(`{"rules":[{"path":"**/*.go","rule":"custom-global-rule"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	resolver, _, err := rules.NewResolver(repo, "", rules.ResolverOptions{})
	if err != nil || resolver.Resolve("file.go") != "custom-global-rule" {
		t.Fatalf("global rule was not loaded: %v", err)
	}
	projectRules := filepath.Join(repo, ".opencodereview")
	if err := os.MkdirAll(projectRules, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRules, "rule.json"), []byte(`{"rules":[{"path":"**/*.go","rule":"custom-project-rule"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	resolver, _, err = rules.NewResolver(repo, "", rules.ResolverOptions{})
	if err != nil || resolver.Resolve("file.go") != "custom-project-rule" {
		t.Fatalf("project rule did not take precedence: %v", err)
	}

	sh := session.New(repo, "main", ep.Model, session.SessionOptions{ReviewMode: session.ReviewModeWorkspace})
	sh.RecordReviewItemDone("file.go", "", "", "fingerprint", nil)
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	root, err := viewer.SessionsRoot()
	if err != nil || root != filepath.Join(storage, "sessions") {
		t.Fatalf("viewer root = %q, %v", root, err)
	}
	repos, err := viewer.DiscoverRepos(root)
	if err != nil || len(repos) != 1 {
		t.Fatalf("viewer repositories = %+v, %v", repos, err)
	}
	if _, err := viewer.LoadSession(root, repos[0].EncodedPath, sh.SessionID); err != nil {
		t.Fatalf("viewer could not load the review session: %v", err)
	}
	state, err := session.LoadResumeState(repo, sh.SessionID)
	if err != nil || len(state.Items) != 1 {
		t.Fatalf("resume state = %+v, %v", state, err)
	}
	summaries, err := session.ListSessions(repo)
	if err != nil || len(summaries) != 1 || summaries[0].SessionID != sh.SessionID {
		t.Fatalf("session list = %+v, %v", summaries, err)
	}
	locations, err := session.FindSessionsByID(sh.SessionID)
	if err != nil || len(locations) != 1 {
		t.Fatalf("session locations = %+v, %v", locations, err)
	}
	all, err := session.ListAllSessionIDs()
	if err != nil || len(all) != 1 || all[0].SessionID != sh.SessionID {
		t.Fatalf("all sessions = %+v, %v", all, err)
	}
	raw, err := session.NewRawFileWriter(repo, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	raw.Write(llm.RawRecord{RequestID: "request-1"})
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	rawPath := filepath.Join(storage, "raw", repos[0].EncodedPath, sh.SessionID+".jsonl")
	if data, err := os.ReadFile(rawPath); err != nil || len(data) == 0 {
		t.Fatalf("raw capture is missing: %v", err)
	}
	if err := session.DeleteSessionAt(locations[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(locations[0].Path); !os.IsNotExist(err) {
		t.Fatalf("session was not deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".opencodereview")); !os.IsNotExist(err) {
		t.Fatalf("default storage was touched: %v", err)
	}
}

func TestOCRHomeUnavailableDoesNotFallBack(t *testing.T) {
	home := t.TempDir()
	storage := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(storage, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("OCR_HOME", storage)
	if err := runConfigSet("llm.model", "test-model"); err == nil {
		t.Fatal("config write succeeded with an unavailable OCR_HOME")
	}
	sh := session.New(t.TempDir(), "main", "test-model", session.SessionOptions{})
	if err := sh.Finalize(); err == nil {
		t.Fatal("session write succeeded with an unavailable OCR_HOME")
	}
	if _, err := session.NewRawFileWriter(t.TempDir(), "test-session"); err == nil {
		t.Fatal("raw capture succeeded with an unavailable OCR_HOME")
	}
	if _, err := os.Stat(filepath.Join(home, ".opencodereview")); !os.IsNotExist(err) {
		t.Fatalf("fell back to default storage: %v", err)
	}
}
