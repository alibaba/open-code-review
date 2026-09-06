#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

// Contract tests for action.yml's timeout/configuration boundary.
//
// Run via: node scripts/github-actions/action-contract.test.js
//
// The action is a composite action, so these tests use a small YAML extractor
// and execute the real shell blocks with fake `ocr` and `npm` binaries. No
// YAML or test-runner dependency is required.

const assert = require("assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { spawnSync } = require("child_process");

const ROOT = path.resolve(__dirname, "../..");
const ACTION_PATH = path.join(ROOT, "action.yml");
const ACTION_TEXT = fs.readFileSync(ACTION_PATH, "utf8");
const CONTRACT_WORKFLOW_PATH = path.join(ROOT, ".github/workflows/action-contract.yml");
const CONTRACT_WORKFLOW_TEXT = fs.readFileSync(CONTRACT_WORKFLOW_PATH, "utf8");
const EXAMPLE_README_PATH = path.join(ROOT, "examples/github_actions/README.md");
const EXAMPLE_README_TEXT = fs.readFileSync(EXAMPLE_README_PATH, "utf8");

function parseScalar(raw) {
  const value = raw.trim();
  if (value.length >= 2 && value[0] === "'" && value[value.length - 1] === "'") {
    return value.slice(1, -1).replace(/''/g, "'");
  }
  if (value.length >= 2 && value[0] === '"' && value[value.length - 1] === '"') {
    try {
      return JSON.parse(value);
    } catch (_err) {
      return value.slice(1, -1);
    }
  }
  return value;
}

function parseInputs(text) {
  const lines = text.split(/\r?\n/);
  const start = lines.findIndex((line) => /^inputs:\s*$/.test(line));
  assert.ok(start >= 0, "action.yml must define inputs");
  const end = lines.findIndex((line, index) => index > start && /^(outputs|runs):\s*$/.test(line));
  const inputs = {};
  const limit = end >= 0 ? end : lines.length;

  for (let index = start + 1; index < limit; index += 1) {
    const match = /^  ([A-Za-z0-9_]+):\s*$/.exec(lines[index]);
    if (!match) continue;
    const name = match[1];
    let defaultValue;
    for (let cursor = index + 1; cursor < limit; cursor += 1) {
      if (/^  [A-Za-z0-9_]+:\s*$/.test(lines[cursor])) break;
      const defaultMatch = /^    default:\s*(.*)$/.exec(lines[cursor]);
      if (defaultMatch) defaultValue = parseScalar(defaultMatch[1]);
    }
    inputs[name] = { default: defaultValue };
  }
  return inputs;
}

function parseSteps(text) {
  const lines = text.split(/\r?\n/);
  const start = lines.findIndex((line) => /^  steps:\s*$/.test(line));
  assert.ok(start >= 0, "action.yml must define composite steps");
  const steps = [];
  let current;

  function finish() {
    if (!current) return;
    const rawLines = current.lines;
    const runDeclarations = rawLines
      .map((line, index) => (/^\s*run:\s*/.test(line) ? { line, index } : undefined))
      .filter(Boolean);
    assert.ok(runDeclarations.length <= 1, `step ${current.name} must not define multiple run blocks`);
    let run;
    if (runDeclarations.length === 1) {
      const { line, index: runMarker } = runDeclarations[0];
      assert.match(line, /^      run:\s*\|\s*$/, `step ${current.name} uses an unsupported run scalar`);
      const body = [];
      for (let index = runMarker + 1; index < rawLines.length; index += 1) {
        const line = rawLines[index];
        if (line.trim() === "") {
          body.push("");
        } else if (/^        /.test(line)) {
          body.push(line.slice(8));
        } else {
          break;
        }
      }
      run = body.join("\n");
    }

    const env = {};
    const envDeclarations = rawLines
      .map((line, index) => (/^\s*env:\s*$/.test(line) ? { line, index } : undefined))
      .filter(Boolean);
    assert.ok(envDeclarations.length <= 1, `step ${current.name} must not define multiple env blocks`);
    const envMarker = envDeclarations.length === 1 ? envDeclarations[0].index : -1;
    if (envMarker >= 0) {
      assert.match(
        envDeclarations[0].line,
        /^      env:\s*$/,
        `step ${current.name} uses unsupported env indentation`
      );
      for (let index = envMarker + 1; index < rawLines.length; index += 1) {
        const line = rawLines[index];
        if (line.trim() === "") continue;
        // A `#` comment is legal YAML inside a mapping block, and the env
        // blocks use them to record why a value is wired the way it is.
        if (/^\s*#/.test(line)) continue;
        if (/^      [A-Za-z_][A-Za-z0-9_-]*:\s*/.test(line)) break;
        const match = /^        ([A-Za-z_][A-Za-z0-9_]*):\s*(.*)$/.exec(line);
        assert.ok(match, `step ${current.name} contains an unsupported env value or indentation`);
        assert.doesNotMatch(
          match[2],
          /^(?:\||>|\|-|>-|\|\+|>\+)$/,
          `step ${current.name} contains an unsupported multiline env value`
        );
        env[match[1]] = parseScalar(match[2]);
      }
    }

    steps.push({ name: current.name, run, env, index: current.index });
    current = undefined;
  }

  for (let index = start + 1; index < lines.length; index += 1) {
    if (/^    -(?:\s|$)/.test(lines[index]) && !/^    - name:\s*/.test(lines[index])) {
      assert.fail(`unsupported nameless composite step at line ${index + 1}`);
    }
    if (/^\s*-\s+name:\s*/.test(lines[index]) && !/^    - name:\s*/.test(lines[index])) {
      assert.fail(`unsupported composite step indentation at line ${index + 1}`);
    }
    const match = /^    - name:\s*(.*)\s*$/.exec(lines[index]);
    if (match) {
      finish();
      current = { name: parseScalar(match[1]), lines: [lines[index]], index };
    } else if (current) {
      current.lines.push(lines[index]);
    }
  }
  finish();
  const names = steps.map((step) => step.name);
  assert.strictEqual(new Set(names).size, names.length, "composite step names must be unique");
  return steps;
}

const INPUTS = parseInputs(ACTION_TEXT);
const STEPS = parseSteps(ACTION_TEXT);

function inputValues(overrides = {}) {
  const values = {};
  for (const [name, definition] of Object.entries(INPUTS)) {
    if (definition.default !== undefined) values[name] = definition.default;
  }
  return Object.assign(values, overrides);
}

function resolveInputExpressions(value, values, stepOutputs = {}) {
  const resolved = String(value)
    .replace(/\$\{\{\s*inputs\.([A-Za-z0-9_]+)\s*\}\}/g, (_match, name) => {
      return values[name] === undefined ? "" : String(values[name]);
    })
    // A step output reads as "" when the step was skipped or never set it,
    // which is the case these contracts exercise; `stepOutputs` overrides it
    // where a test needs the range actually resolved. Other contexts
    // (github.*, env.*) stay unresolved and still fail closed below.
    .replace(/\$\{\{\s*steps\.([A-Za-z0-9_-]+)\.outputs\.([A-Za-z0-9_-]+)\s*\}\}/g, (_match, id, name) => {
      const value = (stepOutputs[id] || {})[name];
      return value === undefined ? "" : String(value);
    });
  assert.doesNotMatch(resolved, /\$\{\{[^}]+\}\}/, "unsupported or unresolved action expression");
  return resolved;
}

function renderedEnv(step, values, stepOutputs = {}) {
  const env = {};
  for (const [name, value] of Object.entries(step.env || {})) {
    env[name] = resolveInputExpressions(value, values, stepOutputs);
  }
  return env;
}

function renderedRun(step, values) {
  assert.ok(step && typeof step.run === "string", `step ${step ? step.name : "<missing>"} must have a bash run block`);
  return resolveInputExpressions(step.run, values);
}

function stepNamed(name) {
  return STEPS.find((step) => step.name === name);
}

function validationStep() {
  return STEPS.find((step) => {
    const haystack = `${step.name}\n${step.run || ""}`;
    return /review(?:[ _-]?task)?[ _-]?timeout/i.test(haystack);
  });
}

function installStep() {
  return STEPS.find((step) => /install\s+opencode|install\s+open.?code.?review/i.test(step.name));
}

function makeFixture() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "open-code-review-action-contract-"));
  const bin = path.join(dir, "bin");
  fs.mkdirSync(bin);
  const callsPath = path.join(dir, "calls.jsonl");
  const npmCallsPath = path.join(dir, "npm-calls.jsonl");
  const configPath = path.join(dir, "config.jsonl");
  const resultPath = path.join(dir, "ocr-result.json");
  const stderrPath = path.join(dir, "ocr-stderr.log");

  const ocrScript = `#!/usr/bin/env node
const fs = require("fs");
const args = process.argv.slice(2);
const env = {};
for (const name of ["OCR_LLM_TIMEOUT", "OCR_LLM_EXTRA_HEADERS", "REVIEW_TASK_TIMEOUT", "OCR_TIMEOUT"]) {
  if (process.env[name] !== undefined) env[name] = process.env[name];
}
const record = { args, env };
fs.appendFileSync(process.env.OCR_CALLS, JSON.stringify(record) + "\\n");
if (args[0] === "config" && args[1] === "set") {
  fs.appendFileSync(process.env.OCR_CONFIG, JSON.stringify(args) + "\\n");
  process.stdout.write("ocr config set " + args.slice(2).join(" ") + "\\n");
} else if (args[0] === "config" && args[1] === "unset") {
  fs.appendFileSync(process.env.OCR_CONFIG, JSON.stringify(args) + "\\n");
  process.stdout.write("ocr config unset " + args.slice(2).join(" ") + "\\n");
} else if (args[0] === "version") {
  const output = Object.prototype.hasOwnProperty.call(process.env, "OCR_FAKE_VERSION_OUTPUT")
    ? process.env.OCR_FAKE_VERSION_OUTPUT
    : "open-code-review 1.9.10 contract-test/fixture";
  process.stdout.write(output);
  if (output && !output.endsWith("\\n")) process.stdout.write("\\n");
  process.exit(Number(process.env.OCR_FAKE_VERSION_STATUS || 0));
} else if (args[0] === "review") {
  process.stdout.write(JSON.stringify({ comments: [], warnings: [] }));
} else {
  process.stdout.write("ocr " + args.join(" ") + "\\n");
}
`;
  const npmScript = `#!/usr/bin/env node
const fs = require("fs");
const args = process.argv.slice(2);
fs.appendFileSync(process.env.OCR_NPM_CALLS, JSON.stringify(args) + "\\n");
process.stdout.write("npm " + args.join(" ") + "\\n");
`;
  for (const [name, body] of [["ocr", ocrScript], ["npm", npmScript]]) {
    const file = path.join(bin, name);
    fs.writeFileSync(file, body, { mode: 0o755 });
  }

  return { dir, bin, callsPath, npmCallsPath, configPath, resultPath, stderrPath };
}

function removeFixture(fixture) {
  fs.rmSync(fixture.dir, { recursive: true, force: true });
}

function runShell(script, env, fixture) {
  const result = spawnSync("/bin/bash", ["--noprofile", "--norc", "-eo", "pipefail", "-c", script], {
    cwd: ROOT,
    env: Object.assign({}, process.env, {
      PATH: `${fixture.bin}:${process.env.PATH || ""}`,
      OCR_CALLS: fixture.callsPath,
      OCR_NPM_CALLS: fixture.npmCallsPath,
      OCR_CONFIG: fixture.configPath,
      GITHUB_OUTPUT: path.join(fixture.dir, "github-output"),
      GITHUB_ENV: path.join(fixture.dir, "github-env"),
    }, env),
    encoding: "utf8",
  });
  return result;
}

function runStep(step, values, fixture, extraEnv = {}, options = {}) {
  let script = renderedRun(step, values);
  if (options.replaceResultPaths) {
    script = script
      .replaceAll("/tmp/ocr-result.json", fixture.resultPath)
      .replaceAll("/tmp/ocr-stderr.log", fixture.stderrPath);
  }
  return runShell(script, Object.assign({}, renderedEnv(step, values), extraEnv), fixture);
}

function resultDescription(result) {
  return `status=${result.status}; stdout=${JSON.stringify(result.stdout)}; stderr=${JSON.stringify(result.stderr)}`;
}

function readJsonLines(file) {
  if (!fs.existsSync(file)) return [];
  return fs
    .readFileSync(file, "utf8")
    .split(/\r?\n/)
    .filter(Boolean)
    .map((line) => JSON.parse(line));
}

function readEnvAssignments(file) {
  const values = {};
  if (!fs.existsSync(file)) return values;
  for (const line of fs.readFileSync(file, "utf8").split(/\r?\n/)) {
    if (!line) continue;
    const separator = line.indexOf("=");
    if (separator < 1) continue;
    values[line.slice(0, separator)] = line.slice(separator + 1);
  }
  return values;
}

function configValues(calls) {
  const values = {};
  for (const args of calls) {
    if (!Array.isArray(args) || args.length < 2) continue;
    if (args[0] !== "config" || args[1] !== "set") continue;
    const key = args[2];
    const value = args[3];
    if (key) values[key] = value;
  }
  return values;
}

function configOperations(fixture) {
  return readJsonLines(fixture.configPath);
}

function allFixtureFiles(root) {
  const files = [];
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const fullPath = path.join(root, entry.name);
    if (entry.isDirectory()) files.push(...allFixtureFiles(fullPath));
    else files.push(fullPath);
  }
  return files;
}

function escapedRegExp(value) {
  return new RegExp(value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
}

function assertValidation(value, expectedValid) {
  const step = validationStep();
  assert.ok(step, "action.yml must add a validation step that references review_task_timeout");
  const fixture = makeFixture();
  try {
    const result = runStep(step, inputValues({ review_task_timeout: value }), fixture);
    if (expectedValid) {
      assert.strictEqual(result.status, 0, `review_task_timeout=${JSON.stringify(value)} should be accepted; ${resultDescription(result)}`);
    } else {
      assert.notStrictEqual(result.status, 0, `review_task_timeout=${JSON.stringify(value)} should be rejected; ${resultDescription(result)}`);
    }
  } finally {
    removeFixture(fixture);
  }
}

function testReviewTaskTimeoutInputNameAndScope() {
  assert.ok(INPUTS.review_task_timeout, "action.yml must define the review_task_timeout input");
  assert.ok(!INPUTS.review_timeout, "the not-yet-released review_timeout input must be renamed");
  assert.strictEqual(
    INPUTS.review_task_timeout.default,
    "15",
    "review_task_timeout default must remain 15 minutes"
  );
  const inputBlock = ACTION_TEXT.match(
    /^  review_task_timeout:\s*$([\s\S]*?)(?=^  [A-Za-z0-9_]+:\s*$|^outputs:|^runs:)/m
  );
  assert.ok(inputBlock, "action.yml must expose review_task_timeout metadata");
  assert.match(
    inputBlock[1],
    /per-(?:file|task)|concurrent task/i,
    "review_task_timeout must describe its per-file/per-task scope"
  );
  assert.doesNotMatch(
    inputBlock[1],
    /whole[- ]review|wall[- ]clock cap/i,
    "review_task_timeout must not promise a whole-review wall-clock cap"
  );
}

function testLlmTimeoutInputDefault() {
  assert.ok(INPUTS.llm_timeout, "action.yml must define the llm_timeout input");
  assert.strictEqual(INPUTS.llm_timeout.default, "300", "llm_timeout default must match the CLI's 5-minute timeout");
}

function testReviewTimeoutValidationAcceptsBoundaries() {
  for (const value of ["1", "10", "120"]) assertValidation(value, true);
}

function testReviewTimeoutValidationRejectsMalformedValues() {
  for (const value of ["", "1.5", "+10", "0", "-1", "-10", "121"]) {
    assertValidation(value, false);
  }
}

function testValidationPrecedesNpmInstall() {
  const validation = validationStep();
  const install = installStep();
  assert.ok(validation, "action.yml must add review_task_timeout validation before installation");
  assert.ok(install, "action.yml must retain an OpenCodeReview installation step");
  assert.ok(validation.index < install.index, "review_task_timeout validation must occur before NPM install");

  const fixture = makeFixture();
  try {
    const values = inputValues({ review_task_timeout: "121", ocr_version: "contract-test" });
    const script = `${renderedRun(validation, values)}\n${renderedRun(install, values)}`;
    const env = Object.assign({}, renderedEnv(validation, values), renderedEnv(install, values));
    const result = runShell(script, env, fixture);
    assert.notStrictEqual(result.status, 0, `invalid review_task_timeout must stop the action; ${resultDescription(result)}`);
    assert.deepStrictEqual(readJsonLines(fixture.npmCallsPath), [], "NPM must not run after timeout validation fails");
  } finally {
    removeFixture(fixture);
  }
}

function testReviewTimeoutForwardedSeparatelyFromLlmTimeout() {
  const run = stepNamed("Run OpenCodeReview");
  assert.ok(run, "action.yml must retain the Run OpenCodeReview step");
  const fixture = makeFixture();
  try {
    const values = inputValues({
      review_task_timeout: "10",
      llm_timeout: "900",
      llm_url: "https://llm.example.invalid/v1",
      llm_auth_token: "unused-token",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
    });
    const result = runStep(
      run,
      values,
      fixture,
      { MERGE_BASE: "base-sha", HEAD_SHA: "head-sha", REVIEW_TASK_TIMEOUT: values.review_task_timeout },
      { replaceResultPaths: true }
    );
    assert.strictEqual(result.status, 0, `Run OpenCodeReview shell block failed; ${resultDescription(result)}`);
    const reviewCall = readJsonLines(fixture.callsPath).find((call) => call.args[0] === "review");
    assert.ok(reviewCall, "Run OpenCodeReview must invoke `ocr review`");
    const timeoutIndices = reviewCall.args
      .map((arg, index) => (arg === "--timeout" ? index : -1))
      .filter((index) => index >= 0);
    assert.strictEqual(
      timeoutIndices.length,
      1,
      `review timeout must be forwarded exactly once; args=${JSON.stringify(reviewCall.args)}`
    );
    assert.strictEqual(reviewCall.args[timeoutIndices[0] + 1], "10", "review_task_timeout must be passed as --timeout");
    assert.strictEqual(reviewCall.env.OCR_LLM_TIMEOUT, "900", "llm_timeout must remain the LLM request timeout");
    assert.notStrictEqual(
      reviewCall.args[timeoutIndices[0] + 1],
      reviewCall.env.OCR_LLM_TIMEOUT,
      "review_task_timeout and llm_timeout must remain distinct settings"
    );
  } finally {
    removeFixture(fixture);
  }
}

function testDefaultLlmTimeoutExportedSeparatelyFromReviewTimeout() {
  const run = stepNamed("Run OpenCodeReview");
  assert.ok(run, "action.yml must retain the Run OpenCodeReview step");
  const fixture = makeFixture();
  try {
    const values = inputValues({
      review_task_timeout: "10",
      llm_url: "https://llm.example.invalid/v1",
      llm_auth_token: "unused-token",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
    });
    const result = runStep(
      run,
      values,
      fixture,
      { MERGE_BASE: "base-sha", HEAD_SHA: "head-sha", REVIEW_TASK_TIMEOUT: values.review_task_timeout },
      { replaceResultPaths: true }
    );
    assert.strictEqual(result.status, 0, `Run OpenCodeReview shell block failed; ${resultDescription(result)}`);
    const reviewCall = readJsonLines(fixture.callsPath).find((call) => call.args[0] === "review");
    assert.ok(reviewCall, "Run OpenCodeReview must invoke `ocr review`");
    const timeoutIndex = reviewCall.args.indexOf("--timeout");
    assert.ok(timeoutIndex >= 0, "Run OpenCodeReview must forward review_task_timeout");
    assert.strictEqual(reviewCall.args[timeoutIndex + 1], "10");
    assert.strictEqual(reviewCall.env.OCR_LLM_TIMEOUT, "300");
    assert.notStrictEqual(reviewCall.args[timeoutIndex + 1], reviewCall.env.OCR_LLM_TIMEOUT);
  } finally {
    removeFixture(fixture);
  }
}

function testEmptyLlmTimeoutNormalizesBeforeReviewInvocation() {
  const run = stepNamed("Run OpenCodeReview");
  assert.ok(run, "action.yml must retain the Run OpenCodeReview step");
  const fixture = makeFixture();
  try {
    const values = inputValues({
      review_task_timeout: "10",
      llm_timeout: "",
      llm_url: "https://llm.example.invalid/v1",
      llm_auth_token: "unused-token",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
    });
    const result = runStep(
      run,
      values,
      fixture,
      { MERGE_BASE: "base-sha", HEAD_SHA: "head-sha", REVIEW_TASK_TIMEOUT: values.review_task_timeout },
      { replaceResultPaths: true }
    );
    assert.strictEqual(result.status, 0, `Run OpenCodeReview shell block failed; ${resultDescription(result)}`);
    const reviewCall = readJsonLines(fixture.callsPath).find((call) => call.args[0] === "review");
    assert.ok(reviewCall, "Run OpenCodeReview must invoke `ocr review`");
    const timeoutIndex = reviewCall.args.indexOf("--timeout");
    assert.strictEqual(reviewCall.env.OCR_LLM_TIMEOUT, "300");
    assert.strictEqual(reviewCall.args[timeoutIndex + 1], "10");
    assert.notStrictEqual(reviewCall.args[timeoutIndex + 1], reviewCall.env.OCR_LLM_TIMEOUT);
  } finally {
    removeFixture(fixture);
  }
}

function testReviewTimeoutLeadingZeroIsNormalizedAcrossSteps() {
  const validation = validationStep();
  const run = stepNamed("Run OpenCodeReview");
  assert.ok(validation, "action.yml must retain review_task_timeout validation");
  assert.ok(run, "action.yml must retain the Run OpenCodeReview step");
  const fixture = makeFixture();
  try {
    const values = inputValues({
      review_task_timeout: "010",
      llm_url: "https://llm.example.invalid/v1",
      llm_auth_token: "unused-token",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
    });
    const validationResult = runStep(validation, values, fixture);
    assert.strictEqual(
      validationResult.status,
      0,
      `review_task_timeout=010 validation failed; ${resultDescription(validationResult)}`
    );
    const exported = readEnvAssignments(path.join(fixture.dir, "github-env"));
    assert.strictEqual(exported.REVIEW_TASK_TIMEOUT, "10", "validation must export normalized decimal review_task_timeout");
    assert.ok(
      !Object.prototype.hasOwnProperty.call(run.env, "REVIEW_TASK_TIMEOUT"),
      "Run OpenCodeReview must consume the normalized GITHUB_ENV value, not rebind the raw input"
    );
    const result = runStep(
      run,
      values,
      fixture,
      { MERGE_BASE: "base-sha", HEAD_SHA: "head-sha", REVIEW_TASK_TIMEOUT: exported.REVIEW_TASK_TIMEOUT },
      { replaceResultPaths: true }
    );
    assert.strictEqual(result.status, 0, `Run OpenCodeReview shell block failed; ${resultDescription(result)}`);
    const reviewCall = readJsonLines(fixture.callsPath).find((call) => call.args[0] === "review");
    assert.ok(reviewCall, "Run OpenCodeReview must invoke `ocr review`");
    const timeoutIndex = reviewCall.args.indexOf("--timeout");
    assert.strictEqual(reviewCall.args[timeoutIndex + 1], "10");
  } finally {
    removeFixture(fixture);
  }
}

function testConfigureBuildsCompleteLlmConfig() {
  const configure = stepNamed("Configure OCR");
  assert.ok(configure, "action.yml must retain the Configure OCR step");
  const fixture = makeFixture();
  try {
    const extraBody = '{"thinking":{"type":"disabled"},"contract":"yes"}';
    const values = inputValues({
      llm_url: "https://llm.example.invalid/v1",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
      llm_auth_token: "configure-token-sentinel",
      llm_extra_body: extraBody,
      language: "English",
    });
    const result = runStep(configure, values, fixture, { OCR_LLM_TOKEN: values.llm_auth_token });
    assert.strictEqual(result.status, 0, `Configure OCR shell block failed; ${resultDescription(result)}`);
    const configured = configValues(readJsonLines(fixture.configPath));
    assert.strictEqual(configured["llm.url"], values.llm_url);
    assert.strictEqual(configured["llm.model"], values.llm_model);
    assert.strictEqual(configured["llm.use_anthropic"], values.llm_use_anthropic);
    assert.strictEqual(typeof configured["llm.auth_token_cmd"], "string", "llm.auth_token_cmd must be configured");
    assert.match(configured["llm.auth_token_cmd"], /OCR_LLM_TOKEN/, "auth_token_cmd must reference the token env var");
    const configuredExtraBody =
      typeof configured["llm.extra_body"] === "string"
        ? JSON.parse(configured["llm.extra_body"])
        : configured["llm.extra_body"];
    assert.deepStrictEqual(configuredExtraBody, JSON.parse(extraBody));
  } finally {
    removeFixture(fixture);
  }
}

function testConfigureNeverPersistsToken() {
  const configure = stepNamed("Configure OCR");
  assert.ok(configure, "action.yml must retain the Configure OCR step");
  const token = "configure-token-sentinel-DO-NOT-PERSIST";
  const fixture = makeFixture();
  try {
    const values = inputValues({
      llm_url: "https://llm.example.invalid/v1",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
      llm_auth_token: token,
      llm_extra_body: '{"thinking":{"type":"disabled"}}',
    });
    const result = runStep(configure, values, fixture, { OCR_LLM_TOKEN: token });
    assert.strictEqual(result.status, 0, `Configure OCR shell block failed; ${resultDescription(result)}`);
    const outputAndFiles = [result.stdout, result.stderr];
    for (const file of allFixtureFiles(fixture.dir)) outputAndFiles.push(fs.readFileSync(file, "utf8"));
    const leaked = outputAndFiles.join("\n");
    assert.doesNotMatch(leaked, escapedRegExp(token), "the token must not appear in config output or files");

    const configured = configValues(readJsonLines(fixture.configPath));
    assert.ok(configured["llm.auth_token_cmd"], "only an auth_token_cmd should be stored for the token");
    assert.match(configured["llm.auth_token_cmd"], /OCR_LLM_TOKEN/);
    assert.doesNotMatch(configured["llm.auth_token_cmd"], escapedRegExp(token));
  } finally {
    removeFixture(fixture);
  }
}

function testConfigureNeutralizesStaleProviderAndStaticToken() {
  const configure = stepNamed("Configure OCR");
  assert.ok(configure, "action.yml must retain the Configure OCR step");
  const fixture = makeFixture();
  try {
    const values = inputValues({
      llm_url: "https://llm.example.invalid/v1",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
      llm_auth_token: "config-token-sentinel",
      llm_auth_header: "x-api-key",
      llm_extra_body: '{"thinking":{"type":"disabled"}}',
    });
    const result = runStep(configure, values, fixture);
    assert.strictEqual(result.status, 0, `Configure OCR shell block failed; ${resultDescription(result)}`);
    const operations = configOperations(fixture);
    const unsetProviderIndex = operations.findIndex(
      (args) => args[0] === "config" && args[1] === "unset" && args[2] === "provider"
    );
    const clearStaticTokenIndex = operations.findIndex(
      (args) => args[0] === "config" && args[1] === "set" && args[2] === "llm.auth_token" && args[3] === ""
    );
    const setTokenCmdIndex = operations.findIndex(
      (args) => args[0] === "config" && args[1] === "set" && args[2] === "llm.auth_token_cmd"
    );
    const firstLegacySetIndex = operations.findIndex(
      (args) => args[0] === "config" && args[1] === "set" && String(args[2]).startsWith("llm.")
    );
    assert.ok(unsetProviderIndex >= 0, "Configure OCR must unset a stale active provider");
    assert.ok(clearStaticTokenIndex >= 0, "Configure OCR must clear stale static llm.auth_token");
    assert.ok(setTokenCmdIndex >= 0, "Configure OCR must configure auth_token_cmd");
    assert.ok(unsetProviderIndex < firstLegacySetIndex, "provider must be unset before legacy llm config is built");
    assert.ok(clearStaticTokenIndex < setTokenCmdIndex, "static token must be cleared before auth_token_cmd is set");

    const configured = configValues(operations);
    assert.strictEqual(configured["llm.auth_header"], "x-api-key", "custom auth header must be stored in llm config");
    assert.strictEqual(configured["llm.protocol"], "openai", "false llm_use_anthropic must set the OpenAI protocol");
  } finally {
    removeFixture(fixture);
  }
}

function testConfigureProtocolTracksUseAnthropic() {
  const configure = stepNamed("Configure OCR");
  assert.ok(configure, "action.yml must retain the Configure OCR step");
  for (const [useAnthropic, expectedProtocol] of [["true", "anthropic"], ["false", "openai"]]) {
    const fixture = makeFixture();
    try {
      const values = inputValues({
        llm_url: "https://llm.example.invalid/v1",
        llm_model: "contract-model",
        llm_use_anthropic: useAnthropic,
        llm_auth_token: "protocol-token-sentinel",
      });
      const result = runStep(configure, values, fixture);
      assert.strictEqual(result.status, 0, `Configure OCR failed for llm_use_anthropic=${useAnthropic}; ${resultDescription(result)}`);
      const configured = configValues(configOperations(fixture));
      assert.strictEqual(configured["llm.use_anthropic"], useAnthropic);
      assert.strictEqual(configured["llm.protocol"], expectedProtocol);
    } finally {
      removeFixture(fixture);
    }
  }
}

function testConfigurePreservesLegacyUseAnthropicResolution() {
  const configure = stepNamed("Configure OCR");
  assert.ok(configure, "action.yml must retain the Configure OCR step");
  const inputBlock = ACTION_TEXT.match(
    /^  llm_use_anthropic:\s*$([\s\S]*?)(?=^  [A-Za-z0-9_]+:\s*$|^outputs:|^runs:)/m
  );
  assert.ok(inputBlock, "action.yml must expose llm_use_anthropic metadata");
  assert.match(
    inputBlock[1],
    /explicitly\s+supplied\s+empty\s+string/i,
    "llm_use_anthropic docs must distinguish an explicit empty value from omitting the required input"
  );
  const cases = [
    ["true", "true", "anthropic"],
    ["TRUE", "true", "anthropic"],
    ["1", "true", "anthropic"],
    ["yes", "true", "anthropic"],
    ["YeS", "true", "anthropic"],
    ["", "true", "anthropic"],
    ["false", "false", "openai"],
    ["FALSE", "false", "openai"],
    ["0", "false", "openai"],
    ["no", "false", "openai"],
    ["unexpected", "false", "openai"],
  ];
  for (const [value, expectedBoolean, expectedProtocol] of cases) {
    const fixture = makeFixture();
    try {
      const values = inputValues({
        llm_url: "https://llm.example.invalid/v1",
        llm_model: "contract-model",
        llm_use_anthropic: value,
        llm_auth_token: "legacy-boolean-token-sentinel",
      });
      const result = runStep(configure, values, fixture);
      assert.strictEqual(
        result.status,
        0,
        `legacy llm_use_anthropic=${JSON.stringify(value)} must remain accepted; ${resultDescription(result)}`
      );
      const configured = configValues(configOperations(fixture));
      assert.strictEqual(configured["llm.use_anthropic"], expectedBoolean);
      assert.strictEqual(configured["llm.protocol"], expectedProtocol);
    } finally {
      removeFixture(fixture);
    }
  }
}

function testConfigureClearsStaleExtraHeadersBeforeTokenCommand() {
  const configure = stepNamed("Configure OCR");
  assert.ok(configure, "action.yml must retain the Configure OCR step");
  const fixture = makeFixture();
  try {
    const values = inputValues({
      llm_url: "https://llm.example.invalid/v1",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
      llm_auth_token: "extra-header-token-sentinel",
      llm_extra_headers: "X-Request-ID=contract-value",
    });
    const result = runStep(configure, values, fixture);
    assert.strictEqual(result.status, 0, `Configure OCR shell block failed; ${resultDescription(result)}`);
    const operations = configOperations(fixture);
    const unsetProviderIndex = operations.findIndex(
      (args) => args[0] === "config" && args[1] === "unset" && args[2] === "provider"
    );
    const clearExtraHeadersIndex = operations.findIndex(
      (args) => args[0] === "config" && args[1] === "set" && args[2] === "llm.extra_headers" && args[3] === ""
    );
    const setTokenCmdIndex = operations.findIndex(
      (args) => args[0] === "config" && args[1] === "set" && args[2] === "llm.auth_token_cmd"
    );
    assert.ok(clearExtraHeadersIndex >= 0, "Configure OCR must clear stale persisted llm.extra_headers");
    assert.ok(unsetProviderIndex < clearExtraHeadersIndex, "provider must be unset before legacy extra headers are cleared");
    assert.ok(clearExtraHeadersIndex < setTokenCmdIndex, "stale extra headers must be cleared before auth_token_cmd is set");
    assert.strictEqual(configValues(operations)["llm.extra_headers"], "");
  } finally {
    removeFixture(fixture);
  }
}

function testConfigureClearsStaleRetryCodesBeforeEndpointConfig() {
  const configure = stepNamed("Configure OCR");
  assert.ok(configure, "action.yml must retain the Configure OCR step");
  const fixture = makeFixture();
  try {
    const values = inputValues({
      llm_url: "https://llm.example.invalid/v1",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
      llm_auth_token: "retry-code-token-sentinel",
    });
    const result = runStep(configure, values, fixture);
    assert.strictEqual(result.status, 0, `Configure OCR shell block failed; ${resultDescription(result)}`);
    const operations = configOperations(fixture);
    const unsetProviderIndex = operations.findIndex(
      (args) => args[0] === "config" && args[1] === "unset" && args[2] === "provider"
    );
    const clearRetryCodesIndex = operations.findIndex(
      (args) => args[0] === "config" && args[1] === "set" && args[2] === "llm.retry_codes" && args[3] === ""
    );
    const setUrlIndex = operations.findIndex(
      (args) => args[0] === "config" && args[1] === "set" && args[2] === "llm.url"
    );
    assert.ok(clearRetryCodesIndex >= 0, "Configure OCR must clear stale persisted llm.retry_codes");
    assert.ok(unsetProviderIndex < clearRetryCodesIndex, "provider must be unset before retry codes are cleared");
    assert.ok(clearRetryCodesIndex < setUrlIndex, "stale retry codes must be cleared before endpoint config is built");
    assert.strictEqual(configValues(operations)["llm.retry_codes"], "");
  } finally {
    removeFixture(fixture);
  }
}

function testRunRetainsExtraHeadersEnvironmentOverride() {
  const run = stepNamed("Run OpenCodeReview");
  assert.ok(run, "action.yml must retain the Run OpenCodeReview step");
  const fixture = makeFixture();
  try {
    const values = inputValues({
      review_task_timeout: "10",
      llm_url: "https://llm.example.invalid/v1",
      llm_auth_token: "unused-token",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
      llm_extra_headers: "X-Request-ID=contract-value",
    });
    const result = runStep(
      run,
      values,
      fixture,
      { MERGE_BASE: "base-sha", HEAD_SHA: "head-sha", REVIEW_TASK_TIMEOUT: values.review_task_timeout },
      { replaceResultPaths: true }
    );
    assert.strictEqual(result.status, 0, `Run OpenCodeReview shell block failed; ${resultDescription(result)}`);
    const reviewCall = readJsonLines(fixture.callsPath).find((call) => call.args[0] === "review");
    assert.ok(reviewCall, "Run OpenCodeReview must invoke `ocr review`");
    assert.strictEqual(reviewCall.env.OCR_LLM_EXTRA_HEADERS, values.llm_extra_headers);
  } finally {
    removeFixture(fixture);
  }
}

function testRunFailsClosedWhenValidatedTaskTimeoutIsMissing() {
  const run = stepNamed("Run OpenCodeReview");
  assert.ok(run, "action.yml must retain the Run OpenCodeReview step");
  const fixture = makeFixture();
  try {
    const values = inputValues({
      llm_url: "https://llm.example.invalid/v1",
      llm_auth_token: "unused-token",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
    });
    const result = runStep(
      run,
      values,
      fixture,
      { MERGE_BASE: "base-sha", HEAD_SHA: "head-sha", REVIEW_TASK_TIMEOUT: "" },
      { replaceResultPaths: true }
    );
    assert.notStrictEqual(result.status, 0, "Run OpenCodeReview must fail when validated timeout state is missing");
    assert.match(
      `${result.stdout}\n${result.stderr}`,
      /validated review_task_timeout is missing/i,
      "missing timeout failure must identify the validation boundary"
    );
    assert.ok(
      !readJsonLines(fixture.callsPath).some((call) => call.args[0] === "review"),
      "Run OpenCodeReview must fail before invoking OCR"
    );
  } finally {
    removeFixture(fixture);
  }
}

// --- effort / max_tokens_budget ---
//
// Both are optional passthroughs to `ocr review`. "Unset" has to stay
// distinguishable from "set to something": appending the flag with an empty
// value would not restore the CLI default, it would hand the CLI an empty
// argument.

function effortAndBudgetValidationStep() {
  return stepNamed("Validate effort and max_tokens_budget");
}

// The Run step needs a validated timeout and a merge base in the environment;
// these tests care only about the argv it builds from there.
function reviewCallWith(overrides) {
  const run = stepNamed("Run OpenCodeReview");
  assert.ok(run, "action.yml must retain the Run OpenCodeReview step");
  const fixture = makeFixture();
  try {
    const values = inputValues(
      Object.assign(
        {
          review_task_timeout: "10",
          llm_url: "https://llm.example.invalid/v1",
          llm_auth_token: "unused-token",
          llm_model: "contract-model",
          llm_use_anthropic: "false",
        },
        overrides
      )
    );
    const result = runStep(
      run,
      values,
      fixture,
      { MERGE_BASE: "base-sha", HEAD_SHA: "head-sha", REVIEW_TASK_TIMEOUT: values.review_task_timeout },
      { replaceResultPaths: true }
    );
    assert.strictEqual(result.status, 0, `Run OpenCodeReview shell block failed; ${resultDescription(result)}`);
    const reviewCall = readJsonLines(fixture.callsPath).find((call) => call.args[0] === "review");
    assert.ok(reviewCall, "Run OpenCodeReview must invoke `ocr review`");
    return reviewCall;
  } finally {
    removeFixture(fixture);
  }
}

// Every value forwarded for `flag`, so a test can tell "absent" from "present
// once" from "duplicated" instead of only asserting inclusion.
function flagValues(args, flag) {
  const values = [];
  for (let index = 0; index < args.length; index += 1) {
    if (args[index] === flag) values.push(args[index + 1]);
  }
  return values;
}

function assertTuningValidation(overrides, expectedValid, expectedMessage) {
  const step = effortAndBudgetValidationStep();
  assert.ok(step, "action.yml must validate effort and max_tokens_budget in a dedicated step");
  const fixture = makeFixture();
  try {
    const result = runStep(step, inputValues(overrides), fixture);
    const description = `${JSON.stringify(overrides)}; ${resultDescription(result)}`;
    if (expectedValid) {
      assert.strictEqual(result.status, 0, `value should be accepted; ${description}`);
    } else {
      assert.notStrictEqual(result.status, 0, `value should be rejected; ${description}`);
      assert.match(
        `${result.stdout}\n${result.stderr}`,
        expectedMessage,
        `the rejection must name the input and its accepted values; ${description}`
      );
    }
  } finally {
    removeFixture(fixture);
  }
}

function testEffortAndBudgetInputsAreOptional() {
  for (const name of ["effort", "max_tokens_budget"]) {
    assert.ok(INPUTS[name], `action.yml must define the ${name} input`);
    assert.strictEqual(
      INPUTS[name].default,
      undefined,
      `${name} must carry no default, so leaving it unset keeps the CLI's own default`
    );
  }
}

function testUnsetEffortAndBudgetAppendNoFlags() {
  const reviewCall = reviewCallWith({});
  for (const flag of ["--effort", "--max-tokens-budget"]) {
    assert.deepStrictEqual(
      flagValues(reviewCall.args, flag),
      [],
      `an unset input must not append ${flag}; args=${JSON.stringify(reviewCall.args)}`
    );
  }
}

function testEffortAndBudgetForwardedToReviewCommand() {
  const reviewCall = reviewCallWith({ effort: "high", max_tokens_budget: "120000" });
  assert.deepStrictEqual(
    flagValues(reviewCall.args, "--effort"),
    ["high"],
    `effort must be forwarded once as --effort; args=${JSON.stringify(reviewCall.args)}`
  );
  assert.deepStrictEqual(
    flagValues(reviewCall.args, "--max-tokens-budget"),
    ["120000"],
    `max_tokens_budget must be forwarded once as --max-tokens-budget; args=${JSON.stringify(reviewCall.args)}`
  );
}

// 0 is a real value, not a synonym for unset: the CLI documents it as
// "unlimited", so it is forwarded rather than dropped, and the two paths agree
// on what the review does.
function testZeroBudgetIsForwardedVerbatim() {
  const reviewCall = reviewCallWith({ max_tokens_budget: "0" });
  assert.deepStrictEqual(
    flagValues(reviewCall.args, "--max-tokens-budget"),
    ["0"],
    `an explicit 0 budget must reach the CLI; args=${JSON.stringify(reviewCall.args)}`
  );
}

function testEffortValidationAcceptsThePresets() {
  // Upper case included: `ocr review` lower-cases the preset itself, so the
  // action must not be stricter than the flag it forwards to.
  for (const value of ["", "low", "medium", "high", "HIGH", "Medium"]) {
    assertTuningValidation({ effort: value }, true);
  }
}

function testEffortValidationRejectsUnknownPresets() {
  for (const value of ["extreme", "lowest", "low medium", " low", "2"]) {
    assertTuningValidation({ effort: value }, false, /effort must be one of low, medium or high/);
  }
}

function testBudgetValidationAcceptsNonNegativeIntegers() {
  for (const value of ["", "0", "1", "120000"]) {
    assertTuningValidation({ max_tokens_budget: value }, true);
  }
}

function testBudgetValidationRejectsMalformedValues() {
  // "010" is not a typo in this list: the CLI parses integer flags with a
  // base-0 strconv, so a leading zero would silently mean octal 8.
  for (const value of ["-1", "-120000", "1.5", "1e6", "+10", "abc", "12 000", "010"]) {
    assertTuningValidation(
      { max_tokens_budget: value },
      false,
      /max_tokens_budget must be a non-negative base-10 integer/
    );
  }
}

function testEffortAndBudgetValidationPrecedesNpmInstall() {
  const validation = effortAndBudgetValidationStep();
  const install = installStep();
  assert.ok(validation, "action.yml must validate effort and max_tokens_budget before installation");
  assert.ok(install, "action.yml must retain an OpenCodeReview installation step");
  assert.ok(validation.index < install.index, "effort/max_tokens_budget validation must occur before NPM install");

  const fixture = makeFixture();
  try {
    const values = inputValues({ effort: "extreme", ocr_version: "contract-test" });
    const script = `${renderedRun(validation, values)}\n${renderedRun(install, values)}`;
    const env = Object.assign({}, renderedEnv(validation, values), renderedEnv(install, values));
    const result = runShell(script, env, fixture);
    assert.notStrictEqual(result.status, 0, `an invalid effort must stop the action; ${resultDescription(result)}`);
    assert.deepStrictEqual(readJsonLines(fixture.npmCallsPath), [], "NPM must not run after effort validation fails");
  } finally {
    removeFixture(fixture);
  }
}

// The effort preset changes how many review rounds run, so a checkpoint
// recorded by a shallower run must not narrow a later, deeper one.
function testEffortParticipatesInTheCheckpointFingerprint() {
  const resolve = stepNamed("Resolve review range");
  assert.ok(resolve, "action.yml must retain the Resolve review range step");
  assert.ok(
    Object.prototype.hasOwnProperty.call(resolve.env, "OCR_FP_EFFORT"),
    "Resolve review range must feed effort into the configuration fingerprint"
  );
  assert.match(
    ACTION_TEXT,
    /process\.env\.OCR_FP_EFFORT,/,
    "the fingerprint digest must actually read OCR_FP_EFFORT"
  );
}

function testOfficialNpmPackageInstallIsPreserved() {
  const install = installStep();
  assert.ok(install, "action.yml must retain the Install OpenCodeReview step");
  const fixture = makeFixture();
  try {
    const values = inputValues({ ocr_version: "1.9.10" });
    const result = runStep(install, values, fixture);
    assert.strictEqual(result.status, 0, `Install OpenCodeReview shell block failed; ${resultDescription(result)}`);
    const npmCall = readJsonLines(fixture.npmCallsPath).find((args) => args[0] === "install");
    assert.ok(npmCall, "Install OpenCodeReview must invoke npm install");
    assert.deepStrictEqual(npmCall.slice(0, 2), ["install", "-g"]);
    assert.strictEqual(npmCall[2], "@alibaba-group/open-code-review@1.9.10");
  } finally {
    removeFixture(fixture);
  }
}

function testInstallEnforcesAuthTokenCommandVersionFloor() {
  const install = installStep();
  assert.ok(install, "action.yml must retain the Install OpenCodeReview step");
  const inputBlock = ACTION_TEXT.match(
    /^  ocr_version:\s*$([\s\S]*?)(?=^  [A-Za-z0-9_]+:\s*$|^outputs:|^runs:)/m
  );
  assert.ok(inputBlock, "action.yml must expose ocr_version metadata");
  assert.match(inputBlock[1], /1\.9\.6/, "ocr_version must document the minimum compatible OCR release");

  const cases = [
    { output: "open-code-review 1.9.5 linux/amd64", valid: false },
    { output: "open-code-review 1.9.6 linux/amd64", valid: true },
    { output: "open-code-review v1.9.10 linux/amd64", valid: true },
    { output: "open-code-review 1.10.0 linux/amd64", valid: true },
    { output: "open-code-review 1.9.6+brokerkit.1 linux/amd64", valid: true },
    { output: "open-code-review 1.9.6-rc.1 linux/amd64", valid: false },
    { output: "open-code-review 01.09.006 linux/amd64", valid: false },
    { output: "open-code-review 1.9.6+! linux/amd64", valid: false },
    { output: "version unavailable", valid: false },
    { output: "", valid: false },
    { output: "version command unavailable", status: 7, valid: false, commandFailure: true },
  ];

  for (const testCase of cases) {
    const fixture = makeFixture();
    try {
      const result = runStep(
        install,
        inputValues({ ocr_version: "contract-test" }),
        fixture,
        {
          OCR_FAKE_VERSION_OUTPUT: testCase.output,
          OCR_FAKE_VERSION_STATUS: String(testCase.status || 0),
        }
      );
      const message = `version output ${JSON.stringify(testCase.output)}; ${resultDescription(result)}`;
      if (testCase.valid) assert.strictEqual(result.status, 0, `supported ${message}`);
      else assert.notStrictEqual(result.status, 0, `unsupported ${message}`);
      if (testCase.commandFailure) {
        assert.match(
          result.stdout,
          /Unable to read the installed OpenCodeReview version/,
          `failed version command must produce an actionable error; ${message}`
        );
      }
    } finally {
      removeFixture(fixture);
    }
  }
}

function testContractHarnessFailsClosedOnUnsupportedYamlShapes() {
  assert.throws(
    () => parseSteps(ACTION_TEXT.replace("    - name: Configure OCR", "   - name: Configure OCR")),
    /unsupported|step/i,
    "changed step indentation must fail instead of silently dropping a step"
  );
  assert.throws(
    () =>
      parseSteps(
        ACTION_TEXT.replace(
          "    - name: Configure OCR",
          "    - uses: example/action@v1\n\n    - name: Configure OCR"
        )
      ),
    /unsupported|step/i,
    "nameless composite steps must fail instead of being appended to the previous named step"
  );
  assert.throws(
    () =>
      parseSteps(
        ACTION_TEXT.replace(
          "    - name: Configure OCR",
          "    -\n      uses: example/action@v1\n\n    - name: Configure OCR"
        )
      ),
    /unsupported|step/i,
    "block-style nameless composite steps must fail instead of being appended to the previous step"
  );
  assert.throws(
    () =>
      parseSteps(
        ACTION_TEXT.replace(
          "      run: |\n        if command -v git",
          "      run: >-\n        if command -v git"
        )
      ),
    /unsupported|run/i,
    "unsupported run scalar styles must fail instead of producing an undefined shell block"
  );
  assert.throws(
    () =>
      parseSteps(
        ACTION_TEXT.replace(
          "        OCR_EXTRA_BODY: ${{ inputs.llm_extra_body }}",
          "        OCR_EXTRA_BODY: >-\n          ${{ inputs.llm_extra_body }}"
        )
      ),
    /unsupported|env/i,
    "multiline env values must fail instead of truncating the environment map"
  );
  assert.throws(
    () => parseSteps(ACTION_TEXT.replace("    - name: Configure OCR", "    - name: Run OpenCodeReview")),
    /duplicate|step/i,
    "duplicate step names must fail instead of making step selection ambiguous"
  );
  assert.throws(
    () => resolveInputExpressions("${{ github.ref }}", {}),
    /unsupported|unresolved|expression/i,
    "non-input action expressions must not remain unresolved in executed test fixtures"
  );
}

function testRequiredStepTopologyAndEnvironmentContracts() {
  const required = {
    "Validate review_task_timeout": ["REVIEW_TASK_TIMEOUT"],
    "Install OpenCodeReview": ["OCR_VERSION"],
    "Configure OCR": [
      "OCR_LLM_URL",
      "OCR_LLM_MODEL",
      "OCR_USE_ANTHROPIC",
      "OCR_LLM_AUTH_HEADER",
      "OCR_EXTRA_BODY",
      "OCR_LANGUAGE",
    ],
    "Validate effort and max_tokens_budget": ["OCR_EFFORT", "OCR_MAX_TOKENS_BUDGET"],
    "Run OpenCodeReview": [
      "OCR_LLM_URL",
      "OCR_LLM_TOKEN",
      "OCR_LLM_MODEL",
      "OCR_USE_ANTHROPIC",
      "OCR_LLM_AUTH_HEADER",
      "OCR_LLM_EXTRA_HEADERS",
      "OCR_LLM_TIMEOUT",
      "OCR_REVIEW_CONCURRENCY",
      "OCR_BACKGROUND",
      "OCR_RULE",
      "OCR_EFFORT",
      "OCR_MAX_TOKENS_BUDGET",
    ],
  };
  for (const [name, envNames] of Object.entries(required)) {
    const step = stepNamed(name);
    assert.ok(step, `action.yml must retain the ${name} step`);
    assert.strictEqual(typeof step.run, "string", `${name} must retain a supported shell block`);
    for (const envName of envNames) {
      assert.ok(Object.prototype.hasOwnProperty.call(step.env, envName), `${name} must define ${envName}`);
    }
  }
}

function testContractsRunInDedicatedWorkflow() {
  assert.match(
    CONTRACT_WORKFLOW_TEXT,
    /^\s+- name:\s*Test GitHub Actions contracts\s*$[\s\S]*?^\s+run:\s*npm run test:github-actions\s*$/m,
    "action-contract.yml must execute the complete GitHub Actions contract suite"
  );
  assert.match(
    CONTRACT_WORKFLOW_TEXT,
    /paths:\s*[\s\S]*?examples\/github_actions\/README\.md/,
    "action-contract.yml must run the contract suite when the GitHub Actions README changes"
  );
  assert.match(
    CONTRACT_WORKFLOW_TEXT,
    /uses:\s*actions\/checkout@[0-9a-f]{40}\s*#\s*v7/,
    "the dedicated contract workflow must pin checkout to an immutable commit"
  );
  assert.match(
    CONTRACT_WORKFLOW_TEXT,
    /container:\s*[\s\S]*?image:\s*node:[0-9]/,
    "the contract job must declare its Node runtime via a pinned container image"
  );
}

function testExampleReadmeDocumentsTimeoutAndVersionContracts() {
  assert.match(
    EXAMPLE_README_TEXT,
    /\| `review_task_timeout` \| `'15'` \|[^\n]*(?:per-file|per-task|concurrent task)[^\n]*\|/i,
    "GitHub Actions README must document the per-task timeout and its default"
  );
  assert.match(
    EXAMPLE_README_TEXT,
    /\| `llm_timeout` \| `'300'` \|[^\n]*(?:LLM|HTTP)[^\n]*seconds[^\n]*\|/i,
    "GitHub Actions README must document the LLM request timeout and its default"
  );
  assert.match(
    EXAMPLE_README_TEXT,
    /ocr_version[^\n]*1\.9\.6/i,
    "GitHub Actions README must document the minimum compatible OCR version"
  );
  const sample = EXAMPLE_README_TEXT.match(
    /### Use a specific OCR version[\s\S]*?ocr_version:\s*['\"]?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)['\"]?/
  );
  assert.ok(sample, "GitHub Actions README must include a valid stable OCR version sample");
  const [major, minor, patch] = sample.slice(1, 4).map(Number);
  assert.ok(
    major > 1 || (major === 1 && (minor > 9 || (minor === 9 && patch >= 6))),
    "GitHub Actions README must not recommend an OCR version below the supported floor"
  );
}

function testExampleReadmeDocumentsEffortAndBudget() {
  assert.match(
    EXAMPLE_README_TEXT,
    /\| `effort` \|[^\n]*low[^\n]*medium[^\n]*high[^\n]*\|/i,
    "GitHub Actions README must document the effort presets"
  );
  assert.match(
    EXAMPLE_README_TEXT,
    /\| `max_tokens_budget` \|[^\n]*0[^\n]*unlimited[^\n]*\|/i,
    "GitHub Actions README must document that a 0 budget means unlimited"
  );
}

const TESTS = [
  ["review_task_timeout names and describes the CLI task deadline", testReviewTaskTimeoutInputNameAndScope],
  ["llm_timeout defaults to the CLI's 5-minute timeout", testLlmTimeoutInputDefault],
  ["review_task_timeout accepts 1/10/120", testReviewTimeoutValidationAcceptsBoundaries],
  ["review_task_timeout rejects malformed values", testReviewTimeoutValidationRejectsMalformedValues],
  ["review_task_timeout validation runs before NPM install", testValidationPrecedesNpmInstall],
  ["review_task_timeout forwards --timeout separately from llm_timeout", testReviewTimeoutForwardedSeparatelyFromLlmTimeout],
  ["default llm_timeout exports independently from review_task_timeout", testDefaultLlmTimeoutExportedSeparatelyFromReviewTimeout],
  ["empty llm_timeout normalizes before review invocation", testEmptyLlmTimeoutNormalizesBeforeReviewInvocation],
  ["review_task_timeout with a leading zero normalizes across steps", testReviewTimeoutLeadingZeroIsNormalizedAcrossSteps],
  ["Configure OCR builds a complete llm config", testConfigureBuildsCompleteLlmConfig],
  ["Configure OCR never persists the token", testConfigureNeverPersistsToken],
  ["Configure OCR neutralizes stale provider and static token", testConfigureNeutralizesStaleProviderAndStaticToken],
  ["Configure OCR sets a protocol consistent with use_anthropic", testConfigureProtocolTracksUseAnthropic],
  ["Configure OCR preserves legacy use_anthropic resolution", testConfigurePreservesLegacyUseAnthropicResolution],
  ["Configure OCR clears stale persisted extra headers", testConfigureClearsStaleExtraHeadersBeforeTokenCommand],
  ["Configure OCR clears stale persisted retry codes", testConfigureClearsStaleRetryCodesBeforeEndpointConfig],
  ["Run OpenCodeReview retains the extra-headers env override", testRunRetainsExtraHeadersEnvironmentOverride],
  ["Run OpenCodeReview fails closed without validated task timeout", testRunFailsClosedWhenValidatedTaskTimeoutIsMissing],
  ["effort and max_tokens_budget are optional and undefaulted", testEffortAndBudgetInputsAreOptional],
  ["unset effort and max_tokens_budget append no review flags", testUnsetEffortAndBudgetAppendNoFlags],
  ["effort and max_tokens_budget reach ocr review once each", testEffortAndBudgetForwardedToReviewCommand],
  ["an explicit 0 token budget is forwarded verbatim", testZeroBudgetIsForwardedVerbatim],
  ["effort accepts the CLI presets in any case", testEffortValidationAcceptsThePresets],
  ["effort rejects values the CLI would not accept", testEffortValidationRejectsUnknownPresets],
  ["max_tokens_budget accepts non-negative integers", testBudgetValidationAcceptsNonNegativeIntegers],
  ["max_tokens_budget rejects malformed values", testBudgetValidationRejectsMalformedValues],
  ["effort/max_tokens_budget validation runs before NPM install", testEffortAndBudgetValidationPrecedesNpmInstall],
  ["effort participates in the checkpoint fingerprint", testEffortParticipatesInTheCheckpointFingerprint],
  ["the official OpenCodeReview NPM install is preserved", testOfficialNpmPackageInstallIsPreserved],
  ["Install OpenCodeReview enforces the auth_token_cmd version floor", testInstallEnforcesAuthTokenCommandVersionFloor],
  ["contract harness fails closed on unsupported YAML shapes", testContractHarnessFailsClosedOnUnsupportedYamlShapes],
  ["required action steps and env contracts are present", testRequiredStepTopologyAndEnvironmentContracts],
  ["GitHub Actions contracts run in a dedicated workflow", testContractsRunInDedicatedWorkflow],
  ["GitHub Actions README documents timeout and version contracts", testExampleReadmeDocumentsTimeoutAndVersionContracts],
  ["GitHub Actions README documents effort and max_tokens_budget", testExampleReadmeDocumentsEffortAndBudget],
];

function main() {
  const failures = [];
  for (const [name, test] of TESTS) {
    try {
      test();
      console.log(`ok - ${name}`);
    } catch (error) {
      failures.push({ name, error });
      console.error(`not ok - ${name}: ${error.message}`);
    }
  }
  if (failures.length > 0) {
    console.error(`\n${failures.length} action contract test(s) failed.`);
    process.exitCode = 1;
  } else {
    console.log(`\nAll ${TESTS.length} action contract tests passed.`);
  }
}

main();
