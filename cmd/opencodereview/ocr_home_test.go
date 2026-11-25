// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/viewer"
)

func TestOCRHomeAuxiliaryStorage(t *testing.T) {
	storage := t.TempDir()
	t.Setenv("OCR_HOME", storage)
	repo := t.TempDir()

	configPath := filepath.Join(storage, "config.json")
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

	sh := session.New(repo, "main", "test-model", session.SessionOptions{ReviewMode: session.ReviewModeWorkspace})
	sh.RecordReviewItemDone("file.go", "", "", "fingerprint", nil)
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
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
	rawPath := filepath.Join(storage, "raw", locations[0].EncodedDir, sh.SessionID+".jsonl")
	if data, err := os.ReadFile(rawPath); err != nil || len(data) == 0 {
		t.Fatalf("raw capture is missing: %v", err)
	}
	if err := session.DeleteSessionAt(locations[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(locations[0].Path); !os.IsNotExist(err) {
		t.Fatalf("session was not deleted: %v", err)
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

func TestOCRHomeCrossProcess(t *testing.T) {
	if os.Getenv("OCR_TEST_SHARED_STORAGE") == "1" {
		rootCmd.SetArgs(os.Args[3:])
		os.Exit(run())
	}

	home := t.TempDir()
	storage := filepath.Join(t.TempDir(), "ocr data")
	repo := retryTestRepo(t)
	srv := httptest.NewServer(newFakeLLM())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := func(cwd string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestOCRHomeCrossProcess$", "--"}, args...)...)
		cmd.Dir = cwd
		for _, entry := range os.Environ() {
			key, _, _ := strings.Cut(entry, "=")
			if !strings.HasPrefix(strings.ToUpper(key), "OCR_") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env,
			testHomeIsolatedEnv+"=1", "OCR_TEST_SHARED_STORAGE=1",
			"HOME="+home, "USERPROFILE="+home, "OCR_HOME="+storage,
			"OCR_ENABLE_TELEMETRY=0", "OCR_NO_UPDATE=1",
		)
		return cmd
	}
	for _, pair := range [][2]string{
		{"llm.url", srv.URL + "/v1/messages"},
		{"llm.auth_token", "test-token"},
		{"llm.model", "claude-test"},
		{"llm.protocol", "anthropic"},
	} {
		if out, err := command(home, "config", "set", pair[0], pair[1]).CombinedOutput(); err != nil {
			t.Fatalf("config set %s: %v\n%s", pair[0], err, out)
		}
	}
	review := command(repo, "review", "--format", "json", "--audience", "agent", "--concurrency", "1")
	var stderr bytes.Buffer
	review.Stderr = &stderr
	out, err := review.Output()
	if err != nil {
		t.Fatalf("review: %v\n%s\n%s", err, stderr.String(), out)
	}
	var result jsonOutput
	if err := json.Unmarshal(out, &result); err != nil || result.SessionID == "" {
		t.Fatalf("decode review session: %v\n%s", err, out)
	}
	repos, err := viewer.DiscoverRepos(filepath.Join(storage, "sessions"))
	if err != nil || len(repos) != 1 {
		t.Fatalf("saved repositories: %+v, %v", repos, err)
	}

	child := command(home, "viewer", "--addr", "127.0.0.1:0", "--open=never")
	stderr.Reset()
	child.Stderr = &stderr
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = child.Wait()
		if t.Failed() {
			t.Logf("viewer stderr: %s", stderr.String())
		}
	}()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || !strings.HasPrefix(scanner.Text(), "Viewer ready: ") {
		t.Fatalf("viewer did not report its URL: %q (%v)", scanner.Text(), scanner.Err())
	}
	path := "/r/" + url.PathEscape(repos[0].EncodedPath) + "/" + url.PathEscape(result.SessionID)
	base := strings.TrimPrefix(scanner.Text(), "Viewer ready: ")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK || !bytes.Contains(data, []byte(result.SessionID)) || !bytes.Contains(data, []byte("claude-test")) {
		t.Fatalf("viewer did not load session %q (HTTP %d, %v)", result.SessionID, resp.StatusCode, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".opencodereview")); !os.IsNotExist(err) {
		t.Fatalf("default storage was touched: %v", err)
	}
}
