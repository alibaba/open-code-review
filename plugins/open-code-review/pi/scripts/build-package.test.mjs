// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import assert from "node:assert/strict"
import { spawnSync } from "node:child_process"
import { cpSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { dirname, join, relative, resolve, sep } from "node:path"
import test from "node:test"
import { fileURLToPath } from "node:url"

const PACKAGE_DIR = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const REPOSITORY_ROOT = resolve(PACKAGE_DIR, "..", "..", "..")
const SKIPPED = new Set(["node_modules", "dist"])

function createWorkspace(t) {
  const root = mkdtempSync(join(tmpdir(), "ocr-pi-build-"))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const packageDir = join(root, "plugins", "open-code-review", "pi")
  const filter = (source) => !relative(PACKAGE_DIR, source).split(sep).some((segment) => SKIPPED.has(segment))
  cpSync(PACKAGE_DIR, packageDir, { recursive: true, filter })
  cpSync(join(REPOSITORY_ROOT, "skills"), join(root, "skills"), { recursive: true })
  cpSync(join(REPOSITORY_ROOT, "LICENSE"), join(root, "LICENSE"))
  return { packageDir, dist: join(packageDir, "dist", "package") }
}

function build(packageDir) {
  return spawnSync(process.execPath, ["scripts/build-package.mjs"], { cwd: packageDir, encoding: "utf8" })
}

function readManifest(path) {
  return JSON.parse(readFileSync(path, "utf8"))
}

test("a rebuild never carries a stale file into the published tree", (t) => {
  const { packageDir, dist } = createWorkspace(t)
  assert.equal(build(packageDir).status, 0)
  writeFileSync(join(dist, "stale.txt"), "left over from an earlier build\n")
  const rebuilt = build(packageDir)
  assert.equal(rebuilt.status, 0, rebuilt.stderr)
  assert.equal(existsSync(join(dist, "stale.txt")), false)
})

test("the built manifest is publishable and ships only the published tree", (t) => {
  const { packageDir, dist } = createWorkspace(t)
  const result = build(packageDir)
  assert.equal(result.status, 0, result.stderr)
  assert.equal(readManifest(join(packageDir, "package.json")).private, true)
  const manifest = readManifest(join(dist, "package.json"))
  assert.equal("private" in manifest, false)
  assert.equal("scripts" in manifest, false)
  assert.equal("devDependencies" in manifest, false)
  assert.deepEqual(manifest.files, ["src", "skills", "README.md", "LICENSE"])
  assert.deepEqual(manifest.pi.extensions, ["./src/index.ts"])
  assert.ok(existsSync(join(dist, "skills", "open-code-review", "SKILL.md")))
  assert.ok(existsSync(join(dist, "skills", "open-code-review-delegate", "SKILL.md")))
  assert.equal(existsSync(join(dist, "test")), false)
})
