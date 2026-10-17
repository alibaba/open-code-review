# Native plugin architecture

Native plugins package reusable review rules and MCP server declarations behind
`ocr-plugin.json`. A package can contribute either or both capabilities. OCR owns
installation, package selection, authorization checks, and the adapters into its
review runtime.

This guide maps the implementation for reviewers. For commands and manifest
fields, see [Native OCR plugins](native-plugins.md).

## Package lifecycle and runtime

The arrows below follow package data into the runtime. Installation and frozen
sync produce copied snapshots; a review loads the selected snapshots before
starting executable contributions.

```mermaid
flowchart TB
    source["GitHub URL, HTTPS archive, or local package"] --> prepare["Prepare and validate package"]
    prepare --> snapshot["Copied snapshot and registry"]
    snapshot --> selection["Select packages and check authorization"]
    policy["Project declaration and lock, when present"] --> selection
    selection --> rules["Rules: append matching review instructions"]
    selection --> mcp["MCP: connect servers during review or scan"]
    manual["Existing manual MCP configuration"] --> mcp
    rules --> host["OCR model and tool execution"]
    mcp --> host
    host --> results["Comments, coverage, sessions, and output"]
```

| Contribution | Package owns | OCR owns | Activation |
| --- | --- | --- | --- |
| Rules | Rule JSON and referenced text | Existing rule priority, file matching, and appended plugin rules | Enabled global package or selected project package |
| MCP | Local command or remote endpoint, arguments, environment, headers, and tool allowlist | MCP clients, tool registration, and client cleanup | Selected and authorized package during review/scan |

Enabling a global package activates its rules and MCP contributions. With a
project declaration, selection is restricted to the locked set.

## Module boundaries

| Area | Responsibility and boundary | Main entry points |
| --- | --- | --- |
| Package contract | Parse contribution declarations and check host requirements; no process launch | [`manifest.go`](../internal/plugin/manifest.go), [`compatibility.go`](../internal/plugin/compatibility.go) |
| Source preparation | Resolve direct sources, select a package, bound downloads/extraction, validate files, and calculate digests | [`install_source.go`](../internal/plugin/install_source.go), [`github_source.go`](../internal/plugin/github_source.go), [`source.go`](../internal/plugin/source.go), [`package.go`](../internal/plugin/package.go), [`tar.go`](../internal/plugin/tar.go) |
| Installed state | Copy snapshots, commit registry updates, enable/disable/remove packages, and compare update execution declarations | [`store.go`](../internal/plugin/store.go), [`update.go`](../internal/plugin/update.go); [`legacy_source.go`](../internal/plugin/legacy_source.go) handles older cached sources |
| Project policy | Resolve declarations into platform locks, sync exact artifacts, verify snapshots, and record local execution authorization | [`project.go`](../internal/plugin/project.go), [`project_sync.go`](../internal/plugin/project_sync.go), [`project_inspection.go`](../internal/plugin/project_inspection.go) |
| Runtime adapters | Load a common selected snapshot set into rules and MCP | [`plugin_rules.go`](../internal/config/rules/plugin_rules.go), [`plugin_runtime.go`](../cmd/opencodereview/plugin_runtime.go) |
| CLI inspection | Parse management commands and render summaries; inspection does not initialize the review runtime | [`plugin_cmd.go`](../cmd/opencodereview/plugin_cmd.go), [`plugin_project_cmd.go`](../cmd/opencodereview/plugin_project_cmd.go), [`mcp_cmd.go`](../cmd/opencodereview/mcp_cmd.go) |
| Results and reuse | Include plugin configuration/content in local resume and CI reuse decisions | [`resume_identity.go`](../internal/session/resume_identity.go), [`project-plugins.js`](../scripts/github-actions/project-plugins.js), [`action.yml`](../action.yml) |

`internal/plugin` holds package/state operations and literal command placeholder
expansion in `command.go`. Contribution-specific integration lives with the rule
resolver and CLI MCP initialization. The rule resolver's `EnabledPlugins()`
supplies the same selected snapshots to the MCP adapter.

## Selection, installation, and authorization

```mermaid
flowchart TB
    start["Review, scan, or rule loading"] --> declared{"Project declaration present?"}
    declared -->|"No"| global["Use enabled global plugins"]
    global --> selected
    declared -->|"Yes"| locked["Use only the declared, locked project plugins"]
    locked --> verify["Verify platform, installed contents, and host requirements"]
    verify --> executable{"Any MCP declaration?"}
    executable -->|"No: rules only"| selected["Selected snapshot set"]
    executable -->|"Yes"| trust{"Local authorization for this project and lock?"}
    trust -->|"Yes"| selected
    trust -->|"No"| stop["Fail with an explicit error"]
    verify -->|"Invalid or missing"| stop
    selected --> adapters["Load rules and runtime contributions"]
```

- `plugin install` copies a global package, disabled by default. `--enable` or
  `plugin enable` makes its rules/MCP contributions available to later runs.
- `plugin add` and `plugin lock` resolve sources and write repository declarations
  and platform locks. They can download packages to inspect them.
- `plugin sync --frozen` installs the exact locked artifacts. It checks recorded
  digests and does not resolve a newer version or authorize execution.
- `plugin trust` records authorization outside the repository, bound to the
  canonical project path, lock, and platform. A changed lock needs a new decision.
- Review and preflight verify installed project contents without downloading
  packages. An invalid project configuration fails instead of choosing global
  plugins. An empty project declaration selects no packaged plugins.

Manual MCP configuration and existing project/user rule files keep their current
behavior. The project declaration selects the packaged contributions only.
Native executable contributions have the user's or runner's permissions; the
installation and trust mechanisms do not sandbox their runtime behavior.

### Read-only management

`mcp list` combines manual server configuration with global or project plugin
declarations. `Project.Inspect` verifies installed project snapshots without
granting or requiring execution authorization, so untrusted snapshots can be
shown as `authorization_required`. Invalid locks, changed/missing snapshots, and
unsupported project requirements still produce errors. Incompatible enabled
global plugins remain visible with an `incompatible` state.

Listing does not connect to MCP endpoints, run setup hooks or credential commands,
or change configuration/trust. `configured` describes the selected declaration,
not connection health. `plugin install SOURCE --dry-run` may download and prepare
the requested source, but does not install, enable, or execute its contributions.

### Updates

Global updates re-read the recorded source and retain the enabled state. Added
or changed execution declarations require `--accept-permissions`; rejection
leaves the previous installation in place. Project updates use a new lock,
frozen sync, and renewed lock authorization. Execution acceptance concerns
declared capabilities; unchanged declarations do not establish trust in updated
program contents. Older snapshots remain available to active runs.

## CI and reusable results

The Action selects rules and plugin policy from the PR base or an explicitly
trusted `policy_sha`. The PR head supplies the code diff. With `project_plugins`
enabled, it creates a private OCR home, runs frozen sync, optionally authorizes
execution through the trusted workflow's `allow_plugin_exec` input, and obtains
a verified fingerprint before review. Model credentials are configured after
plugin preparation. Personal global packages and old trust records are not
inherited into this home.

Local resume identity includes selected manifests, package contents, and rule
text. Executable contributions also retain observable install paths in that
identity. CI reuse uses the trusted policy and verified plugin
fingerprint, including execution permission, without temporary installation
paths. These checks prevent reusing a result after relevant plugin policy changes.

## Suggested review order

| Step | Read together | Question to answer |
| --- | --- | --- |
| 1. Contract | `internal/plugin/manifest.go`, `compatibility.go`, and their tests | Which contributions can a package declare, and how are unsupported requirements rejected? |
| 2. Preparation and storage | `install_source.go`, `github_source.go`, `source.go`, `package.go`, `tar.go`, `store.go`, `update.go`, and their tests | Do supported sources become validated snapshots without executing package code, and do failed updates preserve the old registration? |
| 3. Project selection and trust | `project*.go`, project CLI commands, and project tests | Are locked bytes verified, global fallback prevented, and authorization kept separate from repository declarations? |
| 4. Rules and MCP | `internal/config/rules/plugin_rules.go`, CLI `plugin_runtime.go`, `mcp_cmd.go`, and their tests | Are existing rules/manual MCP preserved, selected snapshots reused consistently, and management reads free of runtime startup? |
| 5. Persistence and CI | Session identity changes, `action.yml`, `project-plugins.js`, Action tests, and examples | Is policy taken from a trusted commit, and does a plugin change invalidate reusable results? |

The implementation supports direct package installation and management. The
independently buildable TUI is reviewed separately in PR #1739. Marketplace
catalog/search support is absent; older cached-source migration is contained in
`legacy_source.go`. Follow [the CLI guide](native-plugins.md) to exercise each
path after reviewing its implementation.
