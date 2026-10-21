// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const assert = require("assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { spawnSync } = require("child_process");
const { findSkipDirectives, readPullRequest, run } = require("./check-pr-commit-policy.js");

const ROOT = path.resolve(__dirname, "../..");
const tests = [];
const VARIABLES = { owner: "example", repo: "project", number: 42 };
const CONTEXT = {
  eventName: "pull_request_target",
  repo: { owner: VARIABLES.owner, repo: VARIABLES.repo },
  issue: { number: VARIABLES.number },
  payload: { pull_request: { title: "fix: keep CI running", body: "Discuss [skip ci]", head: { sha: commit(1).oid } } },
};

function contextWithHead(head, title = CONTEXT.payload.pull_request.title) {
  return {
    ...CONTEXT,
    payload: { pull_request: { ...CONTEXT.payload.pull_request, head: { sha: head }, title } },
  };
}

function test(name, check) {
  tests.push({ name, check });
}

function commit(index, message = "fix: keep CI running") {
  return { oid: index.toString(16).padStart(40, "0"), message };
}

function page(commits, options = {}) {
  return {
    repository: {
      pullRequest: {
        title: "fix: keep CI running",
        baseRefOid: commit(900).oid,
        headRefOid: commits.length ? commits[commits.length - 1].oid : commit(1).oid,
        ...options,
        commits: {
          totalCount: commits.length,
          nodes: commits.map((entry) => ({ commit: entry })),
          pageInfo: { hasNextPage: false, endCursor: null },
          ...options.commits,
        },
      },
    },
  };
}

function client(pages, comparisons = []) {
  const calls = [];
  const comparisonCalls = [];
  return {
    calls,
    comparisonCalls,
    rest: {
      repos: {
        async compareCommitsWithBasehead(options) {
          comparisonCalls.push(options);
          assert.strictEqual(options.owner, VARIABLES.owner);
          assert.strictEqual(options.repo, VARIABLES.repo);
          assert.strictEqual(options.per_page, 100);
          assert.strictEqual(options.page, comparisonCalls.length);
          assert.ok(comparisonCalls.length <= comparisons.length, "unexpected comparison call");
          const data = comparisons[comparisonCalls.length - 1];
          if (data instanceof Error) throw data;
          return { data };
        },
      },
    },
    async graphql(query, variables) {
      assert.ok(query.includes("commits(first: 100, after: $cursor)"));
      assert.ok(!query.includes("body"));
      assert.deepStrictEqual(
        { owner: variables.owner, repo: variables.repo, number: variables.number },
        VARIABLES
      );
      calls.push(variables);
      assert.ok(calls.length <= pages.length, "unexpected API call");
      const response = pages[calls.length - 1];
      if (response instanceof Error) throw response;
      return response;
    },
  };
}

function reporter() {
  const messages = { errors: [], failures: [], info: [] };
  return {
    messages,
    error: (message) => messages.errors.push(message),
    setFailed: (message) => messages.failures.push(message),
    info: (message) => messages.info.push(message),
  };
}

for (const marker of ["[skip ci]", "[ci skip]", "[no ci]", "[skip actions]", "[actions skip]"]) {
  test(`detects ${marker} anywhere in a full message`, () => {
    assert.deepStrictEqual(findSkipDirectives(`fix: title\n\n* format ${marker}`), [marker]);
    assert.deepStrictEqual(findSkipDirectives(`fix: ${marker.toUpperCase()}`), [marker]);
    assert.deepStrictEqual(findSkipDirectives(`${marker} ${marker}`), [marker]);
  });
}

test("detects skip-checks trailer variants, including CRLF", () => {
  for (const trailer of ["skip-checks:true", "skip-checks: true", "SKIP-CHECKS:\tTRUE", "  skip-checks: true\t"]) {
    assert.deepStrictEqual(findSkipDirectives(`fix: title\r\n\r\n\r\n${trailer}\r\n`), ["skip-checks: true"]);
  }
  assert.deepStrictEqual(findSkipDirectives("skip-checks: true"), ["skip-checks: true"]);
});

test("does not reject ordinary prose or false trailers", () => {
  for (const message of ["", "fix: do not skip ci", "[skip-ci]", "[skip ci-ish]", "skip-checks: false", "skip-checks: trueish", "Document skip-checks: true", "skip-checks:\ntrue"]) {
    assert.deepStrictEqual(findSkipDirectives(message), [], message);
  }
});

test("collects distinct directives", () => {
  assert.deepStrictEqual(findSkipDirectives("[skip ci] [no ci]\n\nskip-checks:true"), [
    "[skip ci]", "[no ci]", "skip-checks: true",
  ]);
});

test("follows GraphQL cursors until all reported commits are read", async () => {
  const commits = Array.from({ length: 301 }, (_, index) => commit(index + 1));
  commits[300].message = "chore: format [skip ci]";
  const pages = [];
  for (let offset = 0; offset < commits.length; offset += 100) {
    pages.push(page(commits.slice(offset, offset + 100), {
      headRefOid: commits[300].oid,
      commits: {
        totalCount: commits.length,
        pageInfo: { hasNextPage: offset < 300, endCursor: String(offset + 100) },
      },
    }));
  }
  const github = client(pages);
  const core = reporter();
  await run({ github, context: contextWithHead(commits[300].oid), core });
  assert.deepStrictEqual(github.calls.map((call) => call.cursor), [null, "100", "200", "300"]);
  assert.strictEqual(core.messages.failures.length, 1);
  assert.ok(core.messages.errors[0].includes(commits[300].oid));
});

function comparisonFixture() {
  const commits = Array.from({ length: 301 }, (_, index) => commit(index + 1));
  commits[300].message = "chore: format [skip ci]";
  const pages = [0, 100, 200].map((offset) => page(commits.slice(offset, Math.min(offset + 100, 250)), {
    headRefOid: commits[300].oid,
    commits: {
      totalCount: commits.length,
      pageInfo: { hasNextPage: offset < 200, endCursor: String(offset + 100) },
    },
  }));
  pages.push(pages[0]);
  const comparisons = [0, 100, 200, 300].map((offset) => ({
    total_commits: commits.length,
    commits: commits.slice(offset, offset + 100).map((entry) => ({
      sha: entry.oid, commit: { message: entry.message },
    })),
  }));
  return { commits, pages, comparisons };
}

test("falls back to paginated comparison when GitHub truncates GraphQL at 250 commits", async () => {
  const fixture = comparisonFixture();
  const github = client(fixture.pages, fixture.comparisons);
  const core = reporter();
  await run({ github, context: contextWithHead(commit(301).oid), core });
  assert.deepStrictEqual(github.calls.map((call) => call.cursor), [null, "100", "200", null]);
  assert.strictEqual(github.comparisonCalls.length, 4);
  for (const call of github.comparisonCalls) {
    assert.strictEqual(call.basehead, `${commit(900).oid}...${commit(301).oid}`);
  }
  assert.deepStrictEqual(core.messages.errors, [`Commit ${commit(301).oid} contains [skip ci].`]);
  assert.strictEqual(core.messages.failures.length, 1);
});

for (const [name, replacement] of [
  ["missing comparison", null],
  ["wrong total", { total_commits: 300, commits: [] }],
  ["missing commits", { total_commits: 301, commits: null }],
  ["truncated comparison", { total_commits: 301, commits: [] }],
  ["missing commit", { total_commits: 301, commits: [null] }],
  ["invalid SHA", { total_commits: 301, commits: [{ sha: "invalid", commit: { message: "fix" } }] }],
  ["missing message", { total_commits: 301, commits: [{ sha: commit(1).oid }] }],
  ["API failure", new Error("Unable to read comparison API")],
]) {
  test(`fails closed on ${name} in comparison fallback`, async () => {
    const fixture = comparisonFixture();
    fixture.comparisons[0] = replacement;
    await assert.rejects(readPullRequest(client(fixture.pages, fixture.comparisons), VARIABLES), /PR commit|comparison API/);
  });
}

test("rejects duplicate commits across comparison pages", async () => {
  const fixture = comparisonFixture();
  fixture.comparisons[1] = fixture.comparisons[0];
  await assert.rejects(readPullRequest(client(fixture.pages, fixture.comparisons), VARIABLES), /duplicate PR commit data/);
});

for (const extra of [false, true]) {
  test(`rejects a comparison with ${extra ? "too many commits" : "a missing head"}`, async () => {
    const fixture = comparisonFixture();
    const replacement = { sha: commit(302).oid, commit: { message: "fix" } };
    if (extra) fixture.comparisons[3].commits.push(replacement);
    else fixture.comparisons[3].commits[0] = replacement;
    await assert.rejects(readPullRequest(client(fixture.pages, fixture.comparisons), VARIABLES), /comparison does not match/);
  });
}

for (const changes of [
  { title: "changed title" }, { baseRefOid: commit(901).oid },
  { headRefOid: commit(302).oid }, { commits: { totalCount: 302 } },
]) {
  test(`rejects PR changes during comparison: ${JSON.stringify(changes)}`, async () => {
    const fixture = comparisonFixture();
    fixture.pages[3] = { repository: { pullRequest: { ...fixture.pages[0].repository.pullRequest, ...changes } } };
    await assert.rejects(readPullRequest(client(fixture.pages, fixture.comparisons), VARIABLES), /PR changed during validation/);
  });
}

test("rejects missing metadata after comparison", async () => {
  const fixture = comparisonFixture();
  fixture.pages[3] = { repository: null };
  await assert.rejects(readPullRequest(client(fixture.pages, fixture.comparisons), VARIABLES), /PR changed during validation/);
});

test("checks earlier commits even when the head is clean", async () => {
  const core = reporter();
  await run({ github: client([page([commit(1, "chore: format [skip ci]"), commit(2)])]), context: contextWithHead(commit(2).oid), core });
  assert.deepStrictEqual(core.messages.errors, [`Commit ${commit(1).oid} contains [skip ci].`]);
  assert.strictEqual(core.messages.failures.length, 1);
  assert.ok(core.messages.failures[0].includes("CONTRIBUTING.md#commit-messages"));
});

test("ignores description, author, and draft state", async () => {
  const core = reporter();
  await run({
    github: client([page([commit(1)], { body: "Report [skip ci]", isDraft: true, author: { login: "dependabot[bot]" } })]),
    context: CONTEXT,
    core,
  });
  assert.strictEqual(core.messages.failures.length, 0);
  assert.strictEqual(core.messages.info.length, 1);
});

test("rejects title-only edits without echoing untrusted text", async () => {
  const core = reporter();
  const title = "$(touch /tmp/not-executed) [SKIP CI]\n::error::untrusted-title";
  const message = "[no ci]\n::add-mask::untrusted-message";
  await run({ github: client([page([commit(1, message)], { title })]), context: contextWithHead(commit(1).oid, title), core });
  assert.deepStrictEqual(core.messages.errors, [
    "PR title contains [skip ci].",
    `Commit ${commit(1).oid} contains [no ci].`,
  ]);
  assert.ok(!JSON.stringify(core.messages).includes("untrusted"));
});

test("caps annotations without hiding the total number of failures", async () => {
  const core = reporter();
  const commits = Array.from({ length: 30 }, (_, index) => commit(index + 1, "fix: [skip ci]"));
  await run({ github: client([page(commits)]), context: contextWithHead(commits[29].oid), core });
  assert.strictEqual(core.messages.errors.length, 20);
  assert.ok(core.messages.failures[0].includes("30 PR title/commit message(s)"));
});

test("propagates API errors instead of reporting success", async () => {
  const core = reporter();
  await assert.rejects(run({ github: client([new Error("API unavailable")]), context: CONTEXT, core }), /API unavailable/);
  assert.strictEqual(core.messages.info.length, 0);
});

for (const [name, commits, eventHead] of [
  ["stale API snapshot after a push", [commit(1)], commit(2).oid],
  ["outdated event after another push", [commit(1), commit(2)], commit(1).oid],
  ["missing event head", [commit(1)], undefined],
]) {
  test(`rejects ${name}`, async () => {
    const core = reporter();
    await assert.rejects(run({
      github: client([page(commits)]), context: contextWithHead(eventHead), core,
    }), /PR head does not match the triggering event/);
    assert.strictEqual(core.messages.info.length, 0);
  });
}

for (const [apiTitle, eventTitle] of [
  ["fix: keep CI running", "fix: title [skip ci]"],
  ["fix: title [skip ci]", "fix: keep CI running"],
]) {
  test(`rejects an outdated title snapshot or event: ${apiTitle}`, async () => {
    const core = reporter();
    await assert.rejects(run({
      github: client([page([commit(1)], { title: apiTitle })]),
      context: contextWithHead(commit(1).oid, eventTitle), core,
    }), /PR title does not match the triggering event/);
    assert.strictEqual(core.messages.info.length, 0);
  });
}

for (const context of [
  { ...CONTEXT, eventName: "pull_request" },
  { ...CONTEXT, issue: { number: 0 } },
  { ...CONTEXT, issue: { number: "42; exit 0" } },
]) {
  test(`rejects invalid context ${JSON.stringify(context.issue)} / ${context.eventName}`, async () => {
    await assert.rejects(run({ github: client([]), context, core: reporter() }), /requires a pull_request_target/);
  });
}

const invalidPages = [
  ["missing repository", { repository: null }],
  ["missing PR", { repository: { pullRequest: null } }],
  ["missing title", page([commit(1)], { title: null })],
  ["invalid base", page([commit(1)], { baseRefOid: null })],
  ["invalid head", page([commit(1)], { headRefOid: "untrusted\n::error::" })],
  ["invalid count", page([commit(1)], { commits: { totalCount: "1" } })],
  ["empty PR", page([], { commits: { totalCount: 0 } })],
  ["missing nodes", page([commit(1)], { commits: { nodes: null } })],
  ["missing page info", page([commit(1)], { commits: { pageInfo: null } })],
  ["invalid page info", page([commit(1)], { commits: { pageInfo: {} } })],
  ["missing commit", page([commit(1)], { commits: { nodes: [null] } })],
  ["invalid commit SHA", page([commit(1)], { commits: { nodes: [{ commit: { oid: "bad", message: "fix" } }] } })],
  ["missing message", page([commit(1)], { commits: { nodes: [{ commit: { oid: commit(1).oid } }] } })],
  ["duplicate commits", page([commit(1), commit(1)])],
  ["truncated list", page([commit(1)], { commits: { totalCount: 2 } })],
  ["unexpected head", page([commit(1)], { headRefOid: commit(2).oid })],
  ["missing cursor", page([commit(1)], { commits: { totalCount: 2, pageInfo: { hasNextPage: true, endCursor: null } } })],
  ["empty intermediate page", page([], { commits: { totalCount: 2, pageInfo: { hasNextPage: true, endCursor: "next" } } })],
  ["extra page after total", page([commit(1)], { commits: { pageInfo: { hasNextPage: true, endCursor: "next" } } })],
];

for (const [name, response] of invalidPages) {
  test(`fails closed on ${name}`, async () => {
    await assert.rejects(readPullRequest(client([response]), VARIABLES), /Re-run PR Commit Policy/);
  });
}

const firstPage = page([commit(1)], {
  headRefOid: commit(3).oid,
  commits: { totalCount: 3, pageInfo: { hasNextPage: true, endCursor: "next" } },
});

for (const changes of [{ title: "edited title" }, { baseRefOid: commit(901).oid }, { headRefOid: commit(4).oid }, { commits: { totalCount: 4 } }]) {
  test(`rejects metadata changes between pages: ${JSON.stringify(changes)}`, async () => {
    const lastPage = page([commit(2), commit(3)], { commits: { totalCount: 3 }, ...changes });
    await assert.rejects(readPullRequest(client([firstPage, lastPage]), VARIABLES), /PR changed during validation/);
  });
}

test("rejects cursor loops", async () => {
  const secondPage = page([commit(2)], {
    headRefOid: commit(3).oid,
    commits: { totalCount: 3, pageInfo: { hasNextPage: true, endCursor: "next" } },
  });
  await assert.rejects(readPullRequest(client([firstPage, secondPage]), VARIABLES), /Incomplete PR commit pagination/);
});

for (const initialState of ["fresh", "cleaned sparse"]) {
  test(`policy checkout preserves source for later jobs in a ${initialState} workspace`, () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), "ocr-pr-policy-checkout-"));
    const git = (args, allowedStatuses = [0]) => {
      const result = spawnSync("git", args, {
        cwd: directory,
        encoding: "utf8",
        timeout: 10000,
        env: {
          ...process.env,
          HOME: directory,
          LC_ALL: "C",
          GIT_CONFIG_NOSYSTEM: "1",
          GIT_CONFIG_GLOBAL: os.devNull,
        },
      });
      assert.ifError(result.error);
      assert.ok(allowedStatuses.includes(result.status), `git ${args.join(" ")}: ${result.stderr}`);
      return result.stdout.trim();
    };
    const policyPath = "scripts/github-actions/check-pr-commit-policy.js";
    const files = {
      "go.mod": "module example.test/checkout\n",
      "cmd/opencodereview/main.go": "package main\n\nfunc main() {}\n",
      [policyPath]: "module.exports = {};\n",
    };
    try {
      git(["init", "-q"]);
      for (const [relativePath, content] of Object.entries(files)) {
        const filename = path.join(directory, relativePath);
        fs.mkdirSync(path.dirname(filename), { recursive: true });
        fs.writeFileSync(filename, content);
      }
      git(["add", "."]);
      git(["-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "fixture"]);
      const head = git(["rev-parse", "HEAD"]);
      const checkoutWithPinnedActionCommands = (sparsePattern) => {
        if (sparsePattern) {
          git(["config", "core.sparseCheckout", "true"]);
          fs.writeFileSync(path.join(directory, ".git/info/sparse-checkout"), `${sparsePattern}\n`);
        } else {
          git(["sparse-checkout", "disable"]);
          git(["config", "--local", "--unset-all", "extensions.worktreeConfig"], [0, 5]);
        }
        git(["checkout", "--force", "--detach", head]);
      };
      if (initialState === "cleaned sparse") {
        checkoutWithPinnedActionCommands(policyPath);
        assert.ok(!fs.existsSync(path.join(directory, "go.mod")), "fixture must reproduce sparse source omission");
        git(["config", "--local", "core.sparseCheckout", "false"]);
        git(["sparse-checkout", "disable"]);
      }

      const workflow = fs.readFileSync(path.join(ROOT, ".github/workflows/pr-commit-policy.yml"), "utf8");
      const sparsePattern = workflow.match(/^\s+sparse-checkout:\s*(.+)$/m)?.[1];
      checkoutWithPinnedActionCommands(sparsePattern);
      checkoutWithPinnedActionCommands();
      checkoutWithPinnedActionCommands();

      for (const [relativePath, content] of Object.entries(files)) {
        assert.ok(fs.existsSync(path.join(directory, relativePath)), `${relativePath} must survive later checkouts`);
        assert.strictEqual(fs.readFileSync(path.join(directory, relativePath), "utf8"), content);
      }
      assert.notStrictEqual(git(["config", "--local", "--bool", "core.sparseCheckout"], [0, 1]), "true");
      assert.strictEqual(git(["rev-parse", "HEAD"]), head);
      assert.strictEqual(git(["status", "--porcelain"]), "");
    } finally {
      fs.rmSync(directory, { recursive: true, force: true });
    }
  });
}

test("workflow preserves trust boundaries and runs the exported policy", async () => {
  const workflow = fs.readFileSync(path.join(ROOT, ".github/workflows/pr-commit-policy.yml"), "utf8");
  assert.match(workflow, /pull_request_target:\n    branches: \[main\]/);
  assert.match(workflow, /types: \[opened, synchronize, reopened, edited, ready_for_review\]/);
  assert.match(workflow, /permissions:\n  contents: read\n  pull-requests: read\n/);
  assert.match(workflow, /name: PR Commit Policy\n    runs-on: \[self-hosted, Linux, X64\]\n    timeout-minutes: 5/);
  assert.match(workflow, /container:\n      image: node:24\.18\.0\n      options: --cpus=1 --memory=1g\n/);
  assert.match(workflow, /ref: \$\{\{ github.sha \}\}/);
  assert.match(workflow, /persist-credentials: false/);
  assert.doesNotMatch(workflow, /^\s+sparse-checkout(?:-cone-mode)?:/m);
  assert.match(workflow, /cancel-in-progress: true/);
  assert.ok(!/\bwrite\b|\bsecrets\.|\bpaths(?:-ignore)?:|\bif:|\bcontinue-on-error:|\brun:|\buses:.*(?:cache|artifact)|pull_request\.(?:head|title|body)|--privileged|docker\.sock|\bvolumes:/.test(workflow));
  const script = workflow.split("          script: |\n")[1].trim();
  assert.ok(!script.includes("${{"));
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  const core = reporter();
  await new AsyncFunction("require", "github", "context", "core", script)(
    (modulePath) => {
      assert.strictEqual(modulePath, "./scripts/github-actions/check-pr-commit-policy.js");
      return { run };
    },
    client([page([commit(1)])]), CONTEXT, core
  );
  assert.strictEqual(core.messages.info.length, 1);
  const contract = fs.readFileSync(path.join(ROOT, ".github/workflows/action-contract.yml"), "utf8");
  assert.strictEqual((contract.match(/'\.github\/workflows\/pr-commit-policy.yml'/g) || []).length, 2);
  const manifest = JSON.parse(fs.readFileSync(path.join(ROOT, "package.json"), "utf8"));
  assert.ok(manifest.scripts["test:github-actions"].includes("node scripts/github-actions/check-pr-commit-policy.test.js"));
});

async function main() {
  for (const { name, check } of tests) {
    await check();
    console.log(`PASS ${name}`);
  }
  console.log(`\nAll ${tests.length} PR commit policy tests passed.`);
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
