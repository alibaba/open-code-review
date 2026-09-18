// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

/**
 * pi coding agent extension for OpenCodeReview (`ocr`).
 *
 * Registers two slash commands and two LLM-callable tools:
 *
 * Commands:
 *   /ocr-review           — run a full OCR review; pi presents the findings
 *   /ocr-review-delegate  — OCR selects files and rules, pi's model performs
 *                           the review itself (delegation mode, no OCR LLM)
 *
 * Tools:
 *   ocr_review            — run `ocr review` on workspace changes, a commit,
 *                           or a ref range; returns structured line-level
 *                           findings as JSON
 *   ocr_delegate          — run `ocr delegate preview|rule` for delegation mode
 *
 * The extension has zero runtime dependencies: the only non-Node import is a
 * type-only import of the extension API, which pi erases when loading this
 * file, so the package installs without a build step or node_modules.
 */

import type { ExtensionAPI, ToolDefinition } from "@earendil-works/pi-coding-agent"
import { mkdtemp, readFile, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"

const OCR_NOT_INSTALLED =
  "OpenCodeReview is not installed or 'ocr' is not on PATH. " +
  "Install it with: npm install -g @alibaba-group/open-code-review"

const OCR_TOO_OLD =
  "The installed 'ocr' CLI does not support the flags this extension needs " +
  "(--output/--format/--audience). Upgrade with: npm i -g @alibaba-group/open-code-review@latest"

const REVIEW_TIMEOUT_MS = 30 * 60 * 1000
const DELEGATE_TIMEOUT_MS = 60 * 1000

// pi truncates tool output around 50 KB / 2000 lines. Keep raw JSON below that
// threshold; larger outputs stay on disk where the model can read them in full.
const MAX_INLINE_JSON_CHARS = 45_000

// ---------------------------------------------------------------------------
// ocr CLI subprocess plumbing
// ---------------------------------------------------------------------------

interface RunOptions {
  cwd: string
  timeoutMs: number
  signal?: AbortSignal | undefined
}

interface RunResult {
  stdout: string
  stderr: string
}

async function runOcr(pi: ExtensionAPI, args: readonly string[], options: RunOptions): Promise<RunResult> {
  // Assembled conditionally so an absent signal is never passed as an explicit
  // undefined (exactOptionalPropertyTypes).
  const execOptions: { cwd: string; timeout: number; signal?: AbortSignal } = {
    cwd: options.cwd,
    timeout: options.timeoutMs,
  }
  if (options.signal !== undefined) {
    execOptions.signal = options.signal
  }
  let result: { stdout: string; stderr: string; code: number }
  try {
    result = await pi.exec("ocr", [...args], execOptions)
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    if (/ENOENT|not found/i.test(message)) {
      throw new Error(OCR_NOT_INSTALLED)
    }
    throw new Error(`Failed to start 'ocr ${args[0] ?? ""}': ${message}`)
  }
  if (result.code === 127 || /command not found/.test(result.stderr)) {
    throw new Error(OCR_NOT_INSTALLED)
  }
  if (/unknown flag: --(output|format|audience)/.test(result.stderr)) {
    throw new Error(OCR_TOO_OLD)
  }
  if (result.code !== 0) {
    const detail = result.stderr || result.stdout || `exit code ${result.code}`
    throw new Error(`'ocr ${args[0] ?? ""}' failed: ${detail}`)
  }
  return { stdout: result.stdout, stderr: result.stderr }
}

function pushFlag(args: string[], flag: string, value: string | number | undefined): void {
  if (value !== undefined && value !== "") {
    args.push(flag, String(value))
  }
}

function textResult(text: string): { content: Array<{ type: "text"; text: string }>; details: Record<string, never> } {
  return { content: [{ type: "text", text }], details: {} }
}

// ---------------------------------------------------------------------------
// Full review mode (`ocr review`)
// ---------------------------------------------------------------------------

export interface ReviewParams {
  commit?: string
  from?: string
  to?: string
  resume?: string
  background?: string
  backgroundFile?: string
  exclude?: string
  model?: string
  repo?: string
  concurrency?: number
  timeoutMinutes?: number
  maxTools?: number
  noFilter?: boolean
  preview?: boolean
}

/** Returns an error message when the parameter combination is invalid. */
export function validateReviewParams(params: ReviewParams): string | undefined {
  const hasRange = params.from !== undefined || params.to !== undefined
  if (hasRange && (params.from === undefined || params.to === undefined)) {
    return "Both 'from' and 'to' are required for a branch comparison."
  }
  if (params.commit !== undefined && hasRange) {
    return "Use either 'commit' or a 'from'/'to' range, not both."
  }
  if (params.resume !== undefined && (params.commit !== undefined || hasRange)) {
    return "'resume' cannot be combined with 'commit' or a 'from'/'to' range."
  }
  if (params.preview === true && params.resume !== undefined) {
    return "'preview' and 'resume' cannot be used together."
  }
  if (params.background !== undefined && params.backgroundFile !== undefined) {
    return "Use either 'background' or 'backgroundFile', not both."
  }
  return undefined
}

export function buildReviewArgs(params: ReviewParams): string[] {
  const args = ["review", "--audience", "agent"]
  if (params.preview !== true) {
    args.push("--format", "json")
  }
  pushFlag(args, "--repo", params.repo)
  pushFlag(args, "--commit", params.commit)
  pushFlag(args, "--from", params.from)
  pushFlag(args, "--to", params.to)
  pushFlag(args, "--resume", params.resume)
  pushFlag(args, "--background", params.background)
  pushFlag(args, "--background-file", params.backgroundFile)
  pushFlag(args, "--exclude", params.exclude)
  pushFlag(args, "--model", params.model)
  pushFlag(args, "--concurrency", params.concurrency)
  pushFlag(args, "--timeout", params.timeoutMinutes)
  pushFlag(args, "--max-tools", params.maxTools)
  if (params.noFilter === true) {
    args.push("--no-filter")
  }
  if (params.preview === true) {
    args.push("--preview")
  }
  return args
}

export interface OcrComment {
  path: string
  content: string
  suggestion_code?: string
  existing_code?: string
  start_line: number
  end_line: number
  thinking?: string
  category?: string
  severity?: string
}

export interface OcrReviewJson {
  status?: string
  session_id?: string
  comments?: OcrComment[]
  [key: string]: unknown
}

export function parseReviewJson(raw: string): OcrReviewJson {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    throw new Error("OpenCodeReview returned invalid JSON.")
  }
  if (typeof parsed !== "object" || parsed === null || !Array.isArray((parsed as OcrReviewJson).comments)) {
    throw new Error("OpenCodeReview returned unexpected JSON (missing 'comments' array).")
  }
  return parsed as OcrReviewJson
}

/** One-line severity rollup, ordered critical > high > medium > low. */
export function summarizeSeverities(comments: readonly OcrComment[]): string {
  const order = ["critical", "high", "medium", "low"]
  const counts = new Map<string, number>()
  for (const comment of comments) {
    const severity = comment.severity ?? "unspecified"
    counts.set(severity, (counts.get(severity) ?? 0) + 1)
  }
  const parts = order.filter((severity) => counts.has(severity)).map((severity) => `${severity}: ${counts.get(severity)}`)
  for (const [severity, count] of counts) {
    if (!order.includes(severity)) {
      parts.push(`${severity}: ${count}`)
    }
  }
  return parts.length > 0 ? parts.join(", ") : "no comments"
}

async function executeReview(
  pi: ExtensionAPI,
  params: ReviewParams,
  cwd: string,
  signal: AbortSignal | undefined,
): Promise<ReturnType<typeof textResult>> {
  const invalid = validateReviewParams(params)
  if (invalid !== undefined) {
    throw new Error(invalid)
  }

  if (params.preview === true) {
    const result = await runOcr(pi, buildReviewArgs(params), { cwd, timeoutMs: REVIEW_TIMEOUT_MS, signal })
    return textResult(result.stdout || "No files changed.")
  }

  // Write JSON to a temp file instead of stdout so large reviews are never
  // truncated by pipe buffering (the official OCR skill guidance).
  const dir = await mkdtemp(join(tmpdir(), "ocr-pi-"))
  let keepDir = false
  try {
    const outFile = join(dir, "review.json")
    await runOcr(pi, [...buildReviewArgs(params), "--output", outFile], {
      cwd,
      timeoutMs: REVIEW_TIMEOUT_MS,
      signal,
    })
    let raw: string
    try {
      raw = await readFile(outFile, "utf8")
    } catch {
      throw new Error(
        "'ocr review' exited successfully but wrote no output file. " +
          "Upgrade the CLI with: npm i -g @alibaba-group/open-code-review@latest",
      )
    }
    const review = parseReviewJson(raw)
    if (raw.length <= MAX_INLINE_JSON_CHARS) {
      return textResult(raw)
    }
    // Too large for inline tool output: keep the file on disk and point the
    // model at it, mirroring the spill-file convention of pi's builtin tools.
    keepDir = true
    return textResult(
      `The review JSON (${raw.length} characters) is too large for inline tool output.\n` +
        `Severity summary — ${summarizeSeverities(review.comments ?? [])}.\n` +
        `Full output saved at: ${outFile}\n` +
        `Read that file in full before presenting findings.`,
    )
  } finally {
    if (!keepDir) {
      await rm(dir, { recursive: true, force: true })
    }
  }
}

// ---------------------------------------------------------------------------
// Delegation mode (`ocr delegate preview|rule`)
// ---------------------------------------------------------------------------

export type DelegateAction = "preview" | "rule"

export interface DelegateParams {
  action: DelegateAction
  paths?: string[]
  from?: string
  to?: string
  commit?: string
  repo?: string
  exclude?: string
}

export function buildDelegateArgs(params: DelegateParams): string[] {
  const args = ["delegate", params.action, "--format", "json"]
  pushFlag(args, "--repo", params.repo)
  pushFlag(args, "--from", params.from)
  pushFlag(args, "--to", params.to)
  pushFlag(args, "--commit", params.commit)
  pushFlag(args, "--exclude", params.exclude)
  if (params.action === "rule") {
    for (const path of params.paths ?? []) {
      // Models sometimes prefix paths with '@'; the CLI does not expect it.
      args.push(path.replace(/^@/, ""))
    }
  }
  return args
}

async function executeDelegate(
  pi: ExtensionAPI,
  params: DelegateParams,
  cwd: string,
  signal: AbortSignal | undefined,
): Promise<ReturnType<typeof textResult>> {
  if (params.action === "rule" && (params.paths === undefined || params.paths.length === 0)) {
    throw new Error("The 'rule' action requires at least one file path.")
  }
  const result = await runOcr(pi, buildDelegateArgs(params), {
    cwd,
    timeoutMs: DELEGATE_TIMEOUT_MS,
    signal,
  })
  return textResult(result.stdout || "{}")
}

// ---------------------------------------------------------------------------
// Command prompts
// ---------------------------------------------------------------------------

export function buildReviewPrompt(userArgs: string): string {
  const target = userArgs.trim()
  const targetLine =
    target.length > 0
      ? `User arguments (interpret them and map them onto the tool's parameters): ${target}`
      : "No arguments were given: review the current workspace changes (staged, unstaged, and untracked)."
  return [
    "Run a code review with OpenCodeReview using the ocr_review tool.",
    "",
    targetLine,
    "Map business or requirement context mentioned by the user onto the tool's 'background' parameter. Use preview=true only when the user asks which files would be reviewed.",
    "",
    "When the tool returns:",
    "1. Present the findings grouped by severity: critical, then high, then medium. Silently discard low-severity items that look like false positives or nitpicks.",
    "2. Cite every finding as `path:start_line-end_line` with its [category] and the recommendation from the comment.",
    "3. When start_line and end_line are both 0 the comment failed to position itself: read the file and locate the correct section from the comment content before presenting it.",
    "4. If the result mentions an interrupted review and a session id, mention that it can be resumed with the tool's 'resume' parameter.",
    "5. Do not apply any fixes unless the user explicitly asked for 'review and fix'; otherwise ask for permission first.",
  ].join("\n")
}

export function buildDelegatePrompt(userArgs: string): string {
  const target = userArgs.trim()
  const targetLine =
    target.length > 0
      ? `User arguments (map them onto the tool's from/to/commit parameters): ${target}`
      : "No arguments were given: review the current workspace changes (staged, unstaged, and untracked)."
  return [
    "Run a delegated code review: OpenCodeReview deterministically selects the files and the review rules, and you perform the actual review with your own tools. No OCR LLM endpoint is involved.",
    "",
    targetLine,
    "",
    "Workflow:",
    '1. Call the ocr_delegate tool with action "preview" (plus from/to/commit derived from the user arguments). Note the mode, the refs, merge_base when present, and every entry of reviewable_files.',
    '2. Call ocr_delegate with action "rule", passing all reviewable file paths. It returns the review checklist grouped by rule content.',
    "3. Build a checklist of every reviewable file and work through it:",
    "   - Get each file's diff: range mode → `git diff <merge_base>..<to> -- <path>`; commit mode → `git show <commit> -- <path>`; workspace mode → `git diff HEAD -- <path>` for tracked files, and read untracked files directly (the whole file is new code).",
    "   - Review the file against its rule group; use read/grep freely for any extra context you need.",
    "   - Mark the file reviewed, or skipped with a concrete reason. Every file must be accounted for — never silently omit one, and do not stop after the first high-severity finding.",
    "4. Report the findings grouped by severity. Always report critical and high (bugs, security issues, data-loss risks); report medium with context; report low only when clearly valuable. Silently discard likely false positives.",
    "5. Give each finding: path, start_line, end_line, content (what is wrong and how to fix it), category (bug/security/performance/maintainability/test/style/documentation/other), severity (critical/high/medium/low).",
    "6. Include a coverage summary: total_files, reviewed_files, skipped_files (with reasons), coverage_rate.",
    "7. Do not apply any fixes unless the user explicitly asked for 'review and fix'.",
  ].join("\n")
}

// ---------------------------------------------------------------------------
// Tool parameter schemas (plain JSON Schema — TypeBox-compatible, no imports)
// ---------------------------------------------------------------------------

const stringParam = (description: string) => ({ type: "string", description })
const intParam = (description: string) => ({ type: "integer", minimum: 1, description })

const REVIEW_PARAMS_SCHEMA = {
  type: "object",
  properties: {
    commit: stringParam("Review one commit against its parent."),
    from: stringParam("Base ref for a branch/range comparison. Must be paired with 'to'."),
    to: stringParam("Target ref for a range comparison. Must be paired with 'from'."),
    resume: stringParam("Resume a previous OCR review session by ID."),
    background: stringParam(
      "Business or requirement context the implementation should satisfy. Improves review quality; always pass it when known.",
    ),
    backgroundFile: stringParam(
      "Path to a Markdown file holding the review background (sanitized, max 8000 characters). Cannot be combined with 'background'.",
    ),
    exclude: stringParam("Comma-separated gitignore-style exclusion patterns."),
    model: stringParam("Override the OCR-configured model for this run."),
    repo: stringParam("Repository root. Defaults to the current working directory."),
    concurrency: intParam("Maximum concurrent file reviews (default 8). Lower it when hitting provider rate limits."),
    timeoutMinutes: intParam(
      "Per-review-group timeout in minutes (default 15; effective limit is this value times the number of review rounds).",
    ),
    maxTools: intParam("Maximum tool-call rounds per review subtask."),
    noFilter: { type: "boolean", description: "Keep all review comments and skip the LLM post-filtering call." },
    preview: { type: "boolean", description: "List the files that would be reviewed, without calling an LLM." },
  },
  required: [],
  additionalProperties: false,
}

const DELEGATE_PARAMS_SCHEMA = {
  type: "object",
  properties: {
    action: {
      type: "string",
      enum: ["preview", "rule"],
      description:
        "'preview' lists the reviewable files with mode/ref metadata; 'rule' returns the resolved review rules for the given paths, grouped by content.",
    },
    paths: { type: "array", items: { type: "string" }, description: "File paths for the 'rule' action." },
    from: stringParam("Base ref for range mode. Must be paired with 'to'."),
    to: stringParam("Target ref for range mode. Must be paired with 'from'."),
    commit: stringParam("Commit hash for commit mode."),
    repo: stringParam("Repository root. Defaults to the current working directory."),
    exclude: stringParam("Comma-separated gitignore-style exclusion patterns."),
  },
  required: ["action"],
  additionalProperties: false,
}

// ---------------------------------------------------------------------------
// Extension factory
// ---------------------------------------------------------------------------

export default function openCodeReviewExtension(pi: ExtensionAPI): void {
  pi.registerCommand("ocr-review", {
    description: "Review code changes with OpenCodeReview (ocr)",
    handler: async (args: string) => {
      pi.sendUserMessage(buildReviewPrompt(args), { deliverAs: "followUp" })
    },
  })

  pi.registerCommand("ocr-review-delegate", {
    description: "Delegated review: ocr selects files and rules, pi performs the review",
    handler: async (args: string) => {
      pi.sendUserMessage(buildDelegatePrompt(args), { deliverAs: "followUp" })
    },
  })

  const ocrReviewTool: ToolDefinition<any, any> = {
    name: "ocr_review",
    label: "OpenCodeReview",
    description:
      "Run OpenCodeReview (ocr) on the current workspace changes, a single commit, or a ref range. " +
      "Returns structured line-level findings (path, start_line, end_line, severity, category, suggestion). " +
      "Use when the user asks to review code, review a pull request, review staged/unstaged changes, review a commit, or compare branches. " +
      "Use preview=true to list the files that would be reviewed without calling an LLM. " +
      "Requires the ocr CLI and a configured OCR LLM.",
    promptSnippet:
      "Run OpenCodeReview (ocr) code review on workspace changes, a commit, or a ref range; returns structured line-level findings.",
    promptGuidelines: [
      "When the user asks for a code review, a PR review, or a branch comparison, call the ocr_review tool instead of reading the diff and reviewing the files manually.",
    ],
    parameters: REVIEW_PARAMS_SCHEMA,
    async execute(_toolCallId, params, signal, _onUpdate, ctx) {
      return executeReview(pi, params as ReviewParams, ctx.cwd, signal)
    },
  }
  pi.registerTool(ocrReviewTool)

  const ocrDelegateTool: ToolDefinition<any, any> = {
    name: "ocr_delegate",
    label: "OpenCodeReview Delegate",
    description:
      "Delegation mode for OpenCodeReview: 'preview' returns the reviewable file list with mode/refs " +
      "(workspace, range with merge_base, or commit), 'rule' returns the resolved review rules for given paths. " +
      "Use these to plan a code review that you then perform yourself with read/grep/git — no OCR LLM is involved.",
    promptSnippet:
      "Get OpenCodeReview's deterministic file selection and rule resolution for planning your own delegated code review.",
    promptGuidelines: [
      "Before manually reviewing a changeset the user asked you to review yourself, call ocr_delegate with action 'preview' and then 'rule' so no reviewable file is missed.",
    ],
    parameters: DELEGATE_PARAMS_SCHEMA,
    async execute(_toolCallId, params, signal, _onUpdate, ctx) {
      return executeDelegate(pi, params as DelegateParams, ctx.cwd, signal)
    },
  }
  pi.registerTool(ocrDelegateTool)
}
