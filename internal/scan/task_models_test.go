// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestArgsModelForTask(t *testing.T) {
	a := Args{Model: "run-model"}
	if got := a.ModelForTask(session.PlanTask); got != "run-model" {
		t.Fatalf("nil map: got %q, want run model", got)
	}
	a.TaskModels = llm.TaskModels{"plan_task": "cheap-plan"}
	if got := a.ModelForTask(session.PlanTask); got != "cheap-plan" {
		t.Fatalf("hit: got %q, want override", got)
	}
	// Scan dedup/summary reuse the memory_compression_task id — that key must
	// therefore steer scan's aux tasks too.
	if got := a.ModelForTask(session.MemoryCompressionTask); got != "run-model" {
		t.Fatalf("missing key: got %q, want run model", got)
	}
}
