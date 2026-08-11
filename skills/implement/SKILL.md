---
name: implement
description: "Implement a piece of work based on a spec or set of tickets."
disable-model-invocation: true
---

Implement the work described by the user in the spec or tickets.

Before editing:

1. Read the task source and carry its requirements into every review handoff.
2. Record the exact `BASE_SHA` before making implementation changes. Keep it
   fixed through the final review; do not replace it with a moving `HEAD~N` ref.
3. When the work starts from an existing diff, run the initial `/code-review`
   against `BASE_SHA..HEAD` and carry its native result into this run.
4. Run the focused typecheck, lint, and test commands that will be used for
   validation. Record existing failures as the baseline.

During implementation:

- Classify every review finding as `fix`, `reject-with-evidence`, or
  `accepted-risk`.
- Implement only `fix` findings. Give every rejection concrete evidence.
- For every accepted risk, record its impact, mitigation, acceptance owner,
  and follow-up condition.
- Use /tdd where possible, at pre-agreed seams.
- Run typechecking and single test files regularly.

Before completion:

1. Repeat the baseline checks and block any new failure.
2. Run the full test suite once at the end.
3. Commit one logical implementation change to the current branch.
4. Run the final `/code-review` over `BASE_SHA..HEAD` after the commit.
5. If OCR is interrupted, use `ocr_review_wait` or the same persisted session
   with `resume`; do not treat partial findings as a completed review.

The work is complete only when the final review has a terminal result, no
unaccepted high or critical finding remains, and no new test failure exists.

`code-review` owns finding generation and OCR recovery. `implement` owns
finding disposition, implementation, validation, and the final review gate.
