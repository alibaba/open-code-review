import assert from "node:assert/strict"
import { chmod, mkdtemp, writeFile } from "node:fs/promises"
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

async function withFakeOcr(source, callback) {
  const directory = await mkdtemp(join(tmpdir(), "ocr-opencode-test-"))
  const executable = join(directory, "ocr")
  await writeFile(executable, `#!/usr/bin/env node\n${source}\n`)
  await chmod(executable, 0o755)

  const previousPath = process.env.PATH
  process.env.PATH = `${directory}${delimiter}${previousPath ?? ""}`
  try {
    return await callback(directory)
  } finally {
    process.env.PATH = previousPath
  }
}

function toolContext(worktree, signal = new AbortController().signal) {
  return {
    worktree,
    directory: worktree,
    abort: signal,
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

test("ocr_review creates agent-friendly workspace arguments", async () => {
  await withFakeOcr(
    "console.log(JSON.stringify({status:'success', argv:process.argv.slice(2)}))",
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const output = await hooks.tool.ocr_review.execute(
        { background: "Add rate limiting" },
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
  const worktree = await mkdtemp(join(tmpdir(), "ocr-opencode-test-"))
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
  const directory = await mkdtemp(join(tmpdir(), "ocr-opencode-test-"))
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
