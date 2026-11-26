// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"

const OCR_BINARY = "ocr"
const OCR_INSTALL_COMMAND = "npm install -g @alibaba-group/open-code-review"
const OCR_UPGRADE_COMMAND = "npm install -g @alibaba-group/open-code-review@latest"
const DELEGATE_FORMAT_MIN_VERSION = "1.9.0"
const INLINE_FINDING_LIMIT = 20
const DEFAULT_REVIEW_TIMEOUT_MINUTES = 30
const DEFAULT_PREVIEW_TIMEOUT_MINUTES = 2

const RAW_OUTPUT_LIMIT = 4000
const PAYLOAD_HEAD_LIMIT = 1000
const FINDING_CONTENT_LIMIT = 240

export type ReviewScope = "workspace" | "commit" | "range"
type DelegateAction = "preview" | "rule"

export interface ExecOptions {
  cwd?: string
  signal?: AbortSignal | undefined
  timeout?: number
  maxBuffer?: number
}

export interface ExecResult {
  stdout: string
  stderr: string
  code: number
  killed?: boolean
  truncated?: boolean
}

export type ExecFn = (command: string, args: string[], options?: ExecOptions) => Promise<ExecResult>

export interface ReviewParams {
  scope?: string
  commit?: string
  from?: string
  to?: string
  background?: string
  exclude?: string
  analysis?: boolean
  preview?: boolean
  timeoutMinutes?: number
}

export interface DelegateParams {
  action?: string
  commit?: string
  from?: string
  to?: string
  background?: string
  exclude?: string
  paths?: string[]
  timeoutMinutes?: number
}

export interface ReviewFinding {
  path: string
  startLine: number
  endLine: number
  severity: string
  category: string
  content: string
}

export interface OcrOutcome {
  ok: boolean
  text: string
  findings: ReviewFinding[]
  exitCode: number | null
  outputPath?: string
  action?: string
  data?: unknown
}

export interface RunContext {
  cwd: string
  signal?: AbortSignal | undefined
}

class OcrInputError extends Error {
  constructor(message: string) {
    super(message)
    this.name = "OcrInputError"
  }
}

interface DiffSelection {
  mode: ReviewScope
  commit: string
  from: string
  to: string
}

interface ReviewPlan {
  scope: ReviewScope
  selection: DiffSelection
}

interface DelegatePlan {
  action: DelegateAction
  selection: DiffSelection
  paths: string[]
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : ""
}

function numberOr(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback
}

function timeoutMs(minutes: unknown, fallbackMinutes: number): number {
  if (minutes === undefined || minutes === null) return fallbackMinutes * 60_000
  if (typeof minutes !== "number" || !Number.isFinite(minutes) || minutes <= 0) {
    throw new OcrInputError("timeoutMinutes must be a positive number of minutes.")
  }
  return Math.round(minutes * 60_000)
}

function diffSelection(params: { commit?: string; from?: string; to?: string }): DiffSelection {
  const commit = text(params.commit)
  const from = text(params.from)
  const to = text(params.to)
  if (commit !== "" && (from !== "" || to !== "")) {
    throw new OcrInputError("commit cannot be combined with from/to: pass commit, or from/to, but not both.")
  }
  if (to !== "" && from === "") {
    throw new OcrInputError("to requires from: pass from, or from and to together.")
  }
  if (commit !== "") return { mode: "commit", commit, from, to }
  if (from !== "") return { mode: "range", commit, from, to }
  return { mode: "workspace", commit, from, to }
}

function appendDiffFlags(args: string[], selection: DiffSelection): void {
  if (selection.mode === "commit") {
    args.push("--commit", selection.commit)
    return
  }
  if (selection.mode === "range") {
    args.push("--from", selection.from, "--to", selection.to === "" ? "HEAD" : selection.to)
  }
}

function planReview(params: ReviewParams): ReviewPlan {
  const scope = text(params.scope) === "" ? "workspace" : text(params.scope)
  if (scope !== "workspace" && scope !== "commit" && scope !== "range") {
    throw new OcrInputError(`scope must be "workspace", "commit", or "range"; got "${scope}".`)
  }
  const selection = diffSelection(params)
  if (scope === "workspace" && selection.mode !== "workspace") {
    throw new OcrInputError(
      'scope "workspace" reviews uncommitted changes: drop commit/from/to, or set scope to "commit" or "range".',
    )
  }
  if (scope === "commit" && selection.mode !== "commit") {
    throw new OcrInputError('scope "commit" requires commit and rejects from/to.')
  }
  if (scope === "range" && selection.mode !== "range") {
    throw new OcrInputError('scope "range" requires from, with an optional to, and rejects commit.')
  }
  if (params.analysis === true && selection.mode === "workspace") {
    throw new OcrInputError('analysis needs a commit or a from ref: set scope to "commit" or "range".')
  }
  timeoutMs(params.timeoutMinutes, params.preview === true ? DEFAULT_PREVIEW_TIMEOUT_MINUTES : DEFAULT_REVIEW_TIMEOUT_MINUTES)
  return { scope, selection }
}

function planDelegate(params: DelegateParams): DelegatePlan {
  const action = text(params.action)
  if (action !== "preview" && action !== "rule") {
    throw new OcrInputError('action must be "preview" or "rule".')
  }
  const selection = diffSelection(params)
  const candidates = Array.isArray(params.paths) ? params.paths : []
  const paths = candidates.map((path) => text(path)).filter((path) => path !== "")
  const flagged = paths.find((path) => path.startsWith("-"))
  if (flagged !== undefined) {
    throw new OcrInputError(`paths must be repository-relative file paths and cannot start with "-"; got "${flagged}".`)
  }
  if (action === "rule" && paths.length === 0) {
    throw new OcrInputError('action "rule" requires paths: name at least one file to resolve rules for.')
  }
  if (action === "preview" && paths.length > 0) {
    throw new OcrInputError('paths are only valid with action "rule": preview selects the files itself.')
  }
  timeoutMs(params.timeoutMinutes, DEFAULT_PREVIEW_TIMEOUT_MINUTES)
  return { action, selection, paths }
}

interface BuildReviewOptions {
  repoDir: string
  outputPath: string
}

function reviewArgs(plan: ReviewPlan, params: ReviewParams, options: BuildReviewOptions): string[] {
  // A preview prints a file list rather than a review envelope, so it is asked for in text.
  const args = [
    "review",
    "--audience",
    "agent",
    "--format",
    params.preview === true ? "text" : "json",
    "--repo",
    options.repoDir,
    "--output",
    options.outputPath,
  ]
  appendDiffFlags(args, plan.selection)
  const background = text(params.background)
  if (background !== "") args.push("--background", background)
  const exclude = text(params.exclude)
  if (exclude !== "") args.push("--exclude", exclude)
  if (params.analysis === true) args.push("--analysis")
  if (params.preview === true) args.push("--preview")
  return args
}

function delegateArgs(plan: DelegatePlan, params: DelegateParams, options: { repoDir: string }): string[] {
  const args = ["delegate", plan.action, "--format", "json", "--repo", options.repoDir]
  appendDiffFlags(args, plan.selection)
  const background = text(params.background)
  if (background !== "") args.push("--background", background)
  const exclude = text(params.exclude)
  if (exclude !== "") args.push("--exclude", exclude)
  if (plan.paths.length > 0) args.push(...plan.paths)
  return args
}

function parseJsonObject(raw: string): Record<string, unknown> | null {
  const candidate = raw.trim()
  if (candidate === "") return null
  let value: unknown
  try {
    value = JSON.parse(candidate)
  } catch {
    return null
  }
  if (value === null || typeof value !== "object" || Array.isArray(value)) return null
  return value as Record<string, unknown>
}

function extractFindings(payload: Record<string, unknown>): ReviewFinding[] {
  const comments = payload.comments
  if (!Array.isArray(comments)) return []
  const findings: ReviewFinding[] = []
  for (const entry of comments) {
    if (entry === null || typeof entry !== "object") continue
    const record = entry as Record<string, unknown>
    findings.push({
      path: text(record.path),
      startLine: numberOr(record.start_line, 0),
      endLine: numberOr(record.end_line, 0),
      severity: text(record.severity),
      category: text(record.category),
      content: text(record.content),
    })
  }
  return findings
}

function truncated(value: string, limit: number): string {
  if (value.length <= limit) return value
  return `${value.slice(0, limit - 3)}...`
}

function oneLine(value: string, limit: number): string {
  return truncated(value.replace(/\s+/g, " ").trim(), limit)
}

function formatFinding(finding: ReviewFinding, position: number): string {
  const location = finding.endLine > finding.startLine
    ? `${finding.path}:${finding.startLine}-${finding.endLine}`
    : `${finding.path}:${finding.startLine}`
  const tags = [finding.severity, finding.category].filter((tag) => tag !== "").join("/")
  const label = tags === "" ? "" : ` [${tags}]`
  return `${position}. ${location}${label} ${oneLine(finding.content, FINDING_CONTENT_LIMIT)}`
}

function summarize(payload: Record<string, unknown>, findings: ReviewFinding[], outputPath: string): string {
  const lines: string[] = []
  const status = text(payload.status)
  lines.push(status === "" ? "open-code-review finished." : `open-code-review finished with status "${status}".`)
  const summary = payload.summary
  if (summary !== null && typeof summary === "object") {
    const filesReviewed = numberOr((summary as Record<string, unknown>).files_reviewed, 0)
    lines.push(`Files reviewed: ${filesReviewed}`)
  }
  const message = text(payload.message)
  if (message !== "") lines.push(`Message: ${oneLine(message, RAW_OUTPUT_LIMIT)}`)
  const sessionId = text(payload.session_id)
  if (sessionId !== "") lines.push(`Session: ${sessionId}`)
  if (findings.length === 0) {
    lines.push("No findings.")
    return lines.join("\n")
  }
  lines.push(`${findings.length} findings:`)
  const inline = findings.slice(0, INLINE_FINDING_LIMIT)
  inline.forEach((finding, index) => lines.push(formatFinding(finding, index + 1)))
  const remaining = findings.length - inline.length
  if (remaining > 0) {
    lines.push(`${remaining} more findings are not listed here.`)
    lines.push(`Full JSON: ${outputPath}`)
  }
  return lines.join("\n")
}

function inputFailure(error: unknown): OcrOutcome {
  const detail = error instanceof Error ? error.message : String(error)
  const message = error instanceof OcrInputError ? detail : `Invalid ocr arguments: ${detail}`
  return { ok: false, text: message, findings: [], exitCode: null }
}

export function unexpectedFailure(label: string, error: unknown): OcrOutcome {
  const detail = error instanceof Error ? error.message : String(error)
  return { ok: false, text: `${label} failed unexpectedly: ${oneLine(detail, RAW_OUTPUT_LIMIT)}`, findings: [], exitCode: null }
}

function errorCode(error: unknown): string {
  if (error === null || typeof error !== "object" || !("code" in error)) return ""
  const code = (error as { code?: unknown }).code
  return typeof code === "string" ? code : ""
}

function missingOcrText(): string {
  return [
    "ocr was not found on PATH.",
    `Install it with: ${OCR_INSTALL_COMMAND}`,
    "Then configure an OCR provider: ocr config provider, ocr config model, ocr llm test",
    "Delegation mode works without a configured provider.",
  ].join("\n")
}

function execFailureText(error: unknown): string {
  const code = errorCode(error)
  const message = error instanceof Error ? error.message : String(error)
  if (code === "ENOENT" || message.includes("ENOENT")) return missingOcrText()
  return `Could not run ocr: ${message}`
}

function failedRunText(label: string, exitCode: number, output: string): string {
  const detail = truncated(output.trim(), RAW_OUTPUT_LIMIT)
  const body = detail === "" ? "" : `\n${detail}`
  return `${label} failed (exit ${exitCode}).${body}`
}

function notJsonText(label: string, raw: string, exitCode: number): string {
  const body = raw.trim()
  return `${label} did not return JSON (exit ${exitCode}). Raw output:\n${body === "" ? "(no output)" : truncated(body, RAW_OUTPUT_LIMIT)}`
}

function truncatedText(label: string, exitCode: number): string {
  return `${label} output was truncated (exit ${exitCode}), so the JSON is incomplete and nothing was parsed. Narrow the selection and run it again.`
}

function stoppedText(label: string, exitCode: number): string {
  return `${label} was stopped before it finished (exit ${exitCode}): the call was cancelled or its timeout expired.`
}

function delegateVersionText(): string {
  return `ocr delegate --format json needs ocr ${DELEGATE_FORMAT_MIN_VERSION} or newer. Upgrade with: ${OCR_UPGRADE_COMMAND}`
}

async function discard(directory: string): Promise<void> {
  try {
    await rm(directory, { recursive: true, force: true })
  } catch {
    // Cleanup runs before every return, so a failure here must not replace the outcome it precedes.
  }
}

async function readOutput(path: string, fallback: string): Promise<string> {
  try {
    return await readFile(path, "utf8")
  } catch {
    return fallback
  }
}

export async function runReview(
  exec: ExecFn,
  params: ReviewParams,
  context: RunContext,
): Promise<OcrOutcome> {
  let plan: ReviewPlan
  try {
    plan = planReview(params)
  } catch (error) {
    return inputFailure(error)
  }

  const scratch = await mkdtemp(join(tmpdir(), "ocr-pi-review-"))
  const outputPath = join(scratch, "review.json")
  const args = reviewArgs(plan, params, { repoDir: context.cwd, outputPath })
  const timeout = timeoutMs(
    params.timeoutMinutes,
    params.preview === true ? DEFAULT_PREVIEW_TIMEOUT_MINUTES : DEFAULT_REVIEW_TIMEOUT_MINUTES,
  )

  let result: ExecResult
  try {
    result = await exec(OCR_BINARY, args, { cwd: context.cwd, signal: context.signal, timeout })
  } catch (error) {
    await discard(scratch)
    return { ok: false, text: execFailureText(error), findings: [], exitCode: null }
  }

  if (result.truncated === true) {
    await discard(scratch)
    return { ok: false, text: truncatedText("ocr review", result.code), findings: [], exitCode: result.code }
  }

  if (result.killed === true) {
    await discard(scratch)
    return { ok: false, text: stoppedText("ocr review", result.code), findings: [], exitCode: result.code }
  }

  if (result.code !== 0) {
    await discard(scratch)
    const stderr = (result.stderr ?? "").trim()
    const detail = stderr === "" ? (result.stdout ?? "") : stderr
    return {
      ok: false,
      text: failedRunText("ocr review", result.code, detail),
      findings: [],
      exitCode: result.code,
    }
  }

  const raw = await readOutput(outputPath, result.stdout ?? "")
  if (params.preview === true) {
    await discard(scratch)
    return { ok: true, text: raw.trim() || "No files changed.", findings: [], exitCode: result.code }
  }
  const payload = parseJsonObject(raw)
  if (payload === null) {
    await discard(scratch)
    return { ok: false, text: notJsonText("ocr review", raw, result.code), findings: [], exitCode: result.code }
  }

  const findings = extractFindings(payload)
  const analysis = params.analysis === true ? text(payload.analysis) : ""
  const analysisMissing = params.analysis === true && analysis === ""
  const analysisCut = analysis.length > RAW_OUTPUT_LIMIT
  let report = summarize(payload, findings, outputPath)
  if (analysis !== "") report += `\n\n${truncated(analysis, RAW_OUTPUT_LIMIT)}`
  if (analysisMissing) report += `\n\nAnalysis was requested but the report carries no analysis section.\nFull JSON: ${outputPath}`
  if (analysisCut) report += `\n\nAnalysis was cut at ${RAW_OUTPUT_LIMIT} characters.\nFull JSON: ${outputPath}`
  if (findings.length > INLINE_FINDING_LIMIT || analysisMissing || analysisCut) {
    return { ok: true, text: report, findings, exitCode: result.code, outputPath }
  }
  await discard(scratch)
  return { ok: true, text: report, findings, exitCode: result.code }
}

export async function runDelegate(
  exec: ExecFn,
  params: DelegateParams,
  context: RunContext,
): Promise<OcrOutcome> {
  let plan: DelegatePlan
  try {
    plan = planDelegate(params)
  } catch (error) {
    return inputFailure(error)
  }

  const action = plan.action
  const args = delegateArgs(plan, params, { repoDir: context.cwd })
  const timeout = timeoutMs(params.timeoutMinutes, DEFAULT_PREVIEW_TIMEOUT_MINUTES)

  let result: ExecResult
  try {
    result = await exec(OCR_BINARY, args, { cwd: context.cwd, signal: context.signal, timeout })
  } catch (error) {
    return { ok: false, text: execFailureText(error), findings: [], exitCode: null, action }
  }

  const stdout = result.stdout ?? ""
  const stderr = (result.stderr ?? "").trim()
  if (result.truncated === true) {
    return { ok: false, text: truncatedText("ocr delegate", result.code), findings: [], exitCode: result.code, action }
  }
  if (result.killed === true) {
    return { ok: false, text: stoppedText("ocr delegate", result.code), findings: [], exitCode: result.code, action }
  }
  if (result.code !== 0) {
    const hint = /unknown flag: --format/i.test(stderr) ? `\n\n${delegateVersionText()}` : ""
    const detail = stderr === "" ? stdout : stderr
    return {
      ok: false,
      text: `${failedRunText("ocr delegate", result.code, detail)}${hint}`,
      findings: [],
      exitCode: result.code,
      action,
    }
  }

  if (stdout.trim().length > RAW_OUTPUT_LIMIT) {
    const scratch = await mkdtemp(join(tmpdir(), "ocr-pi-delegate-"))
    const outputPath = join(scratch, "delegate.json")
    await writeFile(outputPath, stdout, "utf8")
    return {
      ok: true,
      text: `${truncated(stdout.trim(), PAYLOAD_HEAD_LIMIT)}\n\nFull JSON: ${outputPath}`,
      findings: [],
      exitCode: result.code,
      action,
      outputPath,
    }
  }

  const payload = parseJsonObject(stdout)
  if (payload === null) {
    return {
      ok: false,
      text: notJsonText("ocr delegate", stdout, result.code),
      findings: [],
      exitCode: result.code,
      action,
    }
  }

  return { ok: true, text: stdout.trim(), findings: [], exitCode: 0, action, data: payload }
}

export function tokenizeCommandArgs(raw: string): string[] {
  const tokens: string[] = []
  let current = ""
  let quote = ""
  let quoted = false
  for (const character of raw) {
    if (quote !== "") {
      if (character === quote) {
        quote = ""
        continue
      }
      current += character
      continue
    }
    if (character === '"' || character === "'") {
      quote = character
      quoted = true
      continue
    }
    if (/\s/.test(character)) {
      if (current !== "" || quoted) tokens.push(current)
      current = ""
      quoted = false
      continue
    }
    current += character
  }
  if (current !== "" || quoted) tokens.push(current)
  return tokens
}

interface TokenCursor {
  current: () => string
  value: () => string
  advance: () => void
  done: () => boolean
}

class CommandArgumentError extends Error {}

function timeoutValue(scan: TokenCursor): number {
  const typed = scan.value()
  const minutes = Number(typed)
  if (!Number.isFinite(minutes) || minutes <= 0) {
    throw new CommandArgumentError(`flag --timeout requires a positive number of minutes; "${typed}" is not one.`)
  }
  return minutes
}

export type CommandParse<T> = { kind: "params"; params: T } | { kind: "freeText" } | { kind: "invalid"; message: string }

function commandParse<T>(build: () => T | null): CommandParse<T> {
  try {
    const params = build()
    return params === null ? { kind: "freeText" } : { kind: "params", params }
  } catch (error) {
    if (!(error instanceof CommandArgumentError)) throw error
    return { kind: "invalid", message: error.message }
  }
}

function cursor(tokens: string[], start: number, example: string): TokenCursor {
  let index = start
  return {
    current: () => tokens[index] ?? "",
    value: () => {
      const flag = tokens[index] ?? ""
      const next = tokens[index + 1]
      if (next === undefined || next.startsWith("-")) {
        const found = next === undefined ? "" : `; "${next}" looks like a flag`
        throw new CommandArgumentError(`flag ${flag} requires a value${found}.\n${example}`)
      }
      index += 2
      return next
    },
    advance: () => {
      index += 1
    },
    done: () => index >= tokens.length,
  }
}

function reviewParams(tokens: string[]): ReviewParams | null {
  const params: ReviewParams = {}
  const scan = cursor(tokens, 0, "Example: /ocr-review --from main --to feature")
  while (!scan.done()) {
    switch (scan.current()) {
      case "review":
        scan.advance()
        break
      case "--from":
        params.from = scan.value()
        params.scope ??= "range"
        break
      case "--to":
        params.to = scan.value()
        params.scope ??= "range"
        break
      case "--commit":
      case "-c":
        params.commit = scan.value()
        params.scope ??= "commit"
        break
      case "--background":
      case "-b":
        params.background = scan.value()
        break
      case "--exclude":
        params.exclude = scan.value()
        break
      case "--timeout":
        params.timeoutMinutes = timeoutValue(scan)
        break
      case "--preview":
      case "-p":
        params.preview = true
        scan.advance()
        break
      case "--analysis":
        params.analysis = true
        scan.advance()
        break
      default:
        return null
    }
  }
  return params
}

export function parseReviewCommand(tokens: string[]): CommandParse<ReviewParams> {
  return commandParse(() => reviewParams(tokens))
}

function delegateParams(tokens: string[]): DelegateParams | null {
  const action = tokens[0] ?? ""
  if (action !== "preview" && action !== "rule") return null
  const params: DelegateParams = { action }
  const paths: string[] = []
  const scan = cursor(tokens, 1, "Example: /ocr-review-delegate preview --from main")
  while (!scan.done()) {
    switch (scan.current()) {
      case "--from":
        params.from = scan.value()
        break
      case "--to":
        params.to = scan.value()
        break
      case "--commit":
      case "-c":
        params.commit = scan.value()
        break
      case "--background":
      case "-b":
        params.background = scan.value()
        break
      case "--exclude":
        params.exclude = scan.value()
        break
      case "--timeout":
        params.timeoutMinutes = timeoutValue(scan)
        break
      default:
        if (scan.current().startsWith("-")) return null
        paths.push(scan.current())
        scan.advance()
    }
  }
  if (paths.length > 0) params.paths = paths
  return params
}

export function parseDelegateCommand(tokens: string[]): CommandParse<DelegateParams> {
  return commandParse(() => delegateParams(tokens))
}

export function reviewCommandPrompt(raw: string): string {
  const request = raw.trim() === "" ? "review the current workspace changes" : raw.trim()
  return [
    "Use the open-code-review skill to run a code review of this repository.",
    `The request is: ${request}`,
    "Prefer the ocr_review tool for a review hosted by the OCR-configured LLM, and ocr_delegate when the review must run with this session's own model.",
  ].join("\n\n")
}

export function delegateCommandPrompt(raw: string): string {
  const request = raw.trim() === "" ? "review the current workspace changes" : raw.trim()
  return [
    "Use the open-code-review skill to run a delegated code review: ocr selects the files and resolves the rules, and this session reviews them with its own model.",
    `The request is: ${request}`,
    "Use the ocr_delegate tool for file selection and rule resolution.",
  ].join("\n\n")
}
