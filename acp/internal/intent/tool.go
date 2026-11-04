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
        "commit": {"type": "string", "description": "Single commit SHA or Git ref. Use HEAD for the latest/most recent commit of the current checkout; HEAD~1 for the commit before it. Preserve an explicitly named ref."}
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
    "question": {"type": "string", "description": "Required for clarify. Write in the language of the current user request, unless the user explicitly requests another response language."},
    "missing": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Slot names still missing, for clarify."
    },
    "reason": {"type": "string", "description": "Required for reject. Write in the language of the current user request, unless the user explicitly requests another response language."},
    "hint": {"type": "string", "description": "Actionable next step, for reject. Use the same response language as reason; preserve literal CLI commands and flags."}
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
- Write user-facing question, reason, and hint in the language of the current user request. Honor an explicit request for a different response language. For mixed-language requests, use the language of the surrounding prose, not code, paths, or CLI flags. Do not default to English because these instructions or previous pending slots are in English.
- Keep schema keys, action values, missing slot names, CLI commands, flags, refs, and paths unchanged. Response language changes presentation only, never the parsed operation or arguments.
- Greetings and unrelated conversation are out of scope: give a brief, friendly reason and a useful /review or /scan hint in the user's language; do not start a review or switch to general chat.
- Submit executable intent through the tool fields, never as a shell command. Never invent flags. Only the flags listed in the extra field description are allowed.
- If user-facing guidance includes a slash command, use /review --commit <ref> for one commit, /review --from <ref> --to <ref> for a range, or /review for workspace changes. There is no /review commit subcommand.
- review workspace: review the current change set (staged, unstaged and untracked). This is the default for "review my changes".
- review range: compare two refs. Both "from" and "to" are required.
- review commit: review one commit. "commit" is required.
- "latest commit", "most recent commit", and "last commit" without another target mean HEAD, the current checkout's latest commit, including a detached HEAD. Do not ask for a SHA when this target is already clear. These requests select a single commit, not workspace changes or a range. Apply this meaning in every user language.
- The commit before the latest one is HEAD~1. If the user names a branch or another ref, preserve that target instead of substituting HEAD. Preserve ref punctuation such as ~ and ^. The adapter validates whether refs exist; do not invent hashes or fetch remote refs.
- "Review a commit" without a ref or a clear relative target still requires clarification. Do not infer HEAD for an unspecified commit, author, or remote target.
- scan: scan source files. "paths" are relative to the scan root; an empty list means the whole root.
- If a required value is missing or ambiguous, use "clarify" with a specific question and list the missing slot names.
- If the request is out of scope (auto-fixing code, general questions, staging only, or anything the CLI cannot do), use "reject" with a short reason and an actionable hint.
- The CLI cannot review only the staged area. Reject that with a hint to use /review or git add first.

Examples (the same meanings apply in other languages):
- Review the latest commit -> {"action":"review","review":{"type":"commit","commit":"HEAD"}}. Equivalent slash command: /review --commit HEAD.
- Review the commit before the latest one -> {"action":"review","review":{"type":"commit","commit":"HEAD~1"}}.
- Review the latest commit on feature -> {"action":"review","review":{"type":"commit","commit":"feature"}}.
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
