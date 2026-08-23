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
    const runMarker = rawLines.findIndex((line) => /^      run:\s*\|\s*$/.test(line));
    let run;
    if (runMarker >= 0) {
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
    const envMarker = rawLines.findIndex((line) => /^      env:\s*$/.test(line));
    if (envMarker >= 0) {
      for (let index = envMarker + 1; index < rawLines.length; index += 1) {
        const line = rawLines[index];
        if (line.trim() === "") continue;
        const match = /^        ([A-Za-z_][A-Za-z0-9_]*):\s*(.*)$/.exec(line);
        if (!match) break;
        env[match[1]] = parseScalar(match[2]);
      }
    }

    steps.push({ name: current.name, run, env, index: current.index });
    current = undefined;
  }

  for (let index = start + 1; index < lines.length; index += 1) {
    const match = /^    - name:\s*(.*)\s*$/.exec(lines[index]);
    if (match) {
      finish();
      current = { name: parseScalar(match[1]), lines: [lines[index]], index };
    } else if (current) {
      current.lines.push(lines[index]);
    }
  }
  finish();
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

function resolveInputExpressions(value, values) {
  return String(value).replace(/\$\{\{\s*inputs\.([A-Za-z0-9_]+)\s*\}\}/g, (_match, name) => {
    return values[name] === undefined ? "" : String(values[name]);
  });
}

function renderedEnv(step, values) {
  const env = {};
  for (const [name, value] of Object.entries(step.env || {})) {
    env[name] = resolveInputExpressions(value, values);
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
    return /review[ _-]?timeout/i.test(haystack);
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
for (const name of ["OCR_LLM_TIMEOUT", "OCR_LLM_EXTRA_HEADERS", "OCR_REVIEW_TIMEOUT", "REVIEW_TIMEOUT", "OCR_TIMEOUT"]) {
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
    if (key === "llm" && typeof value === "string") {
      try {
        const parsed = JSON.parse(value);
        for (const [field, fieldValue] of Object.entries(parsed)) values[`llm.${field}`] = fieldValue;
      } catch (_err) {
        // A malformed aggregate config is deliberately left for assertions.
      }
    } else if (key) {
      values[key] = value;
    }
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
  return new RegExp(value.replace(/[.*+?^${}()|[\[\]\\]/g, "\\$&"));
}

function assertValidation(value, expectedValid) {
  const step = validationStep();
  assert.ok(step, "action.yml must add a validation step that references review_timeout");
  const fixture = makeFixture();
  try {
    const result = runStep(step, inputValues({ review_timeout: value }), fixture);
    if (expectedValid) {
      assert.strictEqual(result.status, 0, `review_timeout=${JSON.stringify(value)} should be accepted; ${resultDescription(result)}`);
    } else {
      assert.notStrictEqual(result.status, 0, `review_timeout=${JSON.stringify(value)} should be rejected; ${resultDescription(result)}`);
    }
  } finally {
    removeFixture(fixture);
  }
}

function testReviewTimeoutInputDefault() {
  assert.ok(INPUTS.review_timeout, "action.yml must define the review_timeout input");
  assert.strictEqual(INPUTS.review_timeout.default, "10", "review_timeout default must be 10 minutes");
}

function testLlmTimeoutInputDefault() {
  assert.ok(INPUTS.llm_timeout, "action.yml must define the llm_timeout input");
  assert.strictEqual(INPUTS.llm_timeout.default, "300", "llm_timeout default must match the CLI's 5-minute timeout");
}

function testReviewTimeoutValidationAcceptsBoundaries() {
  for (const value of ["1", "10", "120"]) assertValidation(value, true);
}

function testReviewTimeoutValidationRejectsMalformedValues() {
  for (const value of ["", "${{ inputs.review_timeout }}", "1.5", "+10", "0", "-1", "-10", "121"]) {
    assertValidation(value, false);
  }
}

function testValidationPrecedesNpmInstall() {
  const validation = validationStep();
  const install = installStep();
  assert.ok(validation, "action.yml must add review_timeout validation before installation");
  assert.ok(install, "action.yml must retain an OpenCodeReview installation step");
  assert.ok(validation.index < install.index, "review_timeout validation must occur before NPM install");

  const fixture = makeFixture();
  try {
    const values = inputValues({ review_timeout: "121", ocr_version: "contract-test" });
    const script = `${renderedRun(validation, values)}\n${renderedRun(install, values)}`;
    const env = Object.assign({}, renderedEnv(validation, values), renderedEnv(install, values));
    const result = runShell(script, env, fixture);
    assert.notStrictEqual(result.status, 0, `invalid review_timeout must stop the action; ${resultDescription(result)}`);
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
      review_timeout: "10",
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
      { MERGE_BASE: "base-sha", HEAD_SHA: "head-sha", REVIEW_TIMEOUT: values.review_timeout },
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
    assert.strictEqual(reviewCall.args[timeoutIndices[0] + 1], "10", "review_timeout must be passed as --timeout");
    assert.strictEqual(reviewCall.env.OCR_LLM_TIMEOUT, "900", "llm_timeout must remain the LLM request timeout");
    assert.notStrictEqual(
      reviewCall.args[timeoutIndices[0] + 1],
      reviewCall.env.OCR_LLM_TIMEOUT,
      "review_timeout and llm_timeout must remain distinct settings"
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
      review_timeout: "10",
      llm_url: "https://llm.example.invalid/v1",
      llm_auth_token: "unused-token",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
    });
    const result = runStep(
      run,
      values,
      fixture,
      { MERGE_BASE: "base-sha", HEAD_SHA: "head-sha", REVIEW_TIMEOUT: values.review_timeout },
      { replaceResultPaths: true }
    );
    assert.strictEqual(result.status, 0, `Run OpenCodeReview shell block failed; ${resultDescription(result)}`);
    const reviewCall = readJsonLines(fixture.callsPath).find((call) => call.args[0] === "review");
    assert.ok(reviewCall, "Run OpenCodeReview must invoke `ocr review`");
    const timeoutIndex = reviewCall.args.indexOf("--timeout");
    assert.ok(timeoutIndex >= 0, "Run OpenCodeReview must forward review_timeout");
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
      review_timeout: "10",
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
      { MERGE_BASE: "base-sha", HEAD_SHA: "head-sha", REVIEW_TIMEOUT: values.review_timeout },
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
  assert.ok(validation, "action.yml must retain review_timeout validation");
  assert.ok(run, "action.yml must retain the Run OpenCodeReview step");
  const fixture = makeFixture();
  try {
    const values = inputValues({
      review_timeout: "010",
      llm_url: "https://llm.example.invalid/v1",
      llm_auth_token: "unused-token",
      llm_model: "contract-model",
      llm_use_anthropic: "false",
    });
    const validationResult = runStep(validation, values, fixture);
    assert.strictEqual(
      validationResult.status,
      0,
      `review_timeout=010 validation failed; ${resultDescription(validationResult)}`
    );
    const exported = readEnvAssignments(path.join(fixture.dir, "github-env"));
    assert.strictEqual(exported.REVIEW_TIMEOUT, "10", "validation must export normalized decimal review_timeout");
    assert.ok(
      !Object.prototype.hasOwnProperty.call(run.env, "REVIEW_TIMEOUT"),
      "Run OpenCodeReview must consume the normalized GITHUB_ENV value, not rebind the raw input"
    );
    const result = runStep(
      run,
      values,
      fixture,
      { MERGE_BASE: "base-sha", HEAD_SHA: "head-sha", REVIEW_TIMEOUT: exported.REVIEW_TIMEOUT },
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

function testConfigureRejectsInvalidUseAnthropicBeforeMutation() {
  const configure = stepNamed("Configure OCR");
  assert.ok(configure, "action.yml must retain the Configure OCR step");
  for (const value of ["TRUE", "1", "t", "yes", ""]) {
    const fixture = makeFixture();
    try {
      const values = inputValues({
        llm_url: "https://llm.example.invalid/v1",
        llm_model: "contract-model",
        llm_use_anthropic: value,
        llm_auth_token: "invalid-boolean-token-sentinel",
      });
      const result = runStep(configure, values, fixture);
      assert.notStrictEqual(result.status, 0, `llm_use_anthropic=${JSON.stringify(value)} must fail`);
      assert.deepStrictEqual(
        configOperations(fixture),
        [],
        `llm_use_anthropic=${JSON.stringify(value)} must fail before any config mutation`
      );
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
      review_timeout: "10",
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
      { MERGE_BASE: "base-sha", HEAD_SHA: "head-sha", REVIEW_TIMEOUT: values.review_timeout },
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

const TESTS = [
  ["review_timeout defaults to 10", testReviewTimeoutInputDefault],
  ["llm_timeout defaults to the CLI's 5-minute timeout", testLlmTimeoutInputDefault],
  ["review_timeout accepts 1/10/120", testReviewTimeoutValidationAcceptsBoundaries],
  ["review_timeout rejects malformed values", testReviewTimeoutValidationRejectsMalformedValues],
  ["review_timeout validation runs before NPM install", testValidationPrecedesNpmInstall],
  ["review_timeout forwards --timeout separately from llm_timeout", testReviewTimeoutForwardedSeparatelyFromLlmTimeout],
  ["default llm_timeout exports independently from review_timeout", testDefaultLlmTimeoutExportedSeparatelyFromReviewTimeout],
  ["empty llm_timeout normalizes before review invocation", testEmptyLlmTimeoutNormalizesBeforeReviewInvocation],
  ["review_timeout with a leading zero normalizes across steps", testReviewTimeoutLeadingZeroIsNormalizedAcrossSteps],
  ["Configure OCR builds a complete llm config", testConfigureBuildsCompleteLlmConfig],
  ["Configure OCR never persists the token", testConfigureNeverPersistsToken],
  ["Configure OCR neutralizes stale provider and static token", testConfigureNeutralizesStaleProviderAndStaticToken],
  ["Configure OCR sets a protocol consistent with use_anthropic", testConfigureProtocolTracksUseAnthropic],
  ["Configure OCR rejects invalid use_anthropic before mutation", testConfigureRejectsInvalidUseAnthropicBeforeMutation],
  ["Configure OCR clears stale persisted extra headers", testConfigureClearsStaleExtraHeadersBeforeTokenCommand],
  ["Configure OCR clears stale persisted retry codes", testConfigureClearsStaleRetryCodesBeforeEndpointConfig],
  ["Run OpenCodeReview retains the extra-headers env override", testRunRetainsExtraHeadersEnvironmentOverride],
  ["the official OpenCodeReview NPM install is preserved", testOfficialNpmPackageInstallIsPreserved],
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
