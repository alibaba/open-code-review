package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

// --- custom flag set that supports short flags (-c, -f etc.) ---

type ocrFlagSet struct {
	fs       *flag.FlagSet
	shortMap map[string]string // maps short key "c" -> full name "commit"
	showHelp bool
}

func newOcrFlagSet(name string) *ocrFlagSet {
	return &ocrFlagSet{
		fs:       flag.NewFlagSet(name, flag.ContinueOnError),
		shortMap: make(map[string]string),
	}
}

// StringVarP registers --name with optional short form -s.
func (a *ocrFlagSet) StringVarP(p *string, name, shorthand string, value, usage string) {
	suffix := ""
	if shorthand != "" {
		a.shortMap[shorthand] = name
		suffix = fmt.Sprintf(" (shorthand: -%s)", shorthand)
	}
	a.fs.StringVar(p, name, value, usage+suffix)
}

// BoolVarP registers --name with optional short form -s.
func (a *ocrFlagSet) BoolVarP(p *bool, name, shorthand string, value bool, usage string) {
	suffix := ""
	if shorthand != "" {
		a.shortMap[shorthand] = name
		suffix = fmt.Sprintf(" (shorthand: -%s)", shorthand)
	}
	a.fs.BoolVar(p, name, value, usage+suffix)
}

func (a *ocrFlagSet) StringVar(p *string, name string, value string, usage string) {
	a.fs.StringVar(p, name, value, usage)
}

func (a *ocrFlagSet) BoolVar(p *bool, name string, value bool, usage string) {
	a.fs.BoolVar(p, name, value, usage)
}

func (a *ocrFlagSet) IntVar(p *int, name string, value int, usage string) {
	a.fs.IntVar(p, name, value, usage)
}

func (a *ocrFlagSet) DurationVar(p *time.Duration, name string, value time.Duration, usage string) {
	a.fs.DurationVar(p, name, value, usage)
}

func (a *ocrFlagSet) PrintDefaults() {
	a.fs.PrintDefaults()
}

func (a *ocrFlagSet) Parse(arguments []string) error {
	expanded := expandShortFlags(arguments, a.shortMap)

	for _, arg := range expanded {
		if arg == "-h" || arg == "--help" {
			a.showHelp = true
			return nil
		}
	}

	return a.fs.Parse(expanded)
}

// expandShortFlags replaces standalone -X args with their long equivalents.
// Only triggers when the arg is exactly -N (single char after dash).
func expandShortFlags(args []string, shortMap map[string]string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if len(arg) == 2 && arg[0] == '-' && arg[1] != '-' {
			key := string(arg[1])
			if full, ok := shortMap[key]; ok {
				out = append(out, "--"+full)
				continue
			}
		}
		out = append(out, arg)
	}
	return out
}

// --- review subcommand options ---

type reviewOptions struct {
	toolConfigPath string
	rulePath       string
	repoDir        string
	from           string
	to             string
	commit         string
	resume         string
	excludes       string // --exclude: comma-separated gitignore-style patterns
	outputFormat   string
	audience       string // --audience: "human" (default) or "agent"
	background     string // --background: optional requirement context
	backgroundFile string // --background-file: path to a Markdown file used as background
	model          string // --model: override resolved LLM model for this review
	concurrency    int
	perFileTimeout int
	maxTools       int
	maxGitProcs    int
	preview        bool
	showHelp       bool
}

func parseReviewFlags(args []string) (reviewOptions, error) {
	a := newOcrFlagSet("ocr review")

	opts := reviewOptions{}

	a.StringVar(&opts.toolConfigPath, "tools", "", "path to JSON tools config file (default: embedded)")
	a.StringVar(&opts.rulePath, "rule", "", "path to JSON file with system review rules")
	a.StringVar(&opts.repoDir, "repo", "", "root directory of the git repository (default: current dir)")
	a.StringVar(&opts.from, "from", "", "source ref to start diff from (e.g., 'main')")
	a.StringVar(&opts.to, "to", "", "target ref to end diff at (e.g., 'feature-branch')")
	a.StringVarP(&opts.commit, "commit", "c", "", "single commit hash or tag to review (vs its parent)")
	a.StringVar(&opts.resume, "resume", "", "resume from a previous review session id")
	a.StringVar(&opts.excludes, "exclude", "", "comma-separated gitignore-style patterns to exclude; merged with rule.json excludes")
	a.StringVarP(&opts.outputFormat, "format", "f", "text", "output format: text or json")
	a.IntVar(&opts.concurrency, "concurrency", 8, "max concurrent file reviews")
	a.IntVar(&opts.perFileTimeout, "timeout", 10, "concurrent task timeout in minutes")
	a.StringVar(&opts.audience, "audience", "human", "output audience: human (show progress) or agent (summary only)")
	a.StringVarP(&opts.background, "background", "b", "", "optional requirement/business context for the review")
	a.StringVarP(&opts.backgroundFile, "background-file", "B", "", "optional requirement/business context from a Markdown file (combined with --background; inline value appears first when both are set)")
	a.StringVar(&opts.model, "model", "", "override LLM model for this review (e.g., claude-opus-4-6)")
	a.IntVar(&opts.maxTools, "max-tools", 0, "max tool call rounds per file (0 = template default; min 10)")
	a.IntVar(&opts.maxGitProcs, "max-git-procs", 16, "max concurrent git subprocesses")
	a.BoolVarP(&opts.preview, "preview", "p", false, "preview which files will be reviewed without running the LLM")

	if err := a.Parse(args); err != nil {
		return opts, fmt.Errorf("parse flags: %w", err)
	}

	opts.showHelp = a.showHelp
	if opts.showHelp {
		return opts, nil
	}

	modeCount := 0
	if opts.from != "" || opts.to != "" {
		modeCount++
	}
	if opts.commit != "" {
		modeCount++
	}
	// modeCount == 0 → workspace mode (no error, allowed)
	if modeCount > 1 {
		return opts, fmt.Errorf("only one review mode allowed (--from/--to or --commit)")
	}
	if opts.from != "" && opts.to == "" {
		return opts, fmt.Errorf("--to is required when --from is specified")
	}
	if opts.to != "" && opts.from == "" {
		return opts, fmt.Errorf("--from is required when --to is specified")
	}
	if opts.preview && opts.resume != "" {
		return opts, fmt.Errorf("--preview and --resume cannot be used together")
	}

	switch opts.audience {
	case "human", "agent":
	default:
		return opts, fmt.Errorf("invalid --audience value %q: must be 'human' or 'agent'", opts.audience)
	}

	const minMaxTools = 10
	if opts.maxTools < 0 {
		return opts, fmt.Errorf("--max-tools must be a non-negative integer (0 means use template default)")
	}
	if opts.maxTools > 0 && opts.maxTools < minMaxTools {
		fmt.Fprintf(os.Stderr, "[ocr] --max-tools %d is below minimum %d, using %d\n", opts.maxTools, minMaxTools, minMaxTools)
		opts.maxTools = minMaxTools
	}

	if opts.maxGitProcs < 0 {
		return opts, fmt.Errorf("--max-git-procs must be a non-negative integer (0 means use default 16)")
	}

	return opts, nil
}

func printReviewUsage() {
	fmt.Println(`OpenCodeReview - AI-Powered Code Review CLI

Usage:
  ocr review [flags]
  ocr r [flags]                (alias)

Examples:
  # Review staged + unstaged + untracked changes in current workspace
  ocr review

  # Review a branch against its base (merge-base mode)
  ocr review --from master --to dev-ref

  # Review a specific commit
  ocr review --commit abc123
  ocr review -c abc123

  # Resume a previous range review
  ocr review --from master --to dev-ref --resume <session-id>

  # Output JSON format
  ocr review --format json
  ocr review -f json

  # Agent mode (summary only, no progress lines)
  ocr review --audience agent

  # Preview which files will be reviewed
  ocr review --preview
  ocr review -c abc123 -p

  # Provide requirement/business context inline, from a Markdown file, or both
  ocr review --background "Adding rate limiting to the login API"
  ocr review --background-file ./docs/requirements.md
  ocr review --background "Focus on auth" --background-file ./docs/requirements.md

Flags:
  --audience string             output audience: human (show progress) or agent (summary only) (default "human")
  -b, --background string       optional requirement/business context for the review
  -B, --background-file string  path to a Markdown file used as review background (combined with --background; inline value appears first when both are set)
  -c, --commit string           single commit hash or tag to review (vs its parent)
  -f, --format string           output format: text or json (default "text")
  --concurrency int             max concurrent file reviews (default 8)
  --max-git-procs int           max concurrent git subprocesses (default 16)
  --from string                 source ref to start diff from (e.g., 'main')
  --max-tools int               max tool call rounds per file (0 = template default; min 10)
  --model string                override LLM model for this review (e.g., claude-opus-4-6)
  -p, --preview                 preview which files will be reviewed without running the LLM
  --repo string                 root directory of the git repository (default: current dir)
  --resume string               resume from a previous review session id
  --rule string                 path to JSON file with system review rules
  --timeout int                 concurrent task timeout in minutes (default 10)
  --to string                   target ref to end diff at (e.g., 'feature-branch')
  --tools string                path to JSON tools config file (default: embedded)`)
}

// --- config subcommand ---

type configAction struct {
	subCmd string // "set", "unset"
	key    string
	value  string
}

func parseConfigArgs(args []string) (configAction, error) {
	if len(args) == 0 {
		return configAction{}, fmt.Errorf("usage: ocr config set <key> <value>\ne.g., ocr config set llm.model claude-opus-4-6")
	}

	subCmd := args[0]
	switch subCmd {
	case "set":
		if len(args) < 3 {
			return configAction{}, fmt.Errorf("usage: ocr config set <key> <value>\ne.g., ocr config set llm.model claude-opus-4-6")
		}
		return configAction{
			subCmd: "set",
			key:    args[1],
			value:  args[2],
		}, nil
	case "unset":
		if len(args) < 2 {
			return configAction{}, fmt.Errorf("usage: ocr config unset custom_providers.<name>\ne.g., ocr config unset custom_providers.my-gateway")
		}
		return configAction{
			subCmd: "unset",
			key:    args[1],
		}, nil
	default:
		return configAction{}, fmt.Errorf("unknown config sub-command: %s\nAvailable: set, unset, provider, model", subCmd)
	}
}

func printConfigUsage() {
	fmt.Println(`Configuration management.

Usage:
  ocr config set <key> <value>
  ocr config unset custom_providers.<name>  Delete a custom provider
  ocr config unset mcp_servers.<name>       Delete an MCP server
  ocr config provider                       Interactive provider setup
  ocr config model                          Interactive model selection

Examples:
  # Provider setup (interactive)
  ocr config provider
  ocr config model

  # Provider setup (non-interactive)
  ocr config set provider anthropic
  ocr config set model claude-opus-4-6
  # Set API key via environment variable (recommended) or config:
  # export ANTHROPIC_API_KEY=sk-ant-xxx
  ocr config set providers.anthropic.api_key "$ANTHROPIC_API_KEY"

  # Custom provider
  ocr config set provider my-gateway
  ocr config set custom_providers.my-gateway.url https://gateway.internal.com/v1
  ocr config set custom_providers.my-gateway.protocol openai
  ocr config set custom_providers.my-gateway.model llama-3-70b
  ocr config set custom_providers.my-gateway.models '["llama-3-70b","llama-3-8b"]'
  ocr config set custom_providers.my-gateway.api_key "$MY_API_KEY"

  # Delete a custom provider
  ocr config unset custom_providers.my-gateway

  # MCP server configuration (stdio transport)
  ocr config set mcp_servers.codegraph.command npx
  ocr config set mcp_servers.codegraph.args '["-y","@anthropic/codegraph-mcp"]'
  ocr config set mcp_servers.codegraph.env '["CODEGRAPH_TOKEN=xxx"]'

  # Delete an MCP server
  ocr config unset mcp_servers.codegraph

  # Legacy endpoint configuration
  ocr config set llm.url https://xx/v1/openai/chat/completions
  ocr config set llm.auth_token xxxxxxxxxx
  ocr config set llm.auth_header x-api-key
  ocr config set llm.model claude-opus-4-6
  ocr config set llm.extra_body '{"thinking":{"type":"disabled"}}'
  ocr config set language English
  ocr config set telemetry.enabled true

Supported keys: provider, model, providers.<name>.<field>, custom_providers.<name>.<field>, mcp_servers.<name>.<field>, llm.url, llm.auth_token, llm.auth_header, llm.model, llm.use_anthropic, llm.extra_body, llm.extra_headers, language, telemetry.enabled, telemetry.exporter, telemetry.otlp_endpoint, telemetry.content_logging
Provider fields: api_key, url, protocol, model, models, auth_header, extra_body, extra_headers
MCP server fields: command, args, env, tools, setup`)
}
