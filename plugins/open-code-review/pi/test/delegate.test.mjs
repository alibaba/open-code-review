// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import assert from "node:assert/strict"
import { existsSync, readFileSync, rmSync } from "node:fs"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { dirname, join } from "node:path"
import test from "node:test"
import { writeScratch } from "../src/ocr.ts"
import { WORKTREE, callTool, createHost, stdoutExec, textOf } from "./helpers.mjs"

const PREVIEW_JSON = JSON.stringify({ schema_version: "1", mode: "range", merge_base: "abc", reviewable_count: 1, reviewable_files: [{ path: "internal/foo.go", status: "modified", insertions: 3, deletions: 1 }] })
const RULE_JSON = JSON.stringify({ schema_version: "1", groups: [{ group_id: 0, source: "default", pattern: "**/*.go", files: ["internal/foo.go"], rule: "Review Go code." }] })

const ARGV_CASES = [
  { name: "preview of the workspace", params: { action: "preview" }, flags: [] },
  { name: "a default range target", params: { action: "preview", from: "main" }, flags: ["--from", "main", "--to", "HEAD"] },
  { name: "an explicit range", params: { action: "preview", from: "main", to: "feature" }, flags: ["--from", "main", "--to", "feature"] },
  { name: "a commit", params: { action: "preview", commit: "abc123" }, flags: ["--commit", "abc123"] },
  { name: "background and exclude", params: { action: "preview", background: "Login rate limiting", exclude: "**/gen/*" }, flags: ["--background", "Login rate limiting", "--exclude", "**/gen/*"] },
  { name: "rule paths after the flags", params: { action: "rule", from: "main", paths: ["internal/foo.go", "internal/bar.go"] }, flags: ["--from", "main", "--to", "HEAD", "--", "internal/foo.go", "internal/bar.go"] },
]

test("ocr_delegate builds the delegate argv from its parameters", async () => {
  for (const item of ARGV_CASES) {
    const state = await createHost(stdoutExec(PREVIEW_JSON))
    await callTool(state, "ocr_delegate", item.params)
    assert.deepEqual(state.calls[0].args, ["delegate", item.params.action, "--format", "json", "--repo", WORKTREE, ...item.flags], item.name)
  }
  const result = await callTool(await createHost(stdoutExec(RULE_JSON)), "ocr_delegate", { action: "rule", paths: ["internal/foo.go"] })
  assert.equal(result.isError, false)
  assert.equal(textOf(result), RULE_JSON)
  assert.equal(result.details.action, "rule")
  assert.equal(result.details.exitCode, 0)
})

const REJECTED = [
  { name: "rule without paths", params: { action: "rule" }, message: /requires paths/ },
  { name: "rule with blank paths", params: { action: "rule", paths: ["  "] }, message: /requires paths/ },
  { name: "preview with paths", params: { action: "preview", paths: ["internal/foo.go"] }, message: /paths are only valid/ },
  { name: "a missing action", params: {}, message: /action must be/ },
  { name: "an unknown action", params: { action: "review" }, message: /action must be/ },
  { name: "commit combined with from", params: { action: "preview", commit: "abc123", from: "main" }, message: /not both/ },
  { name: "to without from", params: { action: "preview", to: "feature" }, message: /to requires from/ },
  { name: "a flag-shaped path", params: { action: "rule", paths: ["--exclude=x"] }, message: /"--exclude=x"/ },
]

test("ocr_delegate rejects inconsistent parameters without running ocr", async () => {
  for (const item of REJECTED) {
    const state = await createHost(stdoutExec(RULE_JSON))
    const result = await callTool(state, "ocr_delegate", item.params)
    assert.equal(result.isError, true, item.name)
    assert.match(textOf(result), item.message, item.name)
    assert.equal(state.calls.length, 0, item.name)
  }
})

test("ocr_delegate gives every run the preview timeout", async () => {
  const state = await createHost(stdoutExec(PREVIEW_JSON))
  await callTool(state, "ocr_delegate", { action: "preview" })
  await callTool(state, "ocr_delegate", { action: "preview", timeoutMinutes: 9 })
  assert.deepEqual(state.calls.map((call) => call.options.timeout), [120_000, 540_000])
})

test("ocr_delegate reports a non-zero exit, and treats an unknown --format as a version problem", async () => {
  const failed = await createHost(stdoutExec("", { stderr: "fatal: bad revision 'nope'", code: 128 }))
  const failure = await callTool(failed, "ocr_delegate", { action: "preview", from: "nope" })
  assert.equal(failure.isError, true)
  assert.equal(failure.details.exitCode, 128)
  assert.match(textOf(failure), /fatal: bad revision 'nope'/)
  assert.match(textOf(failure), /exit 128/)
  const outdated = await createHost(stdoutExec("", { stderr: "Error: unknown flag: --format", code: 1 }))
  const hint = await callTool(outdated, "ocr_delegate", { action: "preview" })
  assert.equal(hint.isError, true)
  assert.match(textOf(hint), /1\.9\.0/)
  assert.match(textOf(hint), /npm install -g @alibaba-group\/open-code-review@latest/)
})

test("ocr_delegate reports an incomplete run as incomplete, never parsing it", async () => {
  const cut = await createHost(stdoutExec(PREVIEW_JSON.slice(0, 20), { code: 137, killed: true, truncated: true }))
  const truncated = await callTool(cut, "ocr_delegate", { action: "preview" })
  assert.equal(truncated.isError, true)
  assert.match(textOf(truncated), /ocr delegate output was truncated/)
  const stopped = await createHost(stdoutExec("", { code: 143, killed: true }))
  const stop = textOf(await callTool(stopped, "ocr_delegate", { action: "preview" }))
  assert.match(stop, /ocr delegate was stopped before it finished/)
  assert.ok(!stop.includes("ocr review"), "a delegate stop must not name the review command")
})

test("ocr_delegate keeps an oversized selection in a file, not in the transcript", async (t) => {
  const files = Array.from({ length: 400 }, (unused, index) => ({ path: `internal/file${index}.go` }))
  const host = await createHost(stdoutExec(JSON.stringify({ ...JSON.parse(PREVIEW_JSON), reviewable_files: files })))

  const result = await callTool(host, "ocr_delegate", { action: "preview" })

  assert.equal(result.isError, false)
  const report = textOf(result)
  assert.ok(!report.includes("internal/file399.go"), "the whole selection must not flood the result")
  const saved = /Full JSON: (.+)/.exec(report)?.[1]
  assert.ok(saved, report)
  t.after(() => rmSync(dirname(saved), { recursive: true, force: true }))
  assert.equal(JSON.parse(readFileSync(saved, "utf8")).reviewable_files.length, 400)
})

test("ocr_delegate never parses output it cannot use", async () => {
  const raw = await callTool(await createHost(stdoutExec("not json at all")), "ocr_delegate", { action: "preview" })
  assert.equal(raw.isError, true)
  assert.match(textOf(raw), /not json at all/)
  assert.match(textOf(raw), /exit 0/)
})

test("a failed scratch write leaves no directory behind", async (t) => {
  const scratch = await mkdtemp(join(tmpdir(), "ocr-pi-test-"))
  t.after(() => rm(scratch, { recursive: true, force: true }))
  await assert.rejects(writeScratch(scratch, join("missing", "delegate.json"), "{}"), /ENOENT/)
  assert.equal(existsSync(scratch), false)
})
