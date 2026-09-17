// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

func TestToolCallIDsAcrossProcessRestarts(t *testing.T) {
	if output := os.Getenv("OCR_TEST_TOOL_IDS_OUTPUT"); output != "" {
		root := t.TempDir()
		recorder := &findingRecorder{}
		agent := NewAgent("ocr", fakeRunner{})
		agent.SetAgentConnection(recorder)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for range 2 {
			if _, err := agent.collectProgress(ctx, cancel, "s-1", orchestrator.Request{CWD: root, Args: []string{"review"}}); err != nil {
				t.Fatal(err)
			}
		}
		data, err := json.Marshal(recorder.updates)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, data, 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[acp.ToolCallId]bool)
	for range 2 {
		output := filepath.Join(t.TempDir(), "notifications.json")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestToolCallIDsAcrossProcessRestarts$")
		cmd.Env = append(os.Environ(), "OCR_TEST_TOOL_IDS_OUTPUT="+output)
		result, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("notification helper: %v: %s", err, result)
		}
		data, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		var notices []acp.SessionNotification
		if err := json.Unmarshal(data, &notices); err != nil {
			t.Fatal(err)
		}
		active := make(map[acp.ToolCallId]bool)
		starts, finishes := 0, 0
		for _, notice := range notices {
			if start := notice.Update.ToolCall; start != nil {
				if start.ToolCallId == "" || seen[start.ToolCallId] {
					t.Errorf("tool call ID reused across calls or process restarts: %q", start.ToolCallId)
				}
				seen[start.ToolCallId] = true
				if start.Status == acp.ToolCallStatusInProgress {
					active[start.ToolCallId] = true
					starts++
				}
			}
			if update := notice.Update.ToolCallUpdate; update != nil {
				if !active[update.ToolCallId] {
					t.Fatalf("update has no active tool call: %q", update.ToolCallId)
				}
				if update.Status != nil && *update.Status == acp.ToolCallStatusCompleted {
					delete(active, update.ToolCallId)
					finishes++
				}
			}
		}
		if starts != 2 || finishes != 2 || len(active) != 0 {
			t.Fatalf("incomplete notifications: starts=%d finishes=%d active=%v", starts, finishes, active)
		}
	}
}
