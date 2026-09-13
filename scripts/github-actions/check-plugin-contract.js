// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

// OpenCodeReview plugin/distribution contract guardrails.
//
// Two independent BLOCKING checks, both runnable as plain Node (no npm deps,
// node >= 14, same convention as scripts/github-actions/check-translation-sync.js):
//
//   1. `links` — in-repo path links. The docs and READMEs embed dozens of
//      `raw.githubusercontent.com/.../main/<path>` curl commands and
//      `github.com/.../blob|tree/main/<path>` links. Nothing verified that
//      `<path>` still exists, so moving or deleting a referenced file silently
//      turned every one of those into a 404 that only users hit. This resolves
//      each URL back to a repo-relative path and asserts it exists. No network
//      access.
//
//      A missing path is BLOCKING: the URL is served from the default branch,
//      so it is a 404 for every reader. A kind mismatch (a `blob`/raw URL on a
//      directory, or a `tree` URL on a file) is only a WARNING, because GitHub
//      redirects between the two views — the link still opens, it is just
//      written against the wrong form.
//
//   2. `manifests` — plugin packaging contract. Every path a plugin or
//      marketplace manifest declares must resolve to a real, non-empty
//      directory, and every SKILL.md / command prompt must carry the
//      frontmatter its loader requires. Without this a rename produces a
//      plugin that installs cleanly and exposes nothing.
//
// Invoked from .github/workflows/plugin-contract.yml:
//   node scripts/github-actions/check-plugin-contract.js links
//   node scripts/github-actions/check-plugin-contract.js manifests
// `all` runs both and fails if either does.
//
// The core logic is exported as pure functions so it can be unit-tested
// without touching the filesystem (see check-plugin-contract.test.js).

const fs = require("fs");
const path = require("path");

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const REPO_SLUG = "alibaba/open-code-review";

// Directories never scanned for links: VCS internals, installed/built output,
// editor state, and binary assets. `imgs/` holds only images, and coverage.out
// is a multi-hundred-KB generated artifact.
const SCAN_SKIP_DIRS = new Set([
  ".git",
  "node_modules",
  "dist",
  "build",
  ".astro",
  ".idea",
  ".vscode-test",
  "imgs",
  ".ossutil_checkpoint",
  "out",
]);

// Files above this size are skipped: no hand-written doc link lives in a
// generated blob, and reading them on every CI run is pure cost.
const MAX_SCAN_BYTES = 512 * 1024;

// Extensions whose files are documentation, and whose exclusion from the link
// scan is therefore worth reporting rather than passing over quietly.
const DOC_EXTENSIONS = /\.(md|mdx|markdown)$/i;

// Per-line escape hatch, mirroring the `allow-non-english: <reason>` marker
// that scripts/verify-english-only.go recognises. Intended for fixtures that
// must reference a deliberately absent path.
const LINK_ALLOW_MARKER = "plugin-contract: allow-missing";

// Fail-closed floor for the link corpus. A scan that quietly stops finding
// anything — a broken walk, a pattern that no longer matches, a renamed docs
// tree — otherwise reports success, which is how a check like this rots into
// decoration. The floor sits far below the real corpus (146 links across 53
// files at the time of writing): tripping it means the scan broke, not that the
// docs shrank.
const MIN_EXPECTED_LINKS = 50;
const MIN_EXPECTED_LINK_FILES = 10;

// Marketplace manifests. `readSources` extracts the pointers to plugin
// directories however that host spells them, and `requiredManifest` is the file
// each directory must contain for the install to expose anything.
//
// Both readers go through pluginEntries(), because a malformed manifest must
// produce a `::error` annotation rather than an uncaught TypeError and a stack
// trace: "plugins is not an array" is a diagnosis, a stack trace is homework.
const MARKETPLACES = [
  {
    file: ".claude-plugin/marketplace.json",
    host: "Claude Code",
    // Claude Code marketplace entries: `source` is a repo-root-relative string.
    readSources: (json) =>
      pluginEntries(json).map((p) => ({
        name: p.name,
        value: typeof p.source === "string" ? p.source : null,
      })),
    requiredManifest: ".claude-plugin/plugin.json",
  },
  {
    file: ".agents/plugins/marketplace.json",
    host: "Codex",
    // Codex marketplace entries: `source` is an object; local plugins carry
    // `{ source: "local", path: "<repo-root-relative>" }`.
    readSources: (json) =>
      pluginEntries(json)
        .filter((p) => p.source && p.source.source === "local")
        .map((p) => ({
          name: p.name,
          value: typeof p.source.path === "string" ? p.source.path : null,
        })),
    requiredManifest: ".codex-plugin/plugin.json",
  },
];

// The `plugins` array of a marketplace manifest, tolerating any malformed
// shape: a non-array `plugins`, or null/non-object entries, yield an empty list
// or are dropped, which the callers report as "lists no installable plugin
// source" instead of crashing.
function pluginEntries(json) {
  if (!json || !Array.isArray(json.plugins)) return [];
  return json.plugins.filter((p) => p && typeof p === "object");
}

// Path-declaring fields inside the plugin manifests themselves.
//
// `base` is the directory the declared value is resolved against, and it is
// NOT uniform across hosts — see the note on the Cursor entry. `target` is the
// directory the value is expected to land on. Pinning both ends is the point:
// `target` failing to exist catches someone moving the skills/commands tree,
// and the resolved value failing to equal `target` catches someone editing a
// manifest. Neither side can move without the other being made to follow.
const PLUGIN_DECLARATIONS = [
  {
    file: "plugins/open-code-review/claude-code/.claude-plugin/plugin.json",
    field: "commands",
    // Claude Code resolves plugin.json paths against the plugin root, i.e. the
    // parent of `.claude-plugin/`.
    base: "plugin-root",
    target: "plugins/open-code-review/claude-code/commands",
    kind: "commands",
    host: "Claude Code",
  },
  {
    file: "plugins/open-code-review/.codex-plugin/plugin.json",
    field: "skills",
    // Codex's plugin format follows Claude Code's: paths are plugin-root
    // relative, which is why this one reads `./skills/`.
    base: "plugin-root",
    target: "plugins/open-code-review/skills",
    kind: "skills",
    host: "Codex",
  },
  {
    file: "plugins/open-code-review/.cursor-plugin/plugin.json",
    field: "skills",
    // This manifest reads `../skills/`, which only lands on the skills tree if
    // Cursor resolves relative to the MANIFEST directory rather than the plugin
    // root its two siblings use. That asymmetry is unverified against Cursor's
    // published plugin spec, so `base` records the assumption the file is
    // currently written under and `unverifiedBase` makes the check say so out
    // loud on every run. Silently passing would be worse than either outcome:
    // it would certify a manifest that, if Cursor actually resolves from the
    // plugin root, points at a `plugins/skills` directory that does not exist.
    base: "manifest-dir",
    unverifiedBase:
      "Cursor's plugin spec has not been checked against this repo. If Cursor " +
      "resolves plugin.json paths from the plugin root (as Claude Code and " +
      "Codex do), `../skills/` points at the non-existent `plugins/skills` and " +
      "must become `./skills/`. Confirm against https://cursor.com/docs/plugins, " +
      "then set base to the verified value and drop this field.",
    target: "plugins/open-code-review/skills",
    kind: "skills",
    host: "Cursor",
  },
];

// Skill trees. Each direct subdirectory must hold a SKILL.md whose frontmatter
// `name` equals the directory name — `npx skills add --skill <name>` and every
// skill loader address a skill by that name.
const SKILL_ROOTS = ["skills", "plugins/open-code-review/skills"];

// Command prompt directories. Each `.md` needs a `description` for the host to
// render it in the slash-command list.
const COMMAND_DIRS = ["plugins/open-code-review/claude-code/commands"];

// ---------------------------------------------------------------------------
// Check 1: in-repo path links
// ---------------------------------------------------------------------------

// Matches the three URL shapes that point at a path inside this repository.
// The scheme is optional because prose sometimes omits it. The path run stops
// at whitespace, a closing paren (markdown link syntax), a backtick, or a
// quote; trailing prose punctuation is trimmed separately.
function repoLinkPattern() {
  // The slug is interpolated into a regex source, so escape every character
  // that would otherwise be read as a metacharacter. Backslash has to come
  // first in the class so it is escaped before it can pair with a later
  // replacement.
  const slug = REPO_SLUG.replace(/[\\^$.*+?()[\]{}|/]/g, "\\$&");
  return new RegExp(
    "(?:https?:\\/\\/)?(?:" +
      `raw\\.githubusercontent\\.com\\/${slug}\\/main\\/` +
      "|" +
      `github\\.com\\/${slug}\\/(blob|tree)\\/main\\/` +
      ")([^\\s)\\]`\"'<>]+)",
    "g"
  );
}

// Trailing characters that belong to the surrounding sentence, not the URL.
// The escaped codepoints are the CJK punctuation the translated docs end
// sentences with, written as escapes to keep this file ASCII-only per
// scripts/verify-english-only.go: U+FF0C fullwidth comma, U+3002 ideographic
// full stop, U+3001 ideographic comma, U+FF09 fullwidth right paren, U+3011
// right black lenticular bracket.
function stripTrailingPunctuation(s) {
  return s.replace(/[.,;:!?*_\uFF0C\u3002\u3001\uFF09\u3011]+$/, "");
}

// Reduce a URL path run to the repo-relative path it addresses: drop any
// fragment or query, trim prose punctuation, and percent-decode.
function toRepoPath(rawPath) {
  let p = rawPath.split("#")[0].split("?")[0];
  p = stripTrailingPunctuation(p);
  p = p.replace(/\/+$/, ""); // a trailing slash is directory notation, not a name
  if (!p) return null;
  try {
    p = decodeURIComponent(p);
  } catch (e) {
    /* leave the raw form; a malformed escape will fail the existence check */
  }
  return p;
}

// Extract every in-repo link from one file's content.
// Returns [{ repoPath, kind: "file"|"tree", line, raw }], skipping lines that
// carry the allow-missing marker.
function extractRepoLinks(content) {
  const lines = String(content).split(/\r?\n/);
  const found = [];
  // Compiled once per file, not once per line: the link scan walks the whole
  // work tree, so per-line compilation would run tens of thousands of times.
  // `lastIndex` is reset before each line because the regex is global.
  const re = repoLinkPattern();
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.includes(LINK_ALLOW_MARKER)) continue;
    re.lastIndex = 0;
    let m;
    while ((m = re.exec(line)) !== null) {
      const repoPath = toRepoPath(m[2]);
      if (!repoPath) continue;
      found.push({
        repoPath,
        // Only `tree/main/` addresses a directory; `blob/main/` and the raw
        // host both address a file.
        kind: m[1] === "tree" ? "tree" : "file",
        line: i + 1,
        raw: m[0],
      });
    }
  }
  return found;
}

// Validate extracted links against a filesystem probe.
// links: [{ file, repoPath, kind, line }]; probe(repoPath) -> "file" | "dir" | null.
// Returns { errors, warnings }, both [{ file, line, message }]: a missing path
// is an error, a blob/tree kind mismatch is only a warning (see the header).
function validateRepoLinks(links, probe) {
  const errors = [];
  const warnings = [];
  for (const l of links) {
    const actual = probe(l.repoPath);
    if (actual === null) {
      errors.push({
        file: l.file,
        line: l.line,
        message:
          `Link points at \`${l.repoPath}\`, which does not exist in the ` +
          `repository. Update the link, or restore the file it addresses ` +
          `(this URL is served from the default branch, so a stale path is a ` +
          `404 for every reader).`,
      });
      continue;
    }
    if (l.kind === "file" && actual !== "file") {
      warnings.push({
        file: l.file,
        line: l.line,
        message:
          `Link addresses \`${l.repoPath}\` as a file (blob/raw URL) but that ` +
          `path is a directory. GitHub redirects, so the link opens; prefer a ` +
          `\`tree/main/\` URL for directories.`,
      });
    } else if (l.kind === "tree" && actual !== "dir") {
      warnings.push({
        file: l.file,
        line: l.line,
        message:
          `Link addresses \`${l.repoPath}\` as a directory (tree URL) but that ` +
          `path is a file. GitHub redirects, so the link opens; prefer a ` +
          `\`blob/main/\` URL for files.`,
      });
    }
  }
  return { errors, warnings };
}

// ---------------------------------------------------------------------------
// Check 2: frontmatter
// ---------------------------------------------------------------------------

// Parse the top-level keys of a leading YAML frontmatter block. Deliberately
// minimal: enough to answer "is this key present and what is its inline
// value", which is all the contract checks ask. A folded value (`key: >`) has
// an empty inline value but is still present, so presence and value are
// reported separately.
// Returns { ok, keys: { name: { value, line } } }.
function parseFrontmatter(content) {
  const lines = String(content).split(/\r?\n/);
  // A null-prototype map so a key literally named `__proto__` is recorded as an
  // ordinary entry instead of hitting the Object prototype setter.
  const keys = Object.create(null);
  if (lines[0] !== "---") return { ok: false, keys };
  let current = null;
  let terminated = false;
  for (let i = 1; i < lines.length; i++) {
    const line = lines[i];
    if (line === "---") {
      terminated = true;
      break;
    }
    const m = /^([A-Za-z0-9_-]+):\s*(.*)$/.exec(line);
    if (m) {
      // Only column-0 matches start a top-level key.
      current = { inline: m[2].trim(), continuation: [], line: i + 1 };
      keys[m[1]] = current;
      continue;
    }
    // Anything indented, and a column-0 sequence dash, continues the current
    // key's value. Capturing continuations matters because YAML lets a scalar
    // live entirely on the following lines (`description:` then an indented
    // paragraph, or `description: >` then a folded block) — reading only the
    // inline part would call a perfectly valid value empty.
    if (current && (/^\s+\S/.test(line) || /^-\s/.test(line))) {
      current.continuation.push(line.trim());
    }
  }
  const out = Object.create(null);
  for (const key of Object.keys(keys)) {
    const k = keys[key];
    // A block indicator (`>`, `|`, with optional chomping and indentation
    // modifiers, which YAML accepts in either order: `>-2` and `>2-` are both
    // valid headers) is syntax, not content: the value is whatever follows on
    // the next lines.
    const inline = /^[>|](?:[-+]\d*|\d*[-+]?)$/.test(k.inline) ? "" : k.inline;
    out[key] = {
      value: unquoteScalar(inline || k.continuation.join(" ")),
      line: k.line,
    };
  }
  return { ok: terminated, keys: out };
}

// Strip one matching pair of surrounding quotes. YAML allows `name: "x"`, and
// comparing the quoted form against a directory name would fail with a message
// nobody could act on.
function unquoteScalar(value) {
  const m = /^(["'])([\s\S]*)\1$/.exec(value);
  return m ? m[2] : value;
}

// A skill's frontmatter must declare `name` (matching its directory, since
// that is how loaders address it) and a non-empty `description` (how agents
// decide to invoke it).
function validateSkillFrontmatter(file, dirName, content) {
  const errors = [];
  const { ok, keys } = parseFrontmatter(content);
  if (!ok) {
    errors.push({
      file,
      message:
        `${file} has no terminated YAML frontmatter block. A SKILL.md must ` +
        `open with \`---\`, declare \`name\` and \`description\`, and close ` +
        `with \`---\`.`,
    });
    return errors;
  }
  if (!keys.name) {
    errors.push({ file, message: `${file} frontmatter is missing \`name\`.` });
  } else if (keys.name.value !== dirName) {
    errors.push({
      file,
      line: keys.name.line,
      message:
        `${file} declares \`name: ${keys.name.value}\` but lives in ` +
        `\`${dirName}/\`. Loaders and \`npx skills add --skill <name>\` ` +
        `address a skill by its directory name, so the two must match.`,
    });
  }
  // The value must be non-empty, not merely present: `description:` with
  // nothing after it leaves an agent no basis for deciding to invoke the skill.
  // A folded scalar (`description: >`) has value ">", so this does not misfire.
  if (!keys.description || !keys.description.value) {
    errors.push({
      file,
      message: `${file} frontmatter is missing a non-empty \`description\`.`,
    });
  }
  return errors;
}

// A command prompt needs a `description`; without it the host has nothing to
// show in the slash-command list.
function validateCommandFrontmatter(file, content) {
  const { ok, keys } = parseFrontmatter(content);
  if (!ok) {
    return [
      {
        file,
        message:
          `${file} has no terminated YAML frontmatter block. A command prompt ` +
          `must open with \`---\`, declare \`description\`, and close with \`---\`.`,
      },
    ];
  }
  if (!keys.description || !keys.description.value) {
    return [
      {
        file,
        message:
          `${file} frontmatter is missing a non-empty \`description\`, which ` +
          `the host renders in the slash-command list.`,
      },
    ];
  }
  return [];
}

// ---------------------------------------------------------------------------
// Check 2: declared path resolution
// ---------------------------------------------------------------------------

// Resolve a manifest-declared relative path to a normalised repo-relative
// path. `manifestFile` is repo-relative; `base` selects the directory the
// value is resolved against ("manifest-dir" = the directory holding the
// manifest, "plugin-root" = its parent, i.e. the plugin directory).
function resolveDeclaredPath(manifestFile, base, value) {
  const manifestDir = path.posix.dirname(manifestFile.replace(/\\/g, "/"));
  const from = base === "plugin-root" ? path.posix.dirname(manifestDir) : manifestDir;
  const raw = String(value);
  // An absolute value is not a plugin-relative path, and `path.join` would
  // quietly treat it as one: joining "/skills/" onto the plugin root collapses
  // onto the expected target and would certify a manifest that every host
  // resolves against the filesystem root instead. Return it unchanged so the
  // comparison against `target` fails and says what was declared.
  if (raw.startsWith("/")) return raw.replace(/\/+$/, "") || "/";
  const joined = path.posix.join(from, raw);
  return joined.replace(/\/+$/, "");
}

// Compare each declared field against its expected target.
// specs: PLUGIN_DECLARATIONS entries; readJson(file) -> object | null.
// Returns { errors, warnings }, both [{ file, message }]. A spec carrying
// `unverifiedBase` always contributes a warning, so an assumption the check
// cannot validate is stated on every run instead of passing silently.
function checkPluginDeclarations(specs, readJson) {
  const errors = [];
  const warnings = [];
  for (const spec of specs) {
    if (spec.unverifiedBase) {
      warnings.push({
        file: spec.file,
        message:
          `${spec.file} declares \`${spec.field}\` against an UNVERIFIED ` +
          `resolution base (${spec.base}). ${spec.unverifiedBase}`,
      });
    }
    const json = readJson(spec.file);
    if (json === null) {
      errors.push({
        file: spec.file,
        message:
          `${spec.host} plugin manifest ${spec.file} is missing or is not ` +
          `valid JSON.`,
      });
      continue;
    }
    const value = json[spec.field];
    if (typeof value !== "string" || value === "") {
      errors.push({
        file: spec.file,
        message:
          `${spec.file} does not declare a \`${spec.field}\` path. The ` +
          `${spec.host} plugin would install without exposing any ` +
          `${spec.kind}.`,
      });
      continue;
    }
    const resolved = resolveDeclaredPath(spec.file, spec.base, value);
    if (resolved !== spec.target) {
      errors.push({
        file: spec.file,
        message:
          `${spec.file} declares \`${spec.field}: "${value}"\`, which resolves ` +
          `to \`${resolved}\` (relative to the ${spec.base}), but the ` +
          `${spec.kind} tree is at \`${spec.target}\`. Either fix the declared ` +
          `path or update PLUGIN_DECLARATIONS in ` +
          `scripts/github-actions/check-plugin-contract.js to match the new layout.`,
      });
    }
  }
  return { errors, warnings };
}

// ---------------------------------------------------------------------------
// GitHub Actions annotation + IO helpers
// ---------------------------------------------------------------------------

// GitHub workflow commands require `%`, CR and LF to be escaped in the message
// data. Messages here interpolate values read from manifests, SKILL.md
// frontmatter and URL paths, any of which can contain a newline or a stray
// percent sign; unescaped, the annotation is truncated at that point and the
// most diagnostic half of the message is lost.
function escapeAnnotationData(s) {
  return String(s).replace(/%/g, "%25").replace(/\r/g, "%0D").replace(/\n/g, "%0A");
}

function emitAnnotation(level, file, message, line) {
  const loc = file ? ` file=${file}${line ? `,line=${line}` : ""}` : "";
  console.log(`::${level}${loc}::${escapeAnnotationData(message)}`);
}

const emitError = (file, message, line) => emitAnnotation("error", file, message, line);
const emitWarning = (file, message, line) => emitAnnotation("warning", file, message, line);

// Read a file as text, or null if it is absent, unreadable, binary, or larger
// than `maxBytes`. The size cap exists only to keep the link scan from reading
// generated blobs, so contract files (SKILL.md, command prompts) are read with
// no cap: silently treating an oversized-but-valid SKILL.md as "missing" would
// point the reader at the wrong problem.
function readFileOrNull(abs, { maxBytes = MAX_SCAN_BYTES } = {}) {
  try {
    const stat = fs.statSync(abs);
    if (!stat.isFile() || stat.size > maxBytes) return null;
    const buf = fs.readFileSync(abs);
    // A NUL byte means binary; decoding it as text would produce noise.
    if (buf.includes(0)) return null;
    return buf.toString("utf8");
  } catch (e) {
    return null;
  }
}

function readJsonOrNull(repoRoot, file) {
  try {
    return JSON.parse(fs.readFileSync(path.join(repoRoot, file), "utf8"));
  } catch (e) {
    return null;
  }
}

// Depth-first walk yielding repo-relative paths of candidate text files.
//
// Symlinks are resolved, matching listEntries: a Dirent reports a symlink as
// neither a file nor a directory, so skipping them would leave a symlinked doc
// (or a whole symlinked subtree) silently unscanned — a blind spot that opens
// the moment someone uses a link to de-duplicate documentation. `seen` holds
// the real paths of directories already walked, which keeps a link pointing at
// an ancestor from looping forever and stops a linked subtree being scanned
// twice.
function walkTextFiles(repoRoot, rel = "", seen = new Set()) {
  const abs = path.join(repoRoot, rel);
  try {
    const real = fs.realpathSync(abs);
    if (seen.has(real)) return [];
    seen.add(real);
  } catch (e) {
    return [];
  }
  const out = [];
  let entries;
  try {
    entries = fs.readdirSync(abs, { withFileTypes: true });
  } catch (e) {
    return out;
  }
  // Sorted so annotations come out in a stable order run to run.
  entries.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  for (const entry of entries) {
    const childRel = rel ? `${rel}/${entry.name}` : entry.name;
    let isDir = entry.isDirectory();
    let isFile = entry.isFile();
    if (!isDir && !isFile) {
      if (!entry.isSymbolicLink()) continue; // socket, FIFO, device
      try {
        const stat = fs.statSync(path.join(abs, entry.name)); // follows the link
        isDir = stat.isDirectory();
        isFile = stat.isFile();
      } catch (e) {
        continue; // dangling link
      }
    }
    if (isDir) {
      if (SCAN_SKIP_DIRS.has(entry.name)) continue;
      out.push(...walkTextFiles(repoRoot, childRel, seen));
    } else if (isFile) {
      out.push(childRel);
    }
  }
  return out;
}

function probeKind(repoRoot, repoPath) {
  try {
    const stat = fs.statSync(path.join(repoRoot, repoPath));
    return stat.isDirectory() ? "dir" : "file";
  } catch (e) {
    return null;
  }
}

// Sorted names of the entries of `rel` whose resolved kind is `want`
// ("dir" or "file"), or null if `rel` cannot be read.
//
// Symlinks are resolved rather than skipped. A Dirent reports a symlink as
// neither a directory nor a file, so filtering on `isDirectory()`/`isFile()`
// alone would hide a symlinked skill directory and report the tree as empty —
// a blocking false positive, and precisely the layout someone might reach for
// to stop maintaining two copies of the skills tree. Resolving here also keeps
// this consistent with probeKind, which stats (and so follows links) too.
function listEntries(repoRoot, rel, want, ext) {
  const dir = path.join(repoRoot, rel);
  let entries;
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch (e) {
    return null;
  }
  const out = [];
  for (const entry of entries) {
    if (ext && !entry.name.endsWith(ext)) continue;
    let kind;
    if (entry.isDirectory()) kind = "dir";
    else if (entry.isFile()) kind = "file";
    else if (entry.isSymbolicLink()) {
      // statSync follows the link; a dangling link throws and is skipped.
      try {
        kind = fs.statSync(path.join(dir, entry.name)).isDirectory() ? "dir" : "file";
      } catch (e) {
        continue;
      }
    } else continue; // socket, FIFO, device: not part of any contract
    if (kind === want) out.push(entry.name);
  }
  return out.sort();
}

// ---------------------------------------------------------------------------
// CLI runners
// ---------------------------------------------------------------------------

// Report a corpus too small to be believable. Pure so it can be tested without
// a filesystem; returns a message or null.
function checkCorpusFloor(linkCount, fileCount, minLinks, minFiles) {
  if (linkCount >= minLinks && fileCount >= minFiles) return null;
  return (
    `Only ${linkCount} link(s) across ${fileCount} file(s) were found, below the ` +
    `floor of ${minLinks}/${minFiles}. The scan itself looks broken (a failed ` +
    `walk, a pattern that no longer matches, or a moved docs tree) — a check ` +
    `that finds nothing must fail rather than report success.`
  );
}

// BLOCKING. Returns a process exit code (0 ok, 1 on a broken link).
function runLinksCheck({
  repoRoot = process.cwd(),
  minLinks = MIN_EXPECTED_LINKS,
  minFiles = MIN_EXPECTED_LINK_FILES,
  maxScanBytes = MAX_SCAN_BYTES,
} = {}) {
  const files = walkTextFiles(repoRoot);
  const links = [];
  const skipped = [];
  for (const file of files) {
    const content = readFileOrNull(path.join(repoRoot, file), { maxBytes: maxScanBytes });
    if (content === null) {
      // A documentation file that could not be read is a hole in the corpus,
      // not a pass. Reporting it keeps this consistent with the fail-closed
      // corpus floor below, which can only catch a scan that broke wholesale
      // and would miss individual documents being skipped in silence.
      if (DOC_EXTENSIONS.test(file)) skipped.push(file);
      continue;
    }
    // Cheap prefilter: most files mention the slug nowhere.
    if (!content.includes(REPO_SLUG)) continue;
    for (const l of extractRepoLinks(content)) links.push({ ...l, file });
  }

  for (const file of skipped) {
    emitWarning(
      file,
      `Skipped by the link scan (unreadable, binary, or larger than ` +
        `${maxScanBytes} bytes), so its in-repo links were NOT verified.`
    );
  }

  const fileCount = new Set(links.map((l) => l.file)).size;
  const floor = checkCorpusFloor(links.length, fileCount, minLinks, minFiles);
  if (floor) {
    emitError(null, `In-repo link check failed: ${floor}`);
    return 1;
  }

  const { errors, warnings } = validateRepoLinks(links, (p) => probeKind(repoRoot, p));
  for (const err of errors) emitError(err.file, err.message, err.line);
  for (const warn of warnings) emitWarning(warn.file, warn.message, warn.line);

  if (errors.length === 0) {
    const unique = new Set(links.map((l) => l.repoPath)).size;
    console.log(
      `In-repo link check passed: ${links.length} link(s) across ` +
        `${fileCount} file(s) resolve to ` +
        `${unique} existing path(s)` +
        (warnings.length || skipped.length
          ? `, with ${warnings.length} blob/tree kind warning(s) and ` +
            `${skipped.length} unscanned doc(s).`
          : ".")
    );
    return 0;
  }
  emitError(
    null,
    `In-repo link check failed: ${errors.length} link(s) point at paths that do ` +
      `not exist. These URLs are served from the default branch, so each one is ` +
      `a 404 for readers.`
  );
  return 1;
}

// BLOCKING. Returns a process exit code (0 ok, 1 on a contract violation).
function runManifestsCheck({ repoRoot = process.cwd() } = {}) {
  const errors = [];
  const warnings = [];
  let checked = 0;

  // Marketplace entries -> plugin directories.
  for (const mp of MARKETPLACES) {
    const json = readJsonOrNull(repoRoot, mp.file);
    if (json === null) {
      errors.push({
        file: mp.file,
        message: `${mp.host} marketplace manifest ${mp.file} is missing or is not valid JSON.`,
      });
      continue;
    }
    const sources = mp.readSources(json);
    if (sources.length === 0) {
      errors.push({
        file: mp.file,
        message:
          `${mp.file} lists no installable plugin source. The ${mp.host} ` +
          `marketplace would resolve to nothing.`,
      });
      continue;
    }
    for (const src of sources) {
      checked++;
      if (src.value === null) {
        errors.push({
          file: mp.file,
          message: `${mp.file} entry "${src.name}" has no usable source path.`,
        });
        continue;
      }
      const dir = path.posix.normalize(src.value).replace(/^\.\//, "").replace(/\/+$/, "");
      if (probeKind(repoRoot, dir) !== "dir") {
        errors.push({
          file: mp.file,
          message:
            `${mp.file} entry "${src.name}" points at \`${dir}\`, which is not ` +
            `a directory in this repository. \`/plugin marketplace add\` would fail.`,
        });
        continue;
      }
      if (probeKind(repoRoot, `${dir}/${mp.requiredManifest}`) !== "file") {
        errors.push({
          file: mp.file,
          message:
            `${mp.file} entry "${src.name}" points at \`${dir}\`, which has no ` +
            `\`${mp.requiredManifest}\`. ${mp.host} would not recognise it as a plugin.`,
        });
      }
    }
  }

  // Plugin manifests -> declared skills/commands paths.
  const declared = checkPluginDeclarations(PLUGIN_DECLARATIONS, (file) =>
    readJsonOrNull(repoRoot, file)
  );
  errors.push(...declared.errors);
  warnings.push(...declared.warnings);
  checked += PLUGIN_DECLARATIONS.length;

  // Skill trees: non-empty, and every SKILL.md well-formed.
  for (const root of SKILL_ROOTS) {
    const dirs = listEntries(repoRoot, root, "dir");
    if (dirs === null) {
      errors.push({
        file: root,
        message: `Skill tree \`${root}\` is missing.`,
      });
      continue;
    }
    if (dirs.length === 0) {
      errors.push({
        file: root,
        message:
          `Skill tree \`${root}\` contains no skill directory. A plugin ` +
          `declaring it would install without exposing any skill.`,
      });
      continue;
    }
    for (const dir of dirs) {
      const file = `${root}/${dir}/SKILL.md`;
      checked++;
      // No size cap: a contract file is read whatever its size.
      const content = readFileOrNull(path.join(repoRoot, file), { maxBytes: Infinity });
      if (content === null) {
        errors.push({
          file,
          message:
            `\`${root}/${dir}/\` has no readable SKILL.md. Every directory in ` +
            `a skill tree must be a loadable skill.`,
        });
        continue;
      }
      errors.push(...validateSkillFrontmatter(file, dir, content));
    }
  }

  // Command directories: non-empty, and every prompt carries a description.
  for (const dir of COMMAND_DIRS) {
    const files = listEntries(repoRoot, dir, "file", ".md");
    if (files === null) {
      errors.push({ file: dir, message: `Command directory \`${dir}\` is missing.` });
      continue;
    }
    if (files.length === 0) {
      errors.push({
        file: dir,
        message:
          `Command directory \`${dir}\` contains no \`.md\` prompt. The plugin ` +
          `would install without exposing any slash command.`,
      });
      continue;
    }
    for (const name of files) {
      const file = `${dir}/${name}`;
      checked++;
      const content = readFileOrNull(path.join(repoRoot, file), { maxBytes: Infinity });
      if (content === null) {
        errors.push({ file, message: `${file} is not readable.` });
        continue;
      }
      errors.push(...validateCommandFrontmatter(file, content));
    }
  }

  for (const err of errors) emitError(err.file, err.message, err.line);
  for (const warn of warnings) emitWarning(warn.file, warn.message, warn.line);

  if (errors.length === 0) {
    console.log(
      `Plugin manifest check passed: ${checked} declared path(s) and ` +
        `manifest(s) resolve to real, non-empty targets` +
        (warnings.length
          ? `, with ${warnings.length} unverified-assumption warning(s).`
          : ".")
    );
    return 0;
  }
  emitError(
    null,
    `Plugin manifest check failed: ${errors.length} problem(s). A plugin whose ` +
      `declared paths do not resolve installs cleanly and exposes nothing.`
  );
  return 1;
}

function main(argv = process.argv.slice(2), env = process.env) {
  const mode = (argv[0] || "all").toLowerCase();
  const repoRoot = env.OCR_REPO_ROOT || process.cwd();
  if (mode === "links" || mode === "link") return runLinksCheck({ repoRoot });
  if (mode === "manifests" || mode === "manifest") return runManifestsCheck({ repoRoot });
  if (mode === "all") {
    const a = runLinksCheck({ repoRoot });
    const b = runManifestsCheck({ repoRoot });
    return a || b;
  }
  emitError(null, `Unknown mode "${mode}". Use "links", "manifests", or "all".`);
  return 2;
}

if (require.main === module) {
  process.exit(main());
}

module.exports = {
  REPO_SLUG,
  LINK_ALLOW_MARKER,
  MARKETPLACES,
  PLUGIN_DECLARATIONS,
  SKILL_ROOTS,
  COMMAND_DIRS,
  MIN_EXPECTED_LINKS,
  MIN_EXPECTED_LINK_FILES,
  DOC_EXTENSIONS,
  MAX_SCAN_BYTES,
  walkTextFiles,
  repoLinkPattern,
  escapeAnnotationData,
  emitAnnotation,
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
};
