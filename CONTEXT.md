# Open Code Review Agent Integration

This context defines how the local Open Code Review CLI is exposed to coding-agent hosts.

## Language

**OCR runner**:
The local `ocr` CLI process that analyzes Git changes and emits structured review findings.
_Avoid_: using `ocr_review` to mean the CLI process.

**Codex review tool**:
The Codex-callable capability that invokes the OCR runner through a local `stdio` MCP server and returns one terminal result.
_Avoid_: session polling, OpenCode tool.

**OpenCode integration**:
The separate plugin under `plugins/open-code-review/opencode/` that registers tools for OpenCode. It does not register tools in Codex.
_Avoid_: treating the OpenCode integration as the Codex integration.

**Review result**:
The structured JSON returned after the OCR runner finishes, fails, is cancelled, or reaches its timeout.
_Avoid_: treating an intermediate progress update as a review result.

**Resume session**:
A persisted OCR review state identified by a session ID that can reuse completed file-level checkpoints for a commit or ref-range review.
_Avoid_: resuming a mutable workspace review or assuming an unfinished file is reusable.

**Progress event**:
A machine-readable JSONL record written to stderr while OCR runs; it signals review activity without mixing with the final JSON result on stdout.
_Avoid_: treating progress events as the terminal review result.
