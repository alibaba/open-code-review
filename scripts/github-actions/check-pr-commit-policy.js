// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const QUERY = `
  query($owner: String!, $repo: String!, $number: Int!, $cursor: String) {
    repository(owner: $owner, name: $repo) {
      pullRequest(number: $number) {
        title
        baseRefOid
        headRefOid
        commits(first: 100, after: $cursor) {
          totalCount
          nodes { commit { oid message } }
          pageInfo { hasNextPage endCursor }
        }
      }
    }
  }
`;

function findSkipDirectives(message) {
  const directives = new Set(
    (message.match(/\[(?:skip ci|ci skip|no ci|skip actions|actions skip)\]/gi) || [])
      .map((directive) => directive.toLowerCase())
  );
  if (/^[\t ]*skip-checks:[\t ]*true[\t ]*\r?$/im.test(message)) {
    directives.add("skip-checks: true");
  }
  return [...directives];
}

function collectCommits(commits, nodes) {
  for (const node of nodes) {
    const commit = node && node.commit;
    if (!commit || !/^[0-9a-f]{40}$/.test(commit.oid) ||
        typeof commit.message !== "string" || commits.has(commit.oid)) {
      throw new Error("Invalid or duplicate PR commit data. Re-run PR Commit Policy.");
    }
    commits.set(commit.oid, commit);
  }
}

async function readComparison(github, variables, snapshot) {
  const commits = new Map();
  for (let page = 1; commits.size < snapshot.count; page += 1) {
    const { data } = await github.rest.repos.compareCommitsWithBasehead({
      owner: variables.owner,
      repo: variables.repo,
      basehead: `${snapshot.base}...${snapshot.head}`,
      per_page: 100,
      page,
    });
    if (!data || data.total_commits !== snapshot.count ||
        !Array.isArray(data.commits) || data.commits.length === 0) {
      throw new Error("Unable to read the complete PR commit comparison. Re-run PR Commit Policy.");
    }
    collectCommits(commits, data.commits.map((entry) => ({
      commit: entry && { oid: entry.sha, message: entry.commit?.message },
    })));
  }
  if (commits.size !== snapshot.count || !commits.has(snapshot.head)) {
    throw new Error("PR commit comparison does not match its head and count. Re-run PR Commit Policy.");
  }
  const response = await github.graphql(QUERY, { ...variables, cursor: null });
  const current = response.repository?.pullRequest;
  if (!current || current.title !== snapshot.title || current.baseRefOid !== snapshot.base ||
      current.headRefOid !== snapshot.head || current.commits?.totalCount !== snapshot.count) {
    throw new Error("PR changed during validation. Check the run for the latest PR update.");
  }
  return { title: snapshot.title, head: snapshot.head, commits: [...commits.values()] };
}

async function readPullRequest(github, variables) {
  let snapshot;
  let cursor = null;
  const cursors = new Set();
  const commits = new Map();

  do {
    const response = await github.graphql(QUERY, { ...variables, cursor });
    const pullRequest = response.repository && response.repository.pullRequest;
    if (!pullRequest || typeof pullRequest.title !== "string" ||
        !/^[0-9a-f]{40}$/.test(pullRequest.baseRefOid) ||
        !/^[0-9a-f]{40}$/.test(pullRequest.headRefOid)) {
      throw new Error("Unable to read PR metadata. Re-run PR Commit Policy.");
    }
    const connection = pullRequest.commits;
    if (!connection || !Number.isInteger(connection.totalCount) ||
        connection.totalCount < 1 || !Array.isArray(connection.nodes) ||
        !connection.pageInfo || typeof connection.pageInfo.hasNextPage !== "boolean") {
      throw new Error("Unable to read the complete PR commit list. Re-run PR Commit Policy.");
    }

    if (!snapshot) {
      snapshot = {
        title: pullRequest.title,
        base: pullRequest.baseRefOid,
        head: pullRequest.headRefOid,
        count: connection.totalCount,
      };
    } else if (snapshot.title !== pullRequest.title ||
               snapshot.base !== pullRequest.baseRefOid ||
               snapshot.head !== pullRequest.headRefOid ||
               snapshot.count !== connection.totalCount) {
      throw new Error("PR changed during validation. Check the run for the latest PR update.");
    }

    collectCommits(commits, connection.nodes);

    if (!connection.pageInfo.hasNextPage) break;
    cursor = connection.pageInfo.endCursor;
    if (typeof cursor !== "string" || !cursor || cursors.has(cursor) ||
        connection.nodes.length === 0 || commits.size >= snapshot.count) {
      throw new Error("Incomplete PR commit pagination. Re-run PR Commit Policy.");
    }
    cursors.add(cursor);
  } while (true);

  if (snapshot.count > 250 && commits.size < snapshot.count) {
    return readComparison(github, variables, snapshot);
  }
  if (commits.size !== snapshot.count || !commits.has(snapshot.head)) {
    throw new Error("PR commit list does not match its head and count. Re-run PR Commit Policy.");
  }
  return { title: snapshot.title, head: snapshot.head, commits: [...commits.values()] };
}

async function run({ github, context, core }) {
  if (context.eventName !== "pull_request_target" ||
      !Number.isInteger(context.issue.number) || context.issue.number < 1) {
    throw new Error("PR Commit Policy requires a pull_request_target event.");
  }
  const pullRequest = await readPullRequest(github, {
    owner: context.repo.owner,
    repo: context.repo.repo,
    number: context.issue.number,
  });
  if (pullRequest.head !== context.payload.pull_request?.head?.sha) {
    throw new Error("PR head does not match the triggering event. Re-run PR Commit Policy for the latest PR update.");
  }
  if (pullRequest.title !== context.payload.pull_request.title) {
    throw new Error("PR title does not match the triggering event. Re-run PR Commit Policy for the latest PR update.");
  }
  const violations = [];
  const titleDirectives = findSkipDirectives(pullRequest.title);
  if (titleDirectives.length) {
    violations.push(`PR title contains ${titleDirectives.join(", ")}.`);
  }
  for (const commit of pullRequest.commits) {
    const directives = findSkipDirectives(commit.message);
    if (directives.length) {
      violations.push(`Commit ${commit.oid} contains ${directives.join(", ")}.`);
    }
  }

  if (violations.length) {
    for (const violation of violations.slice(0, 20)) core.error(violation);
    core.setFailed(
      `Found CI skip directives in ${violations.length} PR title/commit message(s). ` +
      "Remove them from the title and rewrite affected commit messages; " +
      "adding a clean commit does not remove earlier violations. " +
      "See CONTRIBUTING.md#commit-messages. At most 20 violations are shown."
    );
  } else {
    core.info(`PR title and all ${pullRequest.commits.length} commit messages contain no CI skip directives.`);
  }
}

module.exports = { findSkipDirectives, readPullRequest, run };
