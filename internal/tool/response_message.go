// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

// ToolCallResult holds a single tool call and its execution result.
type ToolCallResult struct {
	ToolCallID string // OpenAI-compatible tool call ID
	Name       string // tool name (alias)
	Result     string // output from the tool
}

// TaskCheckpoint signals terminal completion or failure, or carries data back to the LLM.
type TaskCheckpoint struct {
	Data      string
	Completed bool
	Failed    bool
}

// Complete returns a checkpoint signaling task completion.
func Complete() TaskCheckpoint { return TaskCheckpoint{Completed: true} }

// Fail returns a checkpoint signaling terminal task failure.
func Fail(data string) TaskCheckpoint { return TaskCheckpoint{Data: data, Failed: true} }

// Of returns a checkpoint with data.
func Of(data string) TaskCheckpoint { return TaskCheckpoint{Data: data, Completed: false} }

const CommentSucceed = "Successfully commented."
const ToolNotFoundMsg = "Error: Tool not found. The tool you attempted to call does not exist or is not available. Please check the tool name and try again with a valid tool."
