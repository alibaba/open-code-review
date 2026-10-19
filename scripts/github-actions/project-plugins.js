// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const fs = require("fs");
const path = require("path");
const crypto = require("crypto");
const { spawnSync } = require("child_process");

async function resolvePolicySHA({ github, context, policySHA, prNumber }) {
  const pr = context.payload.pull_request;
  let sha = policySHA || (String(pr?.number) === String(prNumber) ? pr?.base?.sha : "");
  if (!sha) {
    const { data } = await github.rest.pulls.get({
      ...context.repo, pull_number: Number(prNumber),
    });
    sha = data?.base?.sha;
  }
  if (typeof sha !== "string" || !/^[0-9a-f]{40}$/i.test(sha)) {
    throw new Error(policySHA
      ? "policy_sha must be a full 40-character commit SHA"
      : `Could not resolve a valid base commit SHA for PR #${prNumber}`);
  }
  return sha.toLowerCase();
}

function preserveActionScripts(root, temporary) {
  // Local actions share the workspace; the policy checkout can replace their files.
  const saved = fs.mkdtempSync(path.join(temporary, "ocr-action-scripts-"));
  const relative = "scripts/github-actions";
  fs.mkdirSync(path.join(saved, relative), { recursive: true });
  for (const name of ["project-plugins.js", "post-review-comments.js"]) {
    fs.copyFileSync(path.join(root, relative, name), path.join(saved, relative, name));
  }
  return saved;
}

function prepareProjectPlugins({ workspace, policySHA, allowExec = false, rule = "", temporary, ocrBinary, run = spawnSync }) {
  if (!ocrBinary || !path.isAbsolute(ocrBinary)) {
    throw new Error("project plugin preparation requires an absolute OCR native binary path");
  }
  const root = fs.realpathSync(workspace);
  const digest = data => crypto.createHash("sha256").update(data).digest("hex");
  const git = (args, input) => run("git", ["--literal-pathspecs", "-C", root, ...args], { input, maxBuffer: 8 * 1024 * 1024 });
  const checked = (result, label) => {
    if (result.error || result.status !== 0) {
      throw new Error(`${label} failed: ${result.error?.message || String(result.stderr || "").trim()}`);
    }
    return Buffer.from(result.stdout || "");
  };
  if (!/^[0-9a-f]{40}$/.test(policySHA || "") ||
      checked(git(["rev-parse", "HEAD"]), "read policy checkout").toString().trim() !== policySHA) {
    throw new Error("policy checkout must match the resolved trusted commit");
  }
  const policyFiles = [];
  function readPolicyFile(name, required) {
    const file = path.resolve(root, name);
    const relative = path.relative(root, file).split(path.sep).join("/");
    if (!relative || relative === ".." || relative.startsWith("../") || path.isAbsolute(relative)) {
      throw new Error("policy files must be inside the trusted checkout");
    }
    const tree = checked(git(["ls-tree", "-z", policySHA, "--", relative]), "read policy tree").toString();
    if (!tree) {
      if (required || fs.existsSync(file)) throw new Error(`policy file is not committed: ${relative}`);
      return null;
    }
    const blob = /^100(?:644|755) blob ([0-9a-f]+)\t/.exec(tree);
    if (!blob) throw new Error(`policy file must be a regular file: ${relative}`);
    const actual = fs.realpathSync(file);
    const actualRelative = path.relative(root, actual);
    if (actualRelative === ".." || actualRelative.startsWith(`..${path.sep}`) || path.isAbsolute(actualRelative)) {
      throw new Error(`policy file resolves outside the checkout: ${relative}`);
    }
    const data = fs.readFileSync(file);
    // Apply the same clean filters as Git, so checkout EOL conversion is not an edit.
    const object = checked(git(["hash-object", `--path=${relative}`, "--stdin"], data), "verify policy contents").toString().trim();
    if (object !== blob[1]) throw new Error(`policy file differs from the trusted commit: ${relative}`);
    policyFiles.push([relative, object]);
    return data;
  }
  readPolicyFile(".opencodereview/plugins.json", true);
  readPolicyFile(".opencodereview/plugins.lock.json", true);
  for (const [name, base, required] of [
    [".opencodereview/rule.json", root, false],
    ...(rule ? [[rule, path.dirname(path.resolve(root, rule)), true]] : []),
  ]) {
    const data = readPolicyFile(name, required);
    if (!data) continue;
    const config = JSON.parse(data);
    for (const entry of config.rules || []) {
      const text = entry.rule;
      if (typeof text === "string" && !/[\n ]/.test(text) && /\.(md|txt|markdown)$/i.test(text)) {
        readPolicyFile(path.resolve(base, text), true);
      }
    }
  }
  const home = fs.mkdtempSync(path.join(temporary, "ocr-project-home-"));
  const env = { ...process.env, HOME: home, USERPROFILE: home };
  const ocr = args => checked(run(ocrBinary, ["plugin", ...args, "--repo", root], {
    env, cwd: root, encoding: "utf8", maxBuffer: 8 * 1024 * 1024,
  }), `ocr plugin ${args[0]}`).toString();
  try {
    ocr(["sync", "--frozen"]);
    if (allowExec) ocr(["trust"]);
    const identity = JSON.parse(ocr(["preflight", "--fingerprint", "--json"]));
    if (typeof identity !== "string" || !/^[0-9a-f]{64}$/.test(identity)) {
      throw new Error("OCR did not return a verified project plugin fingerprint");
    }
    return { home, fingerprint: digest(JSON.stringify([identity, policyFiles])) };
  } catch (error) {
    fs.rmSync(home, { recursive: true, force: true });
    throw error;
  }
}

module.exports = { resolvePolicySHA, preserveActionScripts, prepareProjectPlugins };

if (require.main === module) {
  try {
    const result = prepareProjectPlugins({
      workspace: process.env.GITHUB_WORKSPACE,
      policySHA: process.env.OCR_POLICY_SHA,
      allowExec: process.env.OCR_ALLOW_PLUGIN_EXEC === "true",
      rule: process.env.OCR_RULE || "",
      temporary: process.env.RUNNER_TEMP,
      ocrBinary: process.env.OCR_NATIVE_BINARY,
    });
    fs.appendFileSync(process.env.GITHUB_OUTPUT, `home=${result.home}\nfingerprint=${result.fingerprint}\n`);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
