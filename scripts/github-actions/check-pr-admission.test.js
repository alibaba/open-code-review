// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const assert = require("assert");
const fs = require("fs");
const path = require("path");
const { run } = require("./check-pr-admission.js");

const ROOT = path.resolve(__dirname, "../..");
const POLICY = require("../../.github/pr-admission.json");
const AUTHOR = { id: 101, login: "contributor", type: "User" };
const BOT = { id: 41898282, login: "github-actions[bot]", type: "Bot" };
const REPO = { owner: "example", repo: "project" };
const REOPENED_AT = "2026-01-02T00:00:00Z";
const tests = [];

function test(name, check) {
  tests.push({ name, check });
}

function pull(number, overrides = {}) {
  return {
    number, user: { ...AUTHOR }, state: "open", draft: false,
    created_at: new Date(Date.UTC(2026, 0, 1, 0, 0, number)).toISOString(),
    updated_at: new Date(Date.UTC(2026, 0, 1, 0, 0, number)).toISOString(),
    ...overrides,
  };
}

function pulls(count) {
  return Array.from({ length: count }, (_, index) => pull(index + 1));
}

function reopening(id = 1, created_at = REOPENED_AT) {
  return { id, event: "reopened", created_at };
}

function fixture(options = {}) {
  const current = options.current || pull(11);
  if (options.action === "reopened" && !options.events) current.updated_at = REOPENED_AT;
  const storedPulls = options.pulls || pulls(11);
  const comments = options.comments || [];
  const events = options.events || (options.action === "reopened" ? [reopening()] : []);
  const eventsByNumber = options.eventsByNumber || new Map([[current.number, events]]);
  const calls = [];
  const writes = [];
  const outputs = {};
  const context = {
    eventName: "pull_request_target", repo: REPO, issue: { number: current.number },
    payload: { action: options.action || "opened", pull_request: { ...current } },
  };
  let getCount = 0;
  let listCount = 0;
  const github = { rest: { pulls: {}, issues: {} } };
  const read = (method, result) => async (params) => {
    calls.push({ method, params });
    if (options.fail === method) throw new Error(`API failed: ${method}`);
    return { data: typeof result === "function" ? result(params) : result };
  };
  const write = (method, perform) => async (params) => {
    calls.push({ method, params });
    if (options.fail === method) throw new Error(`API failed: ${method}`);
    writes.push({ method, params });
    perform(params);
    if (options.failAfterWrite === method) {
      options.failAfterWrite = null;
      throw new Error(`Response lost: ${method}`);
    }
    return { data: {} };
  };
  github.rest.pulls.get = read("get", () => {
    getCount += 1;
    return getCount > 1 && options.latest ? options.latest : current;
  });
  github.rest.pulls.list = read("list", () => {
    if (options.snapshots) {
      const snapshot = options.snapshots[Math.min(listCount, options.snapshots.length - 1)];
      listCount += 1;
      return [...snapshot];
    }
    return storedPulls.filter((entry) => entry.state === "open");
  });
  github.rest.issues.listComments = read("listComments", comments);
  github.rest.issues.listEvents = read("listEvents", ({ issue_number }) => {
    const history = eventsByNumber.get(issue_number) || [];
    return typeof history === "function" ? history() : history;
  });
  github.rest.issues.createComment = write("createComment", (params) => {
    assert.deepStrictEqual(params.request, { retries: 0 });
    comments.push({ id: comments.length + 1, user: BOT, body: params.body });
  });
  github.rest.issues.updateComment = write("updateComment", (params) => {
    comments.find((comment) => comment.id === params.comment_id).body = params.body;
  });
  github.rest.pulls.update = write("close", (params) => {
    current.state = params.state;
    const listed = storedPulls.find((entry) => entry.number === params.pull_number);
    if (listed) listed.state = params.state;
  });
  github.paginate = async (method, params) => {
    assert.strictEqual(params.per_page, 100);
    assert.strictEqual(params.owner, REPO.owner);
    assert.strictEqual(params.repo, REPO.repo);
    if (method === github.rest.pulls.list) {
      assert.strictEqual(params.state, "open");
      assert.strictEqual(params.sort, "created");
      assert.strictEqual(params.direction, "asc");
      assert.ok(!("base" in params) && !("head" in params));
    } else if (method !== github.rest.issues.listEvents) {
      assert.strictEqual(params.issue_number, current.number);
    } else {
      assert.ok(Number.isSafeInteger(params.issue_number) && params.issue_number > 0);
    }
    return (await method(params)).data;
  };
  const core = { setOutput: (key, value) => { outputs[key] = value; }, info: () => {} };
  return { github, context, core, calls, writes, outputs, current, storedPulls, comments, events, eventsByNumber };
}

function reopen(subject, created_at = REOPENED_AT) {
  subject.current.state = "open";
  subject.current.updated_at = created_at;
  Object.assign(subject.storedPulls.find((entry) => entry.number === subject.current.number), subject.current);
  subject.events.push(reopening(subject.events.length + 1, created_at));
  subject.context.payload = { action: "reopened", pull_request: { ...subject.current } };
}

async function expectAdmission(subject, allowed) {
  assert.strictEqual(await run(subject), allowed);
  assert.strictEqual(subject.outputs.allowed, String(allowed));
  for (const { method, params } of subject.writes) {
    assert.strictEqual(params.owner, REPO.owner);
    assert.strictEqual(params.repo, REPO.repo);
    if (method === "close") assert.strictEqual(params.pull_number, subject.context.issue.number);
  }
}

test("default policy caps at ten and exempts only the explicit Dependabot identity", () => {
  assert.deepStrictEqual(POLICY, {
    maxOpen: 10, exemptBots: [{ id: 49699333, login: "dependabot[bot]" }],
  });
});

for (const count of [1, 9, 10]) {
  test(`admits ${count} open PRs without any mutations`, async () => {
    const subject = fixture({ current: pull(count), pulls: pulls(count) });
    await expectAdmission(subject, true);
    assert.deepStrictEqual(subject.writes, []);
  });
}

test("closes only the excess triggering PR, after posting the explanation", async () => {
  const subject = fixture();
  await expectAdmission(subject, false);
  assert.deepStrictEqual(subject.writes.map((entry) => entry.method), ["createComment", "close"]);
  assert.match(subject.comments[0].body, /10 open pull requests/);
  assert.match(subject.comments[0].body, /currently have 11 open PRs, including drafts/);
  assert.match(subject.comments[0].body, /not a judgment about the quality/);
  assert.match(subject.comments[0].body, /then reopen this PR/);
  assert.match(subject.comments[0].body, /example\/project\/blob\/HEAD\/\.github\/PR_MODERATION.md/);
  assert.deepStrictEqual(subject.storedPulls.filter((entry) => entry.state === "closed").map((entry) => entry.number), [11]);
});

test("does not retroactively sweep an author's historical backlog", async () => {
  const subject = fixture({ current: pull(24), pulls: pulls(24) });
  await expectAdmission(subject, false);
  assert.strictEqual(subject.storedPulls.filter((entry) => entry.state === "open").length, 23);
  assert.strictEqual(subject.writes.filter((entry) => entry.method === "close").length, 1);
});

test("counts drafts and all base branches, using author IDs even after a rename", async () => {
  const entries = pulls(11).map((entry) => ({
    ...entry, draft: true, base: { ref: `branch-${entry.number}` },
    user: { ...AUTHOR, login: "previous-login" },
  }));
  const subject = fixture({ pulls: entries });
  await expectAdmission(subject, false);
});

test("uses the complete paginated result and ignores other authors and lookalike logins", async () => {
  const entries = pulls(140).map((entry) => ({ ...entry, user: { ...AUTHOR, id: 202 } }));
  const subject = fixture({ current: pull(151), pulls: [...entries, pull(151)] });
  await expectAdmission(subject, true);
  const overLimit = fixture({ current: pull(151), pulls: [...entries, ...pulls(10).map((entry) => pull(entry.number + 140)), pull(151)] });
  await expectAdmission(overLimit, false);
});

test("rechecks pagination when a closure shifts an author's PR across a page boundary", async () => {
  const entries = pulls(102).map((entry) => entry.number <= 91 ? {
    ...entry, user: { ...AUTHOR, id: 202 },
  } : entry);
  const incomplete = [...entries.slice(0, 100), entries[101]];
  const complete = entries.slice(1);
  const subject = fixture({ current: pull(102), pulls: complete, snapshots: [incomplete, complete, complete] });
  await expectAdmission(subject, false);
  assert.strictEqual(subject.calls.filter((entry) => entry.method === "list").length, 3);
  assert.match(subject.comments[0].body, /currently have 11 open PRs/);
});

test("fails without moderation or review when pagination never stabilizes", async () => {
  const subject = fixture({ current: pull(10), snapshots: [pulls(10), pulls(11), pulls(10)] });
  await assert.rejects(run(subject), /did not stabilize/);
  assert.strictEqual(subject.calls.filter((entry) => entry.method === "list").length, 3);
  assert.strictEqual(subject.outputs.allowed, "false");
  assert.deepStrictEqual(subject.writes, []);
});

test("sorts by creation time with PR number as a stable tie breaker", async () => {
  const entries = pulls(11).map((entry) => ({ ...entry, created_at: pull(1).created_at })).reverse();
  const early = fixture({ current: entries.find((entry) => entry.number === 1), pulls: entries });
  await expectAdmission(early, true);
  assert.deepStrictEqual(early.writes, []);
  const late = fixture({ current: entries.find((entry) => entry.number === 11), pulls: entries });
  await expectAdmission(late, false);
});

for (const order of [pulls(15).map((entry) => entry.number), pulls(15).map((entry) => entry.number).reverse()]) {
  test(`burst keeps the first ten with ${order[0] === 1 ? "forward" : "reverse"} event delivery`, async () => {
    const entries = pulls(15);
    const accepted = [];
    for (const number of order) {
      const subject = fixture({ current: entries.find((entry) => entry.number === number), pulls: entries });
      const allowed = await run(subject);
      if (allowed) accepted.push(number);
    }
    assert.deepStrictEqual(accepted.sort((left, right) => left - right), pulls(10).map((entry) => entry.number));
    assert.deepStrictEqual(entries.filter((entry) => entry.state === "open").map((entry) => entry.number), accepted);
  });
}

test("reopening an old PR consumes a slot rather than displacing an existing PR", async () => {
  const subject = fixture({ current: pull(1), action: "reopened" });
  await expectAdmission(subject, false);
  assert.deepStrictEqual(subject.storedPulls.filter((entry) => entry.state === "closed").map((entry) => entry.number), [1]);
});

test("reopening succeeds when a slot becomes available and ignores closed or merged PRs", async () => {
  const entries = pulls(11);
  entries[9].state = "closed";
  entries[9].merged_at = "2026-01-02T00:00:00Z";
  const subject = fixture({ current: pull(1), action: "reopened", pulls: entries });
  await expectAdmission(subject, true);
  assert.deepStrictEqual(subject.writes, []);
});

for (const count of [10, 11]) {
  test(`a stale opened run leaves a reopened PR untouched with ${count} PRs now open`, async () => {
    const subject = fixture({ current: pull(1), action: "reopened", pulls: pulls(10) });
    await expectAdmission(subject, true);
    if (count === 11) subject.storedPulls.push(pull(11));
    subject.context.payload.action = "opened";
    await expectAdmission(subject, false);
    assert.strictEqual(subject.current.state, "open");
    assert.deepStrictEqual(subject.writes, []);
  });
}

for (const reopenedAt of ["2026-01-01T00:00:10Z", "2026-01-01T00:00:11Z", "2026-01-01T00:00:12Z"]) {
  for (const order of [[11, 1], [1, 11]]) {
    test(`mixed openings preserve episode priority at ${reopenedAt} in order ${order}`, async () => {
      const entries = pulls(11);
      entries[0].updated_at = reopenedAt;
      const histories = new Map([[1, [reopening(1, reopenedAt)]]]);
      const winner = Date.parse(reopenedAt) < Date.parse(pull(11).created_at) ? 1 : 11;
      const closed = [];
      for (const number of order) {
        const subject = fixture({
          current: entries.find((entry) => entry.number === number), pulls: entries,
          action: number === 1 ? "reopened" : "opened",
          events: histories.get(number) || [], eventsByNumber: histories,
        });
        await expectAdmission(subject, number === winner);
        closed.push(...subject.writes.filter((entry) => entry.method === "close").map((entry) => entry.params.pull_number));
      }
      assert.deepStrictEqual(closed, [winner === 1 ? 11 : 1]);
      assert.strictEqual(entries.filter((entry) => entry.state === "open").length, 10);
      assert.strictEqual(entries.find((entry) => entry.number === winner).state, "open");
    });
  }
}

for (const order of [[1, 2], [2, 1]]) {
  test(`two reopened PRs use reopening time rather than creation time in order ${order}`, async () => {
    const entries = pulls(11);
    entries[0].updated_at = "2026-01-02T00:00:02Z";
    entries[1].updated_at = "2026-01-02T00:00:01Z";
    const histories = new Map(entries.slice(0, 2).map((entry) => [entry.number, [reopening(entry.number, entry.updated_at)]]));
    for (const number of order) {
      const subject = fixture({
        current: entries[number - 1], pulls: entries, action: "reopened",
        events: histories.get(number), eventsByNumber: histories,
      });
      await expectAdmission(subject, number === 2);
    }
    assert.strictEqual(entries[0].state, "closed");
    assert.strictEqual(entries[1].state, "open");
  });
}

for (const user of [AUTHOR, { ...POLICY.exemptBots[0], type: "Bot" }]) {
  for (const staleFirst of [true, false]) {
    test(`superseded reopen skips review for ${user.login}, stale first: ${staleFirst}`, async () => {
      const subject = fixture({ current: pull(1, { user, head: { sha: "old-head" } }), pulls: [pull(1, { user })], action: "reopened" });
      const stale = subject.context.payload;
      subject.current.head = { sha: "new-head" };
      reopen(subject, "2026-01-02T00:00:02Z");
      const latest = subject.context.payload;
      for (const payload of staleFirst ? [stale, latest] : [latest, stale]) {
        subject.context.payload = payload;
        await expectAdmission(subject, payload === latest);
      }
      assert.notStrictEqual(stale.pull_request.head.sha, latest.pull_request.head.sha);
      assert.deepStrictEqual(subject.writes, []);
    });
  }
}

test("opening history must stabilize even when the open PR numbers do not change", async () => {
  let reads = 0;
  const histories = new Map([[1, () => ++reads === 1 ? [] : [reopening()]]]);
  const subject = fixture({ eventsByNumber: histories });
  await expectAdmission(subject, true);
  assert.strictEqual(subject.calls.filter((entry) => entry.method === "list").length, 3);
  assert.deepStrictEqual(subject.writes, []);
});

test("unstable opening cycles fail without moderation", async () => {
  let reads = 0;
  const histories = new Map([[1, () => [reopening(++reads, `2026-01-02T00:00:0${reads}Z`)]]]);
  const subject = fixture({ eventsByNumber: histories });
  await assert.rejects(run(subject), /did not stabilize/);
  assert.strictEqual(subject.outputs.allowed, "false");
  assert.deepStrictEqual(subject.writes, []);
});

for (const action of ["opened", "reopened"]) {
  test(`skips an ${action} event superseded during the open PR scans`, async () => {
    let reads = 0;
    const previous = action === "opened" ? [] : [reopening()];
    const histories = new Map([[1, () => ++reads === 1 ? previous : [...previous, reopening(2, "2026-01-02T00:00:02Z")]]]);
    const subject = fixture({ current: pull(1), pulls: pulls(1), action, eventsByNumber: histories });
    await expectAdmission(subject, false);
    assert.strictEqual(subject.calls.filter((entry) => entry.method === "list").length, 2);
    assert.deepStrictEqual(subject.writes, []);
  });
}

test("rechecks the opening cycle before closing an excess PR", async () => {
  let reads = 0;
  const histories = new Map([[11, () => ++reads < 4 ? [reopening()] : [reopening(), reopening(2, "2026-01-02T00:00:02Z")]]]);
  const subject = fixture({ action: "reopened", eventsByNumber: histories });
  await expectAdmission(subject, false);
  assert.strictEqual(reads, 4);
  assert.deepStrictEqual(subject.writes, []);
  assert.strictEqual(subject.current.state, "open");
});

test("uses the latest reopening even when history is returned out of order", async () => {
  const subject = fixture({ current: pull(1, { updated_at: "2026-01-02T00:00:02Z" }), pulls: pulls(1), action: "reopened",
    events: [reopening(2, "2026-01-02T00:00:02Z"), { event: "closed" }, reopening()] });
  await expectAdmission(subject, true);
});

for (const updated_at of [undefined, null, "bad-date", 123]) {
  test(`rejects a reopened payload with invalid updated_at: ${updated_at}`, async () => {
    const subject = fixture({ action: "reopened" });
    subject.context.payload.pull_request.updated_at = updated_at;
    await assert.rejects(run(subject), /reopening metadata/);
    assert.strictEqual(subject.outputs.allowed, "false");
    assert.deepStrictEqual(subject.writes, []);
  });
}

test("a missing or lagging reopening history cannot authorize review", async () => {
  for (const events of [[], [reopening(1, "2026-01-01T00:01:00Z")]]) {
    const subject = fixture({ current: pull(11, { updated_at: REOPENED_AT }), action: "reopened", events });
    await assert.rejects(run(subject), /reopening metadata|opening cycle/);
    assert.strictEqual(subject.outputs.allowed, "false");
    assert.deepStrictEqual(subject.writes, []);
  }
});

test("same-second reopenings fail closed instead of admitting indistinguishable events", async () => {
  for (const events of [[reopening(1), reopening(2)], [reopening(2), reopening(1)]]) {
    const subject = fixture({ current: pull(11, { updated_at: REOPENED_AT }), action: "reopened", events });
    await assert.rejects(run(subject), /opening cycle/);
    assert.strictEqual(subject.outputs.allowed, "false");
    assert.deepStrictEqual(subject.writes, []);
  }
});

for (const events of [
  [reopening(0)], [reopening("1")], [reopening(1, "bad-date")],
  [reopening(1, null)], [reopening(1, "2025-01-01T00:00:00Z")], [reopening(), reopening()],
]) {
  test(`invalid reopening history cannot cause moderation: ${JSON.stringify(events)}`, async () => {
    const subject = fixture({ events });
    await assert.rejects(run(subject), /reopening history/);
    assert.strictEqual(subject.outputs.allowed, "false");
    assert.deepStrictEqual(subject.writes, []);
  });
}

test("does not scan unrelated authors' histories or non-target histories below the cap", async () => {
  const subject = fixture({ current: pull(5), pulls: [...pulls(9), pull(10, { user: { ...AUTHOR, id: 202 } })] });
  await expectAdmission(subject, true);
  assert.ok(subject.calls.filter((entry) => entry.method === "listEvents").every((entry) => entry.params.issue_number === 5));
});

test("allows the explicitly configured bot without listing PRs", async () => {
  const subject = fixture({ current: pull(11, { user: { ...POLICY.exemptBots[0], type: "Bot" } }) });
  await expectAdmission(subject, true);
  assert.deepStrictEqual(subject.calls.map((entry) => entry.method), ["get", "listEvents"]);
});

test("stale opened events also skip review for an exempt bot", async () => {
  const subject = fixture({
    current: pull(11, { user: { ...POLICY.exemptBots[0], type: "Bot" } }),
    events: [reopening()], fail: "list",
  });
  await expectAdmission(subject, false);
  assert.deepStrictEqual(subject.writes, []);
});

for (const user of [
  { ...AUTHOR, type: "Bot" },
  { ...POLICY.exemptBots[0], type: "User" },
  { ...POLICY.exemptBots[0], id: 123, type: "Bot" },
  { ...POLICY.exemptBots[0], login: "another[bot]", type: "Bot" },
]) {
  test(`does not exempt a partial bot identity match: ${JSON.stringify(user)}`, async () => {
    const subject = fixture({ current: pull(11, { user }), pulls: pulls(11).map((entry) => ({ ...entry, user })) });
    await expectAdmission(subject, false);
  });
}

test("repository membership is not an exemption", async () => {
  const subject = fixture({ current: pull(11, { author_association: "OWNER" }) });
  await expectAdmission(subject, false);
});

test("skips PRs already closed or merged", async () => {
  for (const merged_at of [null, "2026-01-02T00:00:00Z"]) {
    const subject = fixture({ current: pull(11, { state: "closed", merged_at }) });
    await expectAdmission(subject, false);
    assert.deepStrictEqual(subject.writes, []);
    assert.deepStrictEqual(subject.calls.map((entry) => entry.method), ["get"]);
  }
});

test("does not comment or close a PR that was closed while admission was running", async () => {
  const subject = fixture({ latest: pull(11, { state: "closed" }) });
  await expectAdmission(subject, false);
  assert.deepStrictEqual(subject.writes, []);
});

for (const method of ["listComments", "createComment", "updateComment"]) {
  for (const action of ["opened", "reopened"]) {
    test(`an ${action} run cannot close a new opening cycle started during ${method}`, async () => {
      const subject = fixture({
        action,
        comments: method === "updateComment" ? [{ id: 1, user: BOT, body: "<!-- ocr-pr-open-cap -->\nOld explanation" }] : [],
      });
      const original = subject.github.rest.issues[method];
      const stalePayload = subject.context.payload;
      let latestPayload;
      subject.github.rest.issues[method] = async (params) => {
        const result = await original(params);
        subject.storedPulls[0].state = "closed";
        reopen(subject, "2026-01-02T00:00:02Z");
        latestPayload = subject.context.payload;
        subject.context.payload = stalePayload;
        return result;
      };
      await expectAdmission(subject, false);
      assert.strictEqual(subject.current.state, "open");
      assert.strictEqual(subject.storedPulls.filter((entry) => entry.state === "open").length, 10);
      assert.ok(!subject.writes.some((entry) => entry.method === "close"));
      subject.github.rest.issues[method] = original;
      subject.context.payload = latestPayload;
      await expectAdmission(subject, true);
    });
  }

  test(`does not close a PR already closed during ${method}`, async () => {
    const subject = fixture({
      comments: method === "updateComment" ? [{ id: 1, user: BOT, body: "<!-- ocr-pr-open-cap -->\nOld explanation" }] : [],
    });
    const original = subject.github.rest.issues[method];
    subject.github.rest.issues[method] = async (params) => {
      const result = await original(params);
      subject.current.state = "closed";
      return result;
    };
    await expectAdmission(subject, false);
    assert.ok(!subject.writes.some((entry) => entry.method === "close"));
  });
}

for (const method of ["get", "listEvents"]) {
  test(`failure in the final ${method} check after commenting cannot close a PR`, async () => {
    const subject = fixture();
    const original = subject.github.rest.issues.createComment;
    subject.github.rest.issues.createComment = async (params) => {
      const result = await original(params);
      const endpoint = method === "get" ? subject.github.rest.pulls : subject.github.rest.issues;
      endpoint[method] = async () => { throw new Error(`Final check failed: ${method}`); };
      return result;
    };
    await assert.rejects(run(subject), new RegExp(`Final check failed: ${method}`));
    assert.strictEqual(subject.outputs.allowed, "false");
    assert.strictEqual(subject.current.state, "open");
    assert.deepStrictEqual(subject.writes.map((entry) => entry.method), ["createComment"]);
  });
}

test("retries reuse the bot's own comment, even beyond the first comments page", async () => {
  const subject = fixture({ comments: Array.from({ length: 105 }, (_, index) => ({ id: index + 1, user: AUTHOR, body: "Discussion" })) });
  await expectAdmission(subject, false);
  await expectAdmission(subject, false);
  reopen(subject);
  await expectAdmission(subject, false);
  assert.strictEqual(subject.writes.filter((entry) => entry.method === "createComment").length, 1);
  assert.strictEqual(subject.writes.filter((entry) => entry.method === "updateComment").length, 0);
  assert.strictEqual(subject.writes.filter((entry) => entry.method === "close").length, 2);
});

test("updates the existing explanation when the count changes", async () => {
  const subject = fixture();
  await expectAdmission(subject, false);
  reopen(subject);
  subject.storedPulls.push(pull(12));
  await expectAdmission(subject, false);
  assert.strictEqual(subject.comments.length, 1);
  assert.match(subject.comments[0].body, /currently have 12 open PRs/);
  assert.strictEqual(subject.writes.filter((entry) => entry.method === "updateComment").length, 1);
});

test("a lost creation response fails closed and a job rerun reuses the published comment", async () => {
  const subject = fixture({ failAfterWrite: "createComment" });
  await assert.rejects(run(subject), /Response lost: createComment/);
  assert.strictEqual(subject.outputs.allowed, "false");
  assert.strictEqual(subject.current.state, "open");
  assert.strictEqual(subject.comments.length, 1);
  await expectAdmission(subject, false);
  assert.strictEqual(subject.comments.length, 1);
  assert.strictEqual(subject.writes.filter((entry) => entry.method === "createComment").length, 1);
});

for (const reopened of [false, true]) {
  test(`a lost closure response is not retried and a rerun validates the PR, reopened: ${reopened}`, async () => {
    const subject = fixture({ failAfterWrite: "close" });
    const original = subject.github.rest.pulls.update;
    const stalePayload = subject.context.payload;
    let latestPayload;
    subject.github.rest.pulls.update = async (params) => {
      assert.deepStrictEqual(params.request, { retries: 0 });
      try {
        return await original(params);
      } catch (error) {
        if (reopened) {
          subject.storedPulls[0].state = "closed";
          reopen(subject);
          latestPayload = subject.context.payload;
          subject.context.payload = stalePayload;
        }
        throw error;
      }
    };
    await assert.rejects(run(subject), /Response lost: close/);
    assert.strictEqual(subject.outputs.allowed, "false");
    assert.strictEqual(subject.current.state, reopened ? "open" : "closed");
    await expectAdmission(subject, false);
    if (reopened) {
      subject.context.payload = latestPayload;
      await expectAdmission(subject, true);
      assert.strictEqual(subject.current.state, "open");
    }
    assert.strictEqual(subject.comments.length, 1);
    assert.strictEqual(subject.writes.filter((entry) => entry.method === "close").length, 1);
  });
}

test("spoofed policy comments from humans or other bots cannot suppress the explanation", async () => {
  const comments = [AUTHOR, { ...BOT, type: "User" }, { ...BOT, login: "other[bot]" }, null]
    .map((user, index) => ({ id: index + 1, user, body: "<!-- ocr-pr-open-cap -->\nFake policy" }));
  const subject = fixture({ comments });
  await expectAdmission(subject, false);
  assert.strictEqual(subject.writes[0].method, "createComment");
  assert.strictEqual(comments.length, 5);
});

for (const method of ["get", "list", "listEvents", "listComments", "createComment", "updateComment", "close"]) {
  test(`API failure in ${method} never permits AI review`, async () => {
    const subject = fixture({
      fail: method, current: method === "listEvents" ? pull(1) : pull(11),
      comments: method === "updateComment" ? [{ id: 1, user: BOT, body: "<!-- ocr-pr-open-cap -->\nOld explanation" }] : [],
    });
    await assert.rejects(run(subject), new RegExp(`API failed: ${method}`));
    assert.strictEqual(subject.outputs.allowed, "false");
    assert.ok(!subject.writes.some((entry) => entry.method === "close"));
    if (method !== "close") assert.deepStrictEqual(subject.writes, []);
  });
}

for (const patch of [
  { number: -1 }, { user: null }, { user: { id: "101" } },
  { state: "unknown" }, { created_at: "bad-date" },
]) {
  test(`invalid metadata cannot cause moderation: ${JSON.stringify(patch)}`, async () => {
    const subject = fixture({ current: pull(11, patch) });
    await assert.rejects(run(subject));
    assert.strictEqual(subject.outputs.allowed, "false");
    assert.deepStrictEqual(subject.writes, []);
  });
}

for (const [name, options] of [
  ["missing target", { pulls: pulls(10) }],
  ["duplicate PR", { pulls: [...pulls(11), pull(11)] }],
  ["changed author", { pulls: [...pulls(10), pull(11, { user: { ...AUTHOR, id: 202 } })] }],
  ["changed creation time", { pulls: [...pulls(10), pull(11, { created_at: pull(12).created_at })] }],
  ["missing author", { pulls: [...pulls(10), pull(11, { user: null })] }],
  ["changed PR before closure", { latest: pull(12) }],
  ["changed author before closure", { latest: pull(11, { user: { ...AUTHOR, id: 202 } }) }],
]) {
  test(`rejects inconsistent API snapshots: ${name}`, async () => {
    const subject = fixture(options);
    await assert.rejects(run(subject), /PR/);
    assert.strictEqual(subject.outputs.allowed, "false");
    assert.deepStrictEqual(subject.writes, []);
  });
}

test("rejects untrusted events and mismatched triggering metadata", async () => {
  for (const change of [
    (subject) => { subject.context.eventName = "pull_request"; },
    (subject) => { subject.context.payload.action = "synchronize"; },
    (subject) => { subject.context.payload.pull_request = {}; },
    (subject) => { subject.context.issue.number = 12; },
  ]) {
    const subject = fixture();
    change(subject);
    await assert.rejects(run(subject), /PR/);
    assert.strictEqual(subject.outputs.allowed, "false");
    assert.deepStrictEqual(subject.writes, []);
  }
});

test("validates policy before accessing GitHub", async () => {
  for (const policy of [
    { ...POLICY, maxOpen: 0 }, { ...POLICY, maxOpen: "10" },
    { ...POLICY, exemptBots: null }, { ...POLICY, exemptBots: [{ id: 0, login: "bot" }] },
    { ...POLICY, exemptBots: [{ id: 1, login: "" }] },
  ]) {
    const subject = fixture();
    await assert.rejects(run({ ...subject, policy }), /Invalid PR admission policy/);
    assert.deepStrictEqual(subject.calls, []);
    assert.strictEqual(subject.outputs.allowed, "false");
  }
});

test("workflow gates AI review on the real policy and preserves trusted execution", async () => {
  const workflow = fs.readFileSync(path.join(ROOT, ".github/workflows/ocr-review.yml"), "utf8");
  const admission = workflow.split("  pr-admission:\n")[1].split("\n  code-review:\n")[0];
  const review = workflow.split("\n  code-review:\n")[1];
  assert.match(workflow, /pull_request_target:\n    types: \[opened, reopened\]/);
  assert.match(workflow, /permissions:\n  contents: read\n  pull-requests: write\n/);
  assert.match(admission, /group: pr-admission-\$\{\{ github.repository \}\}-\$\{\{ github.event.pull_request.user.id \}\}/);
  assert.match(admission, /cancel-in-progress: false\n      queue: max/);
  assert.match(admission, /timeout-minutes: 5/);
  assert.match(admission, /ref: \$\{\{ github.event.repository.default_branch \}\}/);
  assert.match(admission, /persist-credentials: false/);
  assert.match(admission, /allowed: \$\{\{ steps.admission.outputs.allowed \}\}/);
  assert.match(admission, /id: admission/);
  assert.match(admission, /uses: actions\/checkout@[0-9a-f]{40}/);
  assert.match(admission, /uses: actions\/github-script@[0-9a-f]{40}/);
  assert.doesNotMatch(admission, /secrets\.|pull_request\.(?:head|title|body)|sparse-checkout|continue-on-error|--privileged|docker\.sock|\brun:/);
  assert.match(review, /needs: pr-admission\n    if: needs.pr-admission.outputs.allowed == 'true'/);
  assert.match(review, /group: \$\{\{ github.workflow \}\}-review-\$\{\{ github.event.pull_request.number \}\}/);
  assert.match(review, /uses: \.\//);
  assert.match(review, /llm_auth_token: \$\{\{ secrets.OCR_LLM_AUTH_TOKEN \}\}/);
  assert.doesNotMatch(review, /always\(\)|continue-on-error/);
  const script = admission.split("          script: |\n")[1].trim();
  assert.ok(!script.includes("${{"));
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  for (const count of [10, 11]) {
    const subject = fixture({ current: pull(count), pulls: pulls(count) });
    await new AsyncFunction("require", "github", "context", "core", script)(
      (modulePath) => {
        assert.strictEqual(modulePath, "./scripts/github-actions/check-pr-admission.js");
        return { run };
      }, subject.github, subject.context, subject.core
    );
    assert.strictEqual(subject.outputs.allowed, String(count <= 10));
  }
  const contract = fs.readFileSync(path.join(ROOT, ".github/workflows/action-contract.yml"), "utf8");
  for (const filename of [".github/workflows/ocr-review.yml", ".github/pr-admission.json"]) {
    assert.strictEqual(contract.split(`'${filename}'`).length - 1, 2);
  }
  const manifest = JSON.parse(fs.readFileSync(path.join(ROOT, "package.json"), "utf8"));
  assert.ok(manifest.scripts["test:github-actions"].includes("node scripts/github-actions/check-pr-admission.test.js"));
});

async function main() {
  for (const { name, check } of tests) {
    await check();
    console.log(`PASS ${name}`);
  }
  console.log(`\nAll ${tests.length} PR admission tests passed.`);
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
