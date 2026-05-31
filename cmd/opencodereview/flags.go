package main

import (
	"flag"
	"fmt"
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
	outputFormat   string
	audience       string // --audience: "human" (default) or "agent"
	background     string // --background: optional requirement context
	concurrency    int
	perFileTimeout int
	preview        bool
	showHelp       bool

	// GitLab platform flags
	platform         string
	mrIID            int
	projectID        string
	baseURL          string
	publish          bool
	prSummary        bool
	clearExisting    bool
	clearInline      bool
	clearSummary     bool
	noInline         bool
	noSummaryComment bool
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
	a.StringVarP(&opts.outputFormat, "format", "f", "text", "output format: text or json")
	a.IntVar(&opts.concurrency, "concurrency", 8, "max concurrent file reviews")
	a.IntVar(&opts.perFileTimeout, "timeout", 10, "concurrent task timeout in minutes")
	a.StringVar(&opts.audience, "audience", "human", "output audience: human (show progress) or agent (summary only)")
	a.StringVarP(&opts.background, "background", "b", "", "optional requirement/business context for the review")
	a.BoolVarP(&opts.preview, "preview", "p", false, "preview which files will be reviewed without running the LLM")

	// GitLab platform flags
	a.StringVar(&opts.platform, "platform", "", "publish platform: gitlab")
	a.IntVar(&opts.mrIID, "mr", 0, "merge request IID (GitLab)")
	a.StringVar(&opts.projectID, "project-id", "", "GitLab project ID or namespace path (e.g. group/sub/project)")
	a.StringVar(&opts.baseURL, "gitlab-base-url", "", "GitLab base URL (default: https://gitlab.com)")
	a.BoolVar(&opts.publish, "publish", false, "publish review results to merge request")
	a.BoolVar(&opts.prSummary, "pr-summary", false, "append/update managed MR description summary")
	a.BoolVar(&opts.clearExisting, "clear-existing", false, "delete OCR comments before publishing")
	a.BoolVar(&opts.clearInline, "clear-inline", false, "delete OCR inline comments and exit")
	a.BoolVar(&opts.clearSummary, "clear-summary", false, "delete OCR summary/request-changes comments and exit")
	a.BoolVar(&opts.noInline, "no-inline", false, "skip inline discussions during publish")
	a.BoolVar(&opts.noSummaryComment, "no-summary-comment", false, "skip summary note during publish")

	if err := a.Parse(args); err != nil {
		return opts, fmt.Errorf("parse flags: %w", err)
	}

	opts.showHelp = a.showHelp
	if opts.showHelp {
		return opts, nil
	}

	if opts.platform != "" && opts.platform != "gitlab" {
		return opts, fmt.Errorf("unsupported platform %q: only 'gitlab' is supported", opts.platform)
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

	switch opts.audience {
	case "human", "agent":
	default:
		return opts, fmt.Errorf("invalid --audience value %q: must be 'human' or 'agent'", opts.audience)
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

  # Output JSON format
  ocr review --format json
  ocr review -f json

  # Agent mode (summary only, no progress lines)
  ocr review --audience agent

  # Preview which files will be reviewed
  ocr review --preview
  ocr review -c abc123 -p

  # Publish to GitLab MR
  GITLAB_TOKEN=... ocr review --platform gitlab --project-id 456 --mr 123 --publish

  # Clear OCR comments from GitLab MR
  GITLAB_TOKEN=... ocr review --platform gitlab --project-id 456 --mr 123 --clear-inline

Flags:
  --audience string          output audience: human (show progress) or agent (summary only) (default "human")
  -b, --background string    optional requirement/business context for the review
  -c, --commit string        single commit hash or tag to review (vs its parent)
  -f, --format string        output format: text or json (default "text")
  --concurrency int          max concurrent file reviews (default 8)
  --from string              source ref to start diff from (e.g., 'main')
  -p, --preview              preview which files will be reviewed without running the LLM
  --repo string              root directory of the git repository (default: current dir)
  --rule string              path to JSON file with system review rules
  --timeout int              concurrent task timeout in minutes (default 10)
  --to string                target ref to end diff at (e.g., 'feature-branch')
  --tools string             path to JSON tools config file (default: embedded)

GitLab flags:
  --platform string          publish platform: gitlab
  --mr int                   merge request IID (GitLab)
  --project-id string        GitLab project ID or namespace path (e.g. group/sub/project)
  --gitlab-base-url string   GitLab base URL (default: https://gitlab.com)
  --publish                  publish review results to merge request
  --pr-summary               append/update managed MR description summary
  --clear-existing           delete OCR comments before publishing
  --clear-inline             delete OCR inline comments and exit
  --clear-summary            delete OCR summary/request-changes comments and exit
  --no-inline                skip inline discussions during publish
  --no-summary-comment       skip summary note during publish`)
}

// --- config subcommand ---

type configAction struct {
	subCmd string // "set"
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
	default:
		return configAction{}, fmt.Errorf("unknown config sub-command: %s\nAvailable: set", subCmd)
	}
}

func printConfigUsage() {
	fmt.Println(`Configuration management.

Usage:
  ocr config set <key> <value>

Examples:
  ocr config set llm.url https://xx/v1/openai/chat/completions
  ocr config set llm.auth_token xxxxxxxxxx
  ocr config set llm.model claude-opus-4-6
  ocr config set llm.extra_body '{"thinking":{"type":"disabled"}}'
  ocr config set language English
  ocr config set telemetry.enabled true

Supported keys: llm.url, llm.auth_token, llm.model, llm.use_anthropic, llm.extra_body, language, telemetry.enabled, telemetry.exporter, telemetry.otlp_endpoint, telemetry.content_logging`)
}
