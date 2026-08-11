---
status: accepted
---

# Baseline-gated implementation review loop

Review-driven implementation keeps one immutable `BASE_SHA` from before the
initial review through the final gate. This prevents a moving `HEAD~N` ref from
silently changing the review scope after implementation commits and makes the
result recoverable across an interrupted OCR call.

## Decision

Capture the exact `BASE_SHA` before the initial `/code-review`. Run the initial
review explicitly with the task context, then let `/implement` classify every
finding as `fix`, `reject-with-evidence`, or `accepted-risk`. `code-review`
owns finding generation and terminal-result recovery; `implement` owns the
disposition and implementation workflow.

Run baseline focused checks before editing, repeat them after the change, and
run the full suite once at the end. Record known baseline failures and block
new failures. Commit one logical implementation change, then run the final
review over `BASE_SHA..HEAD`. If the review call is interrupted, recover it
with `ocr_review_wait` or the same persisted session with `resume` instead of
starting a second review.

Completion requires a terminal final review, no unaccepted high or critical
finding, and no new test failure. An `accepted-risk` remains an open risk and
requires an explicit acceptance owner, impact, mitigation, and follow-up
condition before it can pass the gate.

## Consequences

- Relative refs such as `HEAD~38` are not used as the long-lived review boundary.
- The initial review remains an explicit workflow step; `/implement` owns the
  final gate after its commit.
- Partial OCR findings do not count as a completed review.
- The workflow can finish with a documented accepted risk without claiming
  that the risk was fixed.
