#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

// Unit tests for scripts/github-actions/check-plugin-contract.js.
//
// Run via: node scripts/github-actions/check-plugin-contract.test.js
// (also wired as `npm run test:github-actions`).
//
// Plain Node + assert, no external deps and no `node --test`, mirroring
// check-translation-sync.test.js so both run on node >= 14.

const assert = require("assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const {
  LINK_ALLOW_MARKER,
  MARKETPLACES,
  MIN_EXPECTED_LINKS,
  PLUGIN_DECLARATIONS,
  SKILL_ROOTS,
  COMMAND_DIRS,
  escapeAnnotationData,
  emitAnnotation,
  walkTextFiles,
  checkCorpusFloor,
  stripTrailingPunctuation,
  toRepoPath,
  extractRepoLinks,
  validateRepoLinks,
  parseFrontmatter,
  unquoteScalar,
  validateSkillFrontmatter,
  validateCommandFrontmatter,
  resolveDeclaredPath,
  checkPluginDeclarations,
  pluginEntries,
  listEntries,
  readFileOrNull,
  runLinksCheck,
  runManifestsCheck,
  main,
} = require(path.join(__dirname, "check-plugin-contract.js"));

const RAW = "https://raw.githubusercontent.com/alibaba/open-code-review/main/";
const BLOB = "https://github.com/alibaba/open-code-review/blob/main/";
const TREE = "https://github.com/alibaba/open-code-review/tree/main/";

// --- link extraction -------------------------------------------------------

function testExtractsRawCurlUrl() {
  // The exact shape the install docs use: a `curl -o` with the URL on the
  // continuation line.
  const md = [
    "```bash",
    "mkdir -p .claude/commands",
    "curl -o .claude/commands/review.md \\",
    `  ${RAW}plugins/open-code-review/claude-code/commands/review.md`,
    "```",
  ].join("\n");
  const links = extractRepoLinks(md);
  assert.deepStrictEqual(links.length, 1);
  assert.strictEqual(
    links[0].repoPath,
    "plugins/open-code-review/claude-code/commands/review.md"
  );
  assert.strictEqual(links[0].kind, "file");
  assert.strictEqual(links[0].line, 4);
}

function testBlobIsFileAndTreeIsDir() {
  const md = [`[a](${BLOB}skills/x/SKILL.md)`, `[b](${TREE}plugins/open-code-review)`].join(
    "\n"
  );
  const links = extractRepoLinks(md);
  assert.deepStrictEqual(
    links.map((l) => `${l.kind}:${l.repoPath}`),
    ["file:skills/x/SKILL.md", "tree:plugins/open-code-review"]
  );
}

function testMarkdownLinkParenNotSwallowed() {
  // A markdown inline link must not absorb the closing paren into the path.
  const links = extractRepoLinks(`See [the manifest](${BLOB}skills/a/SKILL.md).`);
  assert.deepStrictEqual(links.map((l) => l.repoPath), ["skills/a/SKILL.md"]);
}

function testTrailingProsePunctuationTrimmed() {
  // Translated docs end sentences with fullwidth punctuation directly after a
  // bare URL; ASCII prose ends with a period or comma.
  const md = [
    `context: ${BLOB}plugins/open-code-review/claude-code/commands/review.md\uFF0C`,
    `see ${BLOB}skills/a/SKILL.md.`,
    `and ${BLOB}skills/b/SKILL.md,`,
  ].join("\n");
  assert.deepStrictEqual(
    extractRepoLinks(md).map((l) => l.repoPath),
    [
      "plugins/open-code-review/claude-code/commands/review.md",
      "skills/a/SKILL.md",
      "skills/b/SKILL.md",
    ]
  );
}

function testFragmentAndQueryStripped() {
  const md = `[x](${BLOB}AGENTS.md#code-style) and [y](${BLOB}README.md?plain=1)`;
  assert.deepStrictEqual(
    extractRepoLinks(md).map((l) => l.repoPath),
    ["AGENTS.md", "README.md"]
  );
}

function testTrailingSlashOnTreeUrl() {
  assert.deepStrictEqual(
    extractRepoLinks(`[d](${TREE}plugins/open-code-review/)`).map((l) => l.repoPath),
    ["plugins/open-code-review"]
  );
}

function testBackticksAndQuotesTerminatePath() {
  const md = [`\`${RAW}install.sh\``, `"${RAW}action.yml"`].join("\n");
  assert.deepStrictEqual(
    extractRepoLinks(md).map((l) => l.repoPath),
    ["install.sh", "action.yml"]
  );
}

function testSchemelessUrlStillMatches() {
  assert.deepStrictEqual(
    extractRepoLinks(`bare github.com/alibaba/open-code-review/blob/main/Makefile here`).map(
      (l) => l.repoPath
    ),
    ["Makefile"]
  );
}

function testOtherReposIgnored() {
  const md = [
    "https://raw.githubusercontent.com/other/project/main/nope.md",
    "https://github.com/anthropics/claude-code/blob/main/nope.md",
  ].join("\n");
  assert.deepStrictEqual(extractRepoLinks(md), []);
}

function testNonPathUrlsIgnored() {
  // Issue, PR, release and bare-repo URLs carry no in-repo path to verify.
  const md = [
    "https://github.com/alibaba/open-code-review/issues/1100",
    "https://github.com/alibaba/open-code-review/pull/424",
    "https://github.com/alibaba/open-code-review/releases/download/v1.0.0/x",
    "https://github.com/alibaba/open-code-review",
  ].join("\n");
  assert.deepStrictEqual(extractRepoLinks(md), []);
}

function testAllowMarkerSkipsLine() {
  const md = `${BLOB}gone/forever.md  <!-- ${LINK_ALLOW_MARKER}: fixture -->`;
  assert.deepStrictEqual(extractRepoLinks(md), []);
}

function testMultipleLinksOnOneLine() {
  const md = `[a](${BLOB}A.md) and [b](${BLOB}B.md)`;
  assert.deepStrictEqual(
    extractRepoLinks(md).map((l) => `${l.line}:${l.repoPath}`),
    ["1:A.md", "1:B.md"]
  );
}

function testStripTrailingPunctuationAndToRepoPath() {
  assert.strictEqual(stripTrailingPunctuation("a/b.md\uFF0C"), "a/b.md");
  assert.strictEqual(stripTrailingPunctuation("a/b.md"), "a/b.md");
  assert.strictEqual(toRepoPath("a/b.md#x?y=1"), "a/b.md");
  assert.strictEqual(toRepoPath("a/b%20c.md"), "a/b c.md");
  assert.strictEqual(toRepoPath("#anchor-only"), null);
  // A malformed percent escape must not throw; the raw form is kept so the
  // existence probe reports it.
  assert.strictEqual(toRepoPath("a/%zz.md"), "a/%zz.md");
}

// --- link validation -------------------------------------------------------

function fakeProbe(map) {
  return (p) => (Object.prototype.hasOwnProperty.call(map, p) ? map[p] : null);
}

function testMissingPathIsBlockingError() {
  const { errors, warnings } = validateRepoLinks(
    [{ file: "docs/a.md", repoPath: "gone.md", kind: "file", line: 3 }],
    fakeProbe({})
  );
  assert.strictEqual(warnings.length, 0);
  assert.strictEqual(errors.length, 1);
  assert.strictEqual(errors[0].file, "docs/a.md");
  assert.strictEqual(errors[0].line, 3);
  assert.ok(/does not exist/.test(errors[0].message));
}

function testExistingPathsPass() {
  const { errors, warnings } = validateRepoLinks(
    [
      { file: "d.md", repoPath: "a.md", kind: "file", line: 1 },
      { file: "d.md", repoPath: "dir", kind: "tree", line: 2 },
    ],
    fakeProbe({ "a.md": "file", dir: "dir" })
  );
  assert.deepStrictEqual(errors, []);
  assert.deepStrictEqual(warnings, []);
}

function testKindMismatchIsOnlyWarning() {
  // GitHub redirects blob<->tree, so a kind mismatch must not fail the build.
  const { errors, warnings } = validateRepoLinks(
    [
      { file: "d.md", repoPath: "internal/agent", kind: "file", line: 4 },
      { file: "d.md", repoPath: "Makefile", kind: "tree", line: 5 },
    ],
    fakeProbe({ "internal/agent": "dir", Makefile: "file" })
  );
  assert.deepStrictEqual(errors, []);
  assert.strictEqual(warnings.length, 2);
  assert.ok(/prefer a `tree\/main\/` URL/.test(warnings[0].message));
  assert.ok(/prefer a `blob\/main\/` URL/.test(warnings[1].message));
}

// --- frontmatter -----------------------------------------------------------

function testParseFrontmatterTopLevelKeysOnly() {
  const md = [
    "---",
    "name: open-code-review",
    "description: >",
    "  folded line one",
    "  folded line two",
    "metadata:",
    "  author: alibaba",
    "---",
    "",
    "# Body",
    "name: not-frontmatter",
  ].join("\n");
  const { ok, keys } = parseFrontmatter(md);
  assert.strictEqual(ok, true);
  assert.strictEqual(keys.name.value, "open-code-review");
  // A folded scalar's value is the block that follows, not the `>` indicator.
  assert.strictEqual(keys.description.value, "folded line one folded line two");
  assert.ok(keys.metadata);
  // Indented keys belong to the parent block, not the top level.
  assert.strictEqual(keys.author, undefined);
}

function testMultiLineScalarFormsAreNotSeenAsEmpty() {
  // A plain multi-line scalar is valid YAML and loaders read it fine; treating
  // it as empty would be a blocking false positive.
  const plain = [
    "---",
    "name: a",
    "description:",
    "  Performs AI-powered code review on Git changes.",
    "---",
  ].join("\n");
  assert.strictEqual(
    parseFrontmatter(plain).keys.description.value,
    "Performs AI-powered code review on Git changes."
  );
  assert.deepStrictEqual(validateSkillFrontmatter("skills/a/SKILL.md", "a", plain), []);

  // Literal and chomped block indicators are syntax too, not content.
  for (const indicator of ["|", ">-", "|+", ">2", ">-2", ">2-", "|2+"]) {
    const md = ["---", "name: a", `description: ${indicator}`, "  body text", "---"].join("\n");
    assert.strictEqual(parseFrontmatter(md).keys.description.value, "body text", indicator);
  }

  // The reverse miss: an indicator with nothing under it really is empty.
  const emptyBlock = ["---", "name: a", "description: >", "---"].join("\n");
  assert.strictEqual(parseFrontmatter(emptyBlock).keys.description.value, "");
  assert.strictEqual(
    validateSkillFrontmatter("skills/a/SKILL.md", "a", emptyBlock).length,
    1
  );

  // A block scalar spanning a blank line keeps both halves.
  const withBlank = [
    "---",
    "name: a",
    "description: >",
    "  first",
    "",
    "  second",
    "---",
  ].join("\n");
  assert.strictEqual(parseFrontmatter(withBlank).keys.description.value, "first second");

  // A column-0 sequence belongs to the key above it, not the top level.
  const seq = ["---", "name: a", "keywords:", "- one", "- two", "---"].join("\n");
  const seqKeys = parseFrontmatter(seq).keys;
  assert.strictEqual(seqKeys.keywords.value, "- one - two");
  assert.strictEqual(seqKeys.one, undefined);
}

function testParseFrontmatterRejectsMissingAndUnterminated() {
  assert.strictEqual(parseFrontmatter("# No frontmatter").ok, false);
  assert.strictEqual(parseFrontmatter("---\nname: x\nbody with no close").ok, false);
}

function testSkillFrontmatterNameMustMatchDirectory() {
  const content = "---\nname: wrong-name\ndescription: x\n---\n";
  const errors = validateSkillFrontmatter("skills/right/SKILL.md", "right", content);
  assert.strictEqual(errors.length, 1);
  assert.ok(/declares `name: wrong-name`/.test(errors[0].message));
  assert.ok(/lives in `right\/`/.test(errors[0].message));
}

function testQuotedScalarIsUnquoted() {
  // YAML permits `name: "x"`; comparing the quoted form against a directory
  // name would fail with a message nobody could act on.
  assert.strictEqual(unquoteScalar('"open-code-review"'), "open-code-review");
  assert.strictEqual(unquoteScalar("'open-code-review'"), "open-code-review");
  // Unbalanced or inner quotes are left alone.
  assert.strictEqual(unquoteScalar("\"mismatched'"), "\"mismatched'");
  assert.strictEqual(unquoteScalar('say "hi"'), 'say "hi"');
  assert.deepStrictEqual(
    validateSkillFrontmatter("skills/a/SKILL.md", "a", '---\nname: "a"\ndescription: x\n---\n'),
    []
  );
}

function testProtoKeyDoesNotHitThePrototype() {
  // Collected into a null-prototype map, so a key named __proto__ is an
  // ordinary entry rather than a prototype setter call.
  const { ok, keys } = parseFrontmatter("---\n__proto__: x\nname: a\n---\n");
  assert.strictEqual(ok, true);
  assert.strictEqual(keys.__proto__.value, "x");
  assert.strictEqual(keys.name.value, "a");
}

function testSkillDescriptionMustBeNonEmpty() {
  // Present-but-empty is as useless to an agent as absent, and the command
  // check already held this line.
  const errors = validateSkillFrontmatter(
    "skills/a/SKILL.md",
    "a",
    "---\nname: a\ndescription:\n---\n"
  );
  assert.strictEqual(errors.length, 1);
  assert.ok(/missing a non-empty `description`/.test(errors[0].message));
}

function testSkillFrontmatterHappyPath() {
  const content = "---\nname: right\ndescription: >\n  text\n---\n";
  assert.deepStrictEqual(
    validateSkillFrontmatter("skills/right/SKILL.md", "right", content),
    []
  );
}

function testSkillFrontmatterMissingKeys() {
  const errors = validateSkillFrontmatter("skills/a/SKILL.md", "a", "---\nlicense: x\n---\n");
  assert.strictEqual(errors.length, 2);
  assert.ok(errors.some((e) => /missing `name`/.test(e.message)));
  assert.ok(errors.some((e) => /missing a non-empty `description`/.test(e.message)));
}

function testSkillFrontmatterAbsentBlock() {
  const errors = validateSkillFrontmatter("skills/a/SKILL.md", "a", "# Just a heading\n");
  assert.strictEqual(errors.length, 1);
  assert.ok(/no terminated YAML frontmatter/.test(errors[0].message));
}

function testCommandFrontmatter() {
  assert.deepStrictEqual(
    validateCommandFrontmatter("c/review.md", "---\ndescription: Run OCR.\n---\n"),
    []
  );
  // Present but empty is as useless to the host as absent.
  assert.strictEqual(
    validateCommandFrontmatter("c/review.md", "---\ndescription:\n---\n").length,
    1
  );
  assert.strictEqual(validateCommandFrontmatter("c/review.md", "Run OCR.\n").length, 1);
}

// --- declared path resolution ---------------------------------------------

function testResolveDeclaredPath() {
  const codex = "plugins/open-code-review/.codex-plugin/plugin.json";
  // plugin-root = parent of the manifest directory.
  assert.strictEqual(
    resolveDeclaredPath(codex, "plugin-root", "./skills/"),
    "plugins/open-code-review/skills"
  );
  // manifest-dir = the directory holding the manifest.
  assert.strictEqual(
    resolveDeclaredPath(codex, "manifest-dir", "../skills/"),
    "plugins/open-code-review/skills"
  );
  assert.strictEqual(
    resolveDeclaredPath(codex, "manifest-dir", "./skills/"),
    "plugins/open-code-review/.codex-plugin/skills"
  );
  assert.strictEqual(
    resolveDeclaredPath(
      "plugins/open-code-review/claude-code/.claude-plugin/plugin.json",
      "plugin-root",
      "./commands"
    ),
    "plugins/open-code-review/claude-code/commands"
  );
}

const CODEX_SPEC = {
  file: "p/.codex-plugin/plugin.json",
  field: "skills",
  base: "plugin-root",
  target: "p/skills",
  kind: "skills",
  host: "Codex",
};

function testAbsoluteDeclaredPathIsRejected() {
  // path.join would treat "/skills/" as a relative fragment and collapse it
  // onto the expected target, certifying a manifest no host can load.
  const codex = "plugins/open-code-review/.codex-plugin/plugin.json";
  assert.strictEqual(resolveDeclaredPath(codex, "plugin-root", "/skills/"), "/skills");
  const { errors } = checkPluginDeclarations([CODEX_SPEC], () => ({ skills: "/skills/" }));
  assert.strictEqual(errors.length, 1);
  assert.ok(/resolves to `\/skills`/.test(errors[0].message));
}

function testCheckPluginDeclarationsHappyPath() {
  const { errors, warnings } = checkPluginDeclarations([CODEX_SPEC], () => ({
    skills: "./skills/",
  }));
  assert.deepStrictEqual(errors, []);
  assert.deepStrictEqual(warnings, []);
}

function testUnverifiedBaseAlwaysWarnsButNeverBlocks() {
  // An assumption the check cannot validate must be stated on every run rather
  // than passing silently, but it must not fail the build either.
  const spec = { ...CODEX_SPEC, unverifiedBase: "spec not checked." };
  const { errors, warnings } = checkPluginDeclarations([spec], () => ({
    skills: "./skills/",
  }));
  assert.deepStrictEqual(errors, []);
  assert.strictEqual(warnings.length, 1);
  assert.ok(/UNVERIFIED resolution base \(plugin-root\)/.test(warnings[0].message));
  assert.ok(/spec not checked\./.test(warnings[0].message));
}

function testRealCursorEntryCarriesTheUnverifiedFlag() {
  // Guards the reasoning recorded in PLUGIN_DECLARATIONS: the Cursor manifest
  // resolves `../skills/` from the manifest directory while its two siblings
  // resolve from the plugin root, and that asymmetry is unverified. If someone
  // confirms Cursor's spec and drops the flag, this test should be dropped with it.
  const cursor = PLUGIN_DECLARATIONS.find((d) => d.host === "Cursor");
  assert.ok(cursor, "expected a Cursor declaration");
  assert.strictEqual(cursor.base, "manifest-dir");
  assert.ok(
    cursor.unverifiedBase,
    "the Cursor base differs from its siblings and must stay flagged as unverified"
  );
}

function testCheckPluginDeclarationsCatchesDrift() {
  const specs = [CODEX_SPEC];
  // Someone renamed the tree in the manifest but not on disk (or vice versa).
  const drifted = checkPluginDeclarations(specs, () => ({ skills: "./agent-skills/" }));
  assert.strictEqual(drifted.errors.length, 1);
  assert.ok(/resolves to `p\/agent-skills`/.test(drifted.errors[0].message));

  // Field dropped entirely: installs clean, exposes nothing.
  const dropped = checkPluginDeclarations(specs, () => ({}));
  assert.strictEqual(dropped.errors.length, 1);
  assert.ok(/does not declare a `skills` path/.test(dropped.errors[0].message));

  // Non-string value.
  const wrongType = checkPluginDeclarations(specs, () => ({ skills: ["./skills/"] }));
  assert.strictEqual(wrongType.errors.length, 1);

  // Manifest missing or unparseable.
  const missing = checkPluginDeclarations(specs, () => null);
  assert.strictEqual(missing.errors.length, 1);
  assert.ok(/missing or is not valid JSON/.test(missing.errors[0].message));
}

// --- malformed marketplace manifests ---------------------------------------

function testPluginEntriesToleratesMalformedShapes() {
  // A malformed manifest must degrade to "lists no installable plugin source",
  // reported as an annotation, rather than throwing a TypeError and exiting on
  // a stack trace.
  assert.deepStrictEqual(pluginEntries(null), []);
  assert.deepStrictEqual(pluginEntries({}), []);
  assert.deepStrictEqual(pluginEntries({ plugins: {} }), []);
  assert.deepStrictEqual(pluginEntries({ plugins: "nope" }), []);
  assert.deepStrictEqual(pluginEntries({ plugins: [null, "x", 7] }), []);
  assert.deepStrictEqual(pluginEntries({ plugins: [{ name: "a" }] }), [{ name: "a" }]);
}

function testMarketplaceReadersDoNotThrowOnMalformedInput() {
  for (const mp of MARKETPLACES) {
    for (const bad of [null, {}, { plugins: {} }, { plugins: [null] }, { plugins: [{}] }]) {
      assert.doesNotThrow(() => mp.readSources(bad), `${mp.file} on ${JSON.stringify(bad)}`);
    }
  }
}

// --- configuration sanity --------------------------------------------------

function testDeclarationTargetsAreCoveredByTheTreesWeValidate() {
  // Every skills/commands target a manifest points at must also be a tree this
  // script validates the contents of; otherwise a manifest could point at a
  // directory nobody checks for well-formed skills.
  const validated = new Set([...SKILL_ROOTS, ...COMMAND_DIRS]);
  for (const spec of PLUGIN_DECLARATIONS) {
    assert.ok(
      validated.has(spec.target),
      `${spec.file} targets ${spec.target}, which is not in SKILL_ROOTS or COMMAND_DIRS`
    );
  }
}

// --- end-to-end over temp fixtures ----------------------------------------
//
// These runners are deliberately NOT pointed at the real repository. The two
// workflow steps already run them against the work tree, and doing it here too
// would mean a stale doc link anywhere fails this unit test with a stack trace
// instead of the `::error file=...,line=...` annotations the checks exist to
// produce — and would fail the unrelated Action Contract workflow, which also
// runs `npm run test:github-actions`.

function withTempDir(fn) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ocr-plugin-contract-"));
  try {
    return fn(dir);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

function testListEntriesResolvesSymlinks() {
  withTempDir((tmp) => {
    // A skills tree whose entries are symlinks - the layout someone would reach
    // for to stop maintaining two copies of the same skills. A Dirent reports a
    // symlink as neither a file nor a directory, so filtering on isDirectory()
    // alone would report this tree as empty and block the build.
    fs.mkdirSync(path.join(tmp, "real", "skill-a"), { recursive: true });
    fs.writeFileSync(
      path.join(tmp, "real", "skill-a", "SKILL.md"),
      "---\nname: skill-a\n---\n"
    );
    fs.writeFileSync(path.join(tmp, "real", "prompt.md"), "---\ndescription: x\n---\n");
    fs.mkdirSync(path.join(tmp, "tree"));
    fs.symlinkSync(path.join(tmp, "real", "skill-a"), path.join(tmp, "tree", "linked-skill"));
    fs.symlinkSync(path.join(tmp, "real", "prompt.md"), path.join(tmp, "tree", "linked.md"));
    fs.symlinkSync(path.join(tmp, "real", "gone"), path.join(tmp, "tree", "dangling"));
    fs.mkdirSync(path.join(tmp, "tree", "plain-dir"));
    fs.writeFileSync(path.join(tmp, "tree", "plain.md"), "x");

    assert.deepStrictEqual(listEntries(tmp, "tree", "dir"), ["linked-skill", "plain-dir"]);
    assert.deepStrictEqual(listEntries(tmp, "tree", "file", ".md"), ["linked.md", "plain.md"]);
    // A dangling link belongs to neither list rather than crashing the walk.
    assert.ok(!listEntries(tmp, "tree", "dir").includes("dangling"));
    assert.ok(!listEntries(tmp, "tree", "file").includes("dangling"));
    // An unreadable directory is null, distinct from an empty one.
    assert.strictEqual(listEntries(tmp, "no-such-dir", "dir"), null);
    assert.deepStrictEqual(listEntries(tmp, "tree/plain-dir", "dir"), []);
  });
}

function testWalkFollowsSymlinksWithoutLooping() {
  withTempDir((tmp) => {
    // Symlinked docs must be scanned, matching listEntries; a link pointing at
    // an ancestor must not loop, and a linked subtree must not be walked twice.
    fs.mkdirSync(path.join(tmp, "real"));
    fs.writeFileSync(path.join(tmp, "real", "doc.md"), "x");
    fs.writeFileSync(path.join(tmp, "plain.md"), "x");
    fs.symlinkSync(path.join(tmp, "real", "doc.md"), path.join(tmp, "linked.md"));
    fs.symlinkSync(path.join(tmp, "real"), path.join(tmp, "linked-dir"));
    fs.symlinkSync(tmp, path.join(tmp, "loop"));
    fs.symlinkSync(path.join(tmp, "nowhere"), path.join(tmp, "dangling.md"));

    const files = walkTextFiles(tmp);
    assert.ok(files.includes("linked.md"), "a symlinked doc must be scanned");
    assert.ok(files.includes("plain.md"));
    // The directory alias and the real directory hold the same file. Whichever
    // name the sorted walk reaches first wins and the other is skipped as
    // already-seen, so `doc.md` is yielded exactly once under exactly one of
    // the two paths — never twice, and never not at all.
    const docPaths = files.filter((f) => f.endsWith("doc.md") && f !== "linked.md");
    assert.deepStrictEqual(docPaths, ["linked-dir/doc.md"], JSON.stringify(files));
    assert.ok(!files.includes("dangling.md"), "a dangling link is not a file");
    // The self-loop terminated: the walk returned at all, and no path repeats.
    assert.strictEqual(new Set(files).size, files.length);
    // Nothing was reached through the self-loop alias.
    assert.ok(!files.some((f) => f.startsWith("loop/")), JSON.stringify(files));
  });
}

function testUnscannedDocsAreReportedNotSkippedSilently() {
  withTempDir((tmp) => {
    // A doc too large for the scan is a hole in the corpus; passing over it in
    // silence would contradict the fail-closed floor.
    fs.writeFileSync(path.join(tmp, "a.md"), "x");
    fs.writeFileSync(path.join(tmp, "doc.md"), `link: ${BLOB}a.md`);
    fs.writeFileSync(path.join(tmp, "huge.md"), "y".repeat(1024));
    fs.writeFileSync(path.join(tmp, "blob.bin"), Buffer.from([0x00, 0x01]));

    const { code, lines } = withCapturedStdout(() =>
      runLinksCheck({ repoRoot: tmp, minLinks: 1, minFiles: 1, maxScanBytes: 512 })
    );
    assert.strictEqual(code, 0);
    const warnings = lines.filter((l) => l.startsWith("::warning"));
    assert.strictEqual(warnings.length, 1, warnings.join("\n"));
    assert.ok(/huge\.md/.test(warnings[0]));
    assert.ok(/links were NOT verified/.test(warnings[0]));
    // A binary file is not documentation, so it is not reported.
    assert.ok(!warnings.some((l) => /blob\.bin/.test(l)));
    assert.ok(lines.some((l) => /1 unscanned doc\(s\)/.test(l)));
  });
}

function testReadFileOrNullSizeCapIsOptional() {
  withTempDir((tmp) => {
    // The size cap exists for the link scan; contract files are read whatever
    // their size, so an oversized-but-valid SKILL.md is never mistaken for a
    // missing one.
    const file = path.join(tmp, "big.md");
    const body = "x".repeat(4096);
    fs.writeFileSync(file, body);
    assert.strictEqual(readFileOrNull(file, { maxBytes: 128 }), null);
    assert.strictEqual(readFileOrNull(file, { maxBytes: Infinity }), body);
    assert.strictEqual(readFileOrNull(path.join(tmp, "absent.md")), null);
    // A binary file is not text, whatever the cap.
    const bin = path.join(tmp, "bin.dat");
    fs.writeFileSync(bin, Buffer.from([0x61, 0x00, 0x62]));
    assert.strictEqual(readFileOrNull(bin, { maxBytes: Infinity }), null);
    // A directory is not a file.
    assert.strictEqual(readFileOrNull(tmp, { maxBytes: Infinity }), null);
  });
}

function withCapturedStdout(fn) {
  const orig = console.log;
  const lines = [];
  console.log = (...args) => lines.push(args.join(" "));
  try {
    const code = fn();
    return { code, lines };
  } finally {
    console.log = orig;
  }
}

function testCorpusFloorFailsClosed() {
  // A scan that finds nothing must fail rather than report success.
  assert.strictEqual(checkCorpusFloor(146, 53, 50, 10), null);
  assert.strictEqual(checkCorpusFloor(50, 10, 50, 10), null);
  assert.ok(/below the floor of 50\/10/.test(checkCorpusFloor(0, 0, 50, 10)));
  assert.ok(/Only 49 link\(s\)/.test(checkCorpusFloor(49, 20, 50, 10)));
  // Enough links but concentrated in too few files: also a broken walk.
  assert.ok(checkCorpusFloor(200, 9, 50, 10));
  // The shipped floor must stay meaningful.
  assert.ok(MIN_EXPECTED_LINKS >= 10);
}

function testLinksRunnerOnFixture() {
  withTempDir((tmp) => {
    fs.writeFileSync(path.join(tmp, "present.md"), "hello");
    fs.mkdirSync(path.join(tmp, "adir"));
    fs.writeFileSync(
      path.join(tmp, "doc.md"),
      [
        `ok file: ${BLOB}present.md`,
        `ok dir: ${TREE}adir`,
        `kind warning: ${BLOB}adir`,
        `broken: ${RAW}gone/missing.md`,
      ].join("\n")
    );
    // Floors lowered: the fixture corpus is intentionally tiny.
    const { code, lines } = withCapturedStdout(() =>
      runLinksCheck({ repoRoot: tmp, minLinks: 1, minFiles: 1 })
    );
    assert.strictEqual(code, 1);
    const errors = lines.filter((l) => l.startsWith("::error"));
    const warnings = lines.filter((l) => l.startsWith("::warning"));
    // Exactly one blocking error (the missing path), and the kind mismatch
    // stays advisory.
    assert.strictEqual(errors.filter((l) => /does not exist/.test(l)).length, 1);
    assert.ok(errors.some((l) => /gone\/missing\.md/.test(l)));
    assert.strictEqual(warnings.length, 1);
    assert.ok(/prefer a `tree\/main\/` URL/.test(warnings[0]));
  });
}

function testLinksRunnerPassesAndReportsCounts() {
  withTempDir((tmp) => {
    fs.writeFileSync(path.join(tmp, "a.md"), "x");
    fs.writeFileSync(path.join(tmp, "doc.md"), `link: ${BLOB}a.md`);
    const { code, lines } = withCapturedStdout(() =>
      runLinksCheck({ repoRoot: tmp, minLinks: 1, minFiles: 1 })
    );
    assert.strictEqual(code, 0);
    assert.ok(lines.some((l) => /In-repo link check passed: 1 link\(s\) across 1 file/.test(l)));
  });
}

function testLinksRunnerTripsTheFloorOnAnEmptyTree() {
  withTempDir((tmp) => {
    const { code, lines } = withCapturedStdout(() => runLinksCheck({ repoRoot: tmp }));
    assert.strictEqual(code, 1);
    assert.ok(lines.some((l) => /below the floor/.test(l)));
  });
}

function testManifestsRunnerReportsMissingManifests() {
  withTempDir((tmp) => {
    // An empty tree: every declared manifest and tree is absent, and each is
    // reported as an annotation rather than throwing.
    const { code, lines } = withCapturedStdout(() => runManifestsCheck({ repoRoot: tmp }));
    assert.strictEqual(code, 1);
    assert.ok(lines.some((l) => /marketplace manifest .* is missing/.test(l)));
    assert.ok(lines.some((l) => /Skill tree `skills` is missing/.test(l)));
    assert.ok(lines.some((l) => /Command directory .* is missing/.test(l)));
  });
}

function testAnnotationDataIsEscaped() {
  // GitHub truncates an annotation at an unescaped newline, losing exactly the
  // part worth reading. Manifest values and frontmatter scalars are
  // interpolated into these messages, so any of them can carry one.
  assert.strictEqual(escapeAnnotationData("a\nb"), "a%0Ab");
  assert.strictEqual(escapeAnnotationData("a\r\nb"), "a%0D%0Ab");
  assert.strictEqual(escapeAnnotationData("100%"), "100%25");
  assert.strictEqual(escapeAnnotationData("plain"), "plain");

  // Escaping belongs to the emitter, so no call site can forget it.
  const { lines } = withCapturedStdout(() => {
    emitAnnotation("warning", "a.json", "one\ntwo", 7);
    return 0;
  });
  assert.strictEqual(lines[0], "::warning file=a.json,line=7::one%0Atwo");
}

function testUnknownModeIsAnError() {
  const { code, lines } = withCapturedStdout(() => main(["nope"], {}));
  assert.strictEqual(code, 2);
  assert.ok(lines.some((l) => /Unknown mode/.test(l)));
}

function testMainDispatchesEachMode() {
  withTempDir((tmp) => {
    // Both modes reach their runner and fail closed on an empty fixture tree.
    const links = withCapturedStdout(() => main(["links"], { OCR_REPO_ROOT: tmp }));
    assert.strictEqual(links.code, 1);
    assert.ok(links.lines.some((l) => /below the floor/.test(l)));

    const manifests = withCapturedStdout(() => main(["manifests"], { OCR_REPO_ROOT: tmp }));
    assert.strictEqual(manifests.code, 1);
    assert.ok(manifests.lines.some((l) => /marketplace manifest/.test(l)));

    // `all` fails if either does.
    const all = withCapturedStdout(() => main([], { OCR_REPO_ROOT: tmp }));
    assert.strictEqual(all.code, 1);
  });
}

function main_() {
  testExtractsRawCurlUrl();
  testBlobIsFileAndTreeIsDir();
  testMarkdownLinkParenNotSwallowed();
  testTrailingProsePunctuationTrimmed();
  testFragmentAndQueryStripped();
  testTrailingSlashOnTreeUrl();
  testBackticksAndQuotesTerminatePath();
  testSchemelessUrlStillMatches();
  testOtherReposIgnored();
  testNonPathUrlsIgnored();
  testAllowMarkerSkipsLine();
  testMultipleLinksOnOneLine();
  testStripTrailingPunctuationAndToRepoPath();
  testMissingPathIsBlockingError();
  testExistingPathsPass();
  testKindMismatchIsOnlyWarning();
  testParseFrontmatterTopLevelKeysOnly();
  testParseFrontmatterRejectsMissingAndUnterminated();
  testMultiLineScalarFormsAreNotSeenAsEmpty();
  testSkillFrontmatterNameMustMatchDirectory();
  testQuotedScalarIsUnquoted();
  testProtoKeyDoesNotHitThePrototype();
  testSkillDescriptionMustBeNonEmpty();
  testSkillFrontmatterHappyPath();
  testSkillFrontmatterMissingKeys();
  testSkillFrontmatterAbsentBlock();
  testCommandFrontmatter();
  testResolveDeclaredPath();
  testAbsoluteDeclaredPathIsRejected();
  testCheckPluginDeclarationsHappyPath();
  testUnverifiedBaseAlwaysWarnsButNeverBlocks();
  testRealCursorEntryCarriesTheUnverifiedFlag();
  testCheckPluginDeclarationsCatchesDrift();
  testPluginEntriesToleratesMalformedShapes();
  testMarketplaceReadersDoNotThrowOnMalformedInput();
  testListEntriesResolvesSymlinks();
  testWalkFollowsSymlinksWithoutLooping();
  testUnscannedDocsAreReportedNotSkippedSilently();
  testReadFileOrNullSizeCapIsOptional();
  testDeclarationTargetsAreCoveredByTheTreesWeValidate();
  testCorpusFloorFailsClosed();
  testLinksRunnerOnFixture();
  testLinksRunnerPassesAndReportsCounts();
  testLinksRunnerTripsTheFloorOnAnEmptyTree();
  testManifestsRunnerReportsMissingManifests();
  testAnnotationDataIsEscaped();
  testUnknownModeIsAnError();
  testMainDispatchesEachMode();
  console.log("All check-plugin-contract tests passed.");
}

try {
  main_();
} catch (err) {
  console.error(err);
  process.exit(1);
}
