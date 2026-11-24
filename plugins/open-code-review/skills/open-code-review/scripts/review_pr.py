# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

import argparse
import json
import os
import re
import shutil
import signal
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path


REPOSITORY = r"[A-Za-z0-9-]+/[A-Za-z0-9_.-]+"
PR_URL = re.compile(rf"https://github\.com/({REPOSITORY})/pull/([1-9][0-9]*)/?")
FIELDS = "url,baseRefName,baseRefOid,headRefOid"
CACHE = (Path.home() / ".cache" / "open-code-review" / "pr").resolve()


def target(value, repo):
    if repo and not re.fullmatch(REPOSITORY, repo):
        raise ValueError("--repo expects OWNER/REPO")
    match = PR_URL.fullmatch(value)
    if match:
        if repo and match[1].lower() != repo.lower():
            raise ValueError("PR URL conflicts with --repo")
        return match[2], match[1]
    if not re.fullmatch(r"[1-9][0-9]*", value):
        raise ValueError("expected a positive PR number or a github.com PR URL")
    return value, repo


def command(*args, cwd=None, env=None):
    result = subprocess.run(args, cwd=cwd, env=env, text=True, stdout=subprocess.PIPE)
    if result.returncode:
        raise ValueError(f"{args[0]} {args[1]} exited {result.returncode} (see stderr)")
    return result.stdout.strip()


def lookup(number, repo, cwd, env):
    scope = ["--repo", f"github.com/{repo}"] if repo else []
    data = json.loads(command("gh", "pr", "view", number, *scope, "--json", FIELDS, cwd=cwd, env=env))
    if not isinstance(data, dict):
        raise ValueError("GitHub did not return a PR object")
    match = PR_URL.fullmatch(str(data.get("url", "")))
    if not match or match[2] != number or (repo and match[1].lower() != repo.lower()):
        raise ValueError("GitHub returned a different PR identity")
    if not isinstance(data.get("baseRefName"), str) or not data["baseRefName"] or not all(
            re.fullmatch(r"[0-9a-f]{40}", str(data.get(key, ""))) for key in ("baseRefOid", "headRefOid")):
        raise ValueError("GitHub did not return a complete target and commit pair")
    return match[1], {"target": data["baseRefName"], "base": data["baseRefOid"], "head": data["headRefOid"]}


def local_remote(repo, env):
    result = subprocess.run(["git", "config", "--local", "--get-regexp", r"^remote\..*\.url$"],
                            env=env, capture_output=True, text=True)
    pattern = rf"(?:git@github\.com:|(?:https://|ssh://git@)github\.com/){re.escape(repo)}(?:\.git)?/?"
    for line in result.stdout.splitlines():
        _, _, url = line.partition(" ")
        if re.fullmatch(pattern, url, flags=re.IGNORECASE):
            return url
    return None


def freeze(folder, repo, number, revisions, env, offline):
    store, tree = folder / "objects.git", folder / "worktree"
    remote = local_remote(repo, env)
    command("git", "init", "--bare", "--template=", store, cwd=folder, env=offline)
    if not remote:
        protocol = command("gh", "config", "get", "git_protocol", "--host", "github.com", env=env)
        remote = f"git@github.com:{repo}.git" if protocol == "ssh" else f"https://github.com/{repo}.git"
    command("git", f"--git-dir={store}", "-c", f"core.hooksPath={os.devnull}", "-c", "maintenance.auto=false",
            "-c", "credential.https://github.com.helper=", "-c", "credential.https://github.com.helper=!gh auth git-credential",
            "fetch", "--atomic", "--no-tags", "--no-write-fetch-head", "--no-recurse-submodules", remote,
            f"{revisions['base']}:refs/pr/base", f"refs/pull/{number}/head:refs/pr/head", cwd=store, env=env)
    for name in ("base", "head"):
        if command("git", "rev-parse", f"refs/pr/{name}^{{commit}}", cwd=store, env=offline) != revisions[name]:
            raise ValueError(f"fetched {name} differs from GitHub; no review started")
    bases = command("git", "merge-base", "--all", revisions["base"], revisions["head"], cwd=store, env=offline).splitlines()
    if len(bases) != 1:
        raise ValueError("no unique merge-base")
    command("git", "worktree", "add", "--detach", tree, revisions["head"], cwd=store, env=offline)
    return bases[0]


def verify_tree_owner(folder, env):
    tree = folder / "worktree"
    common = command("git", "rev-parse", "--path-format=absolute", "--git-common-dir", cwd=tree, env=env)
    if Path(common) != folder / "objects.git":
        raise ValueError(f"worktree does not belong to this snapshot; retained {tree}")


def verify_tree(folder, head, env):
    tree = folder / "worktree"
    verify_tree_owner(folder, env)
    if (command("git", "rev-parse", "HEAD", cwd=tree, env=env) != head or
            command("git", "rev-parse", "--abbrev-ref", "HEAD", cwd=tree, env=env) != "HEAD" or
            command("git", "status", "--porcelain", "--ignored", "--untracked-files=all", cwd=tree, env=env)):
        raise ValueError(f"worktree changed; retained {tree}")


def release(folder, env):
    store, tree = folder / "objects.git", folder / "worktree"
    if store.is_symlink() or tree.is_symlink():
        raise ValueError("refusing to clean up a symlinked snapshot")
    if not store.exists():
        if tree.exists():
            raise ValueError(f"snapshot object store is missing; retained {tree}")
        return
    git = ("git", f"--git-dir={store}", "-c", f"core.hooksPath={os.devnull}")
    entries = command(*git, "worktree", "list", "--porcelain", "-z", env=env).split("\0")
    worktrees = {Path(entry[9:]) for entry in entries if entry.startswith("worktree ")}
    if worktrees - {store, tree}:
        raise ValueError("object store has another worktree; retained snapshot")
    if tree.exists():
        verify_tree_owner(folder, env)
        command(*git, "worktree", "remove", "--force", "--", tree, env=env)
    elif tree in worktrees:
        print("The recorded worktree is missing. Any moved files are left in place but will lose their Git metadata.",
              file=sys.stderr)
    shutil.rmtree(store)


def prepare_review_policy(tree, base, head, env):
    config = Path.home() / ".opencodereview" / "config.json"
    try:
        settings = json.loads(config.read_bytes(), object_pairs_hook=tuple)
    except FileNotFoundError:
        settings = ()
    if not isinstance(settings, tuple):
        raise ValueError("expected an OCR configuration object")
    if any(key.casefold() == "mcp_servers" and value not in (None, ()) for key, value in settings):
        raise ValueError("PR mode requires an OCR configuration without MCP servers; no review started")
    changed = command("git", "diff-tree", "-r", "--no-commit-id", "--name-only", "--no-renames", base, head, "--",
                      ":(top,icase).gitignore", ":(top,icase).gitmodules", ":(top,glob,icase)**/.gitattributes",
                      cwd=tree, env=env)
    if changed:
        raise ValueError(f"PR changes Git filtering or diff policy; independent review required: {changed}")
    command("git", "sparse-checkout", "set", "--no-cone", "--", "/*", "!/.opencodereview", cwd=tree, env=env)
    if os.path.lexists(tree / ".opencodereview"):
        raise ValueError("project review policy is still present in the worktree; no review started")


def review(folder, base, head, background, env):
    args = ["ocr", "review", "--repo", str(folder / "worktree"), "--from", base, "--to", head,
            "--audience", "agent", "--analysis", "--format", "json", "--output", str(folder / "review.json")]
    if background:
        args.extend(["--background", background])
    with subprocess.Popen(args, cwd=folder, env=env, stdout=sys.stderr,
                          start_new_session=os.name == "posix") as child:
        try:
            return child.wait()
        except KeyboardInterrupt:
            if os.name == "posix":
                try:
                    os.killpg(child.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            else:
                child.terminate()
            child.wait()
            raise


def save(manifest, summary):
    pending = manifest.with_suffix(".pending")
    pending.write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
    pending.replace(manifest)


def environments():
    env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_") and key != "GH_REPO"}
    env["PATH"] = os.pathsep.join(part for part in os.get_exec_path() if os.path.isabs(part))
    env.update(GH_PROMPT_DISABLED="1", GIT_TERMINAL_PROMPT="0")
    return env, dict(env, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1", GIT_ALLOW_PROTOCOL="", OCR_NO_UPDATE="1")


def cleanup(manifest, offline):
    manifest = manifest.absolute()
    folder = manifest.parent
    if (manifest.name != "run.json" or folder.parent != CACHE or manifest != manifest.resolve() or
            not re.fullmatch(r"pr-[1-9][0-9]*-[\w-]+", folder.name)):
        raise ValueError("expected a run.json in the PR snapshot cache")
    summary = json.loads(manifest.read_text(encoding="utf-8"))
    if (not isinstance(summary, dict) or summary.get("state") not in ("finished", "failed") or
            summary.get("worktree") != str(folder / "worktree")):
        raise ValueError("expected a completed run belonging to this directory")
    if summary.get("cleanup") != "complete":
        release(folder, offline)
        summary["cleanup"] = "complete"
        save(manifest, summary)
    print(manifest)


def run(number, repo, background, env, offline):
    CACHE.mkdir(parents=True, exist_ok=True)
    folder = Path(tempfile.mkdtemp(prefix=f"pr-{number}-", dir=CACHE))
    manifest = folder / "run.json"
    result = folder / "review.json"
    summary = {"number": int(number), "state": "running", "freshness": "not_checked", "ocr_exit": None, "result": None,
               "worktree": str(folder / "worktree"), "cleanup": "retained"}
    save(manifest, summary)
    print(f"PR review artifacts: {folder}", file=sys.stderr, flush=True)
    step = "lookup"
    code = 1
    try:
        initial_env = {**os.environ, **env} if not repo else env
        repo, snapshot = lookup(number, repo, None, initial_env)
        summary.update(url=f"https://github.com/{repo}/pull/{number}", snapshot=snapshot)
        step = "freeze"
        base = freeze(folder, repo, number, snapshot, env, offline)
        summary["merge_base"] = base
        step = "policy"
        prepare_review_policy(folder / "worktree", base, snapshot["head"], offline)
        summary["policy"] = {"project_rules": "omitted", "mcp_config": "checked_empty"}
        save(manifest, summary)
        step = "review"
        summary["ocr_exit"] = review(folder, base, snapshot["head"], background, offline)
        if summary["ocr_exit"] != 0 or not result.is_file() or not result.stat().st_size:
            raise ValueError(f"OCR exited {summary['ocr_exit']}; no successful review")
        step = "verify_snapshot"
        verify_tree(folder, snapshot["head"], offline)
        code = 0
    except (OSError, ValueError, KeyboardInterrupt) as error:
        summary.update(failed_step=step, error=str(error) or "interrupted")
        if isinstance(error, KeyboardInterrupt):
            code = 130
    finally:
        if result.is_file():
            summary["result"] = str(result)
        if summary["result"]:
            try:
                _, latest = lookup(number, repo, folder, env)
                summary.update(latest=latest, freshness="current" if latest == summary["snapshot"] else "stale")
            except (OSError, ValueError, KeyboardInterrupt) as error:
                summary.update(freshness="unverified", freshness_error=str(error) or "interrupted")
                if isinstance(error, KeyboardInterrupt):
                    summary.setdefault("failed_step", "freshness")
                    code = 130
            summary["checked_at"] = datetime.now(timezone.utc).isoformat()
        summary["state"] = "finished" if code == 0 else "failed"
        save(manifest, summary)
        print(manifest)
    return code


def interrupt(signum, frame):
    raise KeyboardInterrupt


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Review one pinned GitHub pull request using native OCR.")
    operation = parser.add_mutually_exclusive_group(required=True)
    operation.add_argument("pr", nargs="?", help="PR number in the checkout's default gh repository, or a full GitHub PR URL")
    operation.add_argument("--cleanup", type=Path, metavar="RUN_JSON",
                           help="Discard this run's local snapshot, including edits and local-only commits")
    parser.add_argument("--repo", help="GitHub repository (OWNER/REPO)")
    parser.add_argument("--background", help="Business context for the review")
    args = parser.parse_args()
    env, offline = environments()
    if args.cleanup:
        if args.repo or args.background:
            parser.error("--cleanup does not accept review options")
        try:
            cleanup(args.cleanup, offline)
        except (OSError, ValueError) as error:
            sys.exit(f"Cleanup refused or incomplete: {error}")
        sys.exit(0)
    try:
        number, repo = target(args.pr, args.repo)
    except ValueError as error:
        parser.error(str(error))
    signal.signal(signal.SIGTERM, interrupt)
    sys.exit(run(number, repo, args.background, env, offline))
