// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"encoding/json"
	"strings"
)

const submitIntentToolName = "submit_intent"

// rawIntent mirrors the tool schema. The decoder uses DisallowUnknownFields, so
// a model that invents a field fails the parse instead of smuggling a value in.
type rawIntent struct {
	Action   string     `json:"action"`
	Review   *rawReview `json:"review"`
	Scan     *rawScan   `json:"scan"`
	Extra    []string   `json:"extra"`
	Question string     `json:"question"`
	Missing  []string   `json:"missing"`
	Reason   string     `json:"reason"`
	Hint     string     `json:"hint"`
}

type rawReview struct {
	Type   string `json:"type"`
	From   string `json:"from"`
	To     string `json:"to"`
	Commit string `json:"commit"`
}

type rawScan struct {
	Paths []string `json:"paths"`
}

const submitIntentDescription = "Submit exactly one parsed code-review request for the OpenCodeReview CLI."

// submitIntentParameters is the JSON Schema the model must satisfy. Adapter-owned
// fields (binary path, output format, audience, color, repo, rule, model, and
// provider) are absent by construction, which is the first line of defense.
const submitIntentParameters = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "action": {
      "type": "string",
      "enum": ["review", "scan", "clarify", "reject"],
      "description": "review or scan to request work; clarify when a value is missing; reject when the request is out of scope."
    },
    "review": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "type": {
          "type": "string",
          "enum": ["workspace", "range", "commit"],
          "description": "workspace reviews current changes; range compares two refs; commit reviews one commit."
        },
        "from": {"type": "string", "description": "Base ref for range."},
        "to": {"type": "string", "description": "Head ref for range."},
        "commit": {"type": "string", "description": "Single commit SHA or ref for commit."}
      },
      "required": ["type"]
    },
    "scan": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "paths": {
          "type": "array",
          "items": {"type": "string"},
          "description": "Relative paths to scan. Omit or leave empty to scan the whole root."
        }
      }
    },
    "extra": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional review/scan flags. review: --effort, --no-filter, --background, --background-file. scan: --batch, --no-plan, --no-dedup, --no-summary, --background. Do not invent flags."
    },
    "question": {"type": "string", "description": "Required for clarify."},
    "missing": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Slot names still missing, for clarify."
    },
    "reason": {"type": "string", "description": "Required for reject."},
    "hint": {"type": "string", "description": "Actionable next step, for reject."}
  },
  "required": ["action"]
}
`

// SubmitIntentTool returns the tool definition sent to the parsing model.
func SubmitIntentTool() ToolSchema {
	return ToolSchema{
		Name:        submitIntentToolName,
		Description: submitIntentDescription,
		Parameters:  []byte(submitIntentParameters),
	}
}

const systemPrompt = `You convert one user request into exactly one submit_intent tool call for the OpenCodeReview CLI.

Rules:
- Never emit shell commands, and never invent flags. Only the flags listed in the extra field description are allowed.
- review workspace: review the current change set (staged, unstaged and untracked). This is the default for "review my changes".
- review range: compare two refs. Both "from" and "to" are required.
- review commit: review one commit. "commit" is required.
- scan: scan source files. "paths" are relative to the scan root; an empty list means the whole root.
- If a required value is missing or ambiguous, use "clarify" with a specific question and list the missing slot names.
- If the request is out of scope (auto-fixing code, general questions, staging only, or anything the CLI cannot do), use "reject" with a short reason and an actionable hint.
- The CLI cannot review only the staged area. Reject that with a hint to use /review or git add first.
`

func buildUserPrompt(text string, st *State) string {
	var b strings.Builder
	b.WriteString("User request:\n")
	b.WriteString(text)
	if st != nil {
		if pend := st.Pending(); pend != nil {
			if encoded, err := json.Marshal(pend); err == nil {
				b.WriteString("\n\nUnfinished request from the previous turn. These slots are already known:\n")
				b.Write(encoded)
				b.WriteString("\nMerge the new request into these slots and submit the completed intent.")
			}
		}
	}
	return b.String()
}
