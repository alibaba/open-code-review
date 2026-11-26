// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Assembles the tree that gets published: dist/package. Every path resolves
// from this script's location, so the result never depends on the caller's cwd
// or on the platform's shell.
import { cpSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs"
import { dirname, join, relative, resolve } from "node:path"
import { fileURLToPath } from "node:url"

const SCRIPT_DIR = dirname(fileURLToPath(import.meta.url))
const PACKAGE_DIR = resolve(SCRIPT_DIR, "..")
const REPOSITORY_ROOT = resolve(PACKAGE_DIR, "..", "..", "..")
const DIST_DIR = join(PACKAGE_DIR, "dist")
const OUTPUT_DIR = join(DIST_DIR, "package")

const CARRIED_FIELDS = [
  "name",
  "version",
  "description",
  "type",
  "license",
  "repository",
  "keywords",
  "engines",
  "peerDependencies",
]

const PUBLISHED_FILES = ["src", "skills", "README.md", "LICENSE"]

const ENTRIES = [
  { name: "src", from: join(PACKAGE_DIR, "src") },
  { name: "skills", from: join(REPOSITORY_ROOT, "skills") },
  { name: "README.md", from: join(PACKAGE_DIR, "README.md") },
  { name: "LICENSE", from: join(REPOSITORY_ROOT, "LICENSE") },
]

const MANIFEST = join(PACKAGE_DIR, "package.json")

function fail(message) {
  console.error(`build-package: ${message}`)
  process.exit(1)
}

// A stale artifact must never reach a publish, so the output directory goes
// before anything is read.
rmSync(DIST_DIR, { recursive: true, force: true })

const missing = [...ENTRIES.map((entry) => entry.from), MANIFEST].filter((path) => !existsSync(path))
if (missing.length > 0) {
  fail(`missing source path: ${missing.join(", ")}`)
}

const source = JSON.parse(readFileSync(MANIFEST, "utf8"))
if (source.pi === undefined) {
  fail(`${MANIFEST} has no "pi" manifest, so the published package would not load in Pi`)
}

mkdirSync(OUTPUT_DIR, { recursive: true })

for (const entry of ENTRIES) {
  const target = join(OUTPUT_DIR, entry.name)
  cpSync(entry.from, target, { recursive: true })
  console.log(`copied ${relative(PACKAGE_DIR, target)}`)
}

const manifest = {}
for (const field of CARRIED_FIELDS) {
  if (source[field] !== undefined) manifest[field] = source[field]
}
manifest.pi = source.pi
manifest.files = PUBLISHED_FILES
// The source package is private so `npm publish` here cannot ship the development
// tree; only this output is publishable.
delete manifest.private

const manifestPath = join(OUTPUT_DIR, "package.json")
writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`)
console.log(`copied ${relative(PACKAGE_DIR, manifestPath)}`)

console.log(`${manifest.name} ${manifest.version}`)
