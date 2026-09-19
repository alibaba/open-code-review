// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package contract

import (
	"encoding/json"
	"testing"
)

func TestOptionalResultUsage(t *testing.T) {
	for _, raw := range []string{
		`{"status":"success","summary":{"files_reviewed":2,"comments":0},"comments":[]}`,
		`{"status":"success","summary":{"files_reviewed":2,"comments":0,"input_tokens":0,"output_tokens":3,"cache_read_tokens":4,"cache_write_tokens":5,"budget_exceeded":true},"tool_calls":{"total":2,"by_tool":{"read":2}},"comments":[]}`,
	} {
		var review ReviewResult
		var scan ScanResult
		if err := json.Unmarshal([]byte(raw), &review); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(raw), &scan); err != nil {
			t.Fatal(err)
		}
		for _, summary := range []*Summary{review.Summary, scan.Summary} {
			if review.ToolCalls == nil {
				if summary.InputTokens != nil || summary.OutputTokens != nil || summary.BudgetExceeded {
					t.Fatalf("invented legacy usage: %+v", summary)
				}
			} else if summary.InputTokens == nil || *summary.InputTokens != 0 || summary.OutputTokens == nil || *summary.OutputTokens != 3 || summary.CacheReadTokens != 4 || summary.CacheWriteTokens != 5 || !summary.BudgetExceeded {
				t.Fatalf("lost usage: %+v", summary)
			}
		}
		if review.ToolCalls != nil && (scan.ToolCalls == nil || review.ToolCalls.Total != 2 || scan.ToolCalls.ByTool["read"] != 2) {
			t.Fatalf("lost tool calls: %+v %+v", review.ToolCalls, scan.ToolCalls)
		}
	}
}
