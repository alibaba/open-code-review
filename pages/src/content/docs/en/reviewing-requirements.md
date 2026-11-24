---
title: Reviewing against requirements
sidebar:
  order: 8
---

Give OCR the acceptance criteria for a change so it reviews business behavior
alongside code defects. The recommended workflow keeps the requirements in a
local Markdown file and passes it with `--background-file`.

## Requirements file for a change

For a single change, save a concise local Markdown file such as
`docs/requirements/checkout.md`:

```markdown
# Checkout requirements

Scope: src/checkout/**
Review changes against these criteria and cite the requirement ID in findings.

- PAY-1: Reject checkout when the cart is empty.
- PAY-2: Calculate the total on the server; never trust a client-supplied total.
- PAY-3: A repeated idempotency key must not create a second charge.

Out of scope: refunds and subscription billing.
```

From the repository root, with OCR configured and `main` available, run:

```bash
ocr review --from main --to HEAD --background-file docs/requirements/checkout.md
```

`--background-file` supplies context for the whole review. Relative paths resolve
from the **repository root**, even when you invoke OCR in a subdirectory;
absolute paths also work. If you also pass `--background`, the file takes
precedence and the inline text is ignored. Keep the file focused: OCR warns above
2,000 characters and rejects content above 8,000 characters, counted after
sanitization. The input file must be at most 1 MiB and nonempty after sanitization.

## Requirements from an internal knowledge base

For an internal knowledge base, export the relevant requirements, constraints,
and acceptance criteria into a concise local Markdown file and pass it with
`--background-file`. Merely naming a file or adding a remote link in background
prose does not automatically load that document or follow its links. Include
the actual criteria, and verify findings against the specification and tests.

## Limitations

OCR takes requirements as context for a single review. It currently has no
dedicated way to associate reference documents with a module or directory
independently of review rules: path rules select which rule text applies to
matching files, so they are not a mechanism for attaching reference material.
If the knowledge-base use case matures, dedicated support can be designed
separately.

## See Also

- [CLI Reference](../cli-reference/) — `ocr review --background-file` and the other review flags.
- [Review Rules](../review-rules/) — the rule layers, path rules, and rule resolution.
