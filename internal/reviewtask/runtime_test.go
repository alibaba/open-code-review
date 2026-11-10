// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package reviewtask

import (
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
)

type AgentWarning = llmloop.AgentWarning
type MainLoopStop = llmloop.MainLoopStop

const (
	StopNone        = llmloop.StopNone
	StopMaxRounds   = llmloop.StopMaxRounds
	StopEmptyRounds = llmloop.StopEmptyRounds
	StopCompression = llmloop.StopCompression
	StopTokenBudget = llmloop.StopTokenBudget
)

func msg(role, content string) llm.Message { return llm.NewTextMessage(role, content) }
