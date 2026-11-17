// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestGroupDiffs_CollidingKeys(t *testing.T) {
	for _, tc := range []struct {
		name       string
		paths      []string
		response   string
		tokenLimit int
	}{
		{
			name:     "group and literal comma path",
			paths:    []string{"a.go", "b.go", "a.go,b.go"},
			response: `[{"label":"pair","files":[0,1]},{"label":"literal","files":[2]}]`,
		},
		{
			name:     "two multi-file groups",
			paths:    []string{"a.go", "b.go,c.go", "a.go,b.go", "c.go"},
			response: `[{"label":"first","files":[0,1]},{"label":"second","files":[2,3]}]`,
		},
		{
			name:     "splitting a collision would collide with a third group",
			paths:    []string{"a.go,b.go", "c.go", "a.go,b.go,c.go", "a.go", "b.go"},
			response: `[{"label":"first","files":[0,1]},{"label":"second","files":[2]},{"label":"third","files":[3,4]}]`,
		},
		{
			name:       "token splitting creates a collision",
			paths:      []string{"a.go", "b.go", "a.go,b.go", "large.go"},
			response:   `[{"label":"small","files":[0,1]},{"label":"large","files":[2,3]}]`,
			tokenLimit: 100,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diffs := make([]model.Diff, len(tc.paths))
			for i, path := range tc.paths {
				diffs[i] = model.Diff{NewPath: path, Diff: "+x", Insertions: 1}
			}
			if tc.tokenLimit > 0 {
				diffs[len(diffs)-1].Diff = strings.Repeat("word ", 2000)
			}
			before := append([]model.Diff(nil), diffs...)
			client := &fakeGroupingClient{response: tc.response}
			result := groupDiffs(context.Background(), diffs, client, "fake", groupingSkipTemplate(0, 0), tc.tokenLimit, nil)
			if !client.called || result.usage == nil || result.usage.TotalTokens != 100 {
				t.Fatal("the attempted grouping call and its usage must be retained")
			}
			if !reflect.DeepEqual(result.groups, toSingleFileGroups(diffs)) {
				t.Errorf("ambiguous grouping dispatched %d groups instead of %d individual files", len(result.groups), len(diffs))
			}
			if !reflect.DeepEqual(diffs, before) {
				t.Fatal("grouping mutated the input diffs")
			}
		})
	}
}

func TestGroupDiffs_CollisionAfterFileCountSplit(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(fmt.Sprintf("local=%t", local), func(t *testing.T) {
			var diffs []model.Diff
			var paths []string
			var indices []int
			for i := 0; i < maxFilesPerGroup; i++ {
				path := fmt.Sprintf("f%02d.go", i)
				paths = append(paths, path)
				diffs = append(diffs, model.Diff{NewPath: path, Diff: "+x", Insertions: 1})
				indices = append(indices, i)
			}
			diffs = append(diffs, model.Diff{NewPath: strings.Join(paths, ","), Diff: "+y", Insertions: 1})
			indices = append(indices, len(diffs)-1)
			response, err := json.Marshal([]groupingResponse{{Label: "all", Files: indices}})
			if err != nil {
				t.Fatal(err)
			}
			client := &fakeGroupingClient{response: string(response)}
			tpl := groupingSkipTemplate(0, 0)
			if local {
				tpl = groupingSkipTemplate(len(diffs)+1, 200)
			}
			result := groupDiffs(context.Background(), diffs, client, "fake", tpl, 0, nil)
			if client.called == local {
				t.Fatalf("grouping call made=%t, local=%t", client.called, local)
			}
			if !reflect.DeepEqual(result.groups, toSingleFileGroups(diffs)) {
				t.Errorf("file-count splitting left %d groups instead of %d individual files", len(result.groups), len(diffs))
			}
		})
	}
}

func TestGroupDiffs_DistinctCommaPathsKeepTheirPartition(t *testing.T) {
	diffs := []model.Diff{{NewPath: "a.go,b.go"}, {NewPath: "c.go"}, {NewPath: "d.go"}}
	client := &fakeGroupingClient{response: `[{"label":"literal","files":[0]},{"label":"pair","files":[2,1]}]`}
	result := groupDiffs(context.Background(), diffs, client, "fake", groupingSkipTemplate(0, 0), 0, nil)
	want := []FileGroup{
		{Label: "literal", Diffs: diffs[:1]},
		{Label: "pair", Diffs: []model.Diff{diffs[2], diffs[1]}},
	}
	if !reflect.DeepEqual(result.groups, want) {
		t.Fatalf("non-colliding groups changed: %+v", result.groups)
	}
	if fileGroupKey(result.groups[0].Diffs) != "a.go,b.go" || fileGroupKey(result.groups[1].Diffs) != "c.go,d.go" {
		t.Fatal("existing task-key spellings changed")
	}
}

type collisionGroupingClient struct {
	mu    sync.Mutex
	metas []llm.RequestMeta
}

func (c *collisionGroupingClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	meta, ok := llm.RequestMetaFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("request has no session identity")
	}
	if meta.TaskType != string(session.GroupingTask) {
		c.mu.Lock()
		c.metas = append(c.metas, meta)
		c.mu.Unlock()
		return agentTaskDoneResponse(), nil
	}
	var prompt string
	for _, message := range req.Messages {
		if text, ok := message.Content.(string); ok {
			prompt += text
		}
	}
	indices := make(map[string]int)
	for _, path := range []string{"a.go", "b.go", "a.go,b.go"} {
		for i := 0; i < 3; i++ {
			for _, line := range strings.Split(prompt, "\n") {
				if strings.HasPrefix(line, fmt.Sprintf("[%d] ", i)) && strings.Contains(line, " "+path+" (") {
					indices[path] = i
				}
			}
		}
	}
	if len(indices) != 3 {
		return nil, fmt.Errorf("grouping prompt omitted a workspace path: %s", prompt)
	}
	response, err := json.Marshal([]groupingResponse{
		{Label: "pair", Files: []int{indices["a.go"], indices["b.go"]}},
		{Label: "literal", Files: []int{indices["a.go,b.go"]}},
	})
	if err != nil {
		return nil, err
	}
	content := string(response)
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
		Usage:   &llm.UsageInfo{PromptTokens: 1, CompletionTokens: 1},
	}, nil
}

func TestRun_CollidingGroupsKeepSeparateSessionRecords(t *testing.T) {
	dir := initPreviewRepo(t)
	paths := []string{"a.go", "b.go", "a.go,b.go"}
	for _, path := range paths {
		if err := os.WriteFile(filepath.Join(dir, path), []byte("package fixture\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	client := &collisionGroupingClient{}
	tpl := groupingSkipTemplate(0, 0)
	tpl.MaxTokens = 100000
	tpl.MaxToolRequestTimes = 5
	tpl.MainTask = template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}}}
	a := New(Args{
		RepoDir: dir, LLMClient: client, Model: "fake", Template: tpl, MaxConcurrency: 3,
		MainToolDefs: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "task_done"}}},
	})
	t.Cleanup(func() { _ = a.Session().Finalize() })
	if _, err := a.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(a.FileGroups()) != len(paths) {
		t.Errorf("dispatched %d groups for colliding paths, want %d", len(a.FileGroups()), len(paths))
	}
	client.mu.Lock()
	metas := append([]llm.RequestMeta(nil), client.metas...)
	client.mu.Unlock()
	if len(metas) != len(paths) {
		t.Errorf("main requests = %d, want %d", len(metas), len(paths))
	}
	seen := make(map[string]int)
	for _, meta := range metas {
		seen[meta.FilePath]++
		if meta.RequestNo != 1 || meta.TaskType != string(session.MainTask) {
			t.Errorf("a conversation reused another group's identity: %+v", meta)
		}
	}
	for _, path := range paths {
		if seen[path] != 1 {
			t.Errorf("main requests for %q = %d, want 1", path, seen[path])
		}
	}
	manifest := a.RunManifest()
	if manifest == nil || manifest.TerminalState != session.StateComplete || len(manifest.Coverage.Completed) != len(paths) {
		t.Fatalf("review coverage changed: %+v", manifest)
	}
	sessionPath, err := session.SessionFilePath(dir, a.SessionID())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	recorded := make(map[string]int)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record struct {
			Type      string `json:"type"`
			FilePath  string `json:"filePath"`
			TaskType  string `json:"taskType"`
			RequestNo int    `json:"request_no"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record.Type == "llm_request" && record.TaskType == string(session.MainTask) {
			recorded[record.FilePath]++
			if record.RequestNo != 1 {
				t.Errorf("persisted task reused a conversation: %+v", record)
			}
		}
	}
	if !reflect.DeepEqual(recorded, map[string]int{"a.go": 1, "b.go": 1, "a.go,b.go": 1}) {
		t.Errorf("persisted main-task records share group identity: %v", recorded)
	}
}
