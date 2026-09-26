// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/gate"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func gateReviewJSON(t *testing.T, state session.TerminalState, comments []model.LlmComment, failures []llmloop.ToolFailureDetail) string {
	t.Helper()
	var out bytes.Buffer
	provider := &mockResultProvider{manifest: mockManifest(state), toolFailures: failures}
	if err := emitRunResult(context.Background(), provider, comments, time.Now(), "json", "agent", nil, nil, &out, nil); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func executeGate(t *testing.T, input string, args ...string) (string, error) {
	t.Helper()
	cmd := newGateCmd()
	var out bytes.Buffer
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestGateConsumesReviewOutput(t *testing.T) {
	for _, tc := range []struct {
		name     string
		state    session.TerminalState
		comments []model.LlmComment
		failures []llmloop.ToolFailureDetail
		status   gate.Status
	}{
		{"complete without comments", session.StateComplete, nil, nil, gate.Pass},
		{"blocking findings", session.StateComplete, []model.LlmComment{{Severity: "high", Content: "problem"}}, nil, gate.Fail},
		{"missing severity", session.StateComplete, []model.LlmComment{{Content: "problem"}}, nil, gate.Inconclusive},
		{"partial coverage", session.StatePartial, nil, nil, gate.Inconclusive},
		{"failed review", session.StateFailed, nil, nil, gate.Inconclusive},
		{"skipped review", session.StateSkipped, nil, nil, gate.Inconclusive},
		{"comment delivery", session.StateComplete, nil, []llmloop.ToolFailureDetail{{ToolName: "code_comment"}}, gate.Inconclusive},
		{"exploration failure", session.StateComplete, nil, []llmloop.ToolFailureDetail{{ToolName: "file_read"}}, gate.Pass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := gateReviewJSON(t, tc.state, tc.comments, tc.failures)
			output, err := executeGate(t, input, "--input", "-", "--format", "json", "--fail-on-severity", "high")
			var result gate.Result
			if parseErr := json.Unmarshal([]byte(output), &result); parseErr != nil {
				t.Fatalf("not a single JSON decision: %v\n%s", parseErr, output)
			}
			if result.Status != tc.status || (err == nil) != (tc.status == gate.Pass) {
				t.Fatalf("status=%s err=%v; want %s", result.Status, err, tc.status)
			}
		})
	}
}

func TestGateFileAndText(t *testing.T) {
	input := gateReviewJSON(t, session.StateComplete, nil, nil)
	path := filepath.Join(t.TempDir(), "review result.json")
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := executeGate(t, "", "--input", path)
	if err != nil || !strings.Contains(output, "Review gate: pass") || !strings.Contains(output, "complete_coverage") {
		t.Fatalf("output=%s err=%v", output, err)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || string(unchanged) != input {
		t.Fatal("gate changed its input file")
	}
}

func TestGateArgumentsAndMalformedInput(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--input", "-", "unexpected"}, {"--input", "-", "--format", "sarif"},
		{"--input", "-", "--fail-on-severity", "bad"}, {"--input", "-", "--expected-head", "HEAD"},
		{"--input", filepath.Join(t.TempDir(), "missing.json")},
	} {
		output, err := executeGate(t, "{}", args...)
		if err == nil || output != "" {
			t.Fatalf("args=%v: output=%s err=%v", args, output, err)
		}
	}
	output, err := executeGate(t, "{", "--input", "-", "--format", "json")
	var result gate.Result
	if json.Unmarshal([]byte(output), &result) != nil || err == nil || result.Status != gate.Inconclusive {
		t.Fatalf("malformed result: output=%s err=%v", output, err)
	}
}

func TestGateRevisionFlags(t *testing.T) {
	input := gateReviewJSON(t, session.StateComplete, nil, nil)
	var doc map[string]any
	if err := json.Unmarshal([]byte(input), &doc); err != nil {
		t.Fatal(err)
	}
	m := doc["manifest"].(map[string]any)
	m["input"] = map[string]any{"mode": "range", "resolved_base": strings.Repeat("a", 40), "resolved_head": strings.Repeat("b", 40)}
	encoded, _ := json.Marshal(doc)
	args := []string{"--input", "-", "--expected-base", strings.Repeat("a", 40), "--expected-head", strings.Repeat("b", 40)}
	if output, err := executeGate(t, string(encoded), args...); err != nil {
		t.Fatalf("matching input: %s %v", output, err)
	}
	args[len(args)-1] = strings.Repeat("c", 40)
	if output, err := executeGate(t, string(encoded), args...); err == nil || !strings.Contains(output, "revision_mismatch") {
		t.Fatalf("stale input: %s %v", output, err)
	}
}

type gateFailWriter struct{ remaining int }

func (w *gateFailWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return 0, errors.New("output unavailable")
	}
	w.remaining--
	return len(p), nil
}

func TestGateOutputErrors(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		cmd := newGateCmd()
		cmd.SetIn(strings.NewReader(gateReviewJSON(t, session.StateComplete, nil, nil)))
		cmd.SetOut(&gateFailWriter{})
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--input", "-", "--format", format})
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "write gate result") {
			t.Fatalf("format=%s: %v", format, err)
		}
	}
	result := gate.Result{Status: gate.Pass, Checks: []gate.Check{{Name: "coverage", Status: gate.Pass}}}
	if err := writeGateResult(&gateFailWriter{remaining: 1}, "text", result); err == nil {
		t.Fatal("ignored write error after the header")
	}
}

func TestGateRegisteredWithoutGitRequirement(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"gate"})
	if err != nil || cmd.Name() != "gate" || commandNeedsGit(cmd) {
		t.Fatalf("gate registration: %v, %v", cmd, err)
	}
	output, err := executeGate(t, "", "--help")
	if err != nil || !strings.Contains(output, "--fail-on-severity") {
		t.Fatalf("gate help: %s, %v", output, err)
	}
}
