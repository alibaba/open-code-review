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
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// admissionE2EMarkers maps per-file diff markers to file names; three files in
// three different directories keep the group splitter from collapsing them
// into one conversation, so the review actually dispatches parallel requests.
var admissionE2EMarkers = map[string]string{
	"MARKER_ONE":   "pkg/a/a.go",
	"MARKER_TWO":   "pkg/b/b.go",
	"MARKER_THREE": "pkg/c/c.go",
}

// admissionE2EChangeLines is the per-file changed-line count. It must exceed
// the plan-phase skip threshold (5 lines), or the run skips planning and the
// e2e silently loses a path.
const admissionE2EChangeLines = 8

// admissionE2EServer serves the same Anthropic shapes as fakeLLM while tracking
// how many handlers run concurrently. Per file it plays a two-round main loop —
// first a code_comment carrying a probe string, then task_done — so the review
// filter (which only runs once comments exist) fires too. It records which of
// the request kinds actually arrived, so the test asserts the paths it claims
// to gate rather than trusting the fixture.
type admissionE2EServer struct {
	mu sync.Mutex
	// mainRound counts main-loop rounds per file marker.
	mainRound map[string]int
	// sawPlan, sawFilter, sawMain record arriving request kinds.
	sawPlan, sawFilter, sawMain bool

	cur atomic.Int32
	max atomic.Int32
}

func newAdmissionE2EServer() *admissionE2EServer {
	return &admissionE2EServer{mainRound: map[string]int{}}
}

func (s *admissionE2EServer) mark(kind string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch kind {
	case "plan":
		s.sawPlan = true
	case "filter":
		s.sawFilter = true
	case "main":
		s.sawMain = true
	}
}

func (s *admissionE2EServer) sawAll() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sawPlan && s.sawFilter && s.sawMain
}

func (s *admissionE2EServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := s.cur.Add(1)
	for {
		m := s.max.Load()
		if n <= m || s.max.CompareAndSwap(m, n) {
			break
		}
	}
	defer s.cur.Add(-1)

	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(r.Body)
	raw := body.Bytes()
	w.Header().Set("Content-Type", "application/json")

	time.Sleep(40 * time.Millisecond)

	switch {
	case bytes.Contains(raw, []byte(`"report_incorrect_comments"`)):
		// Review filter: approve every comment via the filter's own tool.
		s.mark("filter")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-test",
			"content":[{"type":"tool_use","id":"tu_f","name":"approve_all_comments","input":{}}],
			"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`))
	case bytes.Contains(raw, []byte(`"task_done"`)):
		// Main loop round for one file: comment first, then done.
		s.mark("main")
		marker := admissionE2EMarkerOf(raw)
		s.mu.Lock()
		s.mainRound[marker]++
		round := s.mainRound[marker]
		s.mu.Unlock()
		if round == 1 {
			arg := fmt.Sprintf(`{"path":%q,"content":"FILTERPROBE finding","existing_code":"no such code"}`,
				admissionE2EMarkers[marker])
			_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-test",
				"content":[{"type":"tool_use","id":"tu_c","name":"code_comment","input":{"comments":[%s]}}],
				"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`, arg)))
			return
		}
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-test",
			"content":[{"type":"tool_use","id":"tu_1","name":"task_done","input":{"state":"DONE"}}],
			"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`))
	default:
		// No tools: the grouping call or a per-file plan phase.
		s.mark("plan")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-test",
			"content":[{"type":"text","text":"plan: read the diff, then call task_done"}],
			"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`))
	}
}

// admissionE2EMarkerOf finds which fixture file the request belongs to.
func admissionE2EMarkerOf(raw []byte) string {
	for marker := range admissionE2EMarkers {
		if bytes.Contains(raw, []byte(marker)) {
			return marker
		}
	}
	return "unknown"
}

// admissionE2ERepo builds a three-directory repo with one reviewable commit;
// each file's change is large enough to keep the plan phase from being skipped.
func admissionE2ERepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	retryTestGit(t, dir, "init", "-q", "-b", "main")
	for _, path := range admissionE2EMarkers {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(baseBody(path)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	retryTestGit(t, dir, "add", ".")
	retryTestGit(t, dir, "commit", "-q", "-m", "base")
	for marker, path := range admissionE2EMarkers {
		full := filepath.Join(dir, path)
		if err := os.WriteFile(full, []byte(changeBody(path, marker)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	retryTestGit(t, dir, "add", ".")
	retryTestGit(t, dir, "commit", "-q", "-m", "change")
	return dir
}

func baseBody(path string) string {
	return fmt.Sprintf("package %s\n\nfunc F() int { return 1 }\n", filepath.Base(filepath.Dir(path)))
}

func changeBody(path, marker string) string {
	body := fmt.Sprintf("package %s\n\n// changed %s\nfunc F() int {\n", filepath.Base(filepath.Dir(path)), marker)
	for i := 0; i < admissionE2EChangeLines; i++ {
		body += fmt.Sprintf("\t// line %d\n", i)
	}
	return body + "\treturn 2\n}\n"
}

// admissionE2EHome redirects HOME like startFakeLLM but wires the endpoint
// through the OCR config file instead of OCR_LLM_* env, so the provider entry
// (and its max_in_flight) drives resolution.
func admissionE2EHome(t *testing.T, srvURL string, maxInFlight int) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	entry := map[string]any{
		"url":      srvURL + "/v1/messages",
		"api_key":  "test-token",
		"protocol": "anthropic",
		"model":    "claude-test",
	}
	if maxInFlight > 0 {
		entry["max_in_flight"] = maxInFlight
	}
	config := map[string]any{
		"provider":         "e2e-fake",
		"model":            "claude-test",
		"custom_providers": map[string]any{"e2e-fake": entry},
	}
	data, _ := json.MarshalIndent(config, "", "  ")
	dir := filepath.Join(home, ".opencodereview")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

// TestReviewE2E_AdmissionScopeGatesReviewRequests runs the real review command
// against a fake provider and asserts the admission scope covers the request
// paths the run actually fires — grouping/plan, the per-file main loop, and the
// review filter — end to end through config resolution, the review command's
// context, and the real SDK client.
//
// Baseline (no max_in_flight): parallel file dispatch overlaps at the server.
// With max_in_flight=1: the same review serializes to one in-flight attempt at
// a time. The grace round, memory compression, and re-location paths cannot be
// forced from a fixture this small; their scope preservation is pinned at the
// loop level in internal/llmloop/admission_scope_test.go.
func TestReviewE2E_AdmissionScopeGatesReviewRequests(t *testing.T) {
	repoDir := admissionE2ERepo(t)

	// Baseline: no gate — parallel dispatch must overlap.
	baseline := newAdmissionE2EServer()
	baselineSrv := httptest.NewServer(baseline)
	defer baselineSrv.Close()
	admissionE2EHome(t, baselineSrv.URL, 0)
	if out, err := runReviewCapture(t, repoDir); err != nil {
		t.Fatalf("baseline runReview: %v\n%s", err, out)
	}
	if got := baseline.max.Load(); got < 2 {
		t.Fatalf("baseline max concurrent attempts = %d, want >= 2 (test harness must produce overlap)", got)
	}
	if !baseline.sawAll() {
		t.Fatal("baseline fixture must fire plan, main, and filter requests — else the e2e gates less than it claims")
	}

	// Gated: one in-flight attempt at a time across every fired path.
	gated := newAdmissionE2EServer()
	gatedSrv := httptest.NewServer(gated)
	defer gatedSrv.Close()
	admissionE2EHome(t, gatedSrv.URL, 1)
	if out, err := runReviewCapture(t, repoDir); err != nil {
		t.Fatalf("gated runReview: %v\n%s", err, out)
	}
	if !gated.sawAll() {
		t.Fatal("gated fixture must fire plan, main, and filter requests — else the e2e gates less than it claims")
	}
	if got := gated.max.Load(); got != 1 {
		t.Fatalf("gated max concurrent attempts = %d, want exactly 1", got)
	}
}

func runReviewCapture(t *testing.T, repoDir string) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() {
		err = runReview([]string{"--repo", repoDir, "--from", "HEAD~1", "--to", "HEAD", "--format", "json", "--concurrency", "4"})
	})
	return out, err
}
