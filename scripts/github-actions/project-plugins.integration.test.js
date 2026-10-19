// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const assert = require("assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { spawnSync } = require("child_process");

const root = path.resolve(__dirname, "../..");
const action = fs.readFileSync(path.join(root, "action.yml"), "utf8").replace(/\r\n/g, "\n");
const binary = process.env.OCR_TEST_BINARY;
assert.ok(binary && path.isAbsolute(binary) && fs.statSync(binary).isFile(),
  "OCR_TEST_BINARY must point to an absolute path of a freshly built OCR native binary");

function step(name) {
  const start = action.indexOf(`    - name: ${name}\n`);
  assert.notStrictEqual(start, -1, `missing Action step: ${name}`);
  const end = action.indexOf("\n    - name:", start + 1);
  return end < 0 ? action.slice(start) : action.slice(start, end);
}

function scalar(block, name, indent) {
  const match = new RegExp(`^${" ".repeat(indent)}${name}: (.+)$`, "m").exec(block);
  assert.ok(match, `missing ${name} in Action step`);
  return match[1];
}

function body(block, name, indent) {
  const marker = `${" ".repeat(indent)}${name}: |\n`;
  const start = block.indexOf(marker);
  assert.notStrictEqual(start, -1, `missing ${name} script in Action step`);
  const lines = [];
  for (const line of block.slice(start + marker.length).split("\n")) {
    if (line && !line.startsWith(" ".repeat(indent + 2))) break;
    lines.push(line.slice(indent + 2));
  }
  return lines.join("\n");
}

function expression(value, values) {
  return value.replace(/\$\{\{\s*([^}]+?)\s*\}\}/g, (_, key) => {
    assert.ok(Object.hasOwn(values, key), `unsupported Action expression: ${key}`);
    return values[key];
  });
}

function command(program, args, options) {
  return spawnSync(program, args, { encoding: "utf8", timeout: 30000, ...options });
}

function checked(result) {
  assert.strictEqual(result.status, 0, result.error?.message || `${result.stdout}\n${result.stderr}`);
  return result.stdout.trim();
}

function fixture(executable) {
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), "ocr-policy-integration-"));
  const workspace = path.join(temporary, "policy repo");
  const authorHome = path.join(temporary, "author-home");
  fs.mkdirSync(workspace);
  fs.mkdirSync(authorHome);
  const env = { ...process.env, HOME: authorHome, USERPROFILE: authorHome };
  const git = (...args) => checked(command("git", ["-C", workspace, ...args], { env }));
  const cli = (args, home = authorHome) => JSON.parse(checked(command(binary, [...args, "--json"], {
    cwd: workspace, env: { ...env, HOME: home, USERPROFILE: home },
  })));
  const write = (name, content) => {
    const file = path.join(workspace, name);
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, typeof content === "string" ? content : `${JSON.stringify(content)}\n`);
  };
  const commit = () => {
    git("add", ".");
    git("-c", "commit.gpgsign=false", "commit", "--no-verify", "-m", "Test policy");
    return git("rev-parse", "HEAD");
  };
  const marker = path.join(temporary, "server-started");
  const pkg = (id, version, text, declare = true) => {
    const manifest = { schema_version: 1, id, name: id, version, rules: ["rules.json"] };
    if (executable) {
      manifest.mcp_servers = { test: { command: process.execPath, args: ["${plugin_dir}/server.cjs"] } };
      write(`plugins/${id}/server.cjs`, `require("fs").writeFileSync(${JSON.stringify(marker)}, "unexpected execution");\n`);
    }
    write(`plugins/${id}/ocr-plugin.json`, manifest);
    write(`plugins/${id}/rules.json`, { default_rule: text });
    cli(declare ? ["plugin", "add", path.join(workspace, "plugins", id)] : ["plugin", "lock"]);
  };
  try {
    git("init");
    git("config", "user.name", "Policy Integration");
    git("config", "user.email", "policy@example.invalid");
    git("config", "core.autocrlf", "false");
    write(".gitattributes", "* text=auto eol=lf\n");
    pkg("policy-rules", "1.0.0", "Trusted base rules.");
    const base = commit();
    pkg("policy-rules", "2.0.0", "PR head rules must not load.", false);
    pkg("head-only", "1.0.0", "PR head dependency must not load.");
    const head = commit();
    const headFiles = Object.fromEntries(["plugins.json", "plugins.lock.json"].map(name =>
      [name, fs.readFileSync(path.join(workspace, ".opencodereview", name))]));
    git("checkout", "--detach", base);
    write("README.md", "Default branch advanced after the PR event.\n");
    const laterBase = commit();
    return { temporary, workspace, env, git, cli, base, head, headFiles, laterBase, marker, write };
  } catch (error) {
    fs.rmSync(temporary, { recursive: true, force: true });
    throw error;
  }
}

async function resolve(f, eventName, explicit = "") {
  const pr = { number: 7, base: { sha: f.base }, head: { sha: f.head, repo: { full_name: "fork/project" } } };
  const comment = eventName === "issue_comment";
  f.git("checkout", "--detach", comment ? f.laterBase : f.head);
  const context = {
    eventName, repo: { owner: "upstream", repo: "project" }, sha: comment ? f.laterBase : f.head,
    payload: comment ? { issue: { number: 7, pull_request: {} }, comment: { body: "/open-code-review" } } : { pull_request: pr },
  };
  let lookups = 0;
  const github = { rest: { pulls: { get: async args => {
    assert.deepStrictEqual(args, { owner: "upstream", repo: "project", pull_number: 7 });
    lookups++;
    return { data: pr };
  } } } };
  const values = { "inputs.policy_sha": explicit, "env.PR_NUMBER": "7" };
  const block = step("Resolve trusted policy");
  const environment = {
    GITHUB_ACTION_PATH: root, GITHUB_WORKSPACE: f.workspace, RUNNER_TEMP: f.temporary,
    OCR_POLICY_SHA: expression(scalar(block, "OCR_POLICY_SHA", 8), values),
    OCR_PR_NUMBER: expression(scalar(block, "OCR_PR_NUMBER", 8), values),
  };
  const saved = Object.fromEntries(Object.keys(environment).map(key => [key, process.env[key]]));
  const outputs = {};
  try {
    Object.assign(process.env, environment);
    const run = new Function("github", "context", "core", "require",
      `return (async () => {\n${body(block, "script", 8)}\n})();`);
    await run(github, context, { setOutput: (key, value) => { outputs[key] = value; } }, require);
  } finally {
    for (const [key, value] of Object.entries(saved)) {
      if (value === undefined) delete process.env[key];
      else process.env[key] = value;
    }
  }
  assert.strictEqual(lookups, comment && !explicit ? 1 : 0);
  assert.strictEqual(outputs.sha, explicit || f.base);
  const ref = expression(scalar(step("Checkout base"), "ref", 8), { "steps.policy.outputs.sha": outputs.sha });
  f.git("checkout", "--detach", ref);
  return outputs;
}

function prepare(f, policy, allowExec) {
  const block = step("Prepare project plugins");
  assert.match(block, /if: inputs\.project_plugins == 'true'/);
  const values = {
    "steps.policy.outputs.sha": policy.sha, "steps.policy.outputs.script_root": policy.script_root,
    "inputs.allow_plugin_exec": String(allowExec), "inputs.rule": "",
  };
  const output = path.join(f.temporary, "github-output");
  fs.writeFileSync(output, "");
  const env = {
    ...f.env, GITHUB_WORKSPACE: f.workspace, RUNNER_TEMP: f.temporary,
    GITHUB_OUTPUT: output, OCR_NATIVE_BINARY: binary,
  };
  for (const key of ["OCR_POLICY_SHA", "OCR_ALLOW_PLUGIN_EXEC", "OCR_RULE", "OCR_ACTION_SCRIPTS"]) {
    env[key] = expression(scalar(block, key, 8), values);
  }
  const result = command("bash", ["-e", "-c", body(block, "run", 6)], { cwd: f.workspace, env });
  const outputs = Object.fromEntries(fs.readFileSync(output, "utf8").trim().split("\n").filter(Boolean).map(line => {
    const at = line.indexOf("=");
    return [line.slice(0, at), line.slice(at + 1)];
  }));
  return { result, outputs };
}

async function main() {
  for (const executable of [false, true]) {
    const f = fixture(executable);
    try {
      let fingerprint;
      for (const event of ["pull_request", "pull_request_target", "issue_comment"]) {
        const policy = await resolve(f, event);
        if (executable) {
          const denied = prepare(f, policy, false);
          assert.notStrictEqual(denied.result.status, 0);
          assert.match(denied.result.stderr, /PLUGIN_EXEC_NOT_AUTHORIZED/);
          assert.deepStrictEqual(denied.outputs, {}, "failed preparation must not publish a usable home");
        }
        const { result, outputs } = prepare(f, policy, executable);
        checked(result);
        assert.match(outputs.fingerprint, /^[0-9a-f]{64}$/);
        if (fingerprint) assert.strictEqual(outputs.fingerprint, fingerprint, "event type must not change policy identity");
        fingerprint = outputs.fingerprint;
        const installed = f.cli(["plugin", "preflight"], outputs.home);
        assert.deepStrictEqual(installed.map(item => [item.id, item.version]), [["policy-rules", "1.0.0"]]);
        const rules = JSON.parse(fs.readFileSync(path.join(installed[0].directory, "rules.json"), "utf8"));
        assert.strictEqual(rules.default_rule, "Trusted base rules.");
        assert.deepStrictEqual(f.cli(["plugin", "list"], outputs.home), [], "CI must not install into the global registry");
        assert.ok(!fs.existsSync(f.marker), "preparation must not start MCP commands");

        f.git("checkout", "--detach", f.head);
        const wrongCheckout = prepare(f, policy, true);
        assert.notStrictEqual(wrongCheckout.result.status, 0);
        assert.match(wrongCheckout.result.stderr, /policy checkout must match/);
        f.git("checkout", "--detach", f.base);
        for (const name of ["plugins.json", "plugins.lock.json"]) {
          fs.writeFileSync(path.join(f.workspace, ".opencodereview", name), f.headFiles[name]);
          const modified = prepare(f, policy, true);
          assert.notStrictEqual(modified.result.status, 0);
          assert.match(modified.result.stderr, /policy file differs from the trusted commit/);
          f.git("checkout", "--", `.opencodereview/${name}`);
        }
        f.write("plugins/policy-rules/rules.json", { default_rule: "Changed package outside the lock." });
        const modifiedPackage = prepare(f, policy, true);
        assert.notStrictEqual(modifiedPackage.result.status, 0);
        assert.match(modifiedPackage.result.stderr, /checksum|digest|content/i);
        f.git("checkout", "--", "plugins/policy-rules/rules.json");
        console.log(`PASS ${event}: trusted rules${executable ? "/MCP authorization" : ""}, PR-head checkout/declaration/lock rejected`);
      }
      const policy = await resolve(f, "issue_comment", f.laterBase);
      checked(prepare(f, policy, executable).result);
      assert.ok(!fs.existsSync(f.marker));
    } finally {
      fs.rmSync(f.temporary, { recursive: true, force: true });
    }
  }
  console.log("Project plugin policy integration tests passed (real Git and OCR; simulated GitHub event/API).");
}

main().catch(error => { console.error(error); process.exitCode = 1; });
