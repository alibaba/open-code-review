#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors
"""Official ACP Python client against an independent adapter and mock OCR.

Run from the repository root:
  make -C acp build
  uv run --no-project --managed-python --python 3.12.13 --with-requirements acp/testdata/python-smoke-requirements.txt acp/testdata/python-client-smoke.py
"""

import asyncio
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import threading

if sys.flags.optimize != 0:
    raise SystemExit("smoke test requires assertions; run Python without -O")

import acp


def mock_ocr():
    if "--version" in sys.argv:
        print("ocr version 0.0.0-smoke")
        return
    if "--no-filter" in sys.argv:
        print("[ocr] READY", file=sys.stderr, flush=True)
        wait_for_cancel()
        return
    scan = "scan" in sys.argv
    effort = sys.argv[sys.argv.index("--effort") + 1] if "--effort" in sys.argv else "medium"
    selected = "--path" in sys.argv
    if selected:
        assert sys.argv[sys.argv.index("--path") + 1] == "sample.go"
    print("[ocr] Reviewing selected files\n[ocr] Reading file diff\n[ocr] Review finished",
          file=sys.stderr, flush=True)
    partial = "--effort" in sys.argv and sys.argv[sys.argv.index("--effort") + 1] == "high"
    result = {
        "status": "success", "message": "Selected resource scanned" if selected else ("Smoke scan" if scan else "Smoke review"),
        "summary": {"files_reviewed": 1, "comments": 1, "total_tokens": 120, "input_tokens": 100, "output_tokens": 20},
        "tool_calls": {"total": 3, "by_tool": {"read_file": 2, "code_search": 1}},
        "comments": [{"path": "sample.go" if scan else "nested/sample.go",
                      "start_line": 1, "end_line": 1, "severity": "critical",
                      "category": "bug", "content": "Smoke finding",
                      "suggestion_code": "safe()", "thinking": "DO_NOT_EXPOSE"}],
    }
    if partial:
        result["status"] = "partial"
        result["summary"]["files_reviewed"] = 4
        result["manifest"] = {
            "terminal_state": "partial",
            "coverage": {
                "selected": [{"path": path} for path in ("nested/sample.go", "b.go", "c.go", "server.go")],
                "completed": [{"path": path} for path in ("nested/sample.go", "b.go", "c.go")],
                "reused": [], "waived": [],
                "failed": [{"path": "server.go", "classification": "budget",
                            "reason": "reached the maximum tool-request rounds without finishing"}],
            },
        }
    if not scan:
        if effort == "low":
            result["comments"] = []
            result["summary"]["comments"] = 0
    print(json.dumps(result))


def wait_for_cancel():
    stop = threading.Event()

    def handle(*_):
        stop.set()
        sys.exit(130)

    signal.signal(signal.SIGINT, handle)
    if hasattr(signal, "SIGBREAK"):
        signal.signal(signal.SIGBREAK, handle)
    if hasattr(signal, "pause"):
        signal.pause()
        return
    stop.wait()


def adapter_binary():
    root = Path(__file__).resolve().parents[2] / "dist"
    name = "ocr-acp.exe" if os.name == "nt" else "ocr-acp"
    return root / name


def write_mock_wrapper(root, script):
    if os.name == "nt":
        wrapper = root / "mock-ocr.cmd"
        wrapper.write_text(
            f'@echo off\r\n"{sys.executable}" -u "{script}" --mock-ocr %*\r\n',
            encoding="utf-8",
        )
        return wrapper
    import shlex
    wrapper = root / "mock-ocr"
    wrapper.write_text("#!/bin/sh\nexec " + shlex.quote(sys.executable) + " " +
                       shlex.quote(str(script)) +
                       ' --mock-ocr "$@"\n', encoding="utf-8")
    wrapper.chmod(0o700)
    return wrapper


class SmokeClient:
    def __init__(self):
        self.updates = []
        self.ready = asyncio.Event()
        self.commands_ready = asyncio.Event()

    def on_connect(self, conn):
        pass

    async def session_update(self, session_id, update, **kwargs):
        obj = update.model_dump(mode="json", by_alias=True, exclude_none=True)
        self.updates.append(obj)
        if obj["sessionUpdate"] == "available_commands_update":
            self.commands_ready.set()
        if obj["sessionUpdate"] == "tool_call_update" and "READY" in obj.get("title", ""):
            self.ready.set()


def assert_execution(updates, status, expected_lines):
    calls = [u for u in updates if u["sessionUpdate"] == "tool_call"]
    assert len(calls) == 1, calls
    call = calls[0]
    assert call["kind"] == "other" and not call.get("locations"), call
    assert call["status"] == "in_progress"
    changes = [u for u in updates if u["sessionUpdate"] == "tool_call_update"
               and u["toolCallId"] == call["toolCallId"]]
    assert changes and changes[-1]["status"] == status, changes
    details = "".join(c["content"]["text"] for c in changes[-1].get("content", [])
                      if c.get("type") == "content" and c["content"].get("type") == "text")
    details = details.replace("\r\n", "\n")
    assert all(line in details for line in expected_lines), details
    if len(expected_lines) > 1:
        assert "\n".join(expected_lines) in details, details
    assert "```text\n" in details, details
    assert "Command:" in details and "Working directory:" in details, details
    messages = "".join(u["content"].get("text", "") for u in updates
                       if u["sessionUpdate"] == "agent_message_chunk")
    assert not any(line in messages for line in expected_lines), messages
    assert "Command:" not in messages, messages
    opening_details = "".join(c["content"]["text"] for c in call.get("content", [])
                              if c.get("type") == "content" and c["content"].get("type") == "text")
    assert "Command:" in opening_details and "Working directory:" in opening_details, opening_details
    assert changes[-1]["title"].startswith(("OCR review · ", "OCR scan · ")), changes[-1]



async def smoke():
    binary = adapter_binary()
    with tempfile.TemporaryDirectory(prefix="ocr-acp-smoke-") as temp:
        root = Path(temp).resolve()
        subprocess.run(["git", "init", "-q", str(root)], check=True)
        nested = root / "nested"
        nested.mkdir()
        source = nested / "sample.go"
        source.write_text("package sample\n", encoding="utf-8")
        wrapper = write_mock_wrapper(root, Path(__file__).resolve())
        client = SmokeClient()
        async with acp.spawn_agent_process(client, str(binary), "--ocr-binary", str(wrapper),
                                           transport_kwargs={"shutdown_timeout": 12}) as (conn, process):
            initialized = await conn.initialize(protocol_version=1)
            assert initialized.protocol_version == 1
            session = await conn.new_session(cwd=str(nested), mcp_servers=[])
            await asyncio.wait_for(client.commands_ready.wait(), 5)
            commands = [u for u in client.updates if u["sessionUpdate"] == "available_commands_update"]
            assert {c["name"] for c in commands[0]["availableCommands"]} == {"review", "scan"}
            print("PASS initialize, session/new, command discovery")
            for command in ("/review", "/scan"):
                client.updates.clear()
                response = await conn.prompt(session_id=session.session_id, prompt=[acp.text_block(command)])
                assert response.stop_reason == "end_turn"
                assert_execution(client.updates, "completed", ["[ocr] Reviewing selected files",
                                 "[ocr] Reading file diff", "[ocr] Review finished"])
                serialized = json.dumps(client.updates)
                assert all(s in serialized for s in ("Critical", "Smoke finding", "safe()", "1 file reviewed", "3 calls", "100 input", "20 output"))
                assert "DO_NOT_EXPOSE" not in serialized
                messages = "".join(u["content"].get("text", "") for u in client.updates
                                   if u["sessionUpdate"] == "agent_message_chunk")
                assert source.as_uri() + "#L1" in messages, messages
                assert "```go\nsafe()\n```" in messages, messages
                assert serialized.count("Smoke finding") == 1, serialized
                print(f"PASS {command}: execution log line breaks and completion, located finding, severity, suggestion, summary, no thinking")
            client.updates.clear()
            response = await conn.prompt(session_id=session.session_id,
                                         prompt=[acp.text_block("/review --effort low")])
            assert response.stop_reason == "end_turn"
            assert_execution(client.updates, "completed", ["[ocr] Review finished"])
            finished = [u for u in client.updates if u["sessionUpdate"] == "tool_call_update"][-1]
            details = "".join(c["content"]["text"] for c in finished.get("content", [])
                              if c.get("type") == "content" and c["content"].get("type") == "text")
            assert "Review stages" not in details, details
            assert finished["title"].startswith("OCR review · Completed · "), finished
            messages = "".join(u["content"].get("text", "") for u in client.updates
                               if u["sessionUpdate"] == "agent_message_chunk")
            assert "No findings" in messages or "0 findings" in messages, messages
            assert len([u for u in client.updates if u["sessionUpdate"] == "tool_call"]) == 1
            print("PASS ordinary CLI logs: no findings completes without structured progress")
            client.updates.clear()
            response = await conn.prompt(session_id=session.session_id,
                                         prompt=[acp.text_block("/review --effort high")])
            assert response.stop_reason == "end_turn"
            assert_execution(client.updates, "completed", ["[ocr] Review finished"])
            finished = [u for u in client.updates if u["sessionUpdate"] == "tool_call_update"][-1]
            assert finished["title"].startswith("OCR review · Partial · "), finished
            messages = "".join(u["content"].get("text", "") for u in client.updates
                               if u["sessionUpdate"] == "agent_message_chunk")
            assert "## OCR review · Partial" in messages, messages
            assert "3/4 files completed · 1 failed · 1 finding" in messages, messages
            assert "4 files reviewed" not in messages, messages
            assert "maximum tool-request rounds" in messages, messages
            print("PASS partial review: title, manifest coverage and budget failure agree")
            client.updates.clear()
            response = await conn.prompt(session_id=session.session_id, prompt=[
                acp.text_block("/scan"), acp.resource_link_block("sample", source.as_uri())])
            assert response.stop_reason == "end_turn"
            assert any(u["sessionUpdate"] == "tool_call" for u in client.updates)
            assert "Selected resource scanned" in json.dumps(client.updates)
            print("PASS local ResourceLink scan")
            client.updates.clear()
            client.ready.clear()
            pending = asyncio.create_task(conn.prompt(session_id=session.session_id,
                prompt=[acp.text_block("/review --no-filter")]))
            await asyncio.wait_for(client.ready.wait(), 10)
            assert not pending.done(), "progress was delivered only after completion"
            await conn.cancel(session_id=session.session_id)
            await conn.cancel(session_id=session.session_id)
            response = await asyncio.wait_for(pending, 10)
            assert response.stop_reason == "cancelled"
            assert_execution(client.updates, "failed", ["READY"])
            assert "cancelled" in json.dumps(client.updates)
            print("PASS cancel and repeated cancel, execution tool finalized with the same ID")
            process.stdin.close()
            assert await asyncio.wait_for(process.wait(), 10) == 0
            print("PASS clean EOF shutdown")


if __name__ == "__main__":
    if "--mock-ocr" in sys.argv:
        mock_ocr()
    else:
        asyncio.run(asyncio.wait_for(smoke(), 45))
