import { spawn } from "node:child_process"
import { type Plugin, tool } from "@opencode-ai/plugin"

interface ReviewInput {
  repo?: string
  commit?: string
  from?: string
  to?: string
  resume?: string
  background?: string
  exclude?: string
  model?: string
  concurrency?: number
  timeoutMinutes?: number
  maxTools?: number
  maxGitProcesses?: number
  preview?: boolean
}

interface OcrInvocation {
  command: string
  prefixArgs: string[]
}

interface RunOptions {
  cwd: string
  timeoutMs?: number
  maxOutputBytes?: number
  invocation?: OcrInvocation
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

function buildReviewArgs(input: ReviewInput): string[] {
  const hasRange = input.from !== undefined || input.to !== undefined
  if (hasRange && (!input.from || !input.to)) {
    throw new Error("Both 'from' and 'to' are required for a branch comparison.")
  }
  if (input.commit && hasRange) {
    throw new Error("Use either 'commit' or a 'from'/'to' range, not both.")
  }
  if (input.preview && input.resume) {
    throw new Error("'preview' and 'resume' cannot be used together.")
  }

  const args = ["review", "--audience", "agent"]
  if (!input.preview) {
    args.push("--format", "json")
  }

  pushValue(args, "--repo", input.repo)
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
  const invocation = options.invocation ?? { command: "ocr", prefixArgs: [] }
  const timeoutMs = options.timeoutMs ?? 15 * 60 * 1000
  const maxOutputBytes = options.maxOutputBytes ?? 10 * 1024 * 1024

  return await new Promise<RunResult>((resolve, reject) => {
    const child = spawn(
      invocation.command,
      [...invocation.prefixArgs, ...args],
      {
        cwd: options.cwd,
        env: process.env,
        shell: false,
      },
    )
    child.stdin.end()

    const stdoutChunks: Uint8Array[] = []
    const stderrChunks: Uint8Array[] = []
    let stdoutBytes = 0
    let stderrBytes = 0
    let settled = false
    let timer: ReturnType<typeof setTimeout> | undefined
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

    const failForOutputLimit = (error: Error): void => {
      child.kill("SIGTERM")
      finish(() => reject(error))
    }

    child.stdout.on("data", (chunk: Buffer) => {
      try {
        stdoutBytes = appendChunk(stdoutChunks, stdoutBytes, chunk, maxOutputBytes)
      } catch (error) {
        failForOutputLimit(error as Error)
      }
    })
    child.stderr.on("data", (chunk: Buffer) => {
      try {
        stderrBytes = appendChunk(stderrChunks, stderrBytes, chunk, maxOutputBytes)
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
      child.kill("SIGTERM")
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

    timer = setTimeout(() => {
      child.kill("SIGTERM")
      finish(() => reject(new OcrExecutionError(
        `OpenCodeReview timed out after ${Math.round(timeoutMs / 1000)} seconds.`,
        {
          exitCode: null,
          stdout: Buffer.concat(stdoutChunks).toString("utf8"),
          stderr: Buffer.concat(stderrChunks).toString("utf8"),
        },
      )))
    }, timeoutMs)

    if (options.signal?.aborted) {
      abort()
    }
  })
}

function formatReviewResult(result: RunResult, preview: boolean): string {
  if (preview) {
    return result.stdout || "No files changed."
  }
  try {
    return JSON.stringify(JSON.parse(result.stdout), null, 2)
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
  exclude: optionalString("Comma-separated gitignore-style exclusion patterns."),
  model: optionalString("Override the model configured in OpenCodeReview."),
  concurrency: optionalPositiveInt("Maximum concurrent file reviews."),
  timeoutMinutes: optionalPositiveInt("Per-file OCR timeout in minutes."),
  maxTools: optionalPositiveInt("Maximum tool-call rounds per file; OCR enforces a minimum of 10."),
  maxGitProcesses: optionalPositiveInt("Maximum concurrent Git subprocesses."),
  preview: tool.schema.boolean().optional().describe(
    "List the files that would be reviewed without calling an LLM.",
  ),
}

export const OpenCodeReviewPlugin: Plugin = async ({ client, worktree }) => {
  await client.app.log({
    body: {
      service: "open-code-review",
      level: "info",
      message: "OpenCodeReview tools registered",
    },
  })

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
    },
    tool: {
      ocr_review: tool({
        description:
          "Run OpenCodeReview on workspace changes, one commit, or a ref range. " +
          "Returns structured line-level findings as JSON. Use preview=true to inspect scope without LLM usage.",
        args: reviewArgs,
        async execute(args, context) {
          const input = args as ReviewInput
          const cwd = context.worktree || context.directory || worktree
          const result = await runOcr(buildReviewArgs({
            ...input,
            repo: cwd,
          }), { cwd, signal: context.abort })
          return formatReviewResult(result, input.preview === true)
        },
      }),
      ocr_health: tool({
        description:
          "Check the installed OpenCodeReview version and verify its configured LLM connection.",
        args: {},
        async execute(_args, context) {
          const cwd = context.worktree || context.directory || worktree
          const [version, llm] = await Promise.all([
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
          return [
            version.stdout,
            llm.stdout,
            llm.stderr,
          ].filter(Boolean).join("\n")
        },
      }),
    },
  }
}
