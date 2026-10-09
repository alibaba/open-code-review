#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

"""Exercise the real Bubble Tea onboarding in a POSIX pseudo-terminal.

Build OCR and the existing SDK test fixture first:
  go build -o <scratch>/ocr ./cmd/opencodereview
  go test -c -o <scratch>/mcp-server ./internal/mcp
  python3 -m pip install --target <scratch>/python pyte==0.8.2
  PYTHONPATH=<scratch>/python python3 scripts/test-mcp-pty.py <scratch>/ocr <scratch>/mcp-server <scratch>

Screen assertions use pyte 0.8.2, a test-only VT emulator. Install it in a
temporary directory and include that directory in PYTHONPATH when running.
No external servers, credentials, or user configuration are used.
"""

import codecs
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import sys
import termios
import tempfile
import time

import pyte


class Terminal:
    def __init__(self, command, env):
        self.master, slave = pty.openpty()
        self.slave = slave
        self.original_termios = termios.tcgetattr(slave)
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 45, 120, 0, 0))
        self.process = subprocess.Popen(command, stdin=slave, stdout=slave,
                                        stderr=slave, env=env, start_new_session=True)
        self.output = b""
        self.screen = pyte.Screen(120, 45)
        self.stream = pyte.Stream(self.screen)
        self.decoder = codecs.getincrementaldecoder("utf-8")("replace")

    def drain(self, seconds=0.25):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            if not select.select([self.master], [], [], min(0.05, max(0, deadline-time.monotonic())))[0]:
                continue
            try:
                chunk = os.read(self.master, 65536)
            except OSError:
                break
            if not chunk:
                break
            self.output += chunk
            self.stream.feed(self.decoder.decode(chunk))
            if b"\x1b[6n" in chunk:
                os.write(self.master, b"\x1b[1;1R")
            if b"\x1b[c" in chunk or b"\x1b[>0c" in chunk:
                os.write(self.master, b"\x1b[?1;2c")

    def wait(self, text, seconds=15):
        self.wait_screen(text, seconds)

    def current_screen(self):
        return "\n".join(self.screen.display)

    def wait_screen(self, text, seconds=15):
        deadline = time.monotonic() + seconds
        while text not in self.current_screen():
            self.drain()
            if time.monotonic() > deadline or self.process.poll() is not None:
                raise AssertionError(f"Missing current screen text {text!r}:\n{self.current_screen()}")

    def assert_single_screen_session(self):
        self.drain()
        assert self.output.count(b"\x1b[?1049h") == 1, "Manager re-entered the alternate screen"
        assert self.output.count(b"\x1b[?1049l") == 1, "Manager did not restore the main screen once"
        restored = termios.tcgetattr(self.slave)
        # Darwin can set PENDIN (retype pending input) during tcsetattr. This
        # kernel bookkeeping flag is not a changed input/echo/signal mode.
        pending = getattr(termios, "PENDIN", 0)
        restored[3] &= ~pending
        self.original_termios[3] &= ~pending
        assert restored == self.original_termios, f"Terminal mode not restored: {self.original_termios!r} -> {restored!r}"

    def send(self, keys):
        os.write(self.master, keys.encode())
        self.drain()

    def finish(self):
        deadline = time.monotonic() + 10
        while self.process.poll() is None and time.monotonic() < deadline:
            self.drain()
        assert self.process.poll() == 0, self.output[-2000:]

    def close(self):
        if self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait()
        os.close(self.master)
        os.close(self.slave)


def exercise(ocr, server, scratch):
    env = os.environ.copy()
    for key in ["CI", "GITHUB_ACTIONS", "GITLAB_CI", "TF_BUILD", "BUILDKITE", "JENKINS_URL"]:
        env.pop(key, None)
    env.update(TERM="xterm-256color", OCR_TEST_ONE="1")
    call_marker, start_marker = scratch / "called", scratch / "started"
    env.update(OCR_TEST_CALL_MARKER=str(call_marker), OCR_TEST_START_MARKER=str(start_marker))
    captures = []

    def config_path(name):
        return scratch / name / ".opencodereview" / "config.json"

    def select_home(name):
        home = scratch / name
        home.mkdir(exist_ok=True)
        env.update(HOME=str(home), USERPROFILE=str(home))

    def start(name):
        select_home(name)
        terminal = Terminal([str(ocr), "mcp", "add", name], env)
        captures.append(terminal)
        terminal.wait("Connection name")
        terminal.send("\r")
        terminal.wait("Transport")
        return terminal

    def credential(terminal, name, source):
        terminal.send(name + "\r")
        terminal.send(source + "\r")

    remote = None
    try:
        # One terminal owner: setup replaces the dashboard and cancellation
        # restores focus. The emulator checks the current screen, not history.
        select_home("navigation")
        terminal = Terminal([str(ocr), "mcp"], env)
        captures.append(terminal)
        terminal.wait_screen("MANAGEMENT ACTIONS")
        terminal.send("\r")
        terminal.wait_screen("Connection name")
        assert "MANAGEMENT ACTIONS" not in terminal.current_screen()
        assert "No servers yet" not in terminal.current_screen()
        terminal.send("\x1b")
        terminal.wait_screen("MANAGEMENT ACTIONS")
        assert "Step 1/8" not in terminal.current_screen()
        assert "> Add server" in terminal.current_screen()
        terminal.send("\r")
        terminal.wait_screen("Connection name")
        terminal.send("\x03")
        terminal.finish()
        terminal.assert_single_screen_session()
        assert not config_path("navigation").exists()

        # Cancel before discovery: neither a subprocess nor a config may exist.
        terminal = start("cancelled")
        terminal.send("\r")
        terminal.send(str(server) + "\r")
        terminal.send("\r")
        credential(terminal, "_OCR_MCP_TEST_SERVER", "OCR_TEST_ONE")
        credential(terminal, "_OCR_MCP_TEST_STARTED", "OCR_TEST_START_MARKER")
        terminal.send("\r")
        terminal.wait("Review connection")
        terminal.send("\x1b")
        terminal.finish()
        assert not start_marker.exists() and not config_path("cancelled").exists()

        terminal = start("local")
        terminal.send("\r")
        terminal.send(str(server) + "\r")
        terminal.send("\r")
        terminal.send("\x02")  # Ctrl-B returns to arguments without losing the executable.
        terminal.send("\r")
        credential(terminal, "_OCR_MCP_TEST_SERVER", "OCR_TEST_ONE")
        credential(terminal, "_OCR_MCP_TEST_MARKER", "OCR_TEST_CALL_MARKER")
        credential(terminal, "_OCR_MCP_TEST_STARTED", "OCR_TEST_START_MARKER")
        terminal.send("\r")
        terminal.wait("Review connection")
        assert not start_marker.exists()
        terminal.send("\x1b[B\r")
        terminal.wait("Choose tools")
        assert start_marker.exists() and not call_marker.exists()
        terminal.send(" \r")
        terminal.wait("Save connection")
        terminal.send("\x1b[B\r")
        terminal.finish()
        cfg = json.loads(config_path("local").read_text())
        local = cfg["mcp_servers"]["local"]
        assert local["tools"] == ["echo"] and local["tool_permissions"] == {"echo": "ask"}
        assert len(local["tool_definition_sha256"]["echo"]) == 64
        assert config_path("local").stat().st_mode & 0o777 == 0o600
        assert not call_marker.exists()

        # The manager and permissions use real selector screens, not typed enums.
        select_home("local")
        terminal = Terminal([str(ocr), "mcp", "permissions"], env)
        captures.append(terminal)
        terminal.wait("Global default permission")
        terminal.send("\r")
        terminal.wait("Approval timeout")
        terminal.send("120\r")
        terminal.wait("MCP confirmation")
        terminal.send("\x1b[B\r")
        terminal.finish()
        assert json.loads(config_path("local").read_text())["mcp"]["approval_timeout_seconds"] == 120

        terminal = Terminal([str(ocr), "mcp", "permissions", "local"], env)
        captures.append(terminal)
        terminal.wait("Server default permission")
        terminal.send("\r")
        terminal.wait("Permission for echo")
        terminal.send("\x1b[B\r")
        terminal.wait("MCP confirmation")
        terminal.send("\x1b[B\r")
        terminal.finish()
        assert json.loads(config_path("local").read_text())["mcp_servers"]["local"]["tool_permissions"]["echo"] == "allow"

        terminal = Terminal([str(ocr), "mcp", "tools", "local"], env)
        captures.append(terminal)
        terminal.wait("MCP confirmation")
        terminal.send("\x1b[B\r")
        terminal.wait("Choose tools")
        terminal.send(" \r")
        terminal.drain(0.5)
        terminal.send("\x1b[B\r")
        terminal.finish()
        assert json.loads(config_path("local").read_text())["mcp_servers"]["local"]["tool_permissions"]["echo"] == "ask"
        assert not call_marker.exists()

        # Server-first management is read-only until an explicit action.
        # Keep navigation in one session and check the on-disk state after each action.
        select_home("local")
        started_before = start_marker.stat().st_mtime_ns
        terminal = Terminal([str(ocr), "mcp"], env)
        captures.append(terminal)
        terminal.wait("CONFIGURED SERVERS")
        terminal.send("\r")
        terminal.wait("Server: local")
        terminal.send("\r")
        terminal.wait("Ask before calling")
        terminal.send("\r")
        terminal.wait("Tool: echo")
        assert start_marker.stat().st_mtime_ns == started_before
        terminal.send("\x1b[B\r")
        terminal.wait("Tool permission:")
        terminal.send("\x1b[B\r")
        terminal.wait("Save these permissions")
        terminal.send("y")
        terminal.wait("No prompt (allow)")
        assert json.loads(config_path("local").read_text())["mcp_servers"]["local"]["tool_permissions"]["echo"] == "allow"
        terminal.send("\x1b[A\r")  # Revoke locally, without discovery.
        terminal.wait("Not enabled; hidden from model")
        assert not json.loads(config_path("local").read_text())["mcp_servers"]["local"].get("tools")
        assert start_marker.stat().st_mtime_ns == started_before
        terminal.send("\x1b")  # tools
        terminal.send("\x1b")  # server
        terminal.send("\x1b[B\r")  # refresh
        terminal.wait("Test connection and list tools?")
        terminal.send("n")
        assert start_marker.stat().st_mtime_ns == started_before
        terminal.send("\r")  # same refresh item, no server reselection
        terminal.drain()
        terminal.send("y")
        terminal.wait("Last check: success")
        assert start_marker.stat().st_mtime_ns != started_before and not call_marker.exists()
        terminal.send("\x1b[A\r\r")  # tools, echo
        terminal.send("\r")  # enable discovered tool; ask for reconnect
        terminal.wait("Connect to discover tools?")
        terminal.send("y")
        terminal.wait("Effective: Ask before calling")
        terminal.send("q")
        terminal.finish()
        terminal.assert_single_screen_session()
        assert json.loads(config_path("local").read_text())["mcp_servers"]["local"]["tools"] == ["echo"]

        # Paste a configuration privately, preview, cancel, and then import it.
        select_home("imported")
        pasted = '{"mcpServers":{"imported":{"command":"never-start","env":{"TOKEN":"IMPORT_SECRET_SENTINEL"}}}}'
        terminal = Terminal([str(ocr), "mcp"], env)
        captures.append(terminal)
        terminal.wait("No servers yet")
        terminal.send("\x1b[B\r")
        terminal.wait("Import from file or paste")
        terminal.send("\x1b[B\r")
        terminal.wait("Contents hidden")
        terminal.send("\x1b[200~" + pasted + "\x1b[201~")
        terminal.send("\x13")
        terminal.wait("Connection name for imported server")
        terminal.send("\r")
        terminal.wait("Save disabled connection only")
        terminal.send("y")
        terminal.wait("imported  |  stdio  |  disabled")
        # A failed check returns to details, preserving the config and allowing
        # retry/edit instead of closing the whole manager.
        terminal.send("\x1b[A\r")
        terminal.wait("Server: imported")
        terminal.send("\x1b[B\r")
        terminal.wait("Test connection and list tools?")
        terminal.send("y")
        terminal.wait("Action failed.")
        terminal.wait("failed; retry")
        terminal.send("\x1b[B\r")
        terminal.wait("Connection name")
        terminal.send("\x1b")
        terminal.send("q")
        terminal.finish()
        terminal.assert_single_screen_session()
        imported = json.loads(config_path("imported").read_text())["mcp_servers"]["imported"]
        assert imported["enabled"] is False and not imported.get("tools")
        assert imported["env"] == ["TOKEN=${TOKEN}"]
        assert b"IMPORT_SECRET_SENTINEL" not in terminal.output

        remote_env = env | {"_OCR_MCP_TEST_SERVER": "1", "_OCR_MCP_TEST_HTTP": "1",
                            "_OCR_MCP_TEST_MARKER": str(call_marker)}
        remote = subprocess.Popen([str(server)], env=remote_env, stdout=subprocess.PIPE,
                                  stderr=subprocess.DEVNULL, text=True)
        assert select.select([remote.stdout], [], [], 10)[0], "SDK server did not start"
        url = remote.stdout.readline().strip()
        assert url.startswith("http://127.0.0.1:")
        terminal = start("remote")
        terminal.send("\x1b[B\r")
        terminal.send(url + "\r")
        terminal.send("\r")
        terminal.wait("Review connection")
        terminal.send("\x1b[B\r")
        terminal.wait("Choose tools")
        terminal.send(" \r")
        terminal.send("\x1b[B\r")
        terminal.finish()
        cfg = json.loads(config_path("remote").read_text())
        assert cfg["mcp_servers"]["remote"]["tools"] == ["echo"]
        assert not call_marker.exists()

        # Non-interactive management cannot write or discover implicitly.
        select_home("noninteractive")
        result = subprocess.run([str(ocr), "mcp"], env=env, capture_output=True, timeout=10)
        assert result.returncode == 0 and not config_path("noninteractive").exists()
        print("PASS: real PTY server-first management, tool permission/revocation, consent/refresh, private paste import, stdio + remote discovery, back, checkbox/save, timeout, cancellation, private config, non-TTY, and zero business calls")
    finally:
        for terminal in captures:
            terminal.close()
        if remote:
            remote.terminate()
            remote.wait(timeout=5)


if __name__ == "__main__":
    binary, fixture, base = map(lambda value: Path(value).resolve(), sys.argv[1:])
    with tempfile.TemporaryDirectory(prefix="pty-", dir=base) as directory:
        exercise(binary, fixture, Path(directory))
