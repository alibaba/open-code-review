import test from "node:test"
import assert from "node:assert/strict"

import {
  buildDelegateArgs,
  buildDelegatePrompt,
  buildReviewArgs,
  buildReviewPrompt,
  parseReviewJson,
  summarizeSeverities,
  validateReviewParams,
} from "../src/index.ts"

test("buildReviewArgs defaults to agent audience with json output", () => {
  assert.deepEqual(buildReviewArgs({}), ["review", "--audience", "agent", "--format", "json"])
})

test("buildReviewArgs maps structured params onto CLI flags", () => {
  assert.deepEqual(
    buildReviewArgs({
      from: "main",
      to: "feat",
      background: "ctx",
      concurrency: 4,
      noFilter: true,
    }),
    [
      "review",
      "--audience",
      "agent",
      "--format",
      "json",
      "--from",
      "main",
      "--to",
      "feat",
      "--background",
      "ctx",
      "--concurrency",
      "4",
      "--no-filter",
    ],
  )
})

test("buildReviewArgs preview mode omits json format", () => {
  const args = buildReviewArgs({ preview: true })
  assert.equal(args.includes("--format"), false)
  assert.equal(args.includes("--preview"), true)
})

test("validateReviewParams rejects conflicting combinations", () => {
  assert.match(validateReviewParams({ from: "main" }), /Both 'from' and 'to'/)
  assert.match(validateReviewParams({ commit: "abc", from: "main", to: "feat" }), /not both/)
  assert.match(validateReviewParams({ resume: "s1", commit: "abc" }), /cannot be combined/)
  assert.match(validateReviewParams({ preview: true, resume: "s1" }), /cannot be used together/)
  assert.match(validateReviewParams({ background: "a", backgroundFile: "b" }), /not both/)
  assert.equal(validateReviewParams({ from: "main", to: "feat" }), undefined)
  assert.equal(validateReviewParams({}), undefined)
})

test("parseReviewJson validates the comments array", () => {
  const parsed = parseReviewJson(
    JSON.stringify({
      status: "success",
      comments: [{ path: "a.go", content: "x", start_line: 1, end_line: 2, severity: "high" }],
    }),
  )
  assert.equal(parsed.comments?.length, 1)

  assert.throws(() => parseReviewJson("not json"), /invalid JSON/)
  assert.throws(() => parseReviewJson('{"status":"x"}'), /missing 'comments'/)
})

test("summarizeSeverities orders severities and handles empty input", () => {
  const comments = [
    { path: "a", content: "", start_line: 0, end_line: 0, severity: "low" },
    { path: "a", content: "", start_line: 0, end_line: 0, severity: "critical" },
    { path: "a", content: "", start_line: 0, end_line: 0, severity: "critical" },
  ]
  assert.equal(summarizeSeverities(comments), "critical: 2, low: 1")
  assert.equal(summarizeSeverities([]), "no comments")
})

test("buildDelegateArgs builds preview and rule commands", () => {
  assert.deepEqual(buildDelegateArgs({ action: "preview", from: "main", to: "feat" }), [
    "delegate",
    "preview",
    "--format",
    "json",
    "--from",
    "main",
    "--to",
    "feat",
  ])
  assert.deepEqual(buildDelegateArgs({ action: "rule", paths: ["@a.go", "b.go"] }), [
    "delegate",
    "rule",
    "--format",
    "json",
    "a.go",
    "b.go",
  ])
})

test("command prompts embed user arguments and key instructions", () => {
  const reviewPrompt = buildReviewPrompt("--from main --to feat")
  assert.match(reviewPrompt, /ocr_review/)
  assert.match(reviewPrompt, /--from main --to feat/)
  assert.match(reviewPrompt, /review and fix/)

  assert.match(buildReviewPrompt(""), /workspace changes/)

  const delegatePrompt = buildDelegatePrompt("-c abc123")
  assert.match(delegatePrompt, /ocr_delegate/)
  assert.match(delegatePrompt, /abc123/)
  assert.match(delegatePrompt, /coverage/)
})
