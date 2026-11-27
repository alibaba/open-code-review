// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import assert from "node:assert/strict"
import { existsSync } from "node:fs"
import { dirname, resolve } from "node:path"
import test from "node:test"
import { fileURLToPath } from "node:url"
import { createHost, stdoutExec } from "./helpers.mjs"
import { registerSkillDiscovery } from "../src/index.ts"

const PACKAGE_DIR = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const REPOSITORY_SKILLS = resolve(PACKAGE_DIR, "..", "..", "..", "skills")

function discoveryFrom(sources, notify) {
  const handlers = new Map()
  registerSkillDiscovery({ on: (name, handler) => handlers.set(name, handler) }, sources)
  const handler = handlers.get("resources_discover")
  assert.ok(handler, "no resources_discover handler was registered")
  return handler({ type: "resources_discover", cwd: PACKAGE_DIR, reason: "startup" }, { ui: { notify } })
}

test("the extension contributes a skills directory to the host", async () => {
  const handler = (await createHost(stdoutExec(""))).events.get("resources_discover")
  assert.ok(handler, "no resources_discover handler was registered")
  const result = await handler({ type: "resources_discover", cwd: PACKAGE_DIR, reason: "startup" }, { ui: { notify: () => {} } })
  assert.deepEqual(result, { skillPaths: [REPOSITORY_SKILLS] })
  assert.ok(existsSync(resolve(REPOSITORY_SKILLS, "open-code-review", "SKILL.md")))
  assert.ok(existsSync(resolve(REPOSITORY_SKILLS, "open-code-review-delegate", "SKILL.md")))
})

test("a missing skills directory is reported instead of loaded silently", async () => {
  const notifications = []
  const result = await discoveryFrom(["/nowhere/skills"], (message, type) => notifications.push({ message, type }))
  assert.deepEqual(result, {})
  assert.deepEqual(notifications.map((entry) => entry.type), ["warning"])
  assert.match(notifications[0].message, /no skills directory at \/nowhere\/skills/)
})
