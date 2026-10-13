#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const assert = require("assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { spawnSync } = require("child_process");

const ROOT = path.resolve(__dirname, "../..");
const WORKFLOW = fs.readFileSync(path.join(ROOT, ".github/workflows/release.yml"), "utf8");
const MANIFEST = JSON.parse(fs.readFileSync(path.join(ROOT, "package.json"), "utf8"));
const PACKAGES = [MANIFEST.name, ...Object.keys(MANIFEST.optionalDependencies)].sort();

function jobText(name) {
  const lines = WORKFLOW.split("\n");
  const start = lines.indexOf(`  ${name}:`);
  assert.ok(start >= 0, `missing job ${name}`);
  const end = lines.findIndex((line, index) => index > start && /^  [\w-]+:/.test(line));
  return lines.slice(start, end < 0 ? undefined : end).join("\n");
}

function stepText(job, name) {
  const lines = jobText(job).split("\n");
  const start = lines.indexOf(`      - name: ${name}`);
  assert.ok(start >= 0, `missing step ${name}`);
  const end = lines.findIndex((line, index) => index > start && /^      - /.test(line));
  return lines.slice(start, end < 0 ? undefined : end).join("\n");
}

function runBlock(step) {
  const lines = step.split("\n");
  const start = lines.indexOf("        run: |");
  assert.ok(start >= 0, "expected a multiline run block");
  const script = [];
  for (const line of lines.slice(start + 1)) {
    if (line.trim() && !line.startsWith("          ")) break;
    script.push(line.slice(10));
  }
  return script.join("\n");
}

const PREFLIGHT = stepText("verify-trusted-publishing", "Exchange OIDC token for every package");
const VERSION_GATE = stepText("npm-publish", "Ensure npm supports trusted publishing");

function execute(step, overrides = {}) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "ocr-release-contract-"));
  const bin = path.join(directory, "bin");
  fs.mkdirSync(bin);
  fs.symlinkSync(process.execPath, path.join(bin, "node"));
  fs.writeFileSync(path.join(directory, "package.json"), JSON.stringify(MANIFEST));
  const mock = `#!/usr/bin/env node
const fs = require("fs");
const path = require("path");
const command = path.basename(process.argv[1]);
const args = process.argv.slice(2);
fs.appendFileSync("calls.jsonl", JSON.stringify({ command, args }) + "\\n");
if (command === "npm") {
  if (args[0] === "--version") {
    console.log(process.env.TEST_NPM_VERSION);
  } else if (args[0] === "install") {
    process.exit(Number(process.env.TEST_INSTALL_STATUS || 0));
  } else {
    throw new Error("Unexpected npm operation: " + args.join(" "));
  }
} else if (args[args.length - 1].startsWith("https://oidc.invalid/")) {
  console.log(JSON.stringify({ value: process.env.TEST_ID_TOKEN }));
} else {
  if (process.env.TEST_CURL_FAILURE === "1") process.exit(7);
  const failed = process.env.TEST_FAILED_PACKAGE &&
    args[args.length - 1].endsWith(process.env.TEST_FAILED_PACKAGE.split("/").join("%2f"));
  const body = failed ? JSON.stringify({ message: "Not authorized" }) : process.env.TEST_RESPONSE;
  const outputIndex = args.indexOf("-o");
  if (outputIndex < 0) throw new Error("Expected a curl output file");
  fs.writeFileSync(args[outputIndex + 1], body);
  process.stdout.write(failed ? "401" : process.env.TEST_HTTP_STATUS);
}
`;
  for (const command of ["curl", "npm"]) {
    fs.writeFileSync(path.join(bin, command), mock, { mode: 0o755 });
  }
  const claims = Buffer.from(JSON.stringify({
    repository: "alibaba/open-code-review",
    workflow_ref: "alibaba/open-code-review/.github/workflows/release.yml@refs/heads/main",
    repository_visibility: "public",
  })).toString("base64url");
  try {
    const result = spawnSync("/bin/bash", ["--noprofile", "--norc", "-e", "-c", runBlock(step)], {
      cwd: directory,
      encoding: "utf8",
      timeout: 30000,
      env: {
        PATH: `${bin}:${process.env.PATH}`,
        HOME: directory,
        LC_ALL: "C",
        ACTIONS_ID_TOKEN_REQUEST_URL: "https://oidc.invalid/?request=fixture",
        ACTIONS_ID_TOKEN_REQUEST_TOKEN: "fixture-request-token",
        TEST_ID_TOKEN: `header.${claims}.signature`,
        TEST_HTTP_STATUS: "201",
        TEST_RESPONSE: JSON.stringify({ token: "fixture-npm-token" }),
        TEST_NPM_VERSION: "11.15.0",
        ...overrides,
      },
    });
    assert.ifError(result.error);
    const callsPath = path.join(directory, "calls.jsonl");
    const calls = fs.existsSync(callsPath)
      ? fs.readFileSync(callsPath, "utf8").trim().split("\n").map(JSON.parse)
      : [];
    return { ...result, calls };
  } finally {
    fs.rmSync(directory, { recursive: true, force: true });
  }
}

let failures = 0;
function test(name, check) {
  try {
    check();
    console.log(`PASS ${name}`);
  } catch (error) {
    failures += 1;
    console.error(`FAIL ${name}: ${error.message}`);
  }
}

test("container scripts explicitly select Bash", () => {
  for (const step of [PREFLIGHT, VERSION_GATE]) {
    assert.match(step, /^        shell: bash$/m);
  }
});

test("manual preflight is separated from tag publishing", () => {
  assert.match(WORKFLOW, /^  workflow_dispatch:/m);
  assert.match(jobText("verify-trusted-publishing"), /^    if: github.event_name == 'workflow_dispatch'$/m);
  assert.match(jobText("build"), /^    if: github.event_name == 'push'$/m);
  assert.match(jobText("release"), /^    needs: build$/m);
  assert.match(jobText("npm-publish"), /^    needs: release$/m);
  for (const job of ["build", "release"]) {
    assert.match(jobText(job), /^    runs-on: self-hosted$/m);
  }
  for (const job of ["verify-trusted-publishing", "npm-publish"]) {
    assert.match(jobText(job), /^    runs-on: ubuntu-latest$/m);
    assert.match(jobText(job), /^      id-token: write$/m);
  }
  assert.doesNotMatch(WORKFLOW, /NPM_TOKEN|NODE_AUTH_TOKEN|_authToken/);
});

test("HTTP 201 validates all seven packages without publishing", () => {
  const result = execute(PREFLIGHT);
  assert.strictEqual(result.status, 0, result.stdout + result.stderr);
  const verified = Array.from(result.stdout.matchAll(/^OK\s+(\S+)$/gm), (match) => match[1]).sort();
  assert.strictEqual(PACKAGES.length, 7);
  assert.deepStrictEqual(verified, PACKAGES);
  assert.ok(result.calls.every((call) => call.command === "curl"));
  const requests = result.calls.map((call) => call.args[call.args.length - 1]);
  assert.strictEqual(requests[0], "https://oidc.invalid/?request=fixture&audience=npm:registry.npmjs.org");
  assert.deepStrictEqual(requests.slice(1).sort(), PACKAGES.map((name) =>
    `https://registry.npmjs.org/-/npm/v1/oidc/token/exchange/package/${name.split("/").join("%2f")}`
  ).sort());
  assert.ok(!result.stdout.includes("fixture-npm-token"));
});

test("one unauthorized platform fails preflight", () => {
  const result = execute(PREFLIGHT, { TEST_FAILED_PACKAGE: PACKAGES[0] });
  assert.notStrictEqual(result.status, 0);
  assert.match(result.stdout, /FAILED.*HTTP 401/);
  assert.match(result.stdout, /At least one package is not configured/);
});

for (const response of [{}, { token: null }, { token: "" }, { token: 123 }, { token: [] }]) {
  test(`HTTP 201 rejects an invalid token: ${JSON.stringify(response)}`, () => {
    const result = execute(PREFLIGHT, { TEST_RESPONSE: JSON.stringify(response) });
    assert.notStrictEqual(result.status, 0);
    assert.match(result.stdout, /At least one package is not configured/);
  });
}

test("missing OIDC permissions fail before network access", () => {
  const result = execute(PREFLIGHT, { ACTIONS_ID_TOKEN_REQUEST_TOKEN: "" });
  assert.notStrictEqual(result.status, 0);
  assert.deepStrictEqual(result.calls, []);
});

test("network failures cannot pass preflight", () => {
  const result = execute(PREFLIGHT, { TEST_CURL_FAILURE: "1" });
  assert.notStrictEqual(result.status, 0);
  assert.doesNotMatch(result.stdout, /All packages accept/);
});

for (const version of ["10.9.0", "11.5.1", "11.14.0", "11.15.0", "11.19.0"]) {
  test(`npm version gate handles ${version}`, () => {
    const result = execute(VERSION_GATE, { TEST_NPM_VERSION: version });
    assert.strictEqual(result.status, 0, result.stdout + result.stderr);
    const installs = result.calls.filter((call) => call.args[0] === "install");
    const needsUpgrade = ["10.9.0", "11.5.1", "11.14.0"].includes(version);
    assert.deepStrictEqual(installs.map((call) => call.args), needsUpgrade
      ? [["install", "-g", "npm@11.15.0"]]
      : []);
  });
}

test("failed npm upgrade stops the job", () => {
  const result = execute(VERSION_GATE, { TEST_NPM_VERSION: "10.9.0", TEST_INSTALL_STATUS: "1" });
  assert.notStrictEqual(result.status, 0);
});

test("both publish paths require provenance", () => {
  for (const name of ["Publish platform packages", "Publish to npm"]) {
    const commands = runBlock(stepText("npm-publish", name)).split("\n")
      .filter((line) => /^\s*npm publish\b/.test(line));
    assert.strictEqual(commands.length, 1);
    assert.match(commands[0], /--provenance(?:\s|$)/);
    assert.match(commands[0], /--access public(?:\s|$)/);
  }
});

if (failures) process.exitCode = 1;
