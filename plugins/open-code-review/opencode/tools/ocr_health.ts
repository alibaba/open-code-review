// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors
// OpenCode 2.x custom tool (see https://opencode.ai/docs/custom-tools/).
// File name defines the tool name: ocr_health.

import { spawn } from "node:child_process"
import { tool } from "@opencode-ai/plugin"

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

export default tool({
  description:
    "Check the installed OpenCodeReview version and verify its configured LLM connection.",
  args: {},
  async execute(_args, context) {
    const cwd = context.worktree || context.directory
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
})
