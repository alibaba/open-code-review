// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import type { AgentToolResult, ExtensionAPI, ExtensionCommandContext } from "@earendil-works/pi-coding-agent"
import { existsSync } from "node:fs"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import { Type } from "typebox"
import {
  delegateCommandPrompt,
  parseDelegateCommand,
  parseReviewCommand,
  reviewCommandPrompt,
  runDelegate,
  runReview,
  tokenizeCommandArgs,
  unexpectedFailure,
  type ExecFn,
  type OcrOutcome,
} from "./ocr.ts"

function bindExec(pi: ExtensionAPI): ExecFn {
  return (command, args, options) => pi.exec(command, args, options)
}

async function safeRun(label: string, run: () => Promise<OcrOutcome>): Promise<OcrOutcome> {
  try {
    return await run()
  } catch (error) {
    return unexpectedFailure(label, error)
  }
}

function toToolResult(outcome: OcrOutcome): AgentToolResult<Record<string, unknown>> {
  const details: Record<string, unknown> = {
    exitCode: outcome.exitCode,
    findings: outcome.findings.length,
  }
  if (outcome.outputPath !== undefined) details.outputPath = outcome.outputPath
  if (outcome.action !== undefined) details.action = outcome.action
  return { content: [{ type: "text", text: outcome.text }], details, isError: !outcome.ok }
}

function notifyOutcome(ctx: ExtensionCommandContext, outcome: OcrOutcome): void {
  ctx.ui.notify(outcome.text, outcome.ok ? "info" : "error")
}

export default function openCodeReviewExtension(pi: ExtensionAPI): void {
  // A published tarball carries its own `skills/`, placed there by `npm run build`
  // (scripts/build-package.mjs, published via `npm run release`); a checkout keeps
  // them at the repository root. Both are relative to this file, so moving the
  // package or this file means adjusting them: discovery fails silently otherwise.
  const here = dirname(fileURLToPath(import.meta.url))
  const skillSources = [resolve(here, "../skills"), resolve(here, "../../../../skills")]
  pi.on("resources_discover", () => {
    const found = skillSources.find((candidate) => existsSync(candidate))
    return found ? { skillPaths: [found] } : {}
  })

  const reviewParameters = Type.Object({
    scope: Type.Optional(
      Type.Union([Type.Literal("workspace"), Type.Literal("commit"), Type.Literal("range")], {
        description: 'What to review: "workspace" (default) for uncommitted changes, "commit" for one commit, "range" for from..to.',
      }),
    ),
    commit: Type.Optional(
      Type.String({
        description: 'Commit hash or tag to review against its parent. Requires scope "commit" and cannot be combined with from/to.',
      }),
    ),
    from: Type.Optional(
      Type.String({ description: 'Base ref of the range to review. Requires scope "range".' }),
    ),
    to: Type.Optional(
      Type.String({ description: 'Target ref of the range to review. Requires scope "range" and defaults to HEAD.' }),
    ),
    background: Type.Optional(
      Type.String({ description: "Requirement or business context that helps the review judge intent." }),
    ),
    exclude: Type.Optional(
      Type.String({
        description: "Comma-separated gitignore-style patterns to exclude, merged with the repository rules.",
      }),
    ),
    analysis: Type.Optional(
      Type.Boolean({ description: 'Add behavior, impact, and maintainer decision reports. Requires scope "commit" or "range".' }),
    ),
    preview: Type.Optional(
      Type.Boolean({ description: "List the files the review would cover without calling the LLM." }),
    ),
    timeoutMinutes: Type.Optional(
      Type.Number({
        description: "How long the ocr process may run, in minutes. Defaults to 30 for a review and 2 for a preview.",
      }),
    ),
  })

  pi.registerTool<typeof reviewParameters>({
    name: "ocr_review",
    label: "Open Code Review",
    description: [
      "Review this repository with open-code-review (ocr) and the LLM configured for it.",
      "Returns each finding with file, line range, severity, and category, plus the path of the full JSON when the result is too large to list.",
      "Reviews uncommitted changes by default; scope commit reviews one commit against its parent and scope range reviews a branch range.",
      "Use ocr_delegate instead when the review has to run with this session's own model.",
    ].join(" "),
    promptSnippet: "Review a diff with open-code-review and return line-level findings",
    promptGuidelines: [
      "Use ocr_review when the user asks for a code review backed by the LLM configured for ocr.",
      "Use ocr_review with preview true to list which files a review would cover before spending LLM calls.",
    ],
    parameters: reviewParameters,
    async execute(toolCallId, params, signal, onUpdate, ctx) {
      const outcome = await safeRun("ocr_review", () =>
        runReview(bindExec(pi), params, {
          cwd: ctx.cwd,
          signal: signal ?? ctx.signal,
        }),
      )
      return toToolResult(outcome)
    },
  })

  const delegateParameters = Type.Object({
    action: Type.Union([Type.Literal("preview"), Type.Literal("rule")], {
      description: '"preview" lists the reviewable files; "rule" resolves the review rules for paths.',
    }),
    commit: Type.Optional(
      Type.String({ description: "Commit hash or tag whose changes to select, against its parent." }),
    ),
    from: Type.Optional(Type.String({ description: "Base ref of the range to select." })),
    to: Type.Optional(
      Type.String({ description: "Target ref of the range to select. Requires from and defaults to HEAD." }),
    ),
    background: Type.Optional(
      Type.String({ description: "Requirement or business context to attach to the selection." }),
    ),
    exclude: Type.Optional(
      Type.String({
        description: "Comma-separated gitignore-style patterns to exclude, merged with the repository rules.",
      }),
    ),
    paths: Type.Optional(
      Type.Array(Type.String(), {
        description: 'Repository-relative paths to resolve rules for. Required by action "rule" and rejected by action "preview".',
      }),
    ),
    timeoutMinutes: Type.Optional(
      Type.Number({ description: "How long the ocr process may run, in minutes. Defaults to 2." }),
    ),
  })

  pi.registerTool<typeof delegateParameters>({
    name: "ocr_delegate",
    label: "Open Code Review (delegation mode)",
    description: [
      "Select the files to review and resolve the review rules with open-code-review (ocr) delegation mode, without calling any OCR LLM.",
      "action \"preview\" returns the reviewable files with mode and ref metadata so this session can build the diff; action \"rule\" returns the resolved rules for the given paths.",
      "Review the selected diff with this session's own model afterwards. Requires ocr 1.9.0 or newer.",
    ].join(" "),
    promptSnippet: "Select the files and rules for a review this session runs itself",
    promptGuidelines: [
      "Use ocr_delegate when the user wants this session's own model to perform the review instead of the LLM configured for ocr.",
      "Call ocr_delegate with action preview first to learn the files and refs to review, then review them yourself.",
    ],
    parameters: delegateParameters,
    async execute(toolCallId, params, signal, onUpdate, ctx) {
      const outcome = await safeRun("ocr_delegate", () =>
        runDelegate(bindExec(pi), params, {
          cwd: ctx.cwd,
          signal: signal ?? ctx.signal,
        }),
      )
      return toToolResult(outcome)
    },
  })

  pi.registerCommand("ocr-review", {
    description: "Review with open-code-review. No argument reviews the workspace; ocr flags such as --from or --commit run ocr_review directly; any other text asks the agent to review. A flag with no value is rejected.",
    handler: async (args, ctx) => {
      const parsed = parseReviewCommand(tokenizeCommandArgs(args))
      if (parsed.kind === "invalid") {
        ctx.ui.notify(parsed.message, "error")
        return
      }
      if (parsed.kind === "freeText") {
        pi.sendUserMessage(reviewCommandPrompt(args))
        return
      }
      notifyOutcome(ctx, await safeRun("ocr-review", () => runReview(bindExec(pi), parsed.params, { cwd: ctx.cwd, signal: ctx.signal })))
    },
  })

  pi.registerCommand("ocr-review-delegate", {
    description: "Delegate a review to this session. Starts with preview or rule; any other text asks the agent to run a delegated review. A flag with no value is rejected.",
    handler: async (args, ctx) => {
      const parsed = parseDelegateCommand(tokenizeCommandArgs(args))
      if (parsed.kind === "invalid") {
        ctx.ui.notify(parsed.message, "error")
        return
      }
      if (parsed.kind === "freeText") {
        pi.sendUserMessage(delegateCommandPrompt(args))
        return
      }
      notifyOutcome(ctx, await safeRun("ocr-review-delegate", () => runDelegate(bindExec(pi), parsed.params, { cwd: ctx.cwd, signal: ctx.signal })))
    },
  })
}
