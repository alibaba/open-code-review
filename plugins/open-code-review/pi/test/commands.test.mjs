// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import assert from "node:assert/strict"
import test from "node:test"
import { tokenizeCommandArgs } from "../src/ocr.ts"
import { WORKTREE, createHost, reviewExec, reviewPayload, runCommand, splitOutput, stdoutExec } from "./helpers.mjs"

const RULE_JSON = JSON.stringify({ schema_version: "1", groups: [] })

test("ocr-review runs the flags the user typed and reports the outcome", async () => {
  const state = await createHost(reviewExec(reviewPayload([])))
  await runCommand(state, "ocr-review", "")
  await runCommand(state, "ocr-review", "--from main --to feature --exclude '**/gen/*'")
  await runCommand(state, "ocr-review", '--commit abc123 --background "rate limiting for the login API"')
  assert.deepEqual(splitOutput(state.calls[0].args).args.slice(0, 5), ["review", "--audience", "agent", "--format", "json"])
  assert.deepEqual(splitOutput(state.calls[1].args).args.slice(9), ["--from", "main", "--to", "feature", "--exclude", "**/gen/*"])
  assert.deepEqual(splitOutput(state.calls[2].args).args.slice(9), ["--commit", "abc123", "--background", "rate limiting for the login API"])
  assert.equal(state.notifications[0].type, "info")
  assert.equal(state.userMessages.length, 0)
})

test("ocr-review-delegate runs the action and paths the user typed", async () => {
  const state = await createHost(stdoutExec(RULE_JSON))
  await runCommand(state, "ocr-review-delegate", "preview --from main")
  await runCommand(state, "ocr-review-delegate", "rule internal/foo.go internal/bar.go")
  assert.deepEqual(state.calls[0].args, ["delegate", "preview", "--format", "json", "--repo", WORKTREE, "--from", "main", "--to", "HEAD"])
  assert.deepEqual(state.calls[1].args.slice(-2), ["internal/foo.go", "internal/bar.go"])
  assert.equal(state.calls[1].args[1], "rule")
  assert.equal(state.notifications[0].type, "info")
})

test("both commands hand free text to the agent and run nothing", async () => {
  const review = await createHost(reviewExec(reviewPayload([])))
  await runCommand(review, "ocr-review", "review the last three commits and fix the high severity findings")
  assert.equal(review.calls.length, 0)
  assert.equal(review.userMessages.length, 1)
  assert.match(review.userMessages[0], /review the last three commits/)
  assert.match(review.userMessages[0], /open-code-review/)
  const delegate = await createHost(stdoutExec(RULE_JSON))
  await runCommand(delegate, "ocr-review-delegate", "review my workspace changes with my own model")
  assert.equal(delegate.calls.length, 0)
  assert.match(delegate.userMessages[0], /review my workspace changes with my own model/)
  assert.match(delegate.userMessages[0], /delegat/)
})

test("a slash command names the flag when its timeout is not a positive number", async () => {
  const review = await createHost(reviewExec(reviewPayload([])))
  await runCommand(review, "ocr-review", "--timeout soon")
  assert.equal(review.calls.length, 0)
  assert.match(review.notifications[0].message, /flag --timeout requires a positive number of minutes; "soon" is not one/)
  assert.ok(!review.notifications[0].message.includes("timeoutMinutes"))
  const delegate = await createHost(stdoutExec(RULE_JSON))
  await runCommand(delegate, "ocr-review-delegate", "preview --timeout 0")
  assert.equal(delegate.calls.length, 0)
  assert.match(delegate.notifications[0].message, /"0" is not one/)
})

test("a command turns an unexpected throw into an error notification", async () => {
  const state = await createHost(stdoutExec(42))
  await runCommand(state, "ocr-review-delegate", "preview")
  assert.equal(state.notifications.length, 1)
  assert.equal(state.notifications[0].type, "error")
  assert.match(state.notifications[0].message, /ocr-review-delegate failed unexpectedly/)
})

test("both commands reject a flag that is missing its value", async () => {
  const review = await createHost(reviewExec(reviewPayload([])))
  await runCommand(review, "ocr-review", "--from")
  assert.equal(review.calls.length, 0)
  assert.equal(review.userMessages.length, 0)
  assert.match(review.notifications[0].message, /--from requires a value/)
  const delegate = await createHost(stdoutExec(RULE_JSON))
  await runCommand(delegate, "ocr-review-delegate", "preview --from")
  assert.equal(delegate.calls.length, 0)
  assert.match(delegate.notifications[0].message, /--from requires a value/)
})

test("ocr-review rejects a value that looks like a flag", async () => {
  const state = await createHost(reviewExec(reviewPayload([])))
  await runCommand(state, "ocr-review", "--background --preview")
  assert.equal(state.calls.length, 0)
  assert.match(state.notifications[0].message, /--background requires a value/)
  assert.match(state.notifications[0].message, /"--preview" looks like a flag/)
})

test("ocr-review keeps an expressly quoted empty string as a value", async () => {
  assert.deepEqual(tokenizeCommandArgs('--background "" --preview'), ["--background", "", "--preview"])
})

test("ocr-review reads an escaped quote inside a quoted value", () => {
  assert.deepEqual(tokenizeCommandArgs('--background "fix \\"quoted\\" issue" --preview'), ["--background", 'fix "quoted" issue', "--preview"])
  assert.deepEqual(tokenizeCommandArgs("--background 'it\\'s'"), ["--background", "it's"])
  assert.deepEqual(tokenizeCommandArgs('--exclude "C:\\temp\\**"'), ["--exclude", "C:\\temp\\**"])
  assert.deepEqual(tokenizeCommandArgs('--exclude "C:\\temp\\\\**"'), ["--exclude", "C:\\temp\\**"])
})
