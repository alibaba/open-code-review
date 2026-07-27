#!/usr/bin/env node
"use strict";

// Unit tests for scripts/github-actions/check-translation-sync.js.
//
// Run via: node scripts/github-actions/check-translation-sync.test.js
// (also wired as `npm run test:github-actions`).
//
// Plain Node + assert, no external deps and no `node --test`, mirroring
// post-review-comments.test.js so both run on node >= 14.

const assert = require("assert");
const path = require("path");
const {
  extractHeadings,
  level2Headings,
  structuralSignature,
  compareReadmeStructures,
  counterpartPaths,
  findMissingTranslations,
  getChangedFiles,
  splitList,
} = require(path.join(__dirname, "check-translation-sync.js"));

// A minimal but realistic README skeleton: an intro H1, three `##` sections,
// one of them with a `###` subheading, plus a fenced bash block whose `#`
// comment lines must NOT be parsed as headings.
function readme(headings) {
  // headings: array of "## Text" / "### Text" / "# Text" lines already formed.
  return [
    "# Project",
    "",
    "Intro paragraph.",
    "",
    "```bash",
    "# this is a shell comment, not a heading",
    "## also not a heading",
    "echo hi",
    "```",
    "",
    ...headings.flatMap((h) => [h, "", "body text", ""]),
  ].join("\n");
}

function testExtractHeadingsSkipsCodeFences() {
  const md = readme(["## Alpha", "### Alpha One", "## Beta"]);
  const headings = extractHeadings(md);
  // Only the real headings: H1 + 2x H2 + 1x H3. The fenced `#`/`##` lines are ignored.
  assert.deepStrictEqual(
    headings.map((h) => `${h.level}:${h.text}`),
    ["1:Project", "2:Alpha", "3:Alpha One", "2:Beta"]
  );
  assert.deepStrictEqual(level2Headings(md), ["Alpha", "Beta"]);
  assert.deepStrictEqual(structuralSignature(md), [1, 2, 3, 2]);
}

function testExtractHeadingsTildeFenceAndTrailingHashes() {
  const md = ["# T", "~~~", "## fenced", "~~~", "## Real ##"].join("\n");
  assert.deepStrictEqual(
    extractHeadings(md).map((h) => `${h.level}:${h.text}`),
    ["1:T", "2:Real"] // closing `## Real ##` trailing hashes stripped
  );
}

function testExtractHeadingsInnerShorterFenceDoesNotClose() {
  // CommonMark: a closing fence must be at least as long as the opening one.
  // A block opened with ```` (4 backticks) must NOT be closed by an inner ```
  // (3 backticks); everything up to the matching 4-backtick line stays code.
  const md = [
    "# Title",
    "````", // open, length 4
    "## not a heading (inside code)",
    "```", // shorter than opening -> does NOT close
    "## also inside code",
    "````", // length >= 4 -> real close
    "## Real Heading",
  ].join("\n");
  assert.deepStrictEqual(
    extractHeadings(md).map((h) => `${h.level}:${h.text}`),
    ["1:Title", "2:Real Heading"]
  );
}

// --- README structure comparison -------------------------------------------

function testIdenticalStructurePasses() {
  // Same outline, DIFFERENT heading text (simulating translations). Must pass:
  // the check compares structure, not text.
  const en = readme(["## What is it?", "### Details", "## Usage"]);
  const zh = readme(["## 这是什么？", "### 细节", "## 使用方法"]);
  const ja = readme(["## これは何ですか？", "### 詳細", "## 使い方"]);
  const { ok, errors } = compareReadmeStructures([
    { name: "README.md", content: en },
    { name: "README.zh-CN.md", content: zh },
    { name: "README.ja-JP.md", content: ja },
  ]);
  assert.strictEqual(ok, true, JSON.stringify(errors));
  assert.strictEqual(errors.length, 0);
}

function testAddedHeadingFails() {
  const en = readme(["## Alpha", "## Beta"]);
  const zh = readme(["## Alpha", "## Beta", "## Gamma"]); // extra ##
  const { ok, errors } = compareReadmeStructures([
    { name: "README.md", content: en },
    { name: "README.zh-CN.md", content: zh },
  ]);
  assert.strictEqual(ok, false);
  assert.strictEqual(errors.length, 1);
  assert.strictEqual(errors[0].file, "README.zh-CN.md");
  assert.match(errors[0].message, /level-2 \(##\) heading/);
  assert.match(errors[0].message, /has 3 level-2 .* has 2/);
}

function testDroppedHeadingFails() {
  const en = readme(["## Alpha", "## Beta", "## Gamma"]);
  const ja = readme(["## Alpha", "## Gamma"]); // dropped Beta
  const { ok, errors } = compareReadmeStructures([
    { name: "README.md", content: en },
    { name: "README.ja-JP.md", content: ja },
  ]);
  assert.strictEqual(ok, false);
  assert.strictEqual(errors[0].file, "README.ja-JP.md");
  assert.match(errors[0].message, /has 2 level-2 .* has 3/);
}

function testReorderedHeadingFails() {
  // Two `##` sections with different sub-structure; swapping them keeps the
  // level-2 COUNT identical but changes the outline order -> must fail.
  // The sub-structure must differ so the reorder is detectable structurally:
  // same level-2 count, different heading-level outline.
  const en2 = readme(["## Alpha", "### A1", "## Beta"]); // [1,2,3,2]
  const zh2 = readme(["## Beta", "## Alpha", "### A1"]); // [1,2,2,3]
  const { ok, errors } = compareReadmeStructures([
    { name: "README.md", content: en2 },
    { name: "README.zh-CN.md", content: zh2 },
  ]);
  assert.strictEqual(ok, false);
  assert.strictEqual(errors[0].file, "README.zh-CN.md");
  assert.match(errors[0].message, /reordered or its level changed/);
  assert.match(errors[0].message, /heading #3/); // first divergence position
}

function testMissingReferenceOnlyIsFine() {
  // A single file (only the reference) cannot diverge.
  const { ok } = compareReadmeStructures([
    { name: "README.md", content: readme(["## A"]) },
  ]);
  assert.strictEqual(ok, true);
}

// --- docs en -> zh/ja counterpart sync -------------------------------------

function testCounterpartPaths() {
  assert.deepStrictEqual(
    counterpartPaths("pages/src/content/docs/en/integrations/ci.md"),
    [
      { locale: "zh", path: "pages/src/content/docs/zh/integrations/ci.md" },
      { locale: "ja", path: "pages/src/content/docs/ja/integrations/ci.md" },
    ]
  );
}

function testEnOnlyChangeWarns() {
  const changed = [
    "pages/src/content/docs/en/faq.md",
    "internal/agent/agent.go",
  ];
  const warnings = findMissingTranslations(changed);
  // Both zh and ja counterparts missing -> two warnings.
  assert.strictEqual(warnings.length, 2);
  assert.deepStrictEqual(
    warnings.map((w) => w.counterpart).sort(),
    [
      "pages/src/content/docs/ja/faq.md",
      "pages/src/content/docs/zh/faq.md",
    ]
  );
}

function testEnWithBothCounterpartsNoWarn() {
  const changed = [
    "pages/src/content/docs/en/faq.md",
    "pages/src/content/docs/zh/faq.md",
    "pages/src/content/docs/ja/faq.md",
  ];
  assert.deepStrictEqual(findMissingTranslations(changed), []);
}

function testEnWithOnlyOneCounterpartWarnsForTheOther() {
  const changed = [
    "pages/src/content/docs/en/mcp.md",
    "pages/src/content/docs/zh/mcp.md", // ja still missing
  ];
  const warnings = findMissingTranslations(changed);
  assert.strictEqual(warnings.length, 1);
  assert.strictEqual(warnings[0].locale, "ja");
  assert.strictEqual(warnings[0].counterpart, "pages/src/content/docs/ja/mcp.md");
}

function testNonDocsAndNonEnChangesIgnored() {
  const changed = [
    "pages/src/content/docs/zh/faq.md", // zh-only change: not our trigger
    "README.md",
    "pages/src/content/docs/en/logo.png", // en but not markdown
  ];
  assert.deepStrictEqual(findMissingTranslations(changed), []);
}

// --- changed-files resolution ----------------------------------------------

function testGetChangedFilesFromExplicitEnv() {
  const env = { OCR_CHANGED_FILES: "a.md\npages/src/content/docs/en/x.md\n" };
  assert.deepStrictEqual(getChangedFiles(env), [
    "a.md",
    "pages/src/content/docs/en/x.md",
  ]);
}

function testGetChangedFilesFromGitDiff() {
  const calls = [];
  const fakeExec = (cmd, args) => {
    calls.push({ cmd, args });
    return "pages/src/content/docs/en/faq.md\ninternal/x.go\n";
  };
  const env = { OCR_BASE_SHA: "base", OCR_HEAD_SHA: "head" };
  const out = getChangedFiles(env, fakeExec);
  assert.deepStrictEqual(out, [
    "pages/src/content/docs/en/faq.md",
    "internal/x.go",
  ]);
  assert.strictEqual(calls[0].cmd, "git");
  assert.deepStrictEqual(calls[0].args, ["diff", "--name-only", "base...head"]);
}

function testGetChangedFilesGitFailureIsNonFatal() {
  const env = { OCR_BASE_SHA: "base", OCR_HEAD_SHA: "head" };
  const boom = () => {
    throw new Error("no merge base");
  };
  // Silence the expected ::warning annotation this path emits.
  const orig = console.log;
  console.log = () => {};
  try {
    assert.deepStrictEqual(getChangedFiles(env, boom), []);
  } finally {
    console.log = orig;
  }
}

function testGetChangedFilesEmptyWhenNoInputs() {
  assert.deepStrictEqual(getChangedFiles({}), []);
}

function testSplitList() {
  assert.deepStrictEqual(splitList("a\nb,c\n\n  d  "), ["a", "b", "c", "d"]);
}

function main() {
  testExtractHeadingsSkipsCodeFences();
  testExtractHeadingsTildeFenceAndTrailingHashes();
  testExtractHeadingsInnerShorterFenceDoesNotClose();
  testIdenticalStructurePasses();
  testAddedHeadingFails();
  testDroppedHeadingFails();
  testReorderedHeadingFails();
  testMissingReferenceOnlyIsFine();
  testCounterpartPaths();
  testEnOnlyChangeWarns();
  testEnWithBothCounterpartsNoWarn();
  testEnWithOnlyOneCounterpartWarnsForTheOther();
  testNonDocsAndNonEnChangesIgnored();
  testGetChangedFilesFromExplicitEnv();
  testGetChangedFilesFromGitDiff();
  testGetChangedFilesGitFailureIsNonFatal();
  testGetChangedFilesEmptyWhenNoInputs();
  testSplitList();
  console.log("All check-translation-sync tests passed.");
}

try {
  main();
} catch (err) {
  console.error(err);
  process.exit(1);
}
