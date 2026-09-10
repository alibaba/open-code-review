// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors
// OpenCode 2.x custom tool (see https://opencode.ai/docs/custom-tools/).
// File name defines the tool name: ocr_review.

import { spawn } from "node:child_process"
import { tool } from "@opencode-ai/plugin"

interface ReviewInput {
  commit?: string
  from?: string
  to?: string
  resume?: string
  background?: string
  exclude?: string
  model?: string
  concurrency?: number
  timeoutMinutes?: number
  overallTimeoutMinutes?: number
  maxTools?: number
  maxGitProcesses?: number
  preview?: boolean
}

interface RunOptions {
  cwd: string
  timeoutMs?: number | null
  maxOutputBytes?: number
  signal?: AbortSignal
}

interface RunResult {
  stdout: string
  stderr: string
  exitCode: number
}

class OcrExecutionError extends Error {
  readonly exitCode: number | null
  readonly stderr: string
  readonly stdout: string

  constructor(message: string, result: {
    exitCode: number | null
    stderr?: string
    stdout?: string
  }) {
    super(message)
    this.name = "OcrExecutionError"
    this.exitCode = result.exitCode
    this.stderr = result.stderr ?? ""
    this.stdout = result.stdout ?? ""
  }
}

function pushValue(args: string[], flag: string, value: string | number | undefined): void {
  if (value !== undefined && value !== "") {
    args.push(flag, String(value))
  }
}

function buildReviewArgs(input: ReviewInput, repo: string): string[] {
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

  const args = ["review", "--audience", "agent"]
  if (!input.preview) {
    args.push("--format", "json")
  }
  args.push("--repo", repo)

  pushValue(args, "--commit", input.commit)
  pushValue(args, "--from", input.from)
  pushValue(args, "--to", input.to)
  pushValue(args, "--resume", input.resume)
  pushValue(args, "--background", input.background)
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
  const timeoutMs = options.timeoutMs === undefined ? 15 * 60 * 1000 : options.timeoutMs
  const maxOutputBytes = options.maxOutputBytes ?? 10 * 1024 * 1024

  return await new Promise<RunResult>((resolve, reject) => {
    const child = spawn("ocr", args, {
      cwd: options.cwd,
      env: process.env,
      shell: false,
      detached: true,
    })
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

    child.on("close", (exitCode) => {
      closed = true
      clearTimeout(forceKillTimer)
      finish(() => {
        const result = {
          stdout: Buffer.concat(stdoutChunks).toString("utf8").trim(),
          stderr: Buffer.concat(stderrChunks).toString("utf8").trim(),
          exitCode: exitCode ?? 1,
        }
        if (exitCode !== 0) {
          reject(new OcrExecutionError(
            result.stderr || result.stdout || `OpenCodeReview exited with code ${result.exitCode}.`,
            result,
          ))
          return
        }
        resolve(result)
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

const optionalString = (description: string) =>
  tool.schema.string().optional().describe(description)

const optionalPositiveInt = (description: string) =>
  tool.schema.number().int().positive().optional().describe(description)

export default tool({
  description:
    "Run OpenCodeReview on workspace changes, one commit, or a ref range. " +
    "Returns structured line-level findings as JSON. Use preview=true to inspect scope without LLM usage.",
  args: {
    commit: optionalString("Review one commit against its parent."),
    from: optionalString("Base ref for a branch/range comparison. Must be paired with 'to'."),
    to: optionalString("Target ref for a branch/range comparison. Must be paired with 'from'."),
    resume: optionalString("Resume a previous OCR review session by ID."),
    background: optionalString("Business or requirement context that the implementation should satisfy."),
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
  },
  async execute(args, context) {
    const input = args as ReviewInput
    const cwd = context.worktree || context.directory
    const result = await runOcr(buildReviewArgs(input, cwd), {
      cwd,
      signal: context.abort,
      timeoutMs: input.overallTimeoutMinutes !== undefined
        ? input.overallTimeoutMinutes * 60 * 1000
        : 30 * 60 * 1000,
    })
    return formatReviewResult(result, input.preview === true)
  },
})
