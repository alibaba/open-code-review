// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestExecuteScanResumeChangedRules(t *testing.T) {
	freshOCRHome(t)
	repo := t.TempDir()
	for name, marker := range map[string]string{"a.go": "PROBE_FILE_A", "b.go": "PROBE_FILE_B"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("package probe\n// "+marker+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rulePath := filepath.Join(t.TempDir(), "review-rule.json")
	writeRule := func(marker string) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"rules": []any{map[string]any{
			"path": "**/*.go", "rule": marker + ": inspect this file under this review policy.",
		}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rulePath, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	var requests []string
	failSecond := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			http.Error(w, "cannot read request", http.StatusBadRequest)
			return
		}
		file := "unknown"
		switch {
		case strings.Contains(string(body), "PROBE_FILE_A"):
			file = "a.go"
		case strings.Contains(string(body), "PROBE_FILE_B"):
			file = "b.go"
		}
		rule := "unknown"
		switch {
		case strings.Contains(string(body), "POLICY_V1"):
			rule = "POLICY_V1"
		case strings.Contains(string(body), "POLICY_V2"):
			rule = "POLICY_V2"
		}
		if file == "unknown" || rule == "unknown" {
			t.Errorf("request missing fixture file or effective rule: file=%s rule=%s", file, rule)
		}
		mu.Lock()
		requests = append(requests, file+"/"+rule)
		fail := failSecond && file == "b.go"
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if fail {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","message":"controlled fixture failure"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"probe","object":"chat.completion","model":"probe-model","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"done","type":"function","function":{"name":"task_done","arguments":"{\"state\":\"DONE\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	}))
	defer server.Close()
	t.Setenv("OCR_LLM_URL", server.URL+"/v1/chat/completions")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "probe-model")
	t.Setenv("OCR_LLM_PROTOCOL", "openai")
	t.Setenv("OCR_USE_ANTHROPIC", "false")

	type observation struct {
		output   jsonOutput
		requests []string
	}
	run := func(label, resumeID string) observation {
		t.Helper()
		mu.Lock()
		before := len(requests)
		mu.Unlock()
		args := []string{"--repo", repo, "--rule", rulePath, "--no-plan", "--no-dedup", "--no-summary", "--concurrency", "1", "--format", "json", "--audience", "agent"}
		if resumeID != "" {
			args = append(args, "--resume", resumeID)
		}
		opts, err := parseScanFlags(args)
		if err != nil {
			t.Fatal(err)
		}
		var scanErr error
		raw := captureStdout(t, func() { scanErr = executeScan(opts) })
		if scanErr != nil {
			t.Fatalf("%s: %v", label, scanErr)
		}
		var out jsonOutput
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatalf("%s: invalid JSON: %v; output=%s", label, err, raw)
		}
		mu.Lock()
		sent := append([]string(nil), requests[before:]...)
		mu.Unlock()
		var reused, rerun int64
		if out.Resume != nil {
			reused, rerun = out.Resume.ReusedFiles, out.Resume.RerunFiles
		}
		t.Logf("%s status=%s http_requests=%d reused_files=%d rerun_files=%d warnings=%d sent=%v", label, out.Status, len(sent), reused, rerun, len(out.Warnings), sent)
		return observation{output: out, requests: sent}
	}
	assertResume := func(got observation, reused, rerun int64, sent string) {
		t.Helper()
		if got.output.Status != "success" || len(got.output.Warnings) != 0 || got.output.SessionID == "" {
			t.Fatalf("resume did not complete successfully: %+v", got)
		}
		if got.output.Resume == nil || got.output.Resume.ReusedFiles != reused || got.output.Resume.RerunFiles != rerun {
			t.Fatalf("resume counters = %+v, want reused=%d rerun=%d", got.output.Resume, reused, rerun)
		}
		if actual := strings.Join(got.requests, ","); actual != sent {
			t.Fatalf("requests = %q, want %q", actual, sent)
		}
	}

	writeRule("POLICY_V1")
	initial := run("initial_partial_policy_v1", "")
	if initial.output.Status != "completed_with_errors" || strings.Join(initial.requests, ",") != "a.go/POLICY_V1,b.go/POLICY_V1" || initial.output.SessionID == "" || len(initial.output.Warnings) == 0 {
		t.Fatalf("fixture did not produce one completed and one failed file: %+v", initial)
	}
	mu.Lock()
	failSecond = false
	mu.Unlock()
	unchanged := run("resume_unchanged_policy_control", initial.output.SessionID)
	assertResume(unchanged, 1, 1, "b.go/POLICY_V1")

	writeRule("POLICY_V2")
	changed := run("resume_changed_policy", initial.output.SessionID)
	assertResume(changed, 0, 2, "a.go/POLICY_V2,b.go/POLICY_V2")
	fresh := run("fresh_policy_v2_control", "")
	if fresh.output.Status != "success" || strings.Join(fresh.requests, ",") != "a.go/POLICY_V2,b.go/POLICY_V2" {
		t.Fatalf("fresh scan did not send the new rule for both files: %+v", fresh)
	}
	chained := run("resume_new_policy_again", changed.output.SessionID)
	assertResume(chained, 2, 0, "")

	if err := os.WriteFile(rulePath, []byte(`{"rules":[
		{"path":"**/a.go","rule":"POLICY_V1: inspect this file under this review policy."},
		{"path":"**/b.go","rule":"POLICY_V2: inspect this file under this review policy."}
	]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	partial := run("resume_only_a_policy_changed", chained.output.SessionID)
	assertResume(partial, 1, 1, "a.go/POLICY_V1")

	// Source location, JSON formatting and non-matching rules do not change
	// either file's effective rule and must not invalidate its checkpoint.
	rulePath = filepath.Join(t.TempDir(), "moved-rule.json")
	if err := os.WriteFile(rulePath, []byte(`{"rules":[{"path":"**/*.md","rule":"unrelated policy"},{"path":"**/a.go","rule":"POLICY_V1: inspect this file under this review policy."},{"path":"**/b.go","rule":"POLICY_V2: inspect this file under this review policy."}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	equivalent := run("resume_equivalent_rules", partial.output.SessionID)
	assertResume(equivalent, 2, 0, "")
}
