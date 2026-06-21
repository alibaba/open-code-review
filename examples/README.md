# CI/CD Integration Examples

This directory contains examples for integrating OpenCodeReview (OCR) into various CI/CD pipelines.

## Contents

- **[github_actions/](./github_actions/)** - GitHub Actions integration example
- **[gitlab_ci/](./gitlab_ci/)** - GitLab CI integration example

Each subdirectory contains its own README with detailed setup instructions.

The CI examples also show how to persist `ocr review --save-result` output, mount a shared `OCR_REVIEWS_DIR` for a long-running `ocr viewer`, and provide enterprise rules through `OCR_RULES_DIR` / `--rules-dir`.
