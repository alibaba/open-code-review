Parent document: `/CLAUDE.md`
Related documents:
- `docs/architecture/RUNTIME_FLOWS.md`
- `docs/architecture/DEPENDENCY_GRAPH.md`
- `docs/architecture/RUNTIME_DEPENDENCY_TREE.md`

Read this when:
- You're stepping through a debugger or adding a log line and need to know the real call sequence.
- You need the full CLI command tree.

Purpose:
- Runtime call graph from process entry to the core loop, and the complete cobra command tree.

Scope:
- Included: entry point → command dispatch → orchestrator → loop → LLM client call chains.
- Excluded: flow *meaning* (see `RUNTIME_FLOWS.md`), startup dependency ordering/failure modes (see `RUNTIME_DEPENDENCY_TREE.md`).

---

# Call Graph

## Full command tree

```
ocr
├── version
├── review                cmd/opencodereview/review_cmd.go
├── scan                   scan_cmd.go
├── delegate                delegate_cmd.go
│   ├── preview
│   └── rule <path...>
├── session                 session_cmd.go
│   ├── list
│   ├── show <id>
│   └── comments <id>
├── config                  config_cmd.go
│   ├── set <key> <value>
│   ├── unset <key>
│   ├── provider            (interactive TUI, provider_tui.go)
│   └── model               (interactive TUI)
├── llm                      llm_cmd.go
│   ├── test
│   └── providers
├── rules                    rules_cmd.go
│   └── check <file-path>
├── viewer                    viewer_cmd.go
└── completion
```

`root.go`'s `PersistentPreRunE` runs before every subcommand and only validates the `--color` flag — no config/LLM resolution happens at the root level; each subcommand resolves its own dependencies lazily in its `RunE`.

## `ocr review` call chain

```mermaid
sequenceDiagram
    participant main as main.go
    participant root as root.go / review_cmd.go
    participant shared as shared.go: loadLLMRuntime
    participant llm as llm.ResolveEndpointWithOptions
    participant diffp as diff.Provider
    participant agentpkg as agent.Agent.Run
    participant loop as llmloop.Runner.RunPerFile
    participant client as llm.LLMClient

    main->>root: cobra Execute()
    root->>shared: loadLLMRuntime()
    shared->>shared: toolsconfig.Load()
    shared->>shared: LoadAppConfig()
    shared->>llm: ResolveEndpointWithOptions()
    llm-->>shared: ResolvedEndpoint or hard error
    shared-->>root: client, template, tools
    root->>diffp: load diffs (Workspace/Commit/Range)
    root->>agentpkg: Agent.Run(diffs)
    agentpkg->>agentpkg: whyExcluded() 5-gate filter
    loop over kept files
        agentpkg->>loop: dispatchSubtasks -> RunPerFile
        loop->>client: CompletionsWithCtx()
        client-->>loop: ChatResponse (tool_calls)
        loop->>loop: executeToolCall() per call
        loop-->>agentpkg: collected code_comment calls
    end
    agentpkg->>agentpkg: REVIEW_FILTER_TASK pass
    agentpkg-->>root: comments
    root->>root: 2nd line-resolution pass
    root->>root: render text/JSON/SARIF
```

## `ocr scan` call chain

Same `loop`/`client` core as review; the orchestrator differs:

```
main.go -> root.go -> scan_cmd.go
  -> shared.loadLLMRuntime()
  -> scan.Provider.Enumerate()          (git ls-files or filepath.WalkDir)
  -> scan.Agent.Run()
       -> filterScanItems / filterLargeScans
       -> groupBatches()                (none / by-language / by-directory)
       -> dispatchBatch() per batch, semaphore-bounded within batch
            -> executeSubtask() per file
                 -> llmloop.Runner.RunPerFile()   (shared with review)
       -> maybeRunDedup() per batch
       -> maybeRunProjectSummary()
       -> runner.WaitBackground()        (join async compression before finalize)
       -> session.Finalize()
```

## MCP-augmented tool call

```
llmloop.Runner.executeToolCall(name, args)
  -> tool.Registry.Lookup(name)
       -> built-in (internal/tool/*)               executes directly
       -> MCP-registered (internal/mcp.Provider)    -> mcp.Client.CallTool() -> external server subprocess/endpoint
```

Tool name collisions: built-in/reserved names always win; among MCP servers, first registration wins (`internal/mcp` `RegisterAll`).

## Known gaps / uncertainties:
- The literal span-name construction for `review.run` was not confirmed via direct grep in the telemetry research pass (it may be built from a constant/variable rather than an inline string) — see `docs/operations/OBSERVABILITY.md`.
- Whether `NewClient`/`NewRemoteClient` failure in `internal/mcp` aborts the whole run or just skips that server was not confirmed by direct read of the caller.
