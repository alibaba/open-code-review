# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

"""Minimal stdio-v1 workflow; run with a Python 3 executable named python."""

import json
import sys


def respond(request):
    if not isinstance(request, dict):
        return {"version": 1, "error": "Request must be a JSON object"}
    if request.get("version") != 1 or request.get("stage") != "review":
        return {"version": 1, "error": "Unsupported protocol or stage"}
    if request.get("method") == "begin":
        for field in ("severities", "categories"):
            values = request.get(field)
            if not isinstance(values, list) or not values or not all(isinstance(value, str) and value.strip() for value in values):
                return {"version": 1, "error": f"{field} must be a nonempty array of strings"}
        finding = {
            "type": "object",
            "properties": {
                "path": {"type": "string"},
                "content": {"type": "string"},
                "existing_code": {"type": "string"},
                "severity": {"type": "string", "enum": request["severities"]},
                "category": {"type": "string", "enum": request["categories"]},
            },
            "required": ["path", "content", "existing_code", "severity", "category"],
        }
        return {"version": 1, "task": {
            "instructions": "Review the selected diffs against the resolved rules. Cite code evidence for findings. Mark complete false if required inspection could not finish.",
            "input": "Summarize the inspection and return supported findings.",
            "completion": {"description": "Submit the inspection result.", "parameters": {
                "type": "object", "properties": {
                    "complete": {"type": "boolean"},
                    "summary": {"type": "string"},
                    "comments": {"type": "array", "items": finding},
                }, "required": ["complete", "summary", "comments"],
            }},
        }}
    if request.get("method") == "submit":
        args = request.get("arguments", {})
        if not isinstance(args, dict) or type(args.get("complete")) is not bool or not isinstance(args.get("summary"), str) or not args["summary"].strip() or not isinstance(args.get("comments"), list):
            return {"version": 1, "accepted": False, "feedback": "Supply a boolean complete, nonempty summary and comments array."}
        return {"version": 1, "accepted": True, "complete": args["complete"], "report": {"summary": args["summary"]}, "comments": args["comments"], "feedback": "" if args["complete"] else args["summary"]}
    return {"version": 1, "error": "Unsupported method"}


if __name__ == "__main__":
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
    for line in sys.stdin:
        try:
            request = json.loads(line)
        except json.JSONDecodeError:
            response = {"version": 1, "error": "Invalid JSON request"}
        else:
            response = respond(request)
        print(json.dumps(response, ensure_ascii=True), flush=True)
