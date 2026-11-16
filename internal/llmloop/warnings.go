// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

// AgentWarning describes a non-fatal warning recorded during one subtask's
// review/scan. The name is kept for backwards compatibility with the
// previous internal/agent package.
type AgentWarning struct {
	File    string `json:"file"`
	Message string `json:"message"`
	Type    string `json:"type"`
}
