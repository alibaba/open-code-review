// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"slices"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

// TestTaskTypesMatchLLMValidTaskModels guards the duplicated task-id list:
// internal/llm validates the config file's task_models keys against its own
// list (llm cannot import session), so the two sets must stay in lockstep or a
// renamed TaskType would silently become unconfigurable (or a typo'd id
// unexplainable).
func TestTaskTypesMatchLLMValidTaskModels(t *testing.T) {
	sessionIDs := map[string]struct{}{}
	for _, id := range []TaskType{PlanTask, MainTask, MemoryCompressionTask, ReLocationTask, ReviewFilterTask, GroupingTask} {
		sessionIDs[string(id)] = struct{}{}
	}
	valid := llm.ValidTaskModels()
	if len(valid) != len(sessionIDs) {
		t.Fatalf("llm.ValidTaskModels has %d ids, session has %d: %v", len(valid), len(sessionIDs), valid)
	}
	for _, id := range valid {
		if _, ok := sessionIDs[id]; !ok {
			t.Fatalf("llm accepts %q but session has no such TaskType", id)
		}
	}
	for id := range sessionIDs {
		if !slices.Contains(valid, id) {
			t.Fatalf("session TaskType %q is missing from llm.ValidTaskModels", id)
		}
	}
}
