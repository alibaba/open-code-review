---
status: accepted
---

# Use one OCR-owned retry budget for every LLM protocol

`retry_codes` must work consistently for Chat Completions, Anthropic, and Responses clients. OCR therefore owns one bounded retry loop, passes configured status codes into that loop, and disables SDK-level retries so provider protocols cannot multiply the request budget.

This keeps network, EOF, timeout, standard transient HTTP statuses, and configured 4xx statuses under the same cancellation-aware limit. The trade-off is that SDK-specific retry heuristics are no longer used; the shared OCR policy is the source of truth.
