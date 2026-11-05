// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
)

func TestSessionDiagnoseCommand(t *testing.T) {
	setTestHome(t, t.TempDir())
	repo := t.TempDir()
	sh := session.New(repo, "main", "test-model", session.SessionOptions{})
	rec := sh.GetOrCreateFileSession("group-a").AppendTaskRecord(session.MainTask, nil)
	rec.SetError(errors.New("provider timeout"), 0)
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	root, err := session.Diagnose(repo, sh.SessionID, session.DiagnoseOptions{TaskKey: "group-a"})
	if err != nil {
		t.Fatal(err)
	}
	requestID := root.Tasks[0].Phases[0].Events[0].RequestID
	for _, tc := range []struct {
		name  string
		flags []string
		want  string
	}{
		{"overview", nil, "llm_request_failed"},
		{"task", []string{"--task", "group-a"}, "line=2 llm_request request=1"},
		{"request", []string{"--request", requestID}, "Selected request: " + requestID},
		{"json request", []string{"--request", requestID, "--json"}, `"request_id": "` + requestID + `"`},
		{"event", []string{"--event", "3"}, `"error": "provider timeout"`},
		{"json", []string{"--json"}, `"schema_version": "ocr.session-diagnostics/v1"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newSessionDiagnoseCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"--repo", repo, sh.SessionID}, tc.flags...))
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("output missing %q: %s", tc.want, out.String())
			}
			if strings.HasPrefix(tc.name, "json") {
				var report session.DiagnosticReport
				if err := json.Unmarshal(out.Bytes(), &report); err != nil || len(report.Tasks) != 1 {
					t.Fatalf("JSON output invalid: %v, %s", err, out.String())
				}
			}
		})
	}
	for _, value := range []string{"0", "-1"} {
		cmd := newSessionDiagnoseCmd()
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		cmd.SetArgs([]string{"--repo", repo, sh.SessionID, "--event", value})
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "positive") {
			t.Fatalf("event %s accepted: %v", value, err)
		}
	}
}

func TestSessionDiagnoseRecordedOutcome(t *testing.T) {
	setTestHome(t, t.TempDir())
	repo := t.TempDir()
	sh := session.New(repo, "main", "test-model", session.SessionOptions{})
	sh.GetOrCreateFileSession("a.go").AppendTaskRecord(session.MainTask, nil).SetError(errors.New("temporary provider error"), 0)
	sh.SetFinalManifest(&session.RunManifest{
		SchemaVersion: session.ManifestSchemaVersion,
		TerminalState: session.StatePartial,
		Coverage: session.Coverage{
			Selected:  []session.CoverageItem{{ItemID: "a", Path: "a.go"}, {ItemID: "b", Path: "b.go"}},
			Completed: []session.CoverageItem{{ItemID: "a", Path: "a.go"}},
			Failed:    []session.CoverageItem{{ItemID: "b", Path: "b.go", Classification: session.FailureTimeout, Reason: "provider timeout"}},
		},
		RunFailure: &session.RunFailure{Classification: session.RunFailureTimeout, Reason: "time limit"},
	})
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	cmd := newSessionDiagnoseCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--repo", repo, sh.SessionID})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Recorded outcome: partial", "Coverage: selected=2 completed=1 reused=0 failed=1 waived=0",
		`failed item="b.go" classification="timeout" reason="provider timeout"`,
		`Run failure: classification="timeout" reason="time limit"`, "temporary provider error",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q: %s", want, out.String())
		}
	}
}
