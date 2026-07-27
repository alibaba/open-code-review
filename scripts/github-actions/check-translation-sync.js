"use strict";

// OpenCodeReview translation-sync guardrails.
//
// Two independent checks, both runnable as plain Node (no npm deps, node >= 14,
// same convention as scripts/github-actions/post-review-comments.js):
//
//   1. README structure check (BLOCKING). All README*.md translations must
//      share an identical `##` (level-2) heading structure. Because the files
//      are translations, the heading TEXT differs by language, so this compares
//      STRUCTURE not text: the level-2 heading count and the full heading
//      outline (the ordered sequence of heading levels), which is
//      language-independent. PR #424 normalized all five READMEs to an
//      identical structure; this keeps them from drifting apart. Fails with a
//      non-zero exit and `::error` annotations when any file diverges.
//
//   2. Docs translation-sync annotation (NON-BLOCKING). When a PR changes a
//      file under pages/src/content/docs/en/** without also changing the
//      matching zh/ja counterpart path, emit a `::warning` PR annotation. This
//      never fails the build; it only nudges the author/reviewer.
//
// Invoked from .github/workflows/ci.yml:
//   node scripts/github-actions/check-translation-sync.js readmes   (blocking)
//   node scripts/github-actions/check-translation-sync.js docs      (non-blocking)
//
// The core logic is exported as pure functions so it can be unit-tested without
// touching the filesystem, GitHub, or git (see check-translation-sync.test.js).

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

// The five localized README files. README.md (English) is the reference every
// translation is compared against. Discovered names confirmed in the repo root.
const README_FILES = [
  "README.md",
  "README.zh-CN.md",
  "README.ja-JP.md",
  "README.ko-KR.md",
  "README.ru-RU.md",
];

// Docs live under pages/src/content/docs/<locale>/**. English is authored under
// en/; zh/ and ja/ mirror the exact same relative sub-paths.
const DOCS_EN_PREFIX = "pages/src/content/docs/en/";
const DOCS_LOCALES = ["zh", "ja"];

// ---------------------------------------------------------------------------
// Markdown heading parsing
// ---------------------------------------------------------------------------

// Extract ATX headings (`#`..`######`), skipping any that live inside fenced
// code blocks (``` or ~~~). Fence-awareness matters: the READMEs contain bash
// snippets whose `# comment` lines would otherwise be misread as level-1
// headings. Returns [{ level, text, line }] in document order (1-based line).
function extractHeadings(markdown) {
  const lines = String(markdown).split(/\r?\n/);
  const headings = [];
  let fenceChar = null; // '`' or '~' while inside a fence, else null
  let fenceLen = 0; // length of the opening fence run (CommonMark: the close
  // must use the same char and be at least this long)
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const fence = /^\s{0,3}(`{3,}|~{3,})/.exec(line);
    if (fence) {
      const ch = fence[1][0];
      const len = fence[1].length;
      if (fenceChar === null) {
        fenceChar = ch; // opening fence
        fenceLen = len;
      } else if (fenceChar === ch && len >= fenceLen) {
        fenceChar = null; // matching closing fence (same char, long enough)
        fenceLen = 0;
      }
      // A shorter run of the same char inside the block (len < fenceLen) is
      // just code content, so fall through and skip it below.
      continue;
    }
    if (fenceChar !== null) continue; // inside a code fence
    const h = /^(#{1,6})\s+(.*?)\s*#*\s*$/.exec(line);
    if (h) headings.push({ level: h[1].length, text: h[2].trim(), line: i + 1 });
  }
  return headings;
}

// Ordered list of level-2 (`##`) heading texts.
function level2Headings(markdown) {
  return extractHeadings(markdown)
    .filter((h) => h.level === 2)
    .map((h) => h.text);
}

// Language-independent structure fingerprint: the ordered sequence of heading
// levels for every heading (outside code fences). Text is intentionally ignored
// because translations differ in wording; only the outline SHAPE is comparable.
function structuralSignature(markdown) {
  return extractHeadings(markdown).map((h) => h.level);
}

function sameSequence(a, b) {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

function firstDiffIndex(a, b) {
  const n = Math.min(a.length, b.length);
  for (let i = 0; i < n; i++) if (a[i] !== b[i]) return i;
  return n; // sequences agree up to the shorter one's length
}

function describeHeading(h) {
  if (!h) return "no heading (document ends)";
  return `a level-${h.level} heading ("${h.text}")`;
}

// ---------------------------------------------------------------------------
// Check 1: README structure (blocking)
// ---------------------------------------------------------------------------

// Compare the heading structure of translated READMEs against a reference.
// files: [{ name, content }] — the FIRST entry is the reference (English).
// Returns { ok, errors } where errors is [{ file, message }].
function compareReadmeStructures(files) {
  const errors = [];
  if (files.length < 2) return { ok: true, errors };

  const ref = files[0];
  const refHeadings = extractHeadings(ref.content);
  const refSig = refHeadings.map((h) => h.level);
  const refL2 = refSig.filter((l) => l === 2).length;

  for (let i = 1; i < files.length; i++) {
    const f = files[i];
    const fHeadings = extractHeadings(f.content);
    const fSig = fHeadings.map((h) => h.level);
    const fL2 = fSig.filter((l) => l === 2).length;

    if (sameSequence(refSig, fSig)) continue; // structurally identical

    if (fL2 !== refL2) {
      errors.push({
        file: f.name,
        message:
          `${f.name} has ${fL2} level-2 (##) heading(s), but the reference ` +
          `${ref.name} has ${refL2}. All README translations must share an ` +
          `identical ## section structure (see PR #424).`,
      });
    } else {
      const idx = firstDiffIndex(refSig, fSig);
      errors.push({
        file: f.name,
        message:
          `${f.name} heading outline diverges from ${ref.name} at heading ` +
          `#${idx + 1}: expected ${describeHeading(refHeadings[idx])}, found ` +
          `${describeHeading(fHeadings[idx])}. A heading was reordered or its ` +
          `level changed; README translations must share an identical ` +
          `structure (see PR #424).`,
      });
    }
  }
  return { ok: errors.length === 0, errors };
}

// ---------------------------------------------------------------------------
// Check 2: docs en -> zh/ja counterpart sync (non-blocking)
// ---------------------------------------------------------------------------

// Given an English docs path, return its expected translation counterparts.
function counterpartPaths(enPath) {
  const rest = enPath.slice(DOCS_EN_PREFIX.length);
  return DOCS_LOCALES.map((locale) => ({
    locale,
    path: `pages/src/content/docs/${locale}/${rest}`,
  }));
}

// From a changed-files list, find English docs that changed without their
// zh/ja counterpart also changing. Returns [{ enPath, locale, counterpart }].
function findMissingTranslations(changedFiles) {
  const set = new Set(changedFiles.map((p) => p.replace(/\\/g, "/")));
  const warnings = [];
  for (const raw of changedFiles) {
    const file = raw.replace(/\\/g, "/");
    if (!file.startsWith(DOCS_EN_PREFIX)) continue;
    const lower = file.toLowerCase();
    if (!lower.endsWith(".md") && !lower.endsWith(".mdx")) continue;
    for (const cp of counterpartPaths(file)) {
      if (!set.has(cp.path)) {
        warnings.push({ enPath: file, locale: cp.locale, counterpart: cp.path });
      }
    }
  }
  return warnings;
}

// ---------------------------------------------------------------------------
// GitHub Actions annotation + IO helpers
// ---------------------------------------------------------------------------

function emitError(file, message) {
  const loc = file ? ` file=${file}` : "";
  console.log(`::error${loc}::${message}`);
}

function emitWarning(file, message) {
  const loc = file ? ` file=${file}` : "";
  console.log(`::warning${loc}::${message}`);
}

function splitList(raw) {
  return String(raw)
    .split(/[\r\n,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

// Resolve the PR's changed files. Precedence: an explicit OCR_CHANGED_FILES
// override (newline/comma separated), else a three-dot git diff between the
// base and head SHAs (needs fetch-depth: 0), else an empty set.
function getChangedFiles(env, exec = execFileSync) {
  if (env.OCR_CHANGED_FILES) return splitList(env.OCR_CHANGED_FILES);
  const base = env.OCR_BASE_SHA;
  const head = env.OCR_HEAD_SHA;
  if (base && head) {
    try {
      const out = exec("git", ["diff", "--name-only", `${base}...${head}`], {
        encoding: "utf8",
      });
      return splitList(out);
    } catch (e) {
      emitWarning(
        null,
        `Could not compute changed files via git diff (${e.message}); ` +
          `skipping docs translation-sync check.`
      );
      return [];
    }
  }
  return [];
}

function writeStepSummary(env, warnings) {
  const file = env.GITHUB_STEP_SUMMARY;
  if (!file || warnings.length === 0) return;
  const lines = [
    "### Translation sync (non-blocking)",
    "",
    "These English docs changed without their translation counterparts:",
    "",
  ];
  for (const w of warnings) {
    lines.push(`- \`${w.enPath}\` -> missing \`${w.counterpart}\` (${w.locale})`);
  }
  lines.push("");
  try {
    fs.appendFileSync(file, lines.join("\n") + "\n");
  } catch (e) {
    /* summary is best-effort */
  }
}

// ---------------------------------------------------------------------------
// CLI runners
// ---------------------------------------------------------------------------

// BLOCKING. Returns a process exit code (0 ok, 1 on divergence).
function runReadmeCheck({ repoRoot = process.cwd(), files = README_FILES } = {}) {
  const loaded = [];
  let missing = 0;
  for (const name of files) {
    try {
      loaded.push({ name, content: fs.readFileSync(path.join(repoRoot, name), "utf8") });
    } catch (e) {
      missing++;
      emitError(name, `Expected README translation file ${name} is missing.`);
    }
  }

  // Short-circuit when any expected file is absent: comparing only the files
  // that DID load can mask a real divergence (e.g. a missing reference would
  // silently promote the next file to reference). A missing translation is
  // itself a failure, so fail fast before the structure comparison.
  if (missing > 0) {
    emitError(
      null,
      "README translation structure check failed: one or more expected " +
        "README*.md files are missing (see the file annotations above)."
    );
    return 1;
  }

  const { ok, errors } = compareReadmeStructures(loaded);
  for (const err of errors) emitError(err.file, err.message);

  if (ok) {
    console.log(
      `README translation structure check passed: all ${loaded.length} ` +
        `README files share the same ## section structure.`
    );
    return 0;
  }
  emitError(
    null,
    "README translation structure check failed. All README*.md files must " +
      "share an identical ## section structure."
  );
  return 1;
}

// NON-BLOCKING. Always returns exit code 0; only emits `::warning` annotations.
function runDocsCheck({ env = process.env } = {}) {
  const changed = getChangedFiles(env);
  const warnings = findMissingTranslations(changed);
  if (warnings.length === 0) {
    console.log(
      "Docs translation-sync check: no en-only documentation changes detected."
    );
    return 0;
  }
  for (const w of warnings) {
    emitWarning(
      w.enPath,
      `Translation sync: ${w.enPath} changed but its ${w.locale} counterpart ` +
        `${w.counterpart} was not updated in this PR. Update the translation ` +
        `or note why it is intentionally out of sync.`
    );
  }
  console.log(
    `Docs translation-sync check: ${warnings.length} missing translation ` +
      `counterpart(s) (non-blocking).`
  );
  writeStepSummary(env, warnings);
  return 0;
}

function main(argv = process.argv.slice(2), env = process.env) {
  const mode = (argv[0] || "readmes").toLowerCase();
  const repoRoot = env.OCR_REPO_ROOT || process.cwd();
  if (mode === "docs") return runDocsCheck({ env });
  if (mode === "readmes" || mode === "readme") return runReadmeCheck({ repoRoot });
  emitError(null, `Unknown mode "${mode}". Use "readmes" or "docs".`);
  return 2;
}

if (require.main === module) {
  process.exit(main());
}

module.exports = {
  README_FILES,
  DOCS_EN_PREFIX,
  DOCS_LOCALES,
  extractHeadings,
  level2Headings,
  structuralSignature,
  sameSequence,
  firstDiffIndex,
  compareReadmeStructures,
  counterpartPaths,
  findMissingTranslations,
  getChangedFiles,
  splitList,
  runReadmeCheck,
  runDocsCheck,
  main,
};
