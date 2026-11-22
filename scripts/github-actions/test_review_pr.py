# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

import json
import os
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = Path("skills/open-code-review/scripts/review_pr.py")
URL = "https://github.com/team/project/pull/42"

GH = """
import json, os, sys
from pathlib import Path
if sys.argv[1:3] == ['config', 'get']:
    print('https')
    sys.exit(0)
assert sys.argv[1:4] == ['pr', 'view', '42'], sys.argv
if '--repo' in sys.argv:
    assert sys.argv[sys.argv.index('--repo') + 1] == 'github.com/team/project'
    assert 'GH_REPO' not in os.environ
else:
    assert Path.cwd() == Path(os.environ['SOURCE'])
    if os.environ.get('EXPECT_GH_REPO'):
        assert os.environ.get('GH_REPO') == 'team/project'
data = json.loads(os.environ['METADATA'])
if Path('review.json').exists():
    assert '--repo' in sys.argv
    latest = os.environ.get('LATEST', '{}')
    if latest == 'unavailable':
        sys.exit(1)
    data.update(json.loads(latest))
print(json.dumps(data))
"""

OCR = """
import json, os, subprocess, sys, time
from pathlib import Path
args = sys.argv[1:]
with open('calls.jsonl', 'a') as log:
    log.write(json.dumps(args) + '\\n')
mode = os.environ.get('MODE')
if mode == 'policy':
    tree = Path(args[args.index('--repo') + 1])
    head = args[args.index('--to') + 1]
    assert not (tree / '.opencodereview').exists()
    assert subprocess.check_output(['git', '-C', str(tree), 'show', head + ':.opencodereview/rule.json']).strip() == b'not a rule document'
    if os.environ.get('PR_TEST_NATIVE_OCR'):
        preview = subprocess.check_output([os.environ['PR_TEST_NATIVE_OCR'], 'review', '--preview', '--format', 'json',
                                           '--repo', str(tree), '--from', args[args.index('--from') + 1], '--to', head])
        assert any(entry['path'] == 'change.go' and entry['will_review'] for entry in json.loads(preview)['files'])
if mode == 'wait':
    while not Path('resume').exists():
        time.sleep(0.02)
if mode == 'dirty':
    (Path(args[args.index('--repo') + 1]) / 'keep').write_text('retain me')
if mode == 'no-result':
    sys.exit(0)
Path(args[args.index('--output') + 1]).write_text(json.dumps({'comments': [], 'analysis_meta': {'declared_completion': 'partial'}}))
sys.exit(5 if mode == 'failed' else 0)
"""


class Standalone(unittest.TestCase):
    script = ROOT / SCRIPT

    def setUp(self):
        build = ROOT / "dist" / "pr-helper-tests"
        build.mkdir(parents=True, exist_ok=True)
        workspace = tempfile.TemporaryDirectory(dir=build)
        self.addCleanup(workspace.cleanup)
        self.root = Path(workspace.name)
        installed = self.root / "installed" / self.script.relative_to(ROOT)
        shutil.copytree(self.script.parent.parent, installed.parent.parent)
        self.installed = installed
        self.source = self.root / "source"
        self.source.mkdir()
        tools = self.root / "bin"
        tools.mkdir()
        for name, program in (("gh", GH), ("ocr", OCR)):
            executable = tools / name
            executable.write_text(f"#!{sys.executable}\n" + program)
            executable.chmod(0o755)
        self.env = dict(os.environ, HOME=str(self.root), PATH=str(tools) + os.pathsep + os.environ['PATH'],
                        SOURCE=str(self.source), XDG_CONFIG_HOME=str(self.root / "config"), GIT_CONFIG_NOSYSTEM="1")
        self.git("init", "-q", "-b", "release")
        self.git("config", "user.name", "Test")
        self.git("config", "user.email", "test@example.com")
        self.git("config", "--global", f"url.{self.source.as_uri()}.insteadOf", "https://github.com/team/project.git")
        self.git("remote", "add", "origin", "https://github.com/team/project.git")
        self.common = self.commit("initial")
        self.git("switch", "-qc", "topic")
        self.head = self.commit("topic")
        self.git("update-ref", "refs/pull/42/head", self.head)
        self.git("switch", "-q", "release")
        self.base = self.commit("release")
        self.metadata = dict(url=URL, baseRefName="release", baseRefOid=self.base, headRefOid=self.head)
        self.env["METADATA"] = json.dumps(self.metadata)
        (self.source / "staged").write_text("staged")
        self.git("add", "staged")
        (self.source / "initial").write_text("unstaged")
        (self.source / "untracked").write_text("untracked")

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.source, env=self.env, text=True).strip()

    def commit(self, name, content=None):
        (self.source / name).write_text(name if content is None else content)
        self.git("add", name)
        self.git("commit", "-qm", name)
        return self.git("rev-parse", "HEAD")

    def start(self, *args, cwd=None, **env):
        child = subprocess.Popen([sys.executable, str(self.installed), *args], cwd=cwd or self.source,
                                 env=dict(self.env, **env), stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.addCleanup(self.stop, child)
        return child

    @staticmethod
    def stop(child):
        if child.poll() is None:
            child.terminate()
            child.communicate(timeout=10)

    def finish(self, child, code=0):
        out, err = child.communicate(timeout=30)
        self.assertEqual(child.returncode, code, err + out)
        path = Path(out.strip())
        self.assertTrue(path.is_absolute())
        return path.parent, json.loads(path.read_text())

    def test_pinned_range_and_checkout(self):
        index = (self.source / ".git/index").read_bytes()
        refs = self.git("show-ref")
        folder, run = self.finish(self.start("42", "--background", "preserve context"))
        self.assertEqual(run["snapshot"], {"target": "release", "base": self.base, "head": self.head})
        self.assertEqual(run["merge_base"], self.common)
        self.assertEqual(run["freshness"], "current")
        self.assertEqual(run["cleanup"], "complete")
        self.assertEqual(json.loads((folder / "calls.jsonl").read_text()), [
            "review", "--repo", str(folder / "worktree"), "--from", self.common, "--to", self.head,
            "--audience", "agent", "--analysis", "--format", "json", "--output", run["result"],
            "--background", "preserve context"])
        self.assertEqual(json.loads(Path(run["result"]).read_text())["analysis_meta"]["declared_completion"], "partial")
        self.assertFalse((folder / "worktree").exists())
        self.assertFalse((folder / "objects.git").exists())
        self.assertEqual((self.source / ".git/index").read_bytes(), index)
        self.assertEqual(self.git("show-ref"), refs)
        self.assertEqual(self.git("symbolic-ref", "--short", "HEAD"), "release")
        self.assertEqual(self.git("status", "--porcelain").splitlines(), ['M initial', 'A  staged', '?? untracked'])

    def test_bare_number_uses_upstream_default_in_fork(self):
        self.git("remote", "rename", "origin", "upstream")
        self.git("remote", "add", "origin", "https://github.com/contributor/project.git")
        _, run = self.finish(self.start("42"))
        self.assertEqual(run["url"], URL)
        self.assertEqual(run["freshness"], "current")

    def test_gh_repo_applies_only_to_initial_number_lookup(self):
        _, run = self.finish(self.start("42", GH_REPO="team/project", EXPECT_GH_REPO="1"))
        self.assertEqual(run["url"], URL)
        self.assertEqual(run["freshness"], "current")

    def test_head_policy_is_evidence_only(self):
        self.git("switch", "-q", "topic")
        policy = self.source / ".opencodereview" / "rule.json"
        policy.parent.mkdir()
        policy.write_text("not a rule document")
        self.git("add", ".opencodereview/rule.json")
        head = self.commit("change.go", "package example\n")
        self.git("update-ref", "refs/pull/42/head", head)
        self.git("switch", "-q", "release")
        _, run = self.finish(self.start("42", MODE="policy", METADATA=json.dumps(dict(self.metadata, headRefOid=head))))
        self.assertEqual(run["cleanup"], "complete")

    def test_configured_mcp_stops_before_ocr(self):
        config = self.root / ".opencodereview" / "config.json"
        config.parent.mkdir()
        for server in ({"command": "not-installed"}, {"type": "remote", "url": "https://example.invalid"}):
            with self.subTest(server=server):
                data = {"MCP_SERVERS": {"example": server}}
                config.write_text(json.dumps(data))
                folder, run = self.finish(self.start("42"), code=1)
                self.assertEqual(run["failed_step"], "policy")
                self.assertIn("MCP", run["error"])
                self.assertFalse((folder / "calls.jsonl").exists())
                self.assertEqual(json.loads(config.read_text()), data)

    def test_changed_git_policy_requires_independent_review(self):
        self.git("switch", "-q", "topic")
        for name in (".gitignore", "nested/.gitattributes", ".gitmodules"):
            with self.subTest(name=name):
                self.git("reset", "--hard", self.head)
                (self.source / name).parent.mkdir(parents=True, exist_ok=True)
                content = "# Policy fixture\n"
                if name == ".gitmodules":
                    self.git("update-index", "--add", "--cacheinfo", f"160000,{self.common},vendor/lib")
                    content = '[submodule "lib"]\n\tpath = vendor/lib\n'
                head = self.commit(name, content)
                self.git("update-ref", "refs/pull/42/head", head)
                folder, run = self.finish(self.start("42", METADATA=json.dumps(dict(self.metadata, headRefOid=head))), code=1)
                self.assertEqual(run["failed_step"], "policy")
                self.assertIn(name, run["error"])
                self.assertFalse((folder / "calls.jsonl").exists())

    def test_explicit_identity_and_freshness(self):
        for delta, expected in (({"baseRefName": "main"}, "stale"), ({"baseRefOid": self.common}, "stale"),
                                ({"headRefOid": self.base}, "stale"), (None, "unverified")):
            with self.subTest(delta=delta):
                args = [URL] if delta else ["42", "--repo", "team/project"]
                folder, run = self.finish(self.start(*args, cwd=self.root, LATEST=json.dumps(delta) if delta else 'unavailable'))
                self.assertEqual(run["freshness"], expected)
                self.assertEqual(len((folder / "calls.jsonl").read_text().splitlines()), 1)
                self.assertTrue(Path(run["result"]).is_file())

    def test_conflicting_input_and_moving_head(self):
        invalid = self.start(URL, "--repo", "other/project")
        out, err = invalid.communicate(timeout=10)
        self.assertEqual(invalid.returncode, 2)
        self.assertIn('conflicts', err)
        self.assertFalse(out)
        self.git("update-ref", "refs/pull/42/head", self.base)
        folder, run = self.finish(self.start("42"), code=1)
        self.assertEqual(run["failed_step"], "freeze")
        self.assertEqual(run["cleanup"], "complete")
        self.assertFalse((folder / "calls.jsonl").exists())

    def test_no_unique_merge_base(self):
        tree = self.git("rev-parse", "HEAD^{tree}")
        first = self.git("commit-tree", tree, "-p", self.head, "-p", self.base, "-m", "first")
        second = self.git("commit-tree", tree, "-p", self.base, "-p", self.head, "-m", "second")
        self.git("update-ref", "refs/pull/42/head", second)
        _, run = self.finish(self.start("42", METADATA=json.dumps(dict(self.metadata, baseRefOid=first, headRefOid=second))), code=1)
        self.assertEqual(run["failed_step"], "freeze")
        self.assertIn("merge-base", run["error"])
        self.assertIsNone(run["ocr_exit"])

    def test_cleanup_on_failure(self):
        for mode, status, step in (("dirty", "retained", "cleanup"), ("failed", "complete", "review"),
                                   ("no-result", "complete", "review")):
            with self.subTest(mode=mode):
                folder, run = self.finish(self.start('42', MODE=mode), code=1)
                self.assertEqual((run['cleanup'], run['failed_step']), (status, step))
                self.assertEqual((folder / 'worktree/keep').is_file(), mode == 'dirty')
                self.assertEqual(run['ocr_exit'], 5 if mode == 'failed' else 0)

    def test_concurrent_interrupt(self):
        first, second = self.start('42', MODE='wait'), self.start('42', MODE='wait')
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            folders = [call.parent for call in self.root.glob('.cache/open-code-review/pr/*/calls.jsonl')]
            if len(folders) == 2:
                break
            time.sleep(0.05)
        self.assertEqual(len(folders), 2)
        first.send_signal(signal.SIGTERM)
        stopped, run = self.finish(first, code=130)
        self.assertEqual((run['failed_step'], run['cleanup']), ('review', 'complete'))
        remaining, = set(folders) - {stopped}
        self.assertTrue((remaining / 'worktree/topic').is_file())
        (remaining / 'resume').touch()
        self.finish(second)


class Plugin(Standalone):
    script = ROOT / 'plugins/open-code-review' / SCRIPT

    def test_helper_matches_standalone(self):
        self.assertEqual(self.script.read_bytes(), Standalone.script.read_bytes())


if __name__ == '__main__':
    unittest.main()
