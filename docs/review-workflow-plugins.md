# Review workflow plugins

[English](review-workflow-plugins.md) | [简体中文](i18n/review-workflow-plugins.zh-CN.md) | [日本語](i18n/review-workflow-plugins.ja-JP.md) | [한국어](i18n/review-workflow-plugins.ko-KR.md) | [Русский](i18n/review-workflow-plugins.ru-RU.md)

For the relationship between package installation, authorization, and runtime
adapters, see [Native plugin architecture](plugin-architecture.md).

Use a workflow when a review needs a specific sequence and completion criteria:
for example, inspect a change, then verify each candidate finding before accepting
the result. The package declares those stages and validates the model's submissions;
OCR supplies the diff, rules, tools and execution budgets.

## Try the single-pass example

From this checkout, with Python 3 available as `python`, install the example and
preview a review of your working-tree changes:

```sh
ocr plugin install ./examples/ocr-plugins/single-pass
ocr review --review-plugin single-pass --preview
```

With a model provider configured and changes to review, run:

```sh
ocr review --review-plugin single-pass --format json --output review.json
```

The example has one stage. It asks the model for findings and validates the
submission before OCR publishes them. Inspect `review_plugin` in `review.json`
for the stage's completion status and report. Installation alone does not run
the package; preview does not start Python or contact the model.

These commands use a user-installed package. If the reviewed repository declares
project plugins, add the example to that declaration and authorize it using the
project workflow below. Selecting a global installation does not override the
project's locked set.

## Project selection and execution

For a repository-managed workflow, declare it with `ocr plugin add SOURCE`,
commit `.opencodereview/plugins.json` and `plugins.lock.json`, then install using
`ocr plugin sync --frozen`. Inspect the locked package and run `ocr plugin trust`
before selecting it with `--review-plugin ID`. `ocr plugin preflight --json`
checks the installed contents, compatibility, and local authorization without
starting the workflow or contacting a model. A project declaration restricts
selection to its locked packages; a globally installed workflow cannot bypass
that set. See [Project plugins](native-plugins.md#project-plugins).

A review plugin owns its stage plan, prompts, completion schema, validation and
cross-stage state. OCR provides selected diffs and rules, model/tool execution,
sessions, usage accounting, comment processing and output. The host contains no
plugin-specific stage names or report fields.

Select a package for one invocation:

```sh
ocr review --review-plugin PACKAGE_ID --preview
ocr review --review-plugin PACKAGE_ID --format json --output review.json
```

Installing or enabling a package does not select
its review workflow. Preview loads the manifest without starting the executable
or calling a model. A real selected review runs the package command with the
user's local permissions, like a local MCP server. Explicit selection also loads
that package's rules and MCP servers for this invocation, even when it is globally
disabled; the saved enabled state is unchanged.

## Manifest

```json
{
  "schema_version": 1,
  "id": "example-review",
  "name": "Example Review",
  "version": "1.0.0",
  "review": {
    "protocol": "stdio-v1",
    "command": "${plugin_dir}/bin/${os}-${arch}/reviewer${exe}",
    "args": ["--policy", "${plugin_dir}/policy.md"],
    "stages": ["inspect", "verify"]
  }
}
```

The command is an executable, without shell parsing. Commands and arguments
expand `${plugin_dir}`, `${os}`, `${arch}` and `${exe}` (the last is `.exe` on
Windows and empty elsewhere). A runtime on PATH, such as `python`, also works.
The bundled Python example expects the command `python`. On a machine where only
`python3` exists, change the manifest command to `python3` before packaging or
installation. OCR does not select an interpreter, invoke a shell or install a
runtime automatically.
Packages may combine review, rules and MCP contributions. Stage IDs are unique
portable identifiers; the manifest declares an ordered plan of one to 64 stages.
Other plugins can use different stage counts, names, schemas and decisions.

An installed package snapshot supplies both the rules and workflow for the run.
Updating the package does not change an active run's snapshot. Hosts supporting
only the earlier `engine`/`prompt` experiment require an upgrade for this protocol.
The host reads retired `engine`/`prompt` installation metadata so those packages
can still be listed, removed or updated. Management output marks them as requiring
an update (`review.update_required` in JSON) and omits the run hint. Attempting to
run one asks for an update.

## Compatibility

Workflow plugins may declare the manifest's optional `requires.ocr_min_version`
and `requires.features` fields described in [native plugins](native-plugins.md#compatibility-and-host-requirements).
Unsupported requirements are rejected before launching the process. The workflow
API is experimental. Existing `stdio-v1` plugins continue to use the same
`begin`/`submit` messages; there is no mandatory handshake or JSON-RPC migration.
Unknown response envelope fields remain errors. Opaque `report` and completion
arguments retain their plugin-defined shapes. An incompatible envelope change
must use a new protocol/message version, rather than silently reinterpret v1.

## stdio-v1

OCR starts one process per file group, with the reviewed repository as its working
directory. The process retains state across that group's stages. Requests and
responses are UTF-8 JSON objects, one line each, at most 4 MiB. Emit exactly one
response per request; send diagnostics to stderr. Each response has `version: 1`.
An `error` string means the plugin cannot execute the request and stops the group.

Before each stage, OCR sends:

```json
{"version":1,"method":"begin","stage":"inspect","paths":["main.go"],"severities":["critical","high","medium","low"],"categories":["bug","security","performance","maintainability","test","style","documentation","other"]}
```

The plugin returns a task and optionally its initial report:

```json
{"version":1,"task":{"instructions":"Inspect the selected code.","input":"Check the public return contract.","completion":{"description":"Submit the result.","parameters":{"type":"object","properties":{"checked":{"type":"boolean"}},"required":["checked"]}}},"report":{"pending":["inspection"]}}
```

OCR appends the instructions and input to its normal diff/rule context and keeps
them through context compression. Both `instructions` and `input` must contain
non-whitespace text because they become separate model messages. The model retains
OCR's context tools. Its
`task_done` definition comes from `completion`; direct `code_comment` is withheld
so publication passes through the plugin's validator. No model credentials are
sent in protocol requests. The executable inherits the normal process environment,
which may include provider tokens or proxy settings. Like other programs running
as the user, it can also read accessible configuration files. Install and select
workflows only from authors you trust; this release provides no runtime sandbox.

When the model calls `task_done`, OCR forwards the arguments unchanged:

```json
{"version":1,"method":"submit","stage":"inspect","arguments":{"checked":true}}
```

Accepted submissions require explicit `accepted` and `complete` booleans:

```json
{"version":1,"accepted":true,"complete":true,"report":{"checked":true},"comments":[]}
```

The plugin owns the report structure and its completion criteria. With
`accepted:true`, `complete:false` stops the group as incomplete; `feedback` is
saved as the reason, rather than sent back to the model for another attempt. Accepted
comments still use OCR's normal publication and filtering, including on incomplete
work. Comments must reference a selected path, contain content, and use the native
severity/category values supplied at `begin`. Other comment fields are
`existing_code`, `suggestion_code`, `start_line`, `end_line` and `thinking`.

For a repairable model submission, return `accepted:false`, nonempty `feedback`,
the latest saved `report` and no comments. OCR returns the feedback to the model
within the existing request allowance. The plugin must retain valid partial
entries itself. Crashes, malformed protocol responses and missing required
booleans stop execution; they are not retried as model mistakes.

## Completion and output

Every declared stage must finish, even if previous stages found no issues.
`--effort` does not shorten a plugin's plan. Existing context, tool-round and
timeout settings apply. The group timeout covers the declared stage count. When
the user supplies a token budget, unused allowance carries forward and remaining
stages share it; no additional total token limit is imposed.

JSON output includes `review_plugin`: records with `plugin`, `group`, `stage`,
`complete`, optional `token_limit`, opaque `report` and any execution `error`.
The latest returned report is retained if later model or process execution fails.
Each finished or interrupted stage also saves a `plugin_review` record to the
session, including in text runs. Inspect those reports after the run with
`ocr session show SESSION_ID --json`; its `review_plugin` array contains the saved
stage records. A response that omits `report` leaves the previous report intact.
Unfinished required work enters the existing coverage manifest and causes a
nonzero CLI exit. Ordinary review exit behavior is unchanged.

Plugin reviews require a fresh invocation; `--resume` does not yet restore
arbitrary process state. JSON and session records remain available for inspection.

See [the single-pass example](../examples/ocr-plugins/single-pass) for a Python
implementation and [Behavior Review](https://github.com/Qiyuanqiii/ocr-behavior-review)
for an independent executable with a three-stage method and retained evidence.
