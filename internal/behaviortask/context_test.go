// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
)

func TestContextPaginationReplayAndVerification(test *testing.T) {
	input := fixture(test)
	input.Contexts = append(input.Contexts, Context{ID: "run", Kind: "verification", Source: "provided-ci", Version: "1", CollectedAt: "2026-11-13T00:00:00Z", Revision: input.Resolution.ResolvedHead, Environment: "isolated", ExecutionStatus: "passed", Content: "Captured execution of the supplied revision."})
	runner := newTestRunner(test, input, nil, nil)
	observePair(test, runner)
	outcome, err := runner.dispatch(context.Background(), "context_read", `{"kind":"verification","id":"run","max_bytes":8}`)
	if err != nil {
		test.Fatal(err)
	}
	var observation Observation
	if err := json.Unmarshal([]byte(outcome.Content), &observation); err != nil {
		test.Fatal(err)
	}
	if observation.Context == nil || observation.Context.Revision != input.Resolution.ResolvedHead || !observation.Truncated || observation.Continuation != "8" || observation.ReturnedContent != "Captured" {
		test.Fatalf("context: %+v", observation)
	}
	analysis := validAnalysis(input)
	analysis.Markdown += "\nCaptured verification [[" + observation.ID + "]]."
	if err := runner.validateAnalysis(analysis); err != nil {
		test.Fatal(err)
	}
	count := len(runner.store.items)
	replay, err := runner.dispatch(context.Background(), "context_read", `{"kind":"observation","id":"obs-000001"}`)
	if err != nil || len(runner.store.items) != count || !strings.Contains(replay.Content, "old value") {
		test.Fatalf("observation replay: %s, %v", replay.Content, err)
	}
	if err := runner.validateAnalysis(analysis); err != nil {
		test.Fatal("replay changed evidence identity")
	}
	for _, args := range []string{`{"kind":"observation","id":"missing"}`, `{"kind":"dependency","id":"https://example.com"}`} {
		result, err := runner.dispatch(context.Background(), "context_read", args)
		if err != nil || !strings.Contains(result.Content, "unavailable") {
			test.Fatalf("missing context: %s, %v", result.Content, err)
		}
	}
	if _, err := runner.dispatch(context.Background(), "context_read", `{"kind":"shell","id":"anything"}`); err == nil {
		test.Fatal("unsupported context kind")
	}
	if _, err := runner.dispatch(context.Background(), "context_read", `{"kind":"background","id":"goal","max_bytes":1}`); err == nil {
		test.Fatal("invalid byte limit")
	}
	for _, observation := range runner.store.items {
		if observation.ErrorClass != "" && runner.validateAnalysis(Analysis{Markdown: "[[" + observation.ID + "]]"}) == nil {
			test.Fatal("failed observation accepted as evidence")
		}
	}
}

func TestUTF8PageAndContextValidation(test *testing.T) {
	content := "a\u2192b"
	page, next, err := readPage(content, 0, 4)
	if err != nil || page != "a\u2192" || next != "4" {
		test.Fatalf("page: %q %q %v", page, next, err)
	}
	for _, values := range [][2]int{{-1, 4}, {2, 4}, {10, 4}, {0, 3}, {0, 70000}} {
		if _, _, err := readPage(content, values[0], values[1]); err == nil {
			test.Fatalf("accepted %v", values)
		}
	}
	if page, next, err := readPage("ab\u2192z", 0, 4); err != nil || page != "ab" || next != "2" {
		test.Fatalf("split rune: %q %s %v", page, next, err)
	}
	for _, values := range [][]Context{
		{{ID: "x", Kind: "shell", Source: "user"}},
		{{Kind: "background", Source: "user"}},
		{{ID: "x", Kind: "background"}},
		{{ID: "x", Kind: "background", Source: "user", Content: string([]byte{255})}},
		{{ID: "x", Kind: "issue", Source: "user"}, {ID: "x", Kind: "issue", Source: "user"}},
	} {
		if validateContexts(values) == nil {
			test.Fatal("accepted invalid context")
		}
	}
}

func TestPreparationAndArtifactFailures(test *testing.T) {
	input := fixture(test)
	client := fakeClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		test.Fatal("unexpected model call")
		return nil, nil
	})
	for _, configure := range []func(*Deps){
		func(deps *Deps) { deps.LLMClient = nil },
		func(deps *Deps) { deps.Model = "" },
		func(deps *Deps) { deps.ArtifactDir = "" },
		func(deps *Deps) { deps.MaxTokensBudget = -1 },
		func(deps *Deps) { deps.ArtifactDir = filepath.Join(input.RepoDir, "artifacts") },
		func(deps *Deps) { deps.ArtifactDir = test.TempDir() },
		func(deps *Deps) { deps.ArtifactDir = filepath.Join(test.TempDir(), "missing", "artifacts") },
		func(deps *Deps) { deps.Template = &template.BehaviorTemplate{} },
	} {
		deps := Deps{LLMClient: client, Model: "fake", ArtifactDir: filepath.Join(test.TempDir(), "artifacts")}
		configure(&deps)
		if _, err := NewRunner(context.Background(), deps, input); err == nil {
			test.Fatal("accepted invalid preparation")
		}
	}
	runner := newTestRunner(test, input, client, func(deps *Deps) { deps.Template.MainTask.Messages[0].Content += "{{unknown}}" })
	result, err := runner.Run(context.Background())
	if err == nil || result.Completed || len(result.Errors) == 0 {
		test.Fatalf("unresolved template: %+v, %v", result, err)
	}
	second := newTestRunner(test, input, client, nil)
	if runner.history.SessionID == second.history.SessionID {
		test.Fatal("shared session")
	}
	if err := os.Remove(second.store.path); err != nil {
		test.Fatal(err)
	}
	outcome, err := second.dispatch(context.Background(), "file_read", `{"file_path":"main.go"}`)
	if err == nil || outcome.Action != llmloop.Fail || len(second.store.items) != 0 {
		test.Fatalf("persistence failure: %+v, %v", outcome, err)
	}
	store, err := newObservationStore(test.TempDir(), "comparison")
	if err != nil {
		test.Fatal(err)
	}
	if _, err := newObservationStore(filepath.Dir(store.path), "comparison"); err == nil {
		test.Fatal("observation file overwritten")
	}
	if _, err := store.append(Observation{Arguments: map[string]any{"invalid": make(chan int)}}); err == nil {
		test.Fatal("invalid observation persisted")
	}
}
