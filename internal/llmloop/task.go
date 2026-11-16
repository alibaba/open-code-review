// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

type ToolAction int

const (
	Continue ToolAction = iota
	Complete
	Fail
)

type ToolOutcome struct {
	Content string
	Action  ToolAction
}

type ToolExecutor func(context.Context, string, llm.ToolCall, *session.TaskRecord, string) ToolOutcome

type ToolCallResult struct {
	ToolCallID string
	Name       string
	Result     string
}

type MessageTemplate struct {
	Role    string
	Content string
}

type CompressionPolicy struct {
	Messages        []MessageTemplate
	SummaryTemplate string
}

type GracePolicy struct {
	Messages []llm.Message
	Tools    []llm.ToolDef
}

type Task struct {
	ToolDefs             []llm.ToolDef
	ExecuteTool          ToolExecutor
	MaxRounds            int
	MaxContextTokens     int
	MaxCompletionTokens  int
	EmptyResponseMessage string
	Compression          CompressionPolicy
	Grace                GracePolicy
}

type RunResult struct {
	Completed  bool
	StopReason MainLoopStop
}
