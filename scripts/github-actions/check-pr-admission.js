// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const POLICY = require("../../.github/pr-admission.json");
const COMMENT_MARKER = "<!-- ocr-pr-open-cap -->";

function validatePolicy(policy) {
  if (!Number.isSafeInteger(policy.maxOpen) || policy.maxOpen < 1 ||
      !Array.isArray(policy.exemptBots) || policy.exemptBots.some((bot) =>
        !Number.isSafeInteger(bot.id) || bot.id < 1 || typeof bot.login !== "string" || !bot.login)) {
    throw new Error("Invalid PR admission policy.");
  }
}

function validatePullRequest(pullRequest) {
  if (!pullRequest || !Number.isSafeInteger(pullRequest.number) || pullRequest.number < 1 ||
      !Number.isSafeInteger(pullRequest.user?.id) || pullRequest.user.id < 1 ||
      !["open", "closed"].includes(pullRequest.state) ||
      typeof pullRequest.created_at !== "string" || !Number.isFinite(Date.parse(pullRequest.created_at))) {
    throw new Error("Invalid PR metadata. Re-run PR admission.");
  }
}

async function readOpening(github, repo, pullRequest) {
  const events = await github.paginate(github.rest.issues.listEvents, {
    ...repo, issue_number: pullRequest.number, per_page: 100,
  });
  const reopenings = events.filter((event) => event.event === "reopened");
  const ids = new Set();
  for (const event of reopenings) {
    if (!Number.isSafeInteger(event.id) || event.id < 1 || ids.has(event.id) ||
        typeof event.created_at !== "string" || !Number.isFinite(Date.parse(event.created_at)) ||
        Date.parse(event.created_at) < Date.parse(pullRequest.created_at)) {
      throw new Error("Invalid PR reopening history. Re-run PR admission.");
    }
    ids.add(event.id);
  }
  reopenings.sort((left, right) => Date.parse(right.created_at) - Date.parse(left.created_at) || right.id - left.id);
  const timestamp = Date.parse(reopenings[0]?.created_at || pullRequest.created_at);
  return {
    id: reopenings[0]?.id || 0,
    timestamp,
    ambiguous: reopenings.filter((event) => Date.parse(event.created_at) === timestamp).length > 1,
  };
}

function isCurrentOpening(context, opening) {
  if (context.payload.action === "opened") return opening.id === 0;
  const updatedAt = context.payload.pull_request.updated_at;
  if (typeof updatedAt !== "string" || !Number.isFinite(Date.parse(updatedAt)) || !opening.id) {
    throw new Error("Missing PR reopening metadata. Re-run PR admission.");
  }
  if (Date.parse(updatedAt) < opening.timestamp) return false;
  if (Date.parse(updatedAt) !== opening.timestamp || opening.ambiguous) {
    throw new Error("Cannot identify the PR opening cycle. Retry, or close and reopen the PR in a later second.");
  }
  return true;
}

async function readOpenPullRequests(github, repo, current, limit) {
  let previous;
  for (let attempt = 0; attempt < 3; attempt += 1) {
    const pulls = await readOpenSnapshot(github, repo, current);
    const authored = pulls.filter((pullRequest) => pullRequest.user.id === current.user.id)
      .map((pullRequest) => ({ ...pullRequest }));
    for (const pullRequest of authored) {
      if (authored.length > limit || pullRequest.number === current.number) {
        pullRequest.opening = await readOpening(github, repo, pullRequest);
      }
    }
    const signature = JSON.stringify([
      pulls.map((pullRequest) => [pullRequest.number, pullRequest.user.id, pullRequest.created_at]),
      authored.map((pullRequest) => [pullRequest.number, pullRequest.opening]),
    ]);
    if (signature === previous) {
      return authored.length <= limit ? authored : authored.sort((left, right) =>
        left.opening.timestamp - right.opening.timestamp ||
        Number(Boolean(left.opening.id)) - Number(Boolean(right.opening.id)) || left.number - right.number);
    }
    previous = signature;
  }
  throw new Error("Open PR list did not stabilize. Re-run PR admission.");
}

async function readOpenSnapshot(github, repo, current) {
  const pulls = await github.paginate(github.rest.pulls.list, {
    ...repo, state: "open", sort: "created", direction: "asc", per_page: 100,
  });
  const numbers = new Set();
  for (const pullRequest of pulls) {
    validatePullRequest(pullRequest);
    if (pullRequest.state !== "open" || numbers.has(pullRequest.number)) {
      throw new Error("PR list changed during pagination. Re-run PR admission.");
    }
    numbers.add(pullRequest.number);
  }
  const target = pulls.find((pullRequest) => pullRequest.number === current.number);
  if (!target || target.user.id !== current.user.id || target.created_at !== current.created_at) {
    throw new Error("PR is missing or changed in the open PR list. Re-run PR admission.");
  }
  return pulls.sort((left, right) => left.number - right.number);
}

async function explainClosure(github, repo, number, count, limit) {
  const body = `${COMMENT_MARKER}\n` +
    `This PR exceeds our limit of **${limit} open pull requests per contributor** ` +
    `and is being closed automatically. You currently have ${count} open PRs, including drafts.\n\n` +
    "This limit protects review capacity; it is not a judgment about the quality of this patch " +
    "or whether AI was used. Please get existing PRs merged, or close those you no longer need, " +
    "then reopen this PR when a slot is available. Reopening counts toward the same limit.\n\n" +
    "Discussion remains open. This policy does not lock PRs or block accounts.\n\n" +
    `Policy: https://github.com/${repo.owner}/${repo.repo}/blob/HEAD/.github/PR_MODERATION.md`;
  const comments = await github.paginate(github.rest.issues.listComments, {
    ...repo, issue_number: number, per_page: 100,
  });
  const existing = comments.find((comment) => comment.user?.type === "Bot" &&
    comment.user.login === "github-actions[bot]" && comment.body?.startsWith(`${COMMENT_MARKER}\n`));
  if (!existing) {
    await github.rest.issues.createComment({ ...repo, issue_number: number, body, request: { retries: 0 } });
  } else if (existing.body !== body) {
    await github.rest.issues.updateComment({ ...repo, comment_id: existing.id, body });
  }
}

async function run({ github, context, core, policy = POLICY }) {
  core.setOutput("allowed", "false");
  validatePolicy(policy);
  if (context.eventName !== "pull_request_target" ||
      !["opened", "reopened"].includes(context.payload.action) ||
      !Number.isSafeInteger(context.issue.number) || context.issue.number < 1) {
    throw new Error("PR admission requires an opened or reopened pull_request_target event.");
  }
  const repo = context.repo;
  const number = context.issue.number;
  const { data: current } = await github.rest.pulls.get({ ...repo, pull_number: number });
  validatePullRequest(current);
  if (current.number !== number || current.user.id !== context.payload.pull_request?.user?.id) {
    throw new Error("PR does not match the triggering author. Re-run PR admission.");
  }
  if (current.state !== "open") {
    core.info(`PR #${number} is no longer open; skipping admission and review.`);
    return false;
  }
  if (!isCurrentOpening(context, await readOpening(github, repo, current))) {
    core.info(`PR #${number} has a newer opening cycle; skipping the stale event and review.`);
    return false;
  }
  if (current.user.type === "Bot" && policy.exemptBots.some((bot) =>
    bot.id === current.user.id && bot.login === current.user.login)) {
    core.setOutput("allowed", "true");
    core.info(`PR #${number} belongs to an explicitly exempt automation account.`);
    return true;
  }
  const pulls = await readOpenPullRequests(github, repo, current, policy.maxOpen);
  if (!isCurrentOpening(context, pulls.find((pullRequest) => pullRequest.number === number).opening)) {
    core.info(`PR #${number} was reopened during admission; skipping the stale event and review.`);
    return false;
  }
  const exceedsLimit = pulls.length > policy.maxOpen &&
    pulls.findIndex((pullRequest) => pullRequest.number === number) >= policy.maxOpen;
  if (!exceedsLimit) {
    core.setOutput("allowed", "true");
    core.info(`PR #${number} passes the open-PR limit (${pulls.length} currently open).`);
    return true;
  }

  const { data: latest } = await github.rest.pulls.get({ ...repo, pull_number: number });
  validatePullRequest(latest);
  if (latest.number !== number || latest.user.id !== current.user.id) {
    throw new Error("PR changed before moderation. Re-run PR admission.");
  }
  if (latest.state !== "open") {
    core.info(`PR #${number} is no longer open; skipping closure and review.`);
    return false;
  }
  if (!isCurrentOpening(context, await readOpening(github, repo, latest))) {
    core.info(`PR #${number} was reopened before moderation; skipping closure and review.`);
    return false;
  }
  await explainClosure(github, repo, number, pulls.length, policy.maxOpen);
  await github.rest.pulls.update({ ...repo, pull_number: number, state: "closed" });
  core.info(`Closed PR #${number}: ${pulls.length} open PRs exceeds the limit of ${policy.maxOpen}.`);
  return false;
}

module.exports = { run };
