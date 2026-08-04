// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import assert from "node:assert/strict"
import { access, chmod, copyFile, link, mkdtemp, readFile, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { delimiter, join } from "node:path"
import test from "node:test"
import { OpenCodeReviewPlugin } from "../dist/open-code-review.js"

async function loadPlugin(worktree) {
  const logs = []
  const hooks = await OpenCodeReviewPlugin({
    client: {
      app: {
        log: async (entry) => logs.push(entry),
      },
    },
    worktree,
  })
  return { hooks, logs }
}

async function withTemporaryDirectory(callback) {
  const directory = await mkdtemp(join(tmpdir(), "ocr-opencode-test-"))
  try {
    return await callback(directory)
  } finally {
    await rm(directory, {
      recursive: true,
      force: true,
      maxRetries: 20,
      retryDelay: 50,
    })
  }
}

async function withFakeOcr(source, callback) {
  return await withTemporaryDirectory(async (directory) => {
    if (process.platform === "win32") {
      const executable = join(directory, "ocr.exe")
      try {
        await link(process.execPath, executable)
      } catch {
        await copyFile(process.execPath, executable)
      }
      for (const command of ["review", "version", "llm"]) {
        await writeFile(
          join(directory, command),
          `process.argv.splice(2, 0, ${JSON.stringify(command)})\n${source}\n`,
        )
      }
    } else {
      const executable = join(directory, "ocr")
      await writeFile(executable, `#!/usr/bin/env node\n${source}\n`)
      await chmod(executable, 0o755)
    }

    const previousPath = process.env.PATH
    process.env.PATH = `${directory}${delimiter}${previousPath ?? ""}`
    try {
      return await callback(directory)
    } finally {
      process.env.PATH = previousPath
    }
  })
}

function toolContext(worktree, signal = new AbortController().signal) {
  return {
    worktree,
    directory: worktree,
    abort: signal,
  }
}

async function waitForFile(path, timeoutMs = 1_000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      await access(path)
      return
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
  }
  throw new Error(`Timed out waiting for ${path}`)
}

function isProcessRunning(pid) {
  try {
    process.kill(pid, 0)
    return true
  } catch (error) {
    if (error.code === "ESRCH") return false
    throw error
  }
}

async function waitForProcessExit(pid, timeoutMs = 5_000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (!isProcessRunning(pid)) return
    await new Promise((resolve) => setTimeout(resolve, 25))
  }
  throw new Error(`Process ${pid} did not exit within ${timeoutMs}ms`)
}

async function withShortOverallTimeout(timeoutMs, callback) {
  const originalSetTimeout = globalThis.setTimeout
  const overallTimeouts = []
  globalThis.setTimeout = (handler, delay, ...args) => {
    if (delay >= 60 * 1000) {
      overallTimeouts.push(delay)
      return originalSetTimeout(handler, timeoutMs, ...args)
    }
    return originalSetTimeout(handler, delay, ...args)
  }
  try {
    await callback()
    return overallTimeouts
  } finally {
    globalThis.setTimeout = originalSetTimeout
  }
}

test("module exposes only one OpenCode plugin entry point", async () => {
  const module = await import("../dist/open-code-review.js")
  assert.deepEqual(Object.keys(module), ["OpenCodeReviewPlugin"])
})

test("plugin registers tools and preserves existing user commands", async () => {
  const { hooks, logs } = await loadPlugin("/tmp/project")

  assert.deepEqual(Object.keys(hooks.tool).sort(), ["ocr_health", "ocr_review"])
  assert.equal(logs.length, 1)

  const config = {
    command: {
      "ocr-review": {
        template: "Keep my custom review command.",
      },
    },
  }
  await hooks.config(config)
  assert.equal(config.command["ocr-review"].template, "Keep my custom review command.")
  assert.match(config.command["ocr-health"].template, /ocr_health/)
})

test("plugin still registers tools when the telemetry log call fails", async () => {
  const hooks = await OpenCodeReviewPlugin({
    client: {
      app: {
        log: async () => {
          throw new Error("log service unavailable")
        },
      },
    },
    worktree: "/tmp/project",
  })
  assert.deepEqual(Object.keys(hooks.tool).sort(), ["ocr_health", "ocr_review"])
})

test("fake OCR helper removes its temporary directory", async () => {
  let temporaryDirectory
  await withFakeOcr("", async (directory) => {
    temporaryDirectory = directory
  })
  await assert.rejects(access(temporaryDirectory), { code: "ENOENT" })
})

test("ocr_review creates agent-friendly workspace arguments", async () => {
  await withFakeOcr(
    "console.log(JSON.stringify({status:'success', argv:process.argv.slice(2)}))",
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const output = await hooks.tool.ocr_review.execute(
        { background: "Add rate limiting", timeoutMinutes: 30 },
        toolContext(worktree),
      )
      assert.deepEqual(JSON.parse(output).argv, [
        "review",
        "--audience",
        "agent",
        "--format",
        "json",
        "--repo",
        worktree,
        "--background",
        "Add rate limiting",
        "--timeout",
        "30",
      ])
    },
  )
})

test("ocr_review passes suspicious-looking refs as one argv value without a shell", async () => {
  await withFakeOcr(
    "console.log(JSON.stringify({status:'success', argv:process.argv.slice(2)}))",
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const output = await hooks.tool.ocr_review.execute(
        {
          commit: "main; touch /tmp/unsafe",
          exclude: "**/*.generated.ts,dist/**",
        },
        toolContext(worktree),
      )
      assert.deepEqual(JSON.parse(output).argv, [
        "review",
        "--audience",
        "agent",
        "--format",
        "json",
        "--repo",
        worktree,
        "--commit",
        "main; touch /tmp/unsafe",
        "--exclude",
        "**/*.generated.ts,dist/**",
      ])
    },
  )
})

test("ocr_review rejects incompatible review targets before starting OCR", async () => {
  await withTemporaryDirectory(async (worktree) => {
    const { hooks } = await loadPlugin(worktree)

    await assert.rejects(
      hooks.tool.ocr_review.execute(
        { commit: "abc", from: "main", to: "feature" },
        toolContext(worktree),
      ),
      /either 'commit' or a 'from'\/'to' range/,
    )
    await assert.rejects(
      hooks.tool.ocr_review.execute(
        { from: "main" },
        toolContext(worktree),
      ),
      /Both 'from' and 'to'/,
    )
    await assert.rejects(
      hooks.tool.ocr_review.execute(
        { preview: true, resume: "session-1" },
        toolContext(worktree),
      ),
      /cannot be used together/,
    )
    await assert.rejects(
      hooks.tool.ocr_review.execute(
        { resume: "session-1", commit: "abc" },
        toolContext(worktree),
      ),
      /'resume' cannot be combined/,
    )
    await assert.rejects(
      hooks.tool.ocr_review.execute(
        { resume: "session-1", from: "main", to: "feature" },
        toolContext(worktree),
      ),
      /'resume' cannot be combined/,
    )
  })
})

test("preview omits JSON mode and adds --preview", async () => {
  await withFakeOcr(
    "console.log(process.argv.slice(2).join('\\n'))",
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const output = await hooks.tool.ocr_review.execute(
        { preview: true },
        toolContext(worktree),
      )
      assert.deepEqual(output.split("\n"), [
        "review",
        "--audience",
        "agent",
        "--repo",
        worktree,
        "--preview",
      ])
    },
  )
})

test("ocr_review reports non-zero exits with OCR output", async () => {
  await withFakeOcr(
    "console.error('missing credentials'); process.exit(7)",
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      await assert.rejects(
        hooks.tool.ocr_review.execute({}, toolContext(worktree)),
        (error) => {
          assert.equal(error.name, "OcrExecutionError")
          assert.equal(error.exitCode, 7)
          assert.match(error.message, /missing credentials/)
          return true
        },
      )
    },
  )
})

test("ocr_review explains how to install a missing OCR executable", async () => {
  await withTemporaryDirectory(async (directory) => {
    const previousPath = process.env.PATH
    process.env.PATH = directory
    try {
      const { hooks } = await loadPlugin(directory)
      await assert.rejects(
        hooks.tool.ocr_review.execute({}, toolContext(directory)),
        /npm install -g @alibaba-group\/open-code-review/,
      )
    } finally {
      process.env.PATH = previousPath
    }
  })
})

test("ocr_review terminates when OpenCode cancels the tool", async () => {
  await withFakeOcr(
    "setInterval(() => {}, 1000)",
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const controller = new AbortController()
      setTimeout(() => controller.abort(), 20)
      await assert.rejects(
        hooks.tool.ocr_review.execute(
          {},
          toolContext(worktree, controller.signal),
        ),
        /cancelled by OpenCode/,
      )
    },
  )
})

test("ocr_review force-kills a child that ignores cancellation", async () => {
  await withFakeOcr(
    [
      "require('node:fs').writeFileSync('ocr-child.pid', String(process.pid))",
      "process.on('SIGTERM', () => {})",
      "setInterval(() => {}, 1000)",
    ].join("\n"),
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const controller = new AbortController()
      const execution = hooks.tool.ocr_review.execute(
        {},
        toolContext(worktree, controller.signal),
      )
      const pidPath = join(worktree, "ocr-child.pid")
      await waitForFile(pidPath)
      const pid = Number(await readFile(pidPath, "utf8"))

      controller.abort()
      await assert.rejects(execution, /cancelled by OpenCode/)
      try {
        await waitForProcessExit(pid)
      } finally {
        if (isProcessRunning(pid)) {
          process.kill(pid, "SIGKILL")
        }
      }
    },
  )
})

test("ocr_review kills the whole process group on cancellation", { skip: process.platform === "win32" }, async () => {
  const grandchildSource = [
    "require('node:fs').writeFileSync('grandchild.pid', String(process.pid))",
    "setInterval(() => {}, 1000)",
  ].join("\n")
  await withFakeOcr(
    [
      "const { spawn } = require('node:child_process')",
      `const g = spawn(process.execPath, ['-e', ${JSON.stringify(grandchildSource)}], { stdio: 'ignore' })`,
      "g.unref()",
      "setInterval(() => {}, 1000)",
    ].join("\n"),
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const controller = new AbortController()
      const execution = hooks.tool.ocr_review.execute(
        {},
        toolContext(worktree, controller.signal),
      )
      const pidPath = join(worktree, "grandchild.pid")
      await waitForFile(pidPath)
      const pid = Number(await readFile(pidPath, "utf8"))

      controller.abort()
      await assert.rejects(execution, /cancelled by OpenCode/)
      try {
        await waitForProcessExit(pid)
      } finally {
        if (isProcessRunning(pid)) {
          process.kill(pid, "SIGKILL")
        }
      }
    },
  )
})

test("ocr_review has no implicit process-wide timeout", async () => {
  await withFakeOcr(
    "console.log('{\"status\":\"success\",\"findings\":[]}')",
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const overallTimeouts = await withShortOverallTimeout(20, async () => {
        await hooks.tool.ocr_review.execute({}, toolContext(worktree))
      })
      assert.deepEqual(overallTimeouts, [])
    },
  )
})

test("ocr_review keeps per-file and overall timeouts independent", async () => {
  await withFakeOcr(
    "setInterval(() => {}, 1000)",
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const overallTimeouts = await withShortOverallTimeout(20, async () => {
        await assert.rejects(
          hooks.tool.ocr_review.execute(
            { timeoutMinutes: 30, overallTimeoutMinutes: 45 },
            toolContext(worktree),
          ),
          /timed out after 2700 seconds/,
        )
      })
      assert.deepEqual(overallTimeouts, [45 * 60 * 1000])
    },
  )
})

test("ocr_review enforces one output limit across stdout and stderr", async () => {
  await withFakeOcr(
    [
      "process.stdout.write('a'.repeat(6 * 1024 * 1024))",
      "process.stderr.write('b'.repeat(6 * 1024 * 1024))",
    ].join("\n"),
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      await assert.rejects(
        hooks.tool.ocr_review.execute({}, toolContext(worktree)),
        /output exceeded the 10485760-byte safety limit/,
      )
    },
  )
})

test("ocr_review rejects invalid JSON output", async () => {
  await withFakeOcr(
    "console.log('not json')",
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      await assert.rejects(
        hooks.tool.ocr_review.execute({}, toolContext(worktree)),
        /invalid JSON/,
      )
    },
  )
})

test("ocr_review reports no output instead of invalid JSON when OCR prints nothing", async () => {
  await withFakeOcr("", async (worktree) => {
    const { hooks } = await loadPlugin(worktree)
    const output = await hooks.tool.ocr_review.execute({}, toolContext(worktree))
    assert.match(output, /No changes detected/)
  })
})

test("ocr_review preserves valid JSON after validation", async () => {
  await withFakeOcr(
    "console.log('{\"status\":\"success\",\"findings\":[]}')",
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const output = await hooks.tool.ocr_review.execute(
        {},
        toolContext(worktree),
      )
      assert.equal(output, "{\"status\":\"success\",\"findings\":[]}")
    },
  )
})

test("ocr_health reports both version success and LLM failure", async () => {
  await withFakeOcr(
    [
      "if (process.argv[2] === 'version') {",
      "  console.log('OpenCodeReview 1.2.3')",
      "} else {",
      "  console.error('missing LLM credentials')",
      "  process.exitCode = 7",
      "}",
    ].join("\n"),
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const output = await hooks.tool.ocr_health.execute(
        {},
        toolContext(worktree),
      )
      assert.match(output, /OpenCodeReview 1\.2\.3/)
      assert.match(output, /LLM connection check failed: missing LLM credentials/)
    },
  )
})

test("ocr_health works when no abort signal is provided", async () => {
  await withFakeOcr(
    [
      "if (process.argv[2] === 'version') {",
      "  console.log('OpenCodeReview 1.2.3')",
      "} else {",
      "  console.error('missing LLM credentials')",
      "  process.exitCode = 7",
      "}",
    ].join("\n"),
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const output = await hooks.tool.ocr_health.execute(
        {},
        { worktree, directory: worktree },
      )
      assert.match(output, /OpenCodeReview 1\.2\.3/)
      assert.match(output, /LLM connection check failed: missing LLM credentials/)
    },
  )
})

test("ocr_health preserves OpenCode cancellation", async () => {
  await withFakeOcr(
    "setInterval(() => {}, 1000)",
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const controller = new AbortController()
      const execution = hooks.tool.ocr_health.execute(
        {},
        toolContext(worktree, controller.signal),
      )
      setTimeout(() => controller.abort(), 20)
      await assert.rejects(execution, /cancelled by OpenCode/)
    },
  )
})
