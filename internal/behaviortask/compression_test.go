// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestCompressionKeepsObservationReplay(test *testing.T) {
	input := fixture(test)
	if err := os.WriteFile(filepath.Join(input.RepoDir, "large.go"), []byte(strings.Repeat("sample evidence ", 10000)), 0600); err != nil {
		test.Fatal(err)
	}
	git(test, input.RepoDir, "add", ".")
	git(test, input.RepoDir, "commit", "-qm", "Compression fixture")
	input.Resolution.ResolvedHead = git(test, input.RepoDir, "rev-parse", "HEAD")
	var compressions atomic.Int32
	requests := 0
	client := fakeClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		if len(request.Tools) == 0 {
			compressions.Add(1)
			if !strings.Contains(request.Messages[0].ExtractText(), "Behavior/Impact") || strings.Contains(request.Messages[1].ExtractText(), "{{context}}") {
				test.Error("wrong compression template")
			}
			summary := "Read new large.go in obs-000001; retrieve the recorded observation before citing it."
			return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}, nil
		}
		requests++
		switch requests {
		case 1:
			return response("file_read", `{"file_path":"large.go"}`), nil
		case 2:
			if !strings.Contains(request.Messages[1].ExtractText(), "<previous_behavior_summary>") {
				test.Error("summary not preserved")
			}
			return response("context_read", `{"kind":"observation","id":"obs-000001","max_bytes":1024}`), nil
		default:
			if !strings.Contains(request.Messages[len(request.Messages)-1].ExtractText(), "obs-000001") {
				test.Error("observation unavailable after compression")
			}
			analysis := Analysis{Markdown: "Investigation incomplete; observed source [[obs-000001]]. No tests executed.", DeclaredCompletion: "partial"}
			return response("submit_analysis", marshal(test, analysis)), nil
		}
	})
	runner := newTestRunner(test, input, client, func(deps *Deps) { deps.Template.MaxTokens = 15000 })
	result, err := runner.Run(context.Background())
	if err != nil || !result.Completed || compressions.Load() < 1 || len(result.ObservationIDs) != 1 {
		test.Fatalf("compression run: %+v, %v; compressions=%d", result, err, compressions.Load())
	}
}
