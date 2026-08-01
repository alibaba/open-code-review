#### Issue Template Validation
- **Missing required fields**: Issue templates should have `name`, `description`, and `body` fields
- **Invalid input types**: Verify `type` values in body inputs are valid (dropdown, input, textarea, checkboxes, markdown)
- **Empty options in dropdowns**: Dropdown type inputs must have non-empty `options` list
- **Missing `id` on inputs**: Form inputs without `id` cannot be parsed programmatically

#### Release Configuration
- **Undefined category labels**: Labels referenced in `categories[].labels` should exist in the repository (note: this is a warning, as labels may be created separately)
- **Missing default category**: A `release.yml` without a catch-all category (using `*`) may omit some PRs from release notes

#### General Structure
- **YAML syntax correctness**: Indentation consistency, proper quoting of special characters, valid anchors/aliases usage
- **Spelling errors in YAML keys**: Check for typos in configuration keys that would be silently ignored
