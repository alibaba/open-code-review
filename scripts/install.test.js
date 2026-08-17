#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

// Unit tests for the PATH resolution checks in scripts/install.js.
//
// Run via: node scripts/install.test.js
// (also wired as `npm run test:install`).
//
// Plain Node + assert, no external deps and no `node --test`, mirroring
// check-translation-sync.test.js so both run on node >= 14.
//
// Background: shells cache command locations (the bash/zsh hash table). When
// a terminal ran some other executable named `ocr` before the install, that
// terminal keeps running the cached (wrong) program even after the real ocr
// appears earlier on PATH; only new terminals resolve correctly. install.js
// must detect and report these situations at install time.

const assert = require("assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const {
  ocrCandidateNames,
  pathsEqual,
  pathHasPrefix,
  findOcrOnPath,
  isOurCommand,
  globalBinDir,
  checkOcrCommand,
} = require(path.join(__dirname, "install.js"));

const POSIX = process.platform !== "win32";
const PKG_NAME = "@alibaba-group/open-code-review";

// Build a throwaway layout:
//   root/pkg/bin/ocr.js   the wrapper this install ships
//   root/<dir>/...        fake PATH directories created per test
let root;
let packageRoot;

function setUp() {
  root = fs.mkdtempSync(path.join(os.tmpdir(), "ocr-install-test-"));
  packageRoot = path.join(root, "pkg");
  fs.mkdirSync(path.join(packageRoot, "bin"), { recursive: true });
  fs.writeFileSync(
    path.join(packageRoot, "bin", "ocr.js"),
    "#!/usr/bin/env node\nconsole.log('ocr');\n"
  );
  // The real installer chmods the wrapper to 0755 before linking it.
  fs.chmodSync(path.join(packageRoot, "bin", "ocr.js"), 0o755);
}

function tearDown() {
  fs.rmSync(root, { recursive: true, force: true });
}

function makeDir(name) {
  const dir = path.join(root, name);
  fs.mkdirSync(dir, { recursive: true });
  return dir;
}

// A foreign executable named `ocr` (stands in for e.g. a tmux helper script
// the user had on PATH before installing OpenCodeReview).
function makeForeignOcr(dir, content) {
  const p = path.join(dir, "ocr");
  fs.writeFileSync(p, content || "#!/bin/sh\necho foreign ocr\n");
  fs.chmodSync(p, 0o755);
  return p;
}

// What npm creates on POSIX: <prefix>/bin/ocr -> <pkg>/bin/ocr.js.
function makeOurLink(dir) {
  const p = path.join(dir, "ocr");
  if (POSIX) {
    fs.symlinkSync(path.join(packageRoot, "bin", "ocr.js"), p);
  } else {
    // File symlinks may need elevation on Windows; a shim with the launch
    // target embedded (what npm generates there) exercises the same logic.
    fs.writeFileSync(
      p,
      `#!/bin/sh\nexec node "node_modules/${PKG_NAME}/bin/ocr.js" "$@"\n`
    );
  }
  return p;
}

// npm-style Windows shim: a small generated file referencing the target.
function makeWindowsShim(dir, name) {
  const p = path.join(dir, name || "ocr.cmd");
  fs.writeFileSync(
    p,
    `@ECHO off\r\nSETLOCAL\r\nCALL node "%~dp0\\node_modules\\${PKG_NAME}\\bin\\ocr.js" %*\r\n`
  );
  return p;
}

function testCandidateNames() {
  assert.deepStrictEqual(ocrCandidateNames(false), ["ocr"]);
  // Windows follows the supplied PATHEXT order, like cmd.exe would.
  assert.deepStrictEqual(
    ocrCandidateNames(true, ".EXE;.CMD;.BAT"),
    ["ocr.exe", "ocr.cmd", "ocr.bat", "ocr"]
  );
  // Without PATHEXT the Windows default applies (.com before .exe), and the
  // bare name stays last for POSIX-like shells (Git Bash).
  const def = ocrCandidateNames(true, "");
  assert.strictEqual(def[0], "ocr.com");
  assert.ok(def.includes("ocr.exe"));
  assert.strictEqual(def[def.length - 1], "ocr");
  assert.strictEqual(new Set(def).size, def.length);
}

function testPathHelpersCaseSensitivity() {
  assert.strictEqual(pathsEqual("C:\\A\\b", "c:\\a\\B", true), true);
  assert.strictEqual(pathsEqual("C:\\A\\b", "c:\\a\\B", false), false);
  assert.strictEqual(pathsEqual("/A/b", "/A/b", false), true);
  assert.strictEqual(pathHasPrefix("C:\\Users\\x\\pkg", "c:\\users\\x\\", true), true);
  assert.strictEqual(pathHasPrefix("C:\\Users\\x\\pkg", "c:\\users\\x\\", false), false);
  assert.strictEqual(pathHasPrefix("/usr/lib/pkg", "/usr/lib/", false), true);
}

function testFindOcrOnPathOrderAndEmpty() {
  const a = makeDir("a");
  const b = makeDir("b");
  makeForeignOcr(a);
  makeForeignOcr(b);
  const found = findOcrOnPath([a, b].join(":"), false, ":");
  assert.deepStrictEqual(
    found.map((f) => f.path),
    [path.join(a, "ocr"), path.join(b, "ocr")]
  );
  assert.deepStrictEqual(findOcrOnPath("", false, ":"), []);
}

function testFindOcrOnPathSkipsNonExecutableAndDirs() {
  if (!POSIX) {
    return; // the executable bit is not meaningful on Windows
  }
  const dir = makeDir("mixed");
  const notExec = path.join(dir, "ocr");
  fs.writeFileSync(notExec, "data");
  fs.chmodSync(notExec, 0o644); // present but not executable -> skipped
  assert.deepStrictEqual(findOcrOnPath(dir, false, ":"), []);

  const dirDir = makeDir("asdir");
  fs.mkdirSync(path.join(dirDir, "ocr")); // a directory named ocr -> skipped
  assert.deepStrictEqual(findOcrOnPath(dirDir, false, ":"), []);
}

function testFindOcrOnPathDedupesDuplicateDirs() {
  const a = makeDir("dup");
  makeForeignOcr(a);
  // The same directory listed twice (and via a `..` alias on POSIX, which
  // realpath dedupes) still yields a single entry.
  const pathEnv = POSIX ? [a, a, path.join(a, "..", "dup")].join(":") : [a, a].join(";");
  const found = findOcrOnPath(pathEnv, !POSIX, POSIX ? ":" : ";");
  assert.strictEqual(found.length, 1);
}

function testIsOurCommandSymlinkAndForeign() {
  const dir = makeDir("link");
  const ours = makeOurLink(dir);
  assert.strictEqual(isOurCommand(ours, packageRoot, PKG_NAME, false), true);

  const foreignDir = makeDir("foreign");
  const foreign = makeForeignOcr(foreignDir);
  assert.strictEqual(isOurCommand(foreign, packageRoot, PKG_NAME, false), false);
}

function testIsOurCommandShimContent() {
  const dir = makeDir("shim");
  const shim = makeWindowsShim(dir); // backslash paths inside
  assert.strictEqual(isOurCommand(shim, packageRoot, PKG_NAME, true), true);

  // Casing inside the shim must not matter (Windows paths).
  const upper = path.join(dir, "upper.cmd");
  fs.writeFileSync(
    upper,
    `@ECHO off\r\nCALL node "%~dp0\\NODE_MODULES\\@Alibaba-Group\\open-code-review\\BIN\\OCR.JS" %*\r\n`
  );
  assert.strictEqual(isOurCommand(upper, packageRoot, PKG_NAME, true), true);

  const otherShim = path.join(dir, "other.cmd");
  fs.writeFileSync(otherShim, "@ECHO off\r\nCALL node \"C:\\some\\other\\tool.js\" %*\r\n");
  assert.strictEqual(isOurCommand(otherShim, packageRoot, PKG_NAME, true), false);
}

function testGlobalBinDir() {
  const prefix = path.join(root, "prefix");
  const globalRoot = POSIX
    ? path.join(prefix, "lib", "node_modules")
    : path.join(prefix, "node_modules");
  const globalPkg = path.join(globalRoot, "@alibaba-group", "open-code-review");
  const env = { npm_config_prefix: prefix };

  assert.strictEqual(
    globalBinDir(globalPkg, !POSIX ? true : false, env),
    POSIX ? path.join(prefix, "bin") : prefix
  );
  // Windows prefix casing may differ from the real on-disk casing.
  if (!POSIX) {
    assert.strictEqual(
      globalBinDir(globalPkg.toUpperCase(), true, { npm_config_prefix: prefix.toLowerCase() }),
      prefix.toLowerCase()
    );
  }
  // A local install (outside the prefix) does not expect a PATH command.
  assert.strictEqual(globalBinDir(packageRoot, false, env), null);
  // No prefix reported -> nothing to claim.
  assert.strictEqual(globalBinDir(globalPkg, false, {}), null);
}

function testCheckOcrCommandOk() {
  const dir = makeDir("ok");
  const ours = makeOurLink(dir);
  const r = checkOcrCommand({
    packageRoot,
    pkgName: PKG_NAME,
    pathEnv: dir,
    isWindows: false,
    env: {},
  });
  assert.strictEqual(r.status, "ok");
  assert.strictEqual(r.resolved, ours);
}

// The reported bug: the install's own `ocr` is first on PATH, but another
// `ocr` exists later on it. Terminals open since before the install may have
// hashed that later location and keep running it.
function testCheckOcrCommandStaleHashRisk() {
  const npmBin = makeDir("npm-bin");
  const localBin = makeDir("local-bin");
  const ours = makeOurLink(npmBin);
  const stale = makeForeignOcr(localBin);
  const r = checkOcrCommand({
    packageRoot,
    pkgName: PKG_NAME,
    pathEnv: [npmBin, localBin].join(":"),
    isWindows: false,
    env: {},
  });
  assert.strictEqual(r.status, "stale-hash-risk");
  assert.strictEqual(r.resolved, ours);
  assert.deepStrictEqual(r.stale, [stale]);
}

function testCheckOcrCommandShadowed() {
  const npmBin = makeDir("npm-bin2");
  const localBin = makeDir("local-bin2");
  const ours = makeOurLink(npmBin);
  const winner = makeForeignOcr(localBin);
  const r = checkOcrCommand({
    packageRoot,
    pkgName: PKG_NAME,
    pathEnv: [localBin, npmBin].join(":"), // foreign entry comes first
    isWindows: false,
    env: {},
  });
  assert.strictEqual(r.status, "shadowed");
  assert.strictEqual(r.resolved, winner);
  assert.deepStrictEqual(r.ours, [ours]);
}

function testCheckOcrCommandNotOnPathGlobal() {
  const empty = makeDir("empty");
  const prefix = path.join(root, "prefix2");
  const globalPkg = path.join(prefix, "lib", "node_modules", "@alibaba-group", "open-code-review");
  const r = checkOcrCommand({
    packageRoot: globalPkg,
    pkgName: PKG_NAME,
    pathEnv: empty,
    isWindows: false,
    env: { npm_config_prefix: prefix },
  });
  assert.strictEqual(r.status, "not-on-path");
  assert.strictEqual(r.binDir, path.join(prefix, "bin"));
}

function testCheckOcrCommandUnknownForLocalInstall() {
  const empty = makeDir("empty2");
  const r = checkOcrCommand({
    packageRoot, // not under any npm prefix
    pkgName: PKG_NAME,
    pathEnv: empty,
    isWindows: false,
    env: { npm_config_prefix: path.join(root, "prefix3") },
  });
  assert.strictEqual(r.status, "unknown");

  // A foreign ocr on PATH does not change that: a local install has no
  // PATH expectation, so there is nothing actionable to report.
  const foreignDir = makeDir("foreign-local");
  const foreign = makeForeignOcr(foreignDir);
  const r2 = checkOcrCommand({
    packageRoot,
    pkgName: PKG_NAME,
    pathEnv: foreignDir,
    isWindows: false,
    env: { npm_config_prefix: path.join(root, "prefix3") },
  });
  assert.strictEqual(r2.status, "unknown");
  assert.strictEqual(r2.occupiedBy, undefined);
  assert.notStrictEqual(foreign, null);
}

// Global install whose own `ocr` is missing from PATH while a foreign
// executable occupies the command name: report where ours should live and
// who currently holds the name.
function testCheckOcrCommandNotOnPathOccupied() {
  const foreignDir = makeDir("occupant");
  const foreign = makeForeignOcr(foreignDir);
  const prefix = path.join(root, "prefix4");
  const globalPkg = path.join(prefix, "lib", "node_modules", "@alibaba-group", "open-code-review");
  const r = checkOcrCommand({
    packageRoot: globalPkg,
    pkgName: PKG_NAME,
    pathEnv: foreignDir,
    isWindows: false,
    env: { npm_config_prefix: prefix },
  });
  assert.strictEqual(r.status, "not-on-path");
  assert.strictEqual(r.binDir, path.join(prefix, "bin"));
  assert.strictEqual(r.occupiedBy, foreign);
}

function testCheckOcrCommandWindowsVariants() {
  const dir = makeDir("winbin");
  // Within one directory only the first PATHEXT match is reported.
  fs.writeFileSync(path.join(dir, "ocr.cmd"), "@ECHO off\r\nREM foreign\r\n");
  fs.writeFileSync(path.join(dir, "ocr.exe"), "MZ-not-really");
  const ourDir = makeDir("winbin-ours");
  const shim = makeWindowsShim(ourDir);
  const pathEnv = [dir, ourDir].join(";");

  // Foreign directory first: shadowed. Under the default PATHEXT the
  // directory's ocr.exe wins over its ocr.cmd (.com does not exist here).
  const r = checkOcrCommand({
    packageRoot,
    pkgName: PKG_NAME,
    pathEnv,
    isWindows: true,
    env: {},
  });
  assert.strictEqual(r.status, "shadowed");
  assert.strictEqual(r.resolved, path.join(dir, "ocr.exe"));
  assert.deepStrictEqual(r.ours, [shim]);

  // PATHEXT ordering is honored: with .CMD before .EXE the ocr.cmd wins,
  // exactly as cmd.exe would resolve it.
  const r2 = checkOcrCommand({
    packageRoot,
    pkgName: PKG_NAME,
    pathEnv,
    isWindows: true,
    env: { PATHEXT: ".CMD;.EXE" },
  });
  assert.strictEqual(r2.status, "shadowed");
  assert.strictEqual(r2.resolved, path.join(dir, "ocr.cmd"));

  // Our npm .cmd shim alone resolves as ours.
  const r3 = checkOcrCommand({
    packageRoot,
    pkgName: PKG_NAME,
    pathEnv: ourDir,
    isWindows: true,
    env: {},
  });
  assert.strictEqual(r3.status, "ok");
  assert.strictEqual(r3.resolved, shim);

  // Foreign-only without any prefix info: nothing actionable -> unknown.
  const r4 = checkOcrCommand({
    packageRoot,
    pkgName: PKG_NAME,
    pathEnv: dir,
    isWindows: true,
    env: {},
  });
  assert.strictEqual(r4.status, "unknown");
}

function main() {
  const tests = [
    testCandidateNames,
    testPathHelpersCaseSensitivity,
    testFindOcrOnPathOrderAndEmpty,
    testFindOcrOnPathSkipsNonExecutableAndDirs,
    testFindOcrOnPathDedupesDuplicateDirs,
    testIsOurCommandSymlinkAndForeign,
    testIsOurCommandShimContent,
    testGlobalBinDir,
    testCheckOcrCommandOk,
    testCheckOcrCommandStaleHashRisk,
    testCheckOcrCommandShadowed,
    testCheckOcrCommandNotOnPathGlobal,
    testCheckOcrCommandNotOnPathOccupied,
    testCheckOcrCommandUnknownForLocalInstall,
    testCheckOcrCommandWindowsVariants,
  ];
  for (const t of tests) {
    setUp();
    try {
      t();
    } finally {
      tearDown();
    }
  }
  console.log("All install PATH-resolution tests passed.");
}

try {
  main();
} catch (err) {
  console.error(err);
  process.exit(1);
}
