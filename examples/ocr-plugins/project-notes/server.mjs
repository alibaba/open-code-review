// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// A dependency-free MCP example using the 2025-11-25 stdio protocol.
// Only JSON-RPC messages go to stdout. The host supplies the repository as cwd.
import fs from "node:fs";
import path from "node:path";
import readline from "node:readline";

const tool = {
  name: "project_review_notes",
  description: "Read REVIEW_NOTES.md from the reviewed repository (at most 64 KiB).",
  inputSchema: { type: "object", properties: {}, additionalProperties: false },
  annotations: { readOnlyHint: true, destructiveHint: false, openWorldHint: false },
};

function readNotes() {
  const root = fs.realpathSync(process.cwd());
  let target;
  try {
    target = fs.realpathSync(path.join(root, "REVIEW_NOTES.md"));
  } catch (error) {
    if (error.code === "ENOENT") return "No REVIEW_NOTES.md exists in this repository.";
    throw error;
  }
  const relative = path.relative(root, target);
  if (relative === ".." || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative)) {
    throw new Error("REVIEW_NOTES.md resolves outside the repository.");
  }
  const fd = fs.openSync(target, "r");
  try {
    if (!fs.fstatSync(fd).isFile()) throw new Error("REVIEW_NOTES.md is not a regular file.");
    const buffer = Buffer.alloc(65537);
    let size = 0;
    while (size < buffer.length) {
      const read = fs.readSync(fd, buffer, size, buffer.length - size, null);
      if (!read) break;
      size += read;
    }
    if (size > 65536) throw new Error("REVIEW_NOTES.md exceeds 64 KiB.");
    return buffer.subarray(0, size).toString("utf8");
  } finally {
    fs.closeSync(fd);
  }
}

function dispatch(request) {
  switch (request.method) {
    case "initialize":
      return { protocolVersion: "2025-11-25", capabilities: { tools: {} }, serverInfo: { name: "ocr-project-notes", version: "1.0.0" } };
    case "ping":
      return {};
    case "tools/list":
      return { tools: [tool] };
    case "tools/call": {
      const args = request.params?.arguments ?? {};
      if (request.params?.name !== tool.name || typeof args !== "object" || Array.isArray(args) || Object.keys(args).length) {
        throw Object.assign(new Error("Unknown tool or invalid arguments."), { rpcCode: -32602 });
      }
      try {
        return { content: [{ type: "text", text: readNotes() }] };
      } catch (error) {
        return { isError: true, content: [{ type: "text", text: error.message }] };
      }
    }
    default:
      throw Object.assign(new Error("Method not found."), { rpcCode: -32601 });
  }
}

const input = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
input.on("line", (line) => {
  let request;
  try {
    request = JSON.parse(line);
  } catch {
    process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: null, error: { code: -32700, message: "Invalid JSON." } }) + "\n");
    return;
  }
  if (!request || typeof request !== "object" || request.jsonrpc !== "2.0") return;
  if (request.id === undefined) return;
  let response;
  try {
    response = { jsonrpc: "2.0", id: request.id, result: dispatch(request) };
  } catch (error) {
    response = { jsonrpc: "2.0", id: request.id, error: { code: error.rpcCode ?? -32603, message: error.message } };
  }
  process.stdout.write(JSON.stringify(response) + "\n");
});
