#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

"""Run real review approval screens with the SDK fixture and a local fake LLM.

Usage matches test-mcp-pty.py: <ocr> <mcp-server> <scratch>.
Requires the same pyte dependency. No external API or user config is accessed.
"""

import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

spec = importlib.util.spec_from_file_location("management_pty", Path(__file__).with_name("test-mcp-pty.py"))
management = importlib.util.module_from_spec(spec)
spec.loader.exec_module(management)
Terminal = management.Terminal


class Model(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        tools = request.get("tools", [])
        content = [{"type": "text", "text": "Inspect the small change."}]
        stop = "end_turn"
        if tools:
            self.server.rounds += 1
            aliases = [tool["name"] for tool in tools if tool["name"].startswith("mcp__")]
            assert "echo" not in [tool["name"] for tool in tools]
            if aliases and self.server.rounds <= 2:
                self.server.advertised += 1
                name, args = aliases[0], {"message": "fixture", "token": "pty-secret-sentinel"}
            else:
                name, args = "task_done", {"state": "DONE"}
            content = [{"type": "tool_use", "id": f"call_{self.server.rounds}", "name": name, "input": args}]
            stop = "tool_use"
        reply = json.dumps({"id": "pty", "type": "message", "role": "assistant", "model": "fixture", "content": content,
                            "stop_reason": stop, "usage": {"input_tokens": 10, "output_tokens": 5}}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(reply)))
        self.end_headers()
        self.wfile.write(reply)


def exercise(ocr, fixture, scratch):
    server = ThreadingHTTPServer(("127.0.0.1", 0), Model)
    server.rounds, server.advertised = 0, 0
    threading.Thread(target=server.serve_forever, daemon=True).start()
    env = os.environ.copy()
    for key in ["CI", "GITHUB_ACTIONS", "GITLAB_CI", "TF_BUILD", "BUILDKITE", "JENKINS_URL"]:
        env.pop(key, None)
    env.update(HOME=str(scratch), USERPROFILE=str(scratch), TERM="xterm-256color",
               OCR_LLM_URL=f"http://127.0.0.1:{server.server_port}/v1/messages", OCR_LLM_TOKEN="fixture",
               OCR_LLM_MODEL="fixture", OCR_LLM_PROTOCOL="anthropic", OCR_LLM_AUTH_HEADER="x-api-key",
               OCR_RAW_LOGGING="1", HTTP_PROXY="http://127.0.0.1:1", HTTPS_PROXY="http://127.0.0.1:1",
               NO_PROXY="localhost,127.0.0.1")
    repo = scratch / "repo"
    repo.mkdir()

    def git(*args):
        subprocess.run(["git", "-c", "user.name=fixture", "-c", "user.email=fixture@example.test",
                        "-c", "commit.gpgsign=false", *args], cwd=repo, env=env, check=True, capture_output=True)

    git("init", "-q", "-b", "main")
    source = repo / "main.go"
    source.write_text("package main\nfunc value() int { return 1 }\n")
    git("add", ".")
    git("commit", "-qm", "base")
    source.write_text("package main\nfunc value() int { return 2 }\n")
    git("add", ".")
    git("commit", "-qm", "change")

    def cli(*args):
        result = subprocess.run([str(ocr), *args], cwd=repo, env=env,
                                capture_output=True, text=True, timeout=30)
        assert result.returncode == 0, result.stderr
        return result

    try:
        for case, answer, per_run_prompts, expected_calls in [
            ("allow-once", "1", 2, 4), ("allow-review", "2", 1, 4),
            ("deny-once", "3", 2, 0), ("deny-review", "4", 1, 0),
            ("escape", "\x1b", 2, 0), ("ctrl-c", "\x03", 2, 0), ("timeout", None, 2, 0),
            ("ci-ask", None, 0, 0), ("ci-allow", None, 0, 4),
        ]:
            env.pop("CI", None)
            case_home = scratch / f"{case}-home"
            case_home.mkdir()
            env.update(HOME=str(case_home), USERPROFILE=str(case_home))
            marker = scratch / f"{case}.calls"
            env["OCR_TEST_ONE"] = "1"
            env["OCR_TEST_CALLS"] = str(marker)
            cli("mcp", "add", "fixture", "--type", "stdio", "--command", str(fixture),
                "--env", "_OCR_MCP_TEST_SERVER=${OCR_TEST_ONE}", "--env", "_OCR_MCP_TEST_CALLS=${OCR_TEST_CALLS}", "--yes")
            cli("mcp", "tools", "fixture", "--enable", "echo", "--yes")
            assert not marker.exists(), "Discovery invoked a business tool"
            cli("mcp", "enable", "fixture", "--yes")
            cli("mcp", "permissions", "--timeout", "1", "--yes")
            if case == "ci-allow":
                cli("mcp", "permissions", "fixture", "--tool", "echo=allow", "--yes")
            if case.startswith("ci-"):
                env["CI"] = "true"
            config = case_home / ".opencodereview" / "config.json"
            before = config.read_bytes()
            for _ in range(2):
                server.rounds, server.advertised = 0, 0
                terminal = Terminal([str(ocr), "review", "--repo", str(repo), "--from", "HEAD~1", "--to", "HEAD",
                                     "--audience", "agent", "--no-filter", "--concurrency", "1"], env)
                try:
                    for _ in range(per_run_prompts):
                        terminal.wait_screen("MCP tool approval required")
                        assert "pty-secret-sentinel" not in terminal.current_screen()
                        if answer is not None:
                            terminal.send(answer)
                        else:
                            terminal.drain(1.15)
                        # The next prompt can have the same screen content. Wait
                        # for its complete render before answering again.
                        terminal.drain(0.25)
                    terminal.finish()
                    assert "pty-secret-sentinel" not in terminal.output.decode(errors="replace")
                    if per_run_prompts == 0:
                        assert b"MCP tool approval required" not in terminal.output
                    if case == "ci-ask":
                        assert server.advertised == 0
                    else:
                        assert server.advertised == 2
                finally:
                    terminal.close()
                assert config.read_bytes() == before, "Review approval changed persistent config"
            actual = len(marker.read_text().splitlines()) if marker.exists() else 0
            assert actual == expected_calls, f"{case}: calls={actual}, expected={expected_calls}"
            print(f"PASS {case}: calls={actual}, prompts={per_run_prompts * 2}", flush=True)
        for transcript in scratch.rglob("*.jsonl"):
            assert "pty-secret-sentinel" not in transcript.read_text(), transcript
    finally:
        server.shutdown()
        server.server_close()


if __name__ == "__main__":
    binary, fixture, base = (Path(value).resolve() for value in sys.argv[1:])
    with tempfile.TemporaryDirectory(prefix="review-pty-", dir=base) as directory:
        exercise(binary, fixture, Path(directory))
