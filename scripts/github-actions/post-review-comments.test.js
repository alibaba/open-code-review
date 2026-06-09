#!/usr/bin/env node
"use strict";

const assert = require("assert");
const fs = require("fs");
const path = require("path");
const vm = require("vm");

const repoRoot = path.join(__dirname, "..", "..");
const workflowFiles = [
  ".github/workflows/ocr-review.yml",
  "examples/github_actions/ocr-review.yml",
];

function extractPostReviewScript(workflowPath) {
  const text = fs.readFileSync(path.join(repoRoot, workflowPath), "utf8");
  const lines = text.split("\n");

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const marker = line.match(/^(\s*)script:\s*\|\s*$/);
    if (!marker) continue;

    const blockIndent = marker[1].length + 2;
    const block = [];
    for (let j = i + 1; j < lines.length; j++) {
      const current = lines[j];
      if (current.trim() === "") {
        block.push("");
        continue;
      }
      const indent = current.match(/^ */)[0].length;
      if (indent < blockIndent) break;
      block.push(current.slice(blockIndent));
    }

    const script = block.join("\n");
    if (script.includes("/tmp/ocr-result.json")) {
      return script;
    }
  }

  throw new Error(`post review script not found in ${workflowPath}`);
}

function mockFs(resultText, stderrText) {
  return {
    readFileSync(file) {
      if (file === "/tmp/ocr-result.json") return resultText;
      if (file === "/tmp/ocr-stderr.log") return stderrText;
      throw new Error(`unexpected read: ${file}`);
    },
  };
}

function mockGithub(options) {
  const createReviewCalls = [];
  const issueComments = [];

  return {
    createReviewCalls,
    issueComments,
    rest: {
      pulls: {
        get: async () => ({ data: { head: { sha: "head-sha" } } }),
        createReview: async (params) => {
          createReviewCalls.push(params);
          if (createReviewCalls.length === 1 && options.bulkError) {
            throw new Error(options.bulkError);
          }
          if (createReviewCalls.length > 1 && options.individualError) {
            throw new Error(options.individualError);
          }
          return { data: {} };
        },
      },
      issues: {
        createComment: async (params) => {
          issueComments.push(params);
          return { data: {} };
        },
      },
    },
  };
}

async function runPostReviewScript(workflowPath, options) {
  const script = extractPostReviewScript(workflowPath);
  const github = mockGithub(options);
  const context = {
    repo: { owner: "owner", repo: "repo" },
    issue: { number: 123 },
    eventName: "pull_request_target",
    payload: { pull_request: { head: { sha: "head-sha" } } },
  };
  const sandbox = {
    github,
    context,
    console: { log() {} },
    require(name) {
      if (name === "fs") return options.fs;
      throw new Error(`unexpected require: ${name}`);
    },
  };

  await vm.runInNewContext(`(async () => {\n${script}\n})()`, sandbox, {
    timeout: 1000,
  });

  return github;
}

async function testFailedInlineCommentsAreSummarized(workflowPath) {
  const result = {
    comments: [
      {
        path: "docs/no-line.md",
        content:
          "No-line content with a fenced block:\n\n```js\nconsole.log('still visible');\n```",
        existing_code: "",
        suggestion_code: "",
        start_line: 0,
        end_line: 0,
      },
      {
        path: "src/app.js",
        content: "Failed inline content must remain visible in the PR summary.",
        existing_code: "oldCall();",
        suggestion_code: "newCall();",
        start_line: 10,
        end_line: 10,
      },
    ],
    warnings: [],
  };

  const github = await runPostReviewScript(workflowPath, {
    fs: mockFs(JSON.stringify(result), ""),
    bulkError: 'Unprocessable Entity: "Line could not be resolved"',
    individualError: 'Unprocessable Entity: "Line could not be resolved"',
  });

  assert.strictEqual(github.createReviewCalls.length, 2);
  assert.strictEqual(github.issueComments.length, 1);
  const body = github.issueComments[0].body;
  assert.match(body, /No-line content with a fenced block/);
  assert.match(body, /Failed inline content must remain visible/);
  assert.match(body, /Line could not be resolved/);
}

async function testErrorCommentUsesSafeFence(workflowPath) {
  const github = await runPostReviewScript(workflowPath, {
    fs: mockFs("not json", "stderr includes a fence\n```js\nbroken();\n```"),
  });

  assert.strictEqual(github.issueComments.length, 1);
  const body = github.issueComments[0].body;
  assert.match(body, /\n````\nstderr includes a fence/);
}

async function main() {
  for (const workflowPath of workflowFiles) {
    await testFailedInlineCommentsAreSummarized(workflowPath);
    await testErrorCommentUsesSafeFence(workflowPath);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
