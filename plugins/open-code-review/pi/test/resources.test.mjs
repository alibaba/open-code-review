// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import assert from "node:assert/strict"
import { existsSync } from "node:fs"
import { dirname, resolve } from "node:path"
import test from "node:test"
import { fileURLToPath } from "node:url"
import { createHost, stdoutExec } from "./helpers.mjs"

const PACKAGE_DIR = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const REPOSITORY_SKILLS = resolve(PACKAGE_DIR, "..", "..", "..", "skills")

test("the extension contributes a skills directory to the host", async () => {
  const handler = (await createHost(stdoutExec(""))).events.get("resources_discover")
  assert.ok(handler, "no resources_discover handler was registered")
  const result = await handler({ type: "resources_discover", cwd: PACKAGE_DIR, reason: "startup" })
  assert.deepEqual(result, { skillPaths: [REPOSITORY_SKILLS] })
  assert.ok(existsSync(resolve(REPOSITORY_SKILLS, "open-code-review", "SKILL.md")))
  assert.ok(existsSync(resolve(REPOSITORY_SKILLS, "open-code-review-delegate", "SKILL.md")))
})
