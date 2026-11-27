// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import assert from "node:assert/strict"
import { access } from "node:fs/promises"
import { tmpdir } from "node:os"
import test from "node:test"
import { WORKTREE, callTool, comment, createHost, flagValue, keepOutput, reviewExec, reviewPayload, splitOutput, stdoutExec, textOf } from "./helpers.mjs"

const reviewArgs = (format, flags) => ["review", "--audience", "agent", "--format", format, "--repo", WORKTREE, "--output", "<out>", ...flags]

const ARGV_CASES = [
  { name: "workspace by default", params: {}, flags: [] },
  { name: "commit scope", params: { scope: "commit", commit: "abc123" }, flags: ["--commit", "abc123"] },
  { name: "range with the default target", params: { scope: "range", from: "main" }, flags: ["--from", "main", "--to", "HEAD"] },
  { name: "range with a target", params: { scope: "range", from: "main", to: "feature" }, flags: ["--from", "main", "--to", "feature"] },
  { name: "background, exclude, analysis on a commit", params: { scope: "commit", commit: "HEAD", analysis: true, background: "Add rate limiting", exclude: "**/gen/*" }, flags: ["--commit", "HEAD", "--background", "Add rate limiting", "--exclude", "**/gen/*", "--analysis"] },
  { name: "preview", params: { preview: true }, flags: ["--preview"], format: "text" },
]

test("ocr_review builds the review argv from its parameters", async () => {
  for (const item of ARGV_CASES) {
    const state = await createHost(reviewExec(reviewPayload([])))
    await callTool(state, "ocr_review", item.params)
    const { args, outputPath } = splitOutput(state.calls[0].args)
    assert.deepEqual(args, reviewArgs(item.format ?? "json", item.flags), item.name)
    assert.ok(outputPath.startsWith(tmpdir()), `${item.name}: the output must live in the temp dir`)
  }
})

const REJECTED = [
  { name: "analysis on the workspace scope", params: { analysis: true }, message: /analysis/ },
  { name: "commit scope without a commit", params: { scope: "commit" }, message: /commit/ },
  { name: "range scope without a from ref", params: { scope: "range", to: "dev" }, message: /from/ },
  { name: "commit combined with from", params: { scope: "commit", commit: "abc123", from: "main" }, message: /not both/ },
  { name: "from on the workspace scope", params: { from: "main" }, message: /scope/ },
  { name: "an unknown scope", params: { scope: "everything" }, message: /scope must be/ },
  { name: "a non-positive timeout", params: { timeoutMinutes: -5 }, message: /timeoutMinutes/ },
]

test("ocr_review rejects inconsistent parameters without running ocr", async () => {
  for (const item of REJECTED) {
    const state = await createHost(reviewExec(reviewPayload([])))
    const result = await callTool(state, "ocr_review", item.params)
    assert.equal(result.isError, true, item.name)
    assert.match(textOf(result), item.message, item.name)
    assert.equal(state.calls.length, 0, item.name)
  }
})

test("ocr_review gives a review 30 minutes and a preview 2", async () => {
  const state = await createHost(reviewExec(reviewPayload([])))
  await callTool(state, "ocr_review", {})
  await callTool(state, "ocr_review", { preview: true })
  await callTool(state, "ocr_review", { timeoutMinutes: 5 })
  assert.equal(state.calls[0].command, "ocr")
  assert.equal(state.calls[0].options.cwd, WORKTREE)
  assert.deepEqual(state.calls.map((call) => call.options.timeout), [1_800_000, 120_000, 300_000])
})

test("ocr_review reports the findings it parsed, or none, and removes the output file", async () => {
  const payload = reviewPayload([comment(), comment({ path: "internal/bar.go", start_line: 3, end_line: 4, severity: "low", category: "style" })])
  const state = await createHost(reviewExec(payload))
  const result = await callTool(state, "ocr_review", {})
  const text = textOf(result)
  assert.equal(result.isError, false)
  assert.equal(result.details.findings, 2)
  assert.match(text, /internal\/foo\.go:10-14 \[high\/bug\]/)
  assert.match(text, /internal\/bar\.go:3-4 \[low\/style\]/)
  assert.match(text, /2 findings/)
  await assert.rejects(access(splitOutput(state.calls[0].args).outputPath), "a report that fits leaves no file behind")
  const clean = await callTool(await createHost(reviewExec(reviewPayload([]))), "ocr_review", {})
  assert.equal(clean.isError, false)
  assert.equal(clean.details.findings, 0)
  assert.match(textOf(clean), /No findings/)
})

test("ocr_review reports a failed or unparsable run without inventing findings", async () => {
  const raw = await callTool(await createHost(reviewExec("review finished without json")), "ocr_review", {})
  assert.equal(raw.isError, true)
  assert.match(textOf(raw), /review finished without json/)
  assert.match(textOf(raw), /exit 0/)
  assert.equal(raw.details.findings, 0)
  const stderr = `fatal: not a git repository${"x".repeat(6000)}`
  const state = await createHost(stdoutExec("", { stderr, code: 1 }))
  const failed = await callTool(state, "ocr_review", {})
  assert.equal(failed.isError, true)
  assert.equal(failed.details.exitCode, 1)
  assert.equal(failed.details.findings, 0)
  assert.match(textOf(failed), /fatal: not a git repository/)
  assert.match(textOf(failed), /exit 1/)
  assert.ok(!textOf(failed).includes("x".repeat(4000)), "a huge failure detail must not flood the result")
  const missing = await createHost(async () => { throw Object.assign(new Error("spawn ocr ENOENT"), { code: "ENOENT" }) })
  const absent = await callTool(missing, "ocr_review", {})
  assert.equal(absent.isError, true)
  assert.match(textOf(absent), /npm install -g @alibaba-group\/open-code-review/)
  assert.equal(absent.details.findings, 0)
})

test("ocr_review spills an oversized review to the output file instead of the transcript", async (t) => {
  const comments = Array.from({ length: 120 }, (unused, index) => comment({ path: `internal/file${index}.go`, start_line: index, end_line: index + 1 }))
  const state = await createHost(reviewExec(reviewPayload(comments)))
  keepOutput(t, state)
  const result = await callTool(state, "ocr_review", {})
  const text = textOf(result)
  assert.equal(result.isError, false)
  assert.equal(result.details.findings, 120)
  assert.equal(result.details.outputPath, splitOutput(state.calls[0].args).outputPath)
  assert.match(text, /120 findings/)
  assert.match(text, /100 more findings/)
  assert.ok(!text.includes("internal/file119.go:119-120"), "the transcript must stay compact")
  await access(result.details.outputPath)
})

test("ocr_review reports an incomplete run as incomplete", async () => {
  const stopped = await createHost(stdoutExec("", { code: 143, killed: true }))
  const stop = await callTool(stopped, "ocr_review", {})
  assert.equal(stop.isError, true)
  assert.match(textOf(stop), /ocr review was stopped before it finished/)
  assert.equal(stop.details.findings, 0)
  const cut = await createHost(stdoutExec("Preview: 1 file changed", { code: 137, killed: true, truncated: true }))
  const truncated = await callTool(cut, "ocr_review", { preview: true })
  assert.equal(truncated.isError, true)
  assert.match(textOf(truncated), /ocr review output was truncated/)
})

test("ocr_review turns an unexpected throw into an error result", async (t) => {
  const state = await createHost(stdoutExec(42))
  keepOutput(t, state)
  const result = await callTool(state, "ocr_review", {})
  assert.equal(result.isError, true)
  assert.match(textOf(result), /ocr_review failed unexpectedly/)
  assert.equal(result.details.findings, 0)
})

test("ocr_review forwards the tool signal to the ocr process", async () => {
  const state = await createHost(reviewExec(reviewPayload([])))
  const controller = new AbortController()
  await callTool(state, "ocr_review", {}, { signal: controller.signal })
  assert.equal(state.calls[0].options.signal, controller.signal)
})

test("ocr_review returns the preview file list instead of findings", async () => {
  const state = await createHost(reviewExec("Preview: 1 file(s) changed\n\nWill review (1):\n  [M]  internal/foo.go  +4  -1"))
  const result = await callTool(state, "ocr_review", { preview: true })
  const text = textOf(result)
  assert.equal(flagValue(state.calls[0].args, "--format"), "text")
  assert.match(text, /internal\/foo\.go/)
  assert.ok(!text.includes("No findings"))
  const empty = await createHost(reviewExec(""))
  assert.equal(textOf(await callTool(empty, "ocr_review", { preview: true })), "No files changed.")
})

test("ocr_review renders the analysis markdown instead of the raw report", async () => {
  const payload = reviewPayload([], { analysis: "## Behavior\n\n| Impact | Confidence |", analysis_raw: "RAW" })
  const state = await createHost(reviewExec(payload))
  const text = textOf(await callTool(state, "ocr_review", { scope: "commit", commit: "HEAD", analysis: true }))
  assert.match(text, /## Behavior/)
  assert.match(text, /\| Impact \| Confidence \|/)
  assert.ok(!text.includes("RAW"))
})

test("ocr_review caps an oversized analysis and keeps the file that holds it", async (t) => {
  const state = await createHost(reviewExec(reviewPayload([], { analysis: `## Behavior\n\n${"detail ".repeat(1000)}` })))
  keepOutput(t, state)
  const result = await callTool(state, "ocr_review", { scope: "commit", commit: "HEAD", analysis: true })
  const text = textOf(result)
  assert.match(text, /Analysis was cut at 4000 characters/)
  assert.ok(!text.includes("detail ".repeat(800)), "the rendered analysis must not flood the result")
  assert.equal(result.details.outputPath, splitOutput(state.calls[0].args).outputPath)
  await access(result.details.outputPath)
})

test("ocr_review keeps the output file when the requested analysis is missing", async (t) => {
  const state = await createHost(reviewExec(reviewPayload([])))
  keepOutput(t, state)
  const result = await callTool(state, "ocr_review", { scope: "commit", commit: "HEAD", analysis: true })
  assert.equal(result.details.outputPath, splitOutput(state.calls[0].args).outputPath)
  await access(result.details.outputPath)
})
