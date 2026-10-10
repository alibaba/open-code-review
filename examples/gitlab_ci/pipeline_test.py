#!/usr/bin/env python3

# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

"""Execute the example's real shell blocks with offline CLI/publisher doubles."""

import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import tempfile
import unittest


PIPELINE = Path(__file__).with_name(".gitlab-ci.yml").read_text(encoding="utf-8")


def script_blocks():
    lines = PIPELINE.splitlines()
    start = lines.index("  script:")
    end = lines.index("  artifacts:")
    blocks = []
    for line in lines[start + 1:end]:
        if line == "    - |":
            blocks.append([])
        elif line.startswith("      ") and blocks:
            blocks[-1].append(line[6:])
        elif line.strip() and not line.lstrip().startswith("#"):
            raise AssertionError("Unsupported pipeline script shape: " + line)
    return ["\n".join(block) for block in blocks]


FAKE = r'''
import json
import os
from pathlib import Path
import sys

tool, args = sys.argv[1], sys.argv[2:]
record = {"tool": tool, "args": args}
if tool == "python3":
    record["severity"] = os.environ.get("OCR_FAIL_ON_SEVERITY", "")
with open(os.environ["CALL_LOG"], "a", encoding="utf-8") as out:
    out.write(json.dumps(record) + "\n")
if tool == "git":
    if args[0] == "merge-base":
        print("a" * 40)
    elif args[0] == "rev-parse":
        print("b" * 40)
    else:
        raise AssertionError("Unexpected git operation")
elif tool == "python3":
    Path(".ocr/ocr-stats.env").write_text("OCR_COMMENTS_TOTAL=1\n", encoding="utf-8")
    sys.exit(int(os.environ.get("FAKE_POST_EXIT", "0")))
elif tool == "ocr":
    if args[0] == "review":
        sys.stdout.write(os.environ.get("FAKE_REVIEW_JSON", '{"comments":[],"warnings":[]}'))
        sys.stderr.write("review diagnostic\n")
        sys.exit(int(os.environ.get("FAKE_REVIEW_EXIT", "0")))
    elif args[0] == "gate":
        if "--help" in args:
            sys.exit(int(os.environ.get("FAKE_GATE_HELP_EXIT", "0")))
        code = int(os.environ.get("FAKE_GATE_EXIT", "0"))
        print(json.dumps({"schema_version": "ocr.gate/v1", "status": "inconclusive" if code else "pass"}))
        sys.stderr.write("gate diagnostic\n")
        sys.exit(code)
    elif args[0] == "version":
        print("open-code-review 1.12.7 fixture")
'''


class PipelineTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="ocr-gitlab-contract ")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        fake = self.root / "fake.py"
        fake.write_text(FAKE, encoding="utf-8", newline="\n")
        for tool in ("git", "ocr", "npm", "python3"):
            wrapper = self.bin / tool
            wrapper.write_text(
                "#!/bin/bash\nexec %s %s %s \"$@\"\n" % (
                    shlex.quote(Path(sys.executable).as_posix()), shlex.quote(fake.as_posix()), tool),
                encoding="utf-8", newline="\n")
            wrapper.chmod(0o755)
        self.env = dict(os.environ,
                        CALL_LOG=str(self.root / "calls.jsonl"),
                        PYTHONUTF8="1",
                        OCR_GATE="false", OCR_FAIL_ON_SEVERITY="",
                        OCR_LLM_URL="https://model.invalid", OCR_LLM_AUTH_TOKEN="fixture-token",
                        OCR_LLM_MODEL="fixture-model", OCR_VERSION="fixture",
                        CI_COMMIT_SHA="feature", CI_MERGE_REQUEST_TARGET_BRANCH_NAME="main",
                        CI_MERGE_REQUEST_SOURCE_BRANCH_NAME="feature")

    def run_script(self, overrides=None, blocks=None):
        env = dict(self.env, **(overrides or {}))
        # Git Bash inserts its own git on startup. Install the doubles first
        # inside the shell too; no contract test may contact a real remote.
        shell_bin = re.sub(r"^([A-Za-z]):", lambda m: "/" + m[1].lower(), self.bin.as_posix())
        script = "export PATH=%s:\"$PATH\"\n%s" % (
            shlex.quote(shell_bin), "\n".join(script_blocks() if blocks is None else blocks))
        return subprocess.run(
            [env.get("OCR_TEST_BASH", "/bin/bash"), "--noprofile", "--norc", "-eo", "pipefail", "-c", script],
            cwd=self.root, env=env, capture_output=True, text=True, encoding="utf-8", timeout=60)

    def calls(self):
        path = self.root / "calls.jsonl"
        return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines()] if path.exists() else []

    def gate_calls(self):
        return [c for c in self.calls() if c["tool"] == "ocr" and c["args"][0] == "gate" and "--help" not in c["args"]]

    def test_disabled_preserves_legacy_severity_and_older_cli(self):
        result = self.run_script({"OCR_FAIL_ON_SEVERITY": "critical", "FAKE_GATE_HELP_EXIT": "1"})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any(c["args"][:1] == ["gate"] for c in self.calls()))
        post = next(c for c in self.calls() if c["tool"] == "python3")
        self.assertEqual(post["severity"], "critical")
        self.assertNotIn("--require-publication", post["args"])

    def test_shared_gate_receives_frozen_revisions_and_owns_severity(self):
        result = self.run_script({"OCR_GATE": "TRUE", "OCR_FAIL_ON_SEVERITY": " HIGH "})
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        review = next(c for c in calls if c["args"][:1] == ["review"])
        self.assertEqual(review["args"][1:5], ["--from", "a" * 40, "--to", "b" * 40])
        gate = self.gate_calls()[0]
        for flag, value in (("--expected-base", "a" * 40), ("--expected-head", "b" * 40),
                            ("--fail-on-severity", "high"), ("--input", ".ocr/ocr-result.json")):
            self.assertEqual(gate["args"][gate["args"].index(flag) + 1], value)
        post = next(c for c in calls if c["tool"] == "python3")
        self.assertEqual(post["severity"], "")
        self.assertIn("--require-publication", post["args"])
        self.assertLess(calls.index(review), calls.index(post))
        self.assertLess(calls.index(post), calls.index(gate))

    def test_all_failures_still_publish_and_evaluate_before_final_exit(self):
        for review, post, gate in ((7, 0, 0), (0, 8, 0), (0, 0, 1), (7, 8, 1)):
            with self.subTest(review=review, post=post, gate=gate):
                (self.root / "calls.jsonl").unlink(missing_ok=True)
                result = self.run_script({"OCR_GATE": "true", "FAKE_REVIEW_EXIT": str(review),
                                          "FAKE_POST_EXIT": str(post), "FAKE_GATE_EXIT": str(gate)})
                self.assertEqual(result.returncode, review or post or gate, result.stderr)
                self.assertEqual(len(self.gate_calls()), 1)
                self.assertTrue(any(c["tool"] == "python3" for c in self.calls()))
                for name in ("ocr-result.json", "ocr-stderr.log", "ocr-gate.json", "ocr-gate-stderr.log", "ocr-stats.env"):
                    self.assertTrue((self.root / ".ocr" / name).stat().st_size, name)

    def test_unsupported_cli_fails_before_model_or_publication(self):
        out = self.root / ".ocr"
        out.mkdir()
        (out / "ocr-gate.json").write_text('{"status":"pass"}', encoding="utf-8")
        result = self.run_script({"OCR_GATE": "true", "FAKE_GATE_HELP_EXIT": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("requires an OpenCodeReview build", result.stdout)
        self.assertFalse(any(c["args"][:1] == ["review"] or c["tool"] == "python3" for c in self.calls()))
        self.assertEqual((out / "ocr-gate.json").read_text(encoding="utf-8"), "")

    def test_invalid_policy_fails_before_install(self):
        for overrides in ({"OCR_GATE": "yes"}, {"OCR_GATE": "true", "OCR_FAIL_ON_SEVERITY": "info"},
                          {"OCR_GATE": "true", "OCR_FAIL_ON_SEVERITY": "high\nlow"}):
            with self.subTest(overrides=overrides):
                result = self.run_script(overrides)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.calls(), [])

    def test_fresh_outputs_cannot_reuse_a_previous_pass(self):
        out = self.root / ".ocr"
        out.mkdir()
        (out / "ocr-result.json").write_text('{"status":"complete"}', encoding="utf-8")
        (out / "ocr-gate.json").write_text('{"status":"pass"}', encoding="utf-8")
        result = self.run_script({"OCR_GATE": "true", "FAKE_REVIEW_EXIT": "7",
                                  "FAKE_REVIEW_JSON": "", "FAKE_GATE_EXIT": "1"})
        self.assertEqual(result.returncode, 7)
        self.assertEqual((out / "ocr-result.json").read_text(encoding="utf-8"), "")
        self.assertEqual(json.loads((out / "ocr-gate.json").read_text(encoding="utf-8"))["status"], "inconclusive")

    def test_missing_stage_outcomes_cannot_pass(self):
        result = self.run_script({"OCR_GATE": "true", "OCR_EXIT_CODE": "0", "OCR_GATE_EXIT_CODE": "0"},
                                 blocks=script_blocks()[-1:])
        self.assertEqual(result.returncode, 1)

    def test_failure_artifacts_are_declared(self):
        artifacts = PIPELINE.split("  artifacts:", 1)[1]
        self.assertIn("when: always", artifacts)
        for name in ("ocr-result.json", "ocr-stderr.log", "ocr-gate.json", "ocr-gate-stderr.log"):
            self.assertIn("- .ocr/" + name, artifacts)


if __name__ == "__main__":
    unittest.main()
