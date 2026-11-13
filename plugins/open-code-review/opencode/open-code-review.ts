// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { spawn } from "node:child_process"
import { chmod, mkdtemp, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { type Plugin, tool } from "@opencode-ai/plugin"
import type { Plugin as PluginV2 } from "@opencode/plugin"

// Retry cleanup long enough for transient file locks, such as antivirus scans, to clear.
const backgroundCleanupMaxRetries = 20
const backgroundCleanupRetryDelayMs = 50

interface ReviewInput {
  commit?: string
  from?: string
  to?: string
  resume?: string
  background?: string
  backgroundFile?: string
  exclude?: string
  model?: string
  concurrency?: number
  timeoutMinutes?: number
  overallTimeoutMinutes?: number
  maxTools?: number
  maxGitProcesses?: number
  preview?: boolean
}

interface DelegatePreviewInput {
  commit?: string
  from?: string
  to?: string
  background?: string
  backgroundFile?: string
  exclude?: string
  rule?: string
  maxGitProcesses?: number
  timeoutSeconds?: number
}

interface DelegateRuleInput {
  paths: string[] | string
  commit?: string
  from?: string
  to?: string
  rule?: string
  maxGitProcesses?: number
  timeoutSeconds?: number
}

interface OcrInvocation {
  command: string
  prefixArgs: string[]
}

interface RunOptions {
  cwd: string
  timeoutMs?: number | null
  maxOutputBytes?: number
  invocation?: OcrInvocation
  signal?: AbortSignal
}

interface RunResult {
  stdout: string
  stderr: string
  exitCode: number | null
  signal: NodeJS.Signals | null
}

class OcrExecutionError extends Error {
  readonly exitCode: number | null
  readonly signal: NodeJS.Signals | null
  readonly stderr: string
  readonly stdout: string

  constructor(message: string, result: {
    exitCode: number | null
    signal?: NodeJS.Signals | null
    stderr?: string
    stdout?: string
  }) {
    super(message)
    this.name = "OcrExecutionError"
    this.exitCode = result.exitCode
    this.signal = result.signal ?? null
    this.stderr = result.stderr ?? ""
    this.stdout = result.stdout ?? ""
  }
}

function pushValue(args: string[], flag: string, value: string | number | undefined): void {
  if (value !== undefined && value !== "") {
    args.push(flag, String(value))
  }
}

function buildReviewArgs(input: ReviewInput, repo: string, backgroundFile?: string): string[] {
  const hasRange = input.from !== undefined || input.to !== undefined
  if (hasRange && (!input.from || !input.to)) {
    throw new Error("Both 'from' and 'to' are required for a branch comparison.")
  }
  if (input.commit && hasRange) {
    throw new Error("Use either 'commit' or a 'from'/'to' range, not both.")
  }
  if (input.resume && (input.commit || hasRange)) {
    throw new Error("'resume' cannot be combined with 'commit' or a 'from'/'to' range.")
  }
  if (input.preview && input.resume) {
    throw new Error("'preview' and 'resume' cannot be used together.")
  }
  // OCR warns on stderr and lets the file win, but formatReviewResult drops
  // stderr on exit 0, so the caller would never see it.
  if (input.background && input.backgroundFile) {
    throw new Error("Use either 'background' or 'backgroundFile', not both.")
  }

  const args = ["review", "--audience", "agent"]
  if (!input.preview) {
    args.push("--format", "json")
  }
  args.push("--repo", repo)

  pushValue(args, "--commit", input.commit)
  pushValue(args, "--from", input.from)
  pushValue(args, "--to", input.to)
  pushValue(args, "--resume", input.resume)
  if (backgroundFile !== undefined) {
    pushValue(args, "--background-file", backgroundFile)
  } else {
    pushValue(args, "--background", input.background)
    pushValue(args, "--background-file", input.backgroundFile)
  }
  pushValue(args, "--exclude", input.exclude)
  pushValue(args, "--model", input.model)
  pushValue(args, "--concurrency", input.concurrency)
  pushValue(args, "--timeout", input.timeoutMinutes)
  pushValue(args, "--max-tools", input.maxTools)
  pushValue(args, "--max-git-procs", input.maxGitProcesses)

  if (input.preview) {
    args.push("--preview")
  }
  return args
}

function buildDelegatePreviewArgs(
  input: DelegatePreviewInput,
  repo: string,
  backgroundFile?: string,
): string[] {
  const hasRange = input.from !== undefined || input.to !== undefined
  if (hasRange && (!input.from || !input.to)) {
    throw new Error("Both 'from' and 'to' are required for a branch comparison.")
  }
  if (input.commit && hasRange) {
    throw new Error("Use either 'commit' or a 'from'/'to' range, not both.")
  }
  if (input.background && input.backgroundFile) {
    throw new Error("Use either 'background' or 'backgroundFile', not both.")
  }

  const args = ["delegate", "preview", "--format", "json", "--repo", repo]
  pushValue(args, "--commit", input.commit)
  pushValue(args, "--from", input.from)
  pushValue(args, "--to", input.to)
  if (backgroundFile !== undefined) {
    pushValue(args, "--background-file", backgroundFile)
  } else {
    pushValue(args, "--background", input.background)
    pushValue(args, "--background-file", input.backgroundFile)
  }
  pushValue(args, "--exclude", input.exclude)
  pushValue(args, "--rule", input.rule)
  pushValue(args, "--max-git-procs", input.maxGitProcesses)
  return args
}

function parsePaths(paths: unknown): string[] {
  if (Array.isArray(paths)) {
    return paths.map((p) => String(p).trim()).filter(Boolean)
  }
  if (typeof paths === "string") {
    return paths.split(/[,\s]+/).map((p) => p.trim()).filter(Boolean)
  }
  return []
}

function buildDelegateRuleArgs(input: DelegateRuleInput, repo: string): string[] {
  const rawPaths = parsePaths(input.paths)
  if (rawPaths.length === 0) {
    throw new Error("At least one file path is required to fetch review rules.")
  }
  const hasRange = input.from !== undefined || input.to !== undefined
  if (hasRange && (!input.from || !input.to)) {
    throw new Error("Both 'from' and 'to' are required for a branch comparison.")
  }
  if (input.commit && hasRange) {
    throw new Error("Use either 'commit' or a 'from'/'to' range, not both.")
  }

  const args = ["delegate", "rule", "--format", "json", "--repo", repo]
  pushValue(args, "--commit", input.commit)
  pushValue(args, "--from", input.from)
  pushValue(args, "--to", input.to)
  pushValue(args, "--rule", input.rule)
  pushValue(args, "--max-git-procs", input.maxGitProcesses)
  args.push("--", ...rawPaths)
  return args
}

async function withTemporaryBackgroundFile<T>(
  background: string,
  callback: (path: string) => Promise<T>,
): Promise<T> {
  const directory = await mkdtemp(join(tmpdir(), "ocr-opencode-background-"))
  try {
    if (process.platform !== "win32") {
      await chmod(directory, 0o700)
    }
    const path = join(directory, "background.md")
    await writeFile(path, background, {
      encoding: "utf8",
      flag: "wx",
      mode: 0o600,
    })
    return await callback(path)
  } finally {
    await rm(directory, {
      recursive: true,
      force: true,
      maxRetries: backgroundCleanupMaxRetries,
      retryDelay: backgroundCleanupRetryDelayMs,
    })
  }
}

function appendChunk(
  chunks: Uint8Array[],
  currentBytes: number,
  chunk: Uint8Array,
  maxBytes: number,
): number {
  const nextBytes = currentBytes + chunk.byteLength
  if (nextBytes > maxBytes) {
    throw new Error(`OCR output exceeded the ${maxBytes}-byte safety limit.`)
  }
  chunks.push(chunk)
  return nextBytes
}

async function runOcr(args: string[], options: RunOptions): Promise<RunResult> {
  const invocation = options.invocation ?? { command: "ocr", prefixArgs: [] }
  const timeoutMs = options.timeoutMs === undefined ? 15 * 60 * 1000 : options.timeoutMs
  const maxOutputBytes = options.maxOutputBytes ?? 10 * 1024 * 1024

  return await new Promise<RunResult>((resolve, reject) => {
    const child = spawn(
      invocation.command,
      [...invocation.prefixArgs, ...args],
      {
        cwd: options.cwd,
        env: process.env,
        shell: false,
        detached: process.platform !== "win32",
      },
    )
    child.stdin.end()

    const stdoutChunks: Uint8Array[] = []
    const stderrChunks: Uint8Array[] = []
    let outputBytes = 0
    let settled = false
    let closed = false
    let timer: ReturnType<typeof setTimeout> | undefined
    let forceKillTimer: ReturnType<typeof setTimeout> | undefined
    let abort: (() => void) | undefined

    const finish = (callback: () => void): void => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      if (abort) {
        options.signal?.removeEventListener("abort", abort)
      }
      callback()
    }

    const killProcessGroup = (signal: NodeJS.Signals): void => {
      if (closed || child.pid === undefined) return
      if (process.platform === "win32") {
        spawn("taskkill", ["/pid", String(child.pid), "/T", "/F"], { stdio: "ignore" })
        return
      }
      try {
        process.kill(-child.pid, signal)
      } catch {
        child.kill(signal)
      }
    }

    const terminateChild = (): void => {
      if (closed) return
      killProcessGroup("SIGTERM")
      forceKillTimer ??= setTimeout(() => {
        if (!closed) {
          killProcessGroup("SIGKILL")
        }
      }, 3_000)
    }

    const failForOutputLimit = (error: Error): void => {
      terminateChild()
      finish(() => reject(error))
    }

    child.stdout.on("data", (chunk: Buffer) => {
      try {
        outputBytes = appendChunk(stdoutChunks, outputBytes, chunk, maxOutputBytes)
      } catch (error) {
        failForOutputLimit(error as Error)
      }
    })
    child.stderr.on("data", (chunk: Buffer) => {
      try {
        outputBytes = appendChunk(stderrChunks, outputBytes, chunk, maxOutputBytes)
      } catch (error) {
        failForOutputLimit(error as Error)
      }
    })

    child.on("error", (error) => {
      const message = error.message.includes("ENOENT")
        ? "OpenCodeReview is not installed or 'ocr' is not on PATH. Install it with: npm install -g @alibaba-group/open-code-review"
        : `Failed to start OpenCodeReview: ${error.message}`
      finish(() => reject(new OcrExecutionError(message, { exitCode: null })))
    })

    child.on("close", (exitCode, signal) => {
      closed = true
      clearTimeout(forceKillTimer)
      finish(() => {
        const stdout = Buffer.concat(stdoutChunks).toString("utf8").trim()
        const stderr = Buffer.concat(stderrChunks).toString("utf8").trim()
        if (exitCode === 0) {
          resolve({ stdout, stderr, exitCode, signal: signal ?? null })
          return
        }
        // A signal kill reports a null exit code. Naming the signal keeps it
        // distinguishable from a genuine exit 1 when OCR wrote no output.
        // `?? 1` keeps the message numeric: close always reports one of the
        // two, but neither is typed as non-null.
        const cause = signal
          ? `was terminated by signal ${signal}`
          : `exited with code ${exitCode ?? 1}`
        reject(new OcrExecutionError(
          stderr || stdout || `OpenCodeReview ${cause}.`,
          { exitCode, signal, stdout, stderr },
        ))
      })
    })

    abort = (): void => {
      terminateChild()
      finish(() => reject(new OcrExecutionError(
        "OpenCodeReview was cancelled by OpenCode.",
        {
          exitCode: null,
          stdout: Buffer.concat(stdoutChunks).toString("utf8"),
          stderr: Buffer.concat(stderrChunks).toString("utf8"),
        },
      )))
    }
    options.signal?.addEventListener("abort", abort, { once: true })

    if (timeoutMs !== null) {
      timer = setTimeout(() => {
        terminateChild()
        finish(() => reject(new OcrExecutionError(
          `OpenCodeReview timed out after ${Math.round(timeoutMs / 1000)} seconds.`,
          {
            exitCode: null,
            stdout: Buffer.concat(stdoutChunks).toString("utf8"),
            stderr: Buffer.concat(stderrChunks).toString("utf8"),
          },
        )))
      }, timeoutMs)
    }

    if (options.signal?.aborted) {
      abort()
    }
  })
}

function formatReviewResult(result: RunResult, preview: boolean): string {
  if (preview) {
    return result.stdout || "No files changed."
  }
  if (result.stdout === "") {
    return "No changes detected; OCR produced no output."
  }
  try {
    JSON.parse(result.stdout)
    return result.stdout
  } catch {
    throw new OcrExecutionError("OpenCodeReview returned invalid JSON.", result)
  }
}

function formatDelegateResult(result: RunResult): string {
  if (result.stdout === "") {
    return "{}"
  }
  try {
    JSON.parse(result.stdout)
    return result.stdout
  } catch {
    throw new OcrExecutionError("OpenCodeReview returned invalid JSON.", result)
  }
}

const optionalString = (description: string) =>
  tool.schema.string().optional().describe(description)

const optionalPositiveInt = (description: string) =>
  tool.schema.number().int().positive().optional().describe(description)

const reviewArgs = {
  commit: optionalString("Review one commit against its parent."),
  from: optionalString("Base ref for a branch/range comparison. Must be paired with 'to'."),
  to: optionalString("Target ref for a branch/range comparison. Must be paired with 'from'."),
  resume: optionalString("Resume a previous OCR review session by ID."),
  background: optionalString("Business or requirement context that the implementation should satisfy."),
  backgroundFile: optionalString(
    "Path to a Markdown file holding the review background, for context too long to pass inline. " +
      "A relative path resolves against the repository root; an absolute path is used as given. " +
      "Cannot be combined with 'background'.",
  ),
  exclude: optionalString("Comma-separated gitignore-style exclusion patterns."),
  model: optionalString("Override the model configured in OpenCodeReview."),
  concurrency: optionalPositiveInt("Maximum concurrent file reviews."),
  timeoutMinutes: optionalPositiveInt("Per-file OCR timeout in minutes."),
  overallTimeoutMinutes: optionalPositiveInt(
    "Optional wall-clock timeout for the complete OCR process in minutes.",
  ),
  maxTools: optionalPositiveInt("Maximum tool-call rounds per subtask; OCR enforces a minimum of 50."),
  maxGitProcesses: optionalPositiveInt("Maximum concurrent Git subprocesses."),
  preview: tool.schema.boolean().optional().describe(
    "List the files that would be reviewed without calling an LLM.",
  ),
}

const delegatePreviewArgs = {
  commit: optionalString("Review a single commit against its parent."),
  from: optionalString("Base ref for a branch/range comparison. Must be paired with 'to'."),
  to: optionalString("Target ref for a branch/range comparison. Must be paired with 'from'."),
  background: optionalString("Business or requirement context that the implementation should satisfy."),
  backgroundFile: optionalString(
    "Path to a Markdown file holding the review background, for context too long to pass inline. " +
      "A relative path resolves against the repository root; an absolute path is used as given. " +
      "Cannot be combined with 'background'.",
  ),
  exclude: optionalString("Comma-separated gitignore-style exclusion patterns."),
  rule: optionalString("Path to custom rule.json file."),
  maxGitProcesses: optionalPositiveInt("Maximum concurrent Git subprocesses."),
  timeoutSeconds: optionalPositiveInt("Timeout in seconds for preview (default: 60)."),
}

const delegateRuleArgs = {
  paths: tool.schema
    .union([tool.schema.array(tool.schema.string()), tool.schema.string()])
    .describe("File path(s) to fetch review rules for."),
  commit: optionalString("Single commit hash if reviewing a specific commit."),
  from: optionalString("Base ref for a branch/range comparison. Must be paired with 'to'."),
  to: optionalString("Target ref for a branch/range comparison. Must be paired with 'from'."),
  rule: optionalString("Path to custom rule.json file."),
  maxGitProcesses: optionalPositiveInt("Maximum concurrent Git subprocesses."),
  timeoutSeconds: optionalPositiveInt("Timeout in seconds for rule resolution (default: 60)."),
}

export const OpenCodeReviewPlugin: Plugin = async ({ client, worktree }) => {
  try {
    await client.app.log({
      body: {
        service: "open-code-review",
        level: "info",
        message: "OpenCodeReview tools registered",
      },
    })
  } catch {
    // Best-effort telemetry; a failed log call must not block tool registration.
  }

  return {
    config: async (config) => {
      config.command ??= {}
      config.command["ocr-review"] ??= {
        description: "Review code changes with OpenCodeReview",
        template:
          "Use the ocr_review tool to review the requested target. " +
          "Treat the following text as review intent, target details, and business context: $ARGUMENTS. " +
          "If no target is specified, review the current workspace changes. " +
          "Report findings by severity with exact file and line references.",
      }
      config.command["ocr-health"] ??= {
        description: "Check OpenCodeReview and its LLM connection",
        template:
          "Use the ocr_health tool and explain any configuration problem concisely.",
      }
      config.command["ocr-delegate-review"] ??= {
        description: "Review code changes in delegation mode using OpenCode's model for reasoning",
        template:
          "Perform a code review in delegation mode where OpenCodeReview handles file selection and rules, and you perform the actual code review reasoning. " +
          "Treat the following text as review intent, target details, and business context: $ARGUMENTS. " +
          "If no target is specified, review the current workspace changes. " +
          "First, call ocr_delegate_preview to determine reviewable files. " +
          "Next, call ocr_delegate_rule to retrieve applicable rules for those files. " +
          "Then, inspect the diffs and perform the review using your own model capabilities. " +
          "Finally, summarize findings by severity with exact file and line references, accounting for all previewed files.",
      }
      config.command["ocr-delegate"] ??= config.command["ocr-delegate-review"]
    },
    tool: {
      ocr_review: tool({
        description:
          "Run OpenCodeReview on workspace changes, one commit, or a ref range. " +
          "Returns structured line-level findings as JSON. Use preview=true to inspect scope without LLM usage.",
        args: reviewArgs,
        async execute(args, context) {
          const input = args as ReviewInput
          const { background, ...inputWithoutBackground } = input
          const normalizedBackground = background?.trim()
          const normalizedInput: ReviewInput = normalizedBackground
            ? { ...inputWithoutBackground, background: normalizedBackground }
            : inputWithoutBackground
          const cwd = context.worktree || context.directory || worktree
          const defaultOverallMs = 30 * 60 * 1000
          const options: RunOptions = {
            cwd,
            signal: context.abort,
            timeoutMs: input.overallTimeoutMinutes !== undefined
              ? input.overallTimeoutMinutes * 60 * 1000
              : defaultOverallMs,
          }
          const runReview = async (backgroundFile?: string): Promise<string> => {
            const result = await runOcr(
              buildReviewArgs(normalizedInput, cwd, backgroundFile),
              options,
            )
            return formatReviewResult(result, normalizedInput.preview === true)
          }
          if (normalizedInput.background !== undefined) {
            return await withTemporaryBackgroundFile(normalizedInput.background, runReview)
          }
          return await runReview()
        },
      }),
      ocr_health: tool({
        description:
          "Check the installed OpenCodeReview version and verify its configured LLM connection.",
        args: {},
        async execute(_args, context) {
          const cwd = context.worktree || context.directory || worktree
          const [version, llm] = await Promise.allSettled([
            runOcr(["version"], {
              cwd,
              timeoutMs: 30_000,
              signal: context.abort,
            }),
            runOcr(["llm", "test"], {
              cwd,
              timeoutMs: 60_000,
              signal: context.abort,
            }),
          ])
          const rejected = [version, llm].find(
            (result): result is PromiseRejectedResult => result.status === "rejected",
          )
          if (context.abort?.aborted && rejected) {
            throw rejected.reason
          }

          const parts: string[] = []
          if (version.status === "fulfilled") {
            parts.push(version.value.stdout)
          } else {
            parts.push(`Version check failed: ${version.reason?.message ?? "unknown error"}`)
          }
          if (llm.status === "fulfilled") {
            parts.push(llm.value.stdout, llm.value.stderr)
          } else {
            parts.push(`LLM connection check failed: ${llm.reason?.message ?? "unknown error"}`)
          }
          return parts.filter(Boolean).join("\n")
        },
      }),
      ocr_delegate_preview: tool({
        description:
          "Step 1 of delegated review: determine reviewable files, review mode, and ref metadata without calling an LLM. " +
          "Returns structured JSON containing reviewable files, excluded files, and git metadata.",
        args: delegatePreviewArgs,
        async execute(args, context) {
          const input = args as DelegatePreviewInput
          const { background, ...inputWithoutBackground } = input
          const normalizedBackground = background?.trim()
          const normalizedInput: DelegatePreviewInput = normalizedBackground
            ? { ...inputWithoutBackground, background: normalizedBackground }
            : inputWithoutBackground
          const cwd = context.worktree || context.directory || worktree
          const timeoutMs = (normalizedInput.timeoutSeconds ?? 60) * 1000
          const options: RunOptions = {
            cwd,
            signal: context.abort,
            timeoutMs,
          }
          const runPreview = async (backgroundFile?: string): Promise<string> => {
            const result = await runOcr(
              buildDelegatePreviewArgs(normalizedInput, cwd, backgroundFile),
              options,
            )
            return formatDelegateResult(result)
          }
          if (normalizedInput.background !== undefined) {
            return await withTemporaryBackgroundFile(normalizedInput.background, runPreview)
          }
          return await runPreview()
        },
      }),
      ocr_delegate_rule: tool({
        description:
          "Step 2 of delegated review: retrieve resolved review rules grouped by content for specified file paths without calling an LLM. " +
          "Returns structured JSON grouping files by their applicable checklist rules.",
        args: delegateRuleArgs,
        async execute(args, context) {
          const input = args as DelegateRuleInput
          const cwd = context.worktree || context.directory || worktree
          const timeoutMs = (input.timeoutSeconds ?? 60) * 1000
          const options: RunOptions = {
            cwd,
            signal: context.abort,
            timeoutMs,
          }
          const result = await runOcr(
            buildDelegateRuleArgs(input, cwd),
            options,
          )
          return formatDelegateResult(result)
        },
      }),
    },
  }
}

// ---------------------------------------------------------------------------
// OpenCode 2.x entrypoint (https://opencode.ai/v2/docs/build/plugins).
// The default export at the bottom of this file serves both versions: V2
// reads `id` + `setup`, V1 reads `server`. All OCR logic above is shared.
//
// The V2 API is imported as types only (`import type`), and the definition
// below is a plain object literal: `Plugin.define` is an identity function,
// so V1 never needs the `@opencode/plugin` package at runtime.
//
// V2 limitations (no equivalent in the V2 tool API): tool execution has no
// abort signal, so cancellation relies on the overall timeout, and the
// per-session working directory is resolved from the session location.
// ---------------------------------------------------------------------------

const OCR_REVIEW_DESCRIPTION =
  "Run OpenCodeReview on workspace changes, one commit, or a ref range. " +
  "Returns structured line-level findings as JSON. Use preview=true to inspect scope without LLM usage."

const OCR_HEALTH_DESCRIPTION =
  "Check the installed OpenCodeReview version and verify its configured LLM connection."

const OCR_REVIEW_COMMAND_TEMPLATE =
  "Use the ocr_review tool to review the requested target. " +
  "Treat the following text as review intent, target details, and business context:"

const OCR_REVIEW_COMMAND_SUFFIX =
  ". If no target is specified, review the current workspace changes. " +
  "Report findings by severity with exact file and line references."

const OCR_HEALTH_COMMAND_TEMPLATE =
  "Use the ocr_health tool and explain any configuration problem concisely."

const OCR_DELEGATE_PREVIEW_DESCRIPTION =
  "Step 1 of delegated review: determine reviewable files, review mode, and ref metadata without calling an LLM. " +
  "Returns structured JSON containing reviewable files, excluded files, and git metadata."

const OCR_DELEGATE_RULE_DESCRIPTION =
  "Step 2 of delegated review: retrieve resolved review rules grouped by content for specified file paths without calling an LLM. " +
  "Returns structured JSON grouping files by their applicable checklist rules."

const OCR_DELEGATE_COMMAND_DESCRIPTION =
  "Review code changes in delegation mode using OpenCode's model for reasoning"

const OCR_DELEGATE_COMMAND_TEMPLATE =
  "Perform a code review in delegation mode where OpenCodeReview handles file selection and rules, and you perform the actual code review reasoning. " +
  "Treat the following text as review intent, target details, and business context:"

const OCR_DELEGATE_COMMAND_SUFFIX =
  ". If no target is specified, review the current workspace changes. " +
  "First, call ocr_delegate_preview to determine reviewable files. " +
  "Next, call ocr_delegate_rule to retrieve applicable rules for those files. " +
  "Then, inspect the diffs and perform the review using your own model capabilities. " +
  "Finally, summarize findings by severity with exact file and line references, accounting for all previewed files."

const reviewInputSchema = {
  type: "object",
  properties: {
    commit: { type: "string", description: "Review one commit against its parent." },
    from: { type: "string", description: "Base ref for a branch/range comparison. Must be paired with 'to'." },
    to: { type: "string", description: "Target ref for a branch/range comparison. Must be paired with 'from'." },
    resume: { type: "string", description: "Resume a previous OCR review session by ID." },
    background: { type: "string", description: "Business or requirement context that the implementation should satisfy." },
    backgroundFile: { type: "string", description: "Path to a Markdown file holding the review background, for context too long to pass inline. A relative path resolves against the repository root; an absolute path is used as given. Cannot be combined with 'background'." },
    exclude: { type: "string", description: "Comma-separated gitignore-style exclusion patterns." },
    model: { type: "string", description: "Override the model configured in OpenCodeReview." },
    concurrency: { type: "integer", minimum: 1, description: "Maximum concurrent file reviews." },
    timeoutMinutes: { type: "integer", minimum: 1, description: "Per-file OCR timeout in minutes." },
    overallTimeoutMinutes: { type: "integer", minimum: 1, description: "Optional wall-clock timeout for the complete OCR process in minutes." },
    maxTools: { type: "integer", minimum: 1, description: "Maximum tool-call rounds per subtask; OCR enforces a minimum of 50." },
    maxGitProcesses: { type: "integer", minimum: 1, description: "Maximum concurrent Git subprocesses." },
    preview: { type: "boolean", description: "List the files that would be reviewed without calling an LLM." },
  },
  required: [],
  additionalProperties: false,
}

const delegatePreviewInputSchema = {
  type: "object",
  properties: {
    commit: { type: "string", description: "Review a single commit against its parent." },
    from: { type: "string", description: "Base ref for a branch/range comparison. Must be paired with 'to'." },
    to: { type: "string", description: "Target ref for a branch/range comparison. Must be paired with 'from'." },
    background: { type: "string", description: "Business or requirement context that the implementation should satisfy." },
    backgroundFile: {
      type: "string",
      description:
        "Path to a Markdown file holding the review background, for context too long to pass inline. Cannot be combined with 'background'.",
    },
    exclude: { type: "string", description: "Comma-separated gitignore-style exclusion patterns." },
    rule: { type: "string", description: "Path to custom rule.json file." },
    maxGitProcesses: { type: "integer", minimum: 1, description: "Maximum concurrent Git subprocesses." },
    timeoutSeconds: { type: "integer", minimum: 1, description: "Timeout in seconds for preview (default: 60)." },
  },
  required: [],
  additionalProperties: false,
}

const delegateRuleInputSchema = {
  type: "object",
  properties: {
    paths: {
      type: "array",
      items: { type: "string" },
      description: "File paths to fetch review rules for.",
    },
    commit: { type: "string", description: "Single commit hash if reviewing a specific commit." },
    from: { type: "string", description: "Base ref for a branch/range comparison. Must be paired with 'to'." },
    to: { type: "string", description: "Target ref for a branch/range comparison. Must be paired with 'from'." },
    rule: { type: "string", description: "Path to custom rule.json file." },
    maxGitProcesses: { type: "integer", minimum: 1, description: "Maximum concurrent Git subprocesses." },
    timeoutSeconds: { type: "integer", minimum: 1, description: "Timeout in seconds for rule resolution (default: 60)." },
  },
  required: ["paths"],
  additionalProperties: false,
}

async function resolveSessionCwd(ctx: PluginV2.Context, sessionID: string): Promise<string> {
  try {
    const session = await ctx.session.get({ sessionID })
    const directory = session.location.directory
    const subpath = session.subpath
    return subpath === undefined || subpath === "" ? directory : join(directory, subpath)
  } catch {
    return ctx.location.directory
  }
}

async function setupV2(ctx: PluginV2.Context): Promise<void> {
  await ctx.tool.transform((editor) => {
    editor.add({
      name: "ocr_review",
      description: OCR_REVIEW_DESCRIPTION,
      input: reviewInputSchema,
      execute: async (input, toolCtx) => {
        const review = input as ReviewInput
        const cwd = await resolveSessionCwd(ctx, toolCtx.sessionID)
        const result = await runOcr(buildReviewArgs(review, cwd), {
          cwd,
          timeoutMs: review.overallTimeoutMinutes !== undefined
            ? review.overallTimeoutMinutes * 60 * 1000
            : 30 * 60 * 1000,
        })
        return { content: formatReviewResult(result, review.preview === true) }
      },
    })
    editor.add({
      name: "ocr_health",
      description: OCR_HEALTH_DESCRIPTION,
      input: { type: "object", properties: {}, additionalProperties: false },
      execute: async (_input, toolCtx) => {
        const cwd = await resolveSessionCwd(ctx, toolCtx.sessionID)
        const [version, llm] = await Promise.allSettled([
          runOcr(["version"], { cwd, timeoutMs: 30_000 }),
          runOcr(["llm", "test"], { cwd, timeoutMs: 60_000 }),
        ])
        const parts: string[] = []
        if (version.status === "fulfilled") {
          parts.push(version.value.stdout)
        } else {
          parts.push(`Version check failed: ${version.reason?.message ?? "unknown error"}`)
        }
        if (llm.status === "fulfilled") {
          parts.push(llm.value.stdout, llm.value.stderr)
        } else {
          parts.push(`LLM connection check failed: ${llm.reason?.message ?? "unknown error"}`)
        }
        return { content: parts.filter(Boolean).join("\n") }
      },
    })
    editor.add({
      name: "ocr_delegate_preview",
      description: OCR_DELEGATE_PREVIEW_DESCRIPTION,
      input: delegatePreviewInputSchema,
      execute: async (input, toolCtx) => {
        const preview = input as DelegatePreviewInput
        const cwd = await resolveSessionCwd(ctx, toolCtx.sessionID)
        const timeoutMs = (preview.timeoutSeconds ?? 60) * 1000
        const runPreview = async (backgroundFile?: string): Promise<string> => {
          const result = await runOcr(buildDelegatePreviewArgs(preview, cwd, backgroundFile), {
            cwd,
            timeoutMs,
          })
          return formatDelegateResult(result)
        }
        const normalizedBackground = preview.background?.trim()
        if (normalizedBackground !== undefined && normalizedBackground !== "") {
          const content = await withTemporaryBackgroundFile(normalizedBackground, runPreview)
          return { content }
        }
        const content = await runPreview()
        return { content }
      },
    })
    editor.add({
      name: "ocr_delegate_rule",
      description: OCR_DELEGATE_RULE_DESCRIPTION,
      input: delegateRuleInputSchema,
      execute: async (input, toolCtx) => {
        const rule = input as DelegateRuleInput
        const cwd = await resolveSessionCwd(ctx, toolCtx.sessionID)
        const timeoutMs = (rule.timeoutSeconds ?? 60) * 1000
        const result = await runOcr(buildDelegateRuleArgs(rule, cwd), {
          cwd,
          timeoutMs,
        })
        return { content: formatDelegateResult(result) }
      },
    })
  })

  // Like the V1 `??=` guards above, never override commands the user
  // already defined under the same names.
  const registeredCommands = await ctx.command.list()
  const commandNames = new Set(registeredCommands.data.map((command) => command.name))

  await ctx.command.transform((editor) => {
    // Text-only prompts, matching the V1 $ARGUMENTS templates: spreading the
    // incoming prompt attachments would violate exactOptionalPropertyTypes
    // and risk stale attachment offsets after the text rewrite.
    if (!commandNames.has("ocr-review")) {
      editor.add({
        name: "ocr-review",
        description: "Review code changes with OpenCodeReview",
        execute: async ({ sessionID, prompt, delivery }) => {
          await ctx.session.prompt({
            sessionID,
            text: `${OCR_REVIEW_COMMAND_TEMPLATE}${prompt.text ?? ""}${OCR_REVIEW_COMMAND_SUFFIX}`,
            delivery,
          })
        },
      })
    }
    if (!commandNames.has("ocr-health")) {
      editor.add({
        name: "ocr-health",
        description: "Check OpenCodeReview and its LLM connection",
        execute: async ({ sessionID, delivery }) => {
          await ctx.session.prompt({
            sessionID,
            text: OCR_HEALTH_COMMAND_TEMPLATE,
            delivery,
          })
        },
      })
    }
    if (!commandNames.has("ocr-delegate-review")) {
      editor.add({
        name: "ocr-delegate-review",
        description: OCR_DELEGATE_COMMAND_DESCRIPTION,
        execute: async ({ sessionID, prompt, delivery }) => {
          await ctx.session.prompt({
            sessionID,
            text: `${OCR_DELEGATE_COMMAND_TEMPLATE}${prompt.text ?? ""}${OCR_DELEGATE_COMMAND_SUFFIX}`,
            delivery,
          })
        },
      })
    }
    if (!commandNames.has("ocr-delegate")) {
      editor.add({
        name: "ocr-delegate",
        description: OCR_DELEGATE_COMMAND_DESCRIPTION,
        execute: async ({ sessionID, prompt, delivery }) => {
          await ctx.session.prompt({
            sessionID,
            text: `${OCR_DELEGATE_COMMAND_TEMPLATE}${prompt.text ?? ""}${OCR_DELEGATE_COMMAND_SUFFIX}`,
            delivery,
          })
        },
      })
    }
  })
}

export default {
  id: "open-code-review",
  setup: setupV2,
  server: OpenCodeReviewPlugin,
}
