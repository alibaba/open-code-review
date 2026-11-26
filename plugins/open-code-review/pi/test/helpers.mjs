// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { rm, writeFile } from "node:fs/promises"
import { dirname } from "node:path"
import openCodeReview from "../src/index.ts"

export const WORKTREE = "/repo/worktree"

export async function createHost(exec) {
  const state = { calls: [], tools: new Map(), commands: new Map(), events: new Map(), userMessages: [], notifications: [] }
  await openCodeReview({
    exec: (command, args, options) => {
      state.calls.push({ command, args, options })
      return exec(command, args, options)
    },
    registerTool: (tool) => state.tools.set(tool.name, tool),
    registerCommand: (name, options) => state.commands.set(name, options),
    on: (name, handler) => state.events.set(name, handler),
    sendUserMessage: (content) => state.userMessages.push(content),
  })
  return state
}

export async function callTool(state, name, params, extra = {}) {
  const context = { cwd: WORKTREE, ui: { notify: () => {} }, ...extra }
  return await state.tools.get(name).execute("call-1", params, extra.signal, undefined, context)
}

export async function runCommand(state, name, args) {
  const context = { cwd: WORKTREE, ui: { notify: (message, type) => state.notifications.push({ message, type }) } }
  return await state.commands.get(name).handler(args, context)
}

export function flagValue(args, flag) {
  const index = args.indexOf(flag)
  return index === -1 ? undefined : args[index + 1]
}

export function splitOutput(args) {
  const outputPath = flagValue(args, "--output")
  return { outputPath, args: args.map((arg) => (arg === outputPath ? "<out>" : arg)) }
}

export function stdoutExec(stdout, extra = {}) {
  return async () => ({ stdout, stderr: "", code: 0, killed: false, truncated: false, ...extra })
}

export function reviewExec(payload) {
  return async (command, args) => {
    await writeFile(flagValue(args, "--output"), typeof payload === "string" ? payload : JSON.stringify(payload))
    return { stdout: "", stderr: "", code: 0, killed: false, truncated: false }
  }
}

export function keepOutput(t, state) {
  t.after(() => rm(dirname(splitOutput(state.calls[0].args).outputPath), { recursive: true, force: true }))
}

export function comment(overrides = {}) {
  const base = { path: "internal/foo.go", content: "Missing nil check before dereferencing.", start_line: 10, end_line: 14, severity: "high", category: "bug" }
  return { ...base, ...overrides }
}

export function reviewPayload(comments, extra = {}) {
  return { status: "success", summary: { files_reviewed: 3, comments: comments.length }, comments, session_id: "session-123", ...extra }
}

export function textOf(result) {
  return result.content.map((part) => part.text).join("\n")
}
