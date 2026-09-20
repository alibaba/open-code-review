// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import "strings"

// Reasoning models (e.g. MiniMax-M3) emit a "<think>...</think>" rationale
// block before the actual tool-call JSON. The block's prose regularly contains
// braces, so extractTopLevelJSON locks onto the first "{" inside the thinking
// text and either fails to balance or extracts a prose fragment that fails
// Unmarshal — the whole tool call then dies with "Error: 'comments' array is
// required. Got args: {}".
//
// stripThinkBlocks removes every balanced "<think>...</think>" span before
// parseToolArgs runs. An unclosed "<think>" (truncated reasoning) is left in
// place: with no "</think>" there is no reliable boundary, and the caller's
// existing error path is the honest outcome.
func stripThinkBlocks(raw string) string {
	const (
		openTag  = "<think>"
		closeTag = "</think>"
	)
	for {
		start := strings.Index(raw, openTag)
		if start < 0 {
			return raw
		}
		rel := strings.Index(raw[start:], closeTag)
		if rel < 0 {
			return raw
		}
		end := start + rel + len(closeTag)
		raw = raw[:start] + raw[end:]
	}
}
