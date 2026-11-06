// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/tool"
)

func recoveryRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"src/use-toggle.tsx": "export const useToggle = () => <div />;\n",
		"src/app.ts":         "import { useToggle } from './use-toggle';\nuseToggle();\n",
		"src/other.ts":       "export const other = true;\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	recoveryGit(t, dir, "init", "-q")
	recoveryGit(t, dir, "add", ".")
	recoveryGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "fixture")
	return dir, recoveryGit(t, dir, "rev-parse", "HEAD")
}

func recoveryGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestRunMainTask_FileReadRecoveryEvidence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		nextPath   string
		wantStatus string
	}{
		{name: "corrected extension", nextPath: "src/use-toggle.tsx", wantStatus: "candidate_read"},
		{name: "unrelated success", nextPath: "src/other.ts", wantStatus: "not_observed"},
		{name: "no later read", wantStatus: "not_observed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			dir, head := recoveryRepo(t)
			responses := []*llm.ChatResponse{fileReadToolCallResponse("missing", `{"file_path":"src/use-toggle.ts"}`)}
			if tc.nextPath != "" {
				args, err := json.Marshal(map[string]string{"file_path": tc.nextPath})
				if err != nil {
					t.Fatal(err)
				}
				responses = append(responses, fileReadToolCallResponse("later", string(args)))
			}
			responses = append(responses, taskDoneResponse())
			client := &fakeClient{responses: responses}
			deps := newTestDeps(client)
			deps.Tools = tool.NewRegistry()
			deps.Tools.Register(tool.NewFileRead(&tool.FileReader{RepoDir: dir, Mode: tool.ModeRange, Ref: head}))
			runner := NewRunner(deps)
			completed, _, err := runner.RunMainTask(context.Background(), []llm.Message{llm.NewTextMessage("user", "Review src/app.ts")}, "src/app.ts")
			if err != nil || !completed {
				t.Fatalf("completed = %v, err = %v", completed, err)
			}
			failures := runner.ToolFailures()
			if len(failures) != 1 || failures[0].Arguments != `{"file_path":"src/use-toggle.ts"}` {
				t.Fatalf("historical failures = %+v", failures)
			}
			data, err := json.Marshal(failures)
			if err != nil {
				t.Fatal(err)
			}
			var output []struct {
				Recovery *struct {
					Status string `json:"status"`
				} `json:"recovery"`
			}
			if err := json.Unmarshal(data, &output); err != nil {
				t.Fatal(err)
			}
			if output[0].Recovery == nil || output[0].Recovery.Status != tc.wantStatus {
				t.Fatalf("failure JSON = %s, want recovery status %q", data, tc.wantStatus)
			}
			recovery := failures[0].Recovery
			if recovery.TargetCommit != head || recovery.CandidatePath != "src/use-toggle.tsx" {
				t.Fatalf("candidate identity = %+v", recovery)
			}
			if tc.wantStatus == "candidate_read" {
				read := recovery.SuccessfulRead
				if read == nil || read.ToolCallNumber != 2 || read.FilePath != "src/use-toggle.tsx" || read.StartLine != 1 || read.EndLine != 2 || read.TotalLines != 2 || read.IsTruncated {
					t.Fatalf("successful read = %+v", read)
				}
			} else if recovery.SuccessfulRead != nil {
				t.Fatalf("unexpected successful read = %+v", recovery.SuccessfulRead)
			}
			feedback := client.requests[1].Messages
			feedbackText, _ := feedback[len(feedback)-1].Content.(string)
			if !strings.Contains(feedbackText, `Candidate: "src/use-toggle.tsx"`) {
				t.Fatalf("missing candidate suggestion in %v", feedback)
			}
			if strings.Contains(failures[0].Error, "Candidate:") {
				t.Fatalf("suggestion changed historical error: %s", failures[0].Error)
			}
		})
	}
}

func recoveryRunner(fr *tool.FileReader) *Runner {
	reg := tool.NewRegistry()
	reg.Register(tool.NewFileRead(fr))
	reg.Freeze()
	return NewRunner(Deps{Tools: reg})
}

func executeRecoveryRead(r *Runner, task, arguments string) tool.TaskCheckpoint {
	return r.executeToolCall(context.Background(), task, llm.ToolCall{
		Function: llm.FunctionCall{Name: "file_read", Arguments: arguments},
	}, nil, "")
}

func TestFileReadRecovery_AssociationBoundaries(t *testing.T) {
	dir, first := recoveryRepo(t)
	recoveryGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-qm", "other target")
	second := recoveryGit(t, dir, "rev-parse", "HEAD")
	const missing = `{"file_path":"src/use-toggle.ts"}`
	const candidate = `{"file_path":"src/use-toggle.tsx"}`
	type step struct {
		task, ref, args string
	}
	for _, tc := range []struct {
		name          string
		steps         []step
		wantFailures  int
		wantReadCalls []int64
	}{
		{name: "other task", steps: []step{{"a", first, missing}, {"b", first, candidate}}, wantFailures: 1},
		{name: "other commit", steps: []step{{"a", first, missing}, {"a", second, candidate}}, wantFailures: 1},
		{name: "success before failure", steps: []step{{"a", first, candidate}, {"a", first, missing}}, wantFailures: 1},
		{name: "candidate read fails", steps: []step{{"a", first, missing}, {"a", first, `{"file_path":"src/use-toggle.tsx","start_line":100}`}}, wantFailures: 2},
		{name: "all prior failures", steps: []step{{"a", first, missing}, {"a", first, missing}, {"a", first, candidate}}, wantFailures: 2, wantReadCalls: []int64{3, 3}},
		{name: "first success retained", steps: []step{{"a", first, missing}, {"a", first, candidate}, {"a", first, candidate}}, wantFailures: 1, wantReadCalls: []int64{2}},
		{name: "success does not cover new failure", steps: []step{{"a", first, missing}, {"a", first, candidate}, {"a", first, missing}}, wantFailures: 2, wantReadCalls: []int64{2, 0}},
		{name: "different candidate", steps: []step{{"a", first, `{"file_path":"src/other.tsx"}`}, {"a", first, candidate}}, wantFailures: 1},
		{name: "malformed arguments", steps: []step{{"a", first, `{bad`}, {"a", first, candidate}}, wantFailures: 1},
		{name: "empty path is not a read", steps: []step{{"a", first, missing}, {"a", first, `{}`}}, wantFailures: 1},
		{name: "unknown target", steps: []step{{"a", "HEAD", missing}, {"a", "HEAD", candidate}}, wantFailures: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fr := &tool.FileReader{RepoDir: dir, Mode: tool.ModeCommit}
			r := recoveryRunner(fr)
			for _, step := range tc.steps {
				fr.Ref = step.ref
				executeRecoveryRead(r, step.task, step.args)
			}
			failures := r.ToolFailures()
			if len(failures) != tc.wantFailures {
				t.Fatalf("failures = %+v, want %d", failures, tc.wantFailures)
			}
			for i, failure := range failures {
				var want int64
				if i < len(tc.wantReadCalls) {
					want = tc.wantReadCalls[i]
				}
				recovery := failure.Recovery
				if recovery == nil {
					t.Fatalf("failure %d has no recovery state", i)
				}
				if want == 0 {
					if recovery.Status != "not_observed" || recovery.SuccessfulRead != nil {
						t.Fatalf("failure %d falsely associated: %+v", i, recovery)
					}
				} else if recovery.Status != "candidate_read" || recovery.SuccessfulRead == nil || recovery.SuccessfulRead.ToolCallNumber != want {
					t.Fatalf("failure %d recovery = %+v, want call %d", i, recovery, want)
				}
			}
		})
	}
}

func TestFileReadRecovery_RangeAndSnapshot(t *testing.T) {
	dir, _ := recoveryRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "src/use-toggle.tsx"), []byte(strings.Repeat("export {};\n", 600)), 0644); err != nil {
		t.Fatal(err)
	}
	recoveryGit(t, dir, "add", ".")
	recoveryGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "large candidate")
	head := recoveryGit(t, dir, "rev-parse", "HEAD")
	for _, tc := range []struct {
		name, args string
		start, end int
		truncated  bool
	}{
		{name: "partial", args: `{"file_path":"src/use-toggle.tsx","start_line":2,"end_line":3}`, start: 2, end: 3},
		{name: "truncated", args: `{"file_path":"src/use-toggle.tsx"}`, start: 1, end: 500, truncated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := recoveryRunner(&tool.FileReader{RepoDir: dir, Mode: tool.ModeRange, Ref: head})
			executeRecoveryRead(r, "task", `{"file_path":"src/use-toggle.ts"}`)
			before := r.ToolFailures()
			executeRecoveryRead(r, "task", tc.args)
			if before[0].Recovery.Status != "not_observed" || before[0].Recovery.SuccessfulRead != nil {
				t.Fatalf("earlier snapshot changed: %+v", before[0].Recovery)
			}
			failures := r.ToolFailures()
			read := failures[0].Recovery.SuccessfulRead
			if read == nil || read.Arguments != tc.args || read.StartLine != tc.start || read.EndLine != tc.end || read.TotalLines != 601 || read.IsTruncated != tc.truncated {
				t.Fatalf("read metadata = %+v", read)
			}
			failures[0].Recovery.CandidatePath = "mutated"
			read.FilePath = "mutated"
			read.Arguments = "mutated"
			again := r.ToolFailures()[0].Recovery
			if again.CandidatePath != "src/use-toggle.tsx" || again.SuccessfulRead.FilePath != "src/use-toggle.tsx" || again.SuccessfulRead.Arguments != tc.args {
				t.Fatalf("snapshot mutated runner: %+v", again)
			}
		})
	}
}

func TestFileReadRecovery_ConcurrentTasks(t *testing.T) {
	dir, head := recoveryRepo(t)
	r := recoveryRunner(&tool.FileReader{RepoDir: dir, Mode: tool.ModeRange, Ref: head, Runner: gitcmd.New(2)})
	const tasks = 8
	var wg sync.WaitGroup
	for i := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task := fmt.Sprintf("task-%d", i)
			executeRecoveryRead(r, task, `{"file_path":"src/use-toggle.ts"}`)
			_ = r.ToolFailures()
			executeRecoveryRead(r, task, `{"file_path":"src/use-toggle.tsx"}`)
		}()
	}
	wg.Wait()
	failures := r.ToolFailures()
	if len(failures) != tasks || r.ToolCalls()["file_read"] != 2*tasks {
		t.Fatalf("failures = %d, calls = %v", len(failures), r.ToolCalls())
	}
	seen := make(map[int64]bool)
	for i, failure := range failures {
		recovery := failure.Recovery
		if recovery == nil || recovery.Status != "candidate_read" || recovery.SuccessfulRead == nil || recovery.SuccessfulRead.ToolCallNumber <= failure.ToolCallNumber {
			t.Fatalf("failure %d recovery = %+v", i, recovery)
		}
		if seen[recovery.SuccessfulRead.ToolCallNumber] {
			t.Fatal("one task's successful read was associated with another task")
		}
		seen[recovery.SuccessfulRead.ToolCallNumber] = true
		if i > 0 && failure.ToolCallNumber <= failures[i-1].ToolCallNumber {
			t.Fatal("failures are not ordered")
		}
	}
}
