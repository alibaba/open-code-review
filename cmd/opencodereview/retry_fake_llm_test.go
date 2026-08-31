// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
)

// retryTestFixedTime is an arbitrary fixed instant for hand-fed attempt
// timestamps, so derived durations stay deterministic.
var retryTestFixedTime = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

// Shared fake-LLM fixture for the #368 retry-report tests. Used by both the
// automated end-to-end tests and the tag-gated manual harness
// (manual_e2e_retry_test.go), so the two cannot drift apart in what they
// consider a retryable server.

// fakeLLM serves the Anthropic messages API. Per reviewed file it can inject a
// 429 (which the SDK must retry on its own) or a permanent 402 (which it must
// not retry).
type fakeLLM struct {
	mu sync.Mutex
	// attemptsByFile counts real HTTP attempts per file, so a test can assert
	// the SDK retried rather than OCR re-requesting.
	attemptsByFile map[string]int
	// groupingCalls counts requests recognized as the GROUPING_TASK call, so a
	// test can prove the recognition still works — see
	// assertGroupingRecognized.
	groupingCalls int
	// rateLimitOnce lists files whose first attempt returns 429.
	rateLimitOnce map[string]bool
	// hardFail lists files whose every attempt returns 402.
	hardFail map[string]bool
}

// newFakeLLM returns a server that succeeds on the first attempt for every
// file. rateLimitOnce and hardFail are set by the caller before use.
func newFakeLLM() *fakeLLM {
	return &fakeLLM{
		attemptsByFile: map[string]int{},
		rateLimitOnce:  map[string]bool{},
		hardFail:       map[string]bool{},
	}
}

// failAll makes every file in the fixture fail permanently. Tests that mean
// "the whole review failed" use this rather than naming files, so they keep
// meaning that when the fixture's file set changes.
func (f *fakeLLM) failAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, name := range markers {
		f.hardFail[name] = true
	}
}

// assertGroupingRecognized fails if no request matched groupingSystemPrompt.
// Without it a reworded GROUPING_TASK system prompt would degrade these tests
// silently: the fake would miss the grouping call, fall through to the plan
// response, fail to parse as a grouping reply, and leave groupDiffs falling back
// to per-file dispatch — the very shape the grouping branch constructs on
// purpose. Every assertion would still pass with the coverage gone.
//
// One test calling this is enough. A prompt reword is a global event, so a
// single guard catches it; it is deliberately not in startFakeLLM's cleanup,
// because a test whose change set never reaches the grouping call would then
// fail for the wrong reason.
func (f *fakeLLM) assertGroupingRecognized(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.groupingCalls == 0 {
		t.Fatalf("no request matched %q — did GROUPING_TASK's system prompt change? "+
			"Point groupingSystemPrompt at a fragment that still appears in "+
			"internal/config/template/prompts/grouping_task_system.md", groupingSystemPrompt)
	}
}

// attemptCounts snapshots the per-file real HTTP attempt counts.
func (f *fakeLLM) attemptCounts() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]int, len(f.attemptsByFile))
	for k, v := range f.attemptsByFile {
		out[k] = v
	}
	return out
}

// markers maps a per-file token that appears only in that file's diff onto the
// file name. The prompt's change_files section lists the *other* file's path,
// so matching on the path itself would misattribute requests; the marker only
// occurs in the reviewed file's diff body.
//
// Every reviewed file needs its own marker for attribution to work, so this map
// is also what defines the fixture's file set — see retryTestRepo.
var markers = map[string]string{
	"MARKER_ALPHA": "a.go",
	"MARKER_BETA":  "b.go",
	"MARKER_GAMMA": "c.go",
	"MARKER_DELTA": "d.go",
}

// groupingSystemPrompt is a stable fragment of GROUPING_TASK's system prompt,
// copied from internal/config/template/prompts/grouping_task_system.md. The
// grouping call carries file metadata only, never diff bodies, so it has no
// marker to attribute it by; this is what identifies it instead.
//
// Nothing links the two files at compile time, so a reword there silently stops
// matching here. assertGroupingRecognized is what turns that into a failure.
const groupingSystemPrompt = "file grouping assistant"

// groupingReply is one element of the JSON array GROUPING_TASK must return.
type groupingReply struct {
	Label string   `json:"label"`
	Files []string `json:"files"`
}

func (f *fakeLLM) fileOf(body []byte) string {
	for marker, name := range markers {
		if bytes.Contains(body, []byte(marker)) {
			return name
		}
	}
	return "unknown"
}

func (f *fakeLLM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(r.Body)
	raw := body.Bytes()
	file := f.fileOf(raw)
	hasTools := bytes.Contains(raw, []byte(`"tools"`))

	// Answer the grouping call with one group per file. These tests are about
	// per-request retry accounting, which needs each file to be its own request;
	// letting the model bundle them would collapse the very thing under test.
	// Served before the injection switch on purpose: the grouping call carries no
	// marker, so it must never absorb a file's 429 or 402.
	if bytes.Contains(raw, []byte(groupingSystemPrompt)) {
		f.mu.Lock()
		f.groupingCalls++
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		groups := make([]groupingReply, 0, len(markers))
		for _, name := range markers {
			groups = append(groups, groupingReply{Label: name, Files: []string{name}})
		}
		// The grouping answer is JSON that has to survive being carried inside a
		// JSON string, so let the encoder do both levels of quoting.
		inner, err := json.Marshal(groups)
		if err != nil {
			panic(err)
		}
		text, err := json.Marshal(string(inner))
		if err != nil {
			panic(err)
		}
		_, _ = fmt.Fprintf(w, `{"id":"msg_group","type":"message","role":"assistant","model":"claude-test",
			"content":[{"type":"text","text":%s}],
			"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`, text)
		return
	}

	f.mu.Lock()
	f.attemptsByFile[file]++
	n := f.attemptsByFile[file]
	rateLimit := f.rateLimitOnce[file] && n == 1
	fail := f.hardFail[file]
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("request-id", fmt.Sprintf("req_%s_%d", strings.TrimSuffix(file, ".go"), n))

	switch {
	case fail:
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"payment required"}}`))
		return
	case rateLimit:
		// Retry-After: 1 keeps the SDK's observed backoff deterministic enough to
		// assert on without making the test wait long.
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
		return
	}

	// Main-task rounds carry tool definitions; the plan phase does not.
	if hasTools {
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-test",
			"content":[{"type":"tool_use","id":"tu_1","name":"task_done","input":{"state":"DONE"}}],
			"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`))
		return
	}
	_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-test",
		"content":[{"type":"text","text":"plan: read the diff, then call task_done"}],
		"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`))
}

// retryTestGit runs git in dir with a fixed identity so the fixture repo does
// not depend on the developer's git config.
func retryTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{
		"-c", "user.email=ocr@example.test",
		"-c", "user.name=ocr",
		"-c", "commit.gpgsign=false",
	}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// retryTestRepo builds a repo with one reviewable commit range (HEAD~1..HEAD),
// each file carrying its own marker in the second commit only.
//
// The file set is `markers`, which holds enough files to reach
// GROUPING_MIN_FILES. That matters: below it, grouping is decided locally and a
// low-churn change set is bundled into a single group — one request covering
// every file, which would collapse the per-request accounting these tests exist
// to check. At or above it the grouping call runs, and the fake answers it with
// one group per file.
func retryTestRepo(t *testing.T) string {
	t.Helper()
	names := make([]string, 0, len(markers))
	for _, name := range markers {
		names = append(names, name)
	}
	dir := t.TempDir()
	retryTestGit(t, dir, "init", "-q", "-b", "main")
	for _, name := range names {
		body := fmt.Sprintf("package p\n\nfunc %s() int { return 1 }\n", strings.TrimSuffix(name, ".go"))
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	retryTestGit(t, dir, "add", ".")
	retryTestGit(t, dir, "commit", "-q", "-m", "base")
	for marker, name := range markers {
		body := fmt.Sprintf("package p\n\n// changed %s\nfunc %s() int {\n\treturn 2\n}\n",
			marker, strings.TrimSuffix(name, ".go"))
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	retryTestGit(t, dir, "add", ".")
	retryTestGit(t, dir, "commit", "-q", "-m", "change")
	return dir
}

// startFakeLLM starts srv and points the OCR_LLM_* endpoint resolution at it,
// with HOME/XDG_CONFIG_HOME redirected so the developer's real config and
// session directory (both under $HOME/.opencodereview) are never touched.
// Those paths resolve through os.UserHomeDir, which reads USERPROFILE on
// Windows and never falls back to HOME, so redirecting HOME alone left these
// runs writing to the real profile. Set both; the one that does not apply is
// harmless.
func startFakeLLM(t *testing.T, srv *fakeLLM) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	server := httptest.NewServer(srv)
	t.Cleanup(server.Close)

	t.Setenv("OCR_LLM_URL", server.URL+"/v1/messages")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "claude-test")
	t.Setenv("OCR_LLM_PROTOCOL", "anthropic")
	t.Setenv("OCR_LLM_AUTH_HEADER", "x-api-key")
	t.Setenv("OCR_LLM_TIMEOUT", "30")
}

// poisonedRetryCollector returns a collector holding one request that recorded
// an attempt and was never finalized, which is exactly the invariant violation
// Freeze refuses to publish. It is the only way to reach the construction-error
// branch from the outside: every production path finalizes on every exit.
func poisonedRetryCollector() *llm.RetryCollector {
	c := llm.NewRetryCollector()
	m := llm.RequestMeta{Model: "claude-test", FilePath: "ghost.go", TaskType: "main_task", RequestNo: 1}
	base := retryTestFixedTime
	c.RecordAttempt(m, llm.AttemptRecord{}, base, base)
	return c
}
