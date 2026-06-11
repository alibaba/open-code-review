package main

import (
	"context"
	"fmt"
	"os"

	"github.com/open-code-review/open-code-review/internal/diff"
	"github.com/open-code-review/open-code-review/internal/publish/gitflic"
)

// runPublish routes `ocr publish <target>` subcommands.
func runPublish(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printPublishUsage()
		return nil
	}
	switch args[0] {
	case "gitflic":
		return runPublishGitflic(args[1:])
	default:
		return fmt.Errorf("unknown publish target: %s\nRun 'ocr publish -h' for usage", args[0])
	}
}

type publishGitflicOptions struct {
	file     string
	owner    string
	project  string
	mr       string
	apiURL   string
	repoDir  string
	from     string
	to       string
	dryRun   bool
	showHelp bool
}

func parsePublishGitflicFlags(args []string) (*publishGitflicOptions, error) {
	opts := &publishGitflicOptions{}
	fs := newOcrFlagSet("publish gitflic")

	defaultFrom := ""
	if target := os.Getenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME"); target != "" {
		defaultFrom = "origin/" + target
	}

	fs.StringVarP(&opts.file, "file", "f", "-", "Review result JSON from 'ocr review --format json' ('-' = stdin)")
	fs.StringVar(&opts.owner, "owner", os.Getenv("CI_PROJECT_NAMESPACE"), "Project owner alias (default: $CI_PROJECT_NAMESPACE)")
	fs.StringVar(&opts.project, "project", os.Getenv("CI_PROJECT_NAME"), "Project alias (default: $CI_PROJECT_NAME)")
	fs.StringVar(&opts.mr, "mr", os.Getenv("CI_MERGE_REQUEST_LOCAL_ID"), "Merge request local id (default: $CI_MERGE_REQUEST_LOCAL_ID)")
	fs.StringVar(&opts.apiURL, "api-url", os.Getenv("GITFLIC_API_URL"), "GitFlic REST API base URL (default: $GITFLIC_API_URL or "+gitflic.DefaultBaseURL+")")
	fs.StringVar(&opts.repoDir, "repo", ".", "Git repository root")
	fs.StringVar(&opts.from, "from", defaultFrom, "Base ref of the reviewed range (default: origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME)")
	fs.StringVar(&opts.to, "to", os.Getenv("CI_COMMIT_SHA"), "Head ref of the reviewed range (default: $CI_COMMIT_SHA)")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "Print discussions instead of posting them")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	opts.showHelp = fs.showHelp
	return opts, nil
}

func runPublishGitflic(args []string) error {
	opts, err := parsePublishGitflicFlags(args)
	if err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if opts.showHelp {
		printPublishUsage()
		return nil
	}

	for _, required := range []struct{ name, value string }{
		{"--owner", opts.owner},
		{"--project", opts.project},
		{"--mr", opts.mr},
		{"--from", opts.from},
		{"--to", opts.to},
	} {
		if required.value == "" {
			return fmt.Errorf("%s is required (not set via flag or CI environment)", required.name)
		}
	}

	token := os.Getenv("GITFLIC_TOKEN")
	if token == "" && !opts.dryRun {
		return fmt.Errorf("GITFLIC_TOKEN environment variable is required")
	}

	result, err := gitflic.LoadReviewResult(opts.file)
	if err != nil {
		return err
	}

	repoDir, err := resolveRepoDir(opts.repoDir)
	if err != nil {
		return fmt.Errorf("resolve repo: %w", err)
	}
	if err := requireGitRepo(repoDir); err != nil {
		return err
	}

	ctx := context.Background()
	diffs, err := diff.NewProvider(repoDir, opts.from, opts.to, nil).GetDiff(ctx)
	if err != nil {
		// Without diffs inline positions cannot be computed; comments will
		// still be posted via the fallback note.
		fmt.Fprintf(os.Stderr, "Warning: cannot read diff %s..%s, posting all comments as fallback: %v\n", opts.from, opts.to, err)
		diffs = nil
	}

	publisher := gitflic.NewPublisher(gitflic.NewClient(opts.apiURL, token))
	stats, err := publisher.Publish(ctx, gitflic.Options{
		Owner:   opts.owner,
		Project: opts.project,
		MRID:    opts.mr,
		DryRun:  opts.dryRun,
	}, result, diffs)
	if err != nil {
		return err
	}

	fmt.Printf("Posted %d inline comment(s), %d via fallback note (%d total).\n",
		stats.Inline, stats.Fallback, len(result.Comments))
	return nil
}

func printPublishUsage() {
	fmt.Println(`Post review results to a code host.

Usage:
  ocr publish gitflic [flags]

Reads the JSON produced by 'ocr review --format json' and posts it onto a
GitFlic merge request: inline discussions where possible, plus a summary note.

Flags:
  -f, --file       Review result JSON file ('-' = stdin, default)
      --owner      Project owner alias (default: $CI_PROJECT_NAMESPACE)
      --project    Project alias (default: $CI_PROJECT_NAME)
      --mr         Merge request local id (default: $CI_MERGE_REQUEST_LOCAL_ID)
      --api-url    GitFlic REST API base URL (default: $GITFLIC_API_URL or https://api.gitflic.ru)
      --repo       Git repository root (default: current dir)
      --from       Base ref of the reviewed range (default: origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME)
      --to         Head ref of the reviewed range (default: $CI_COMMIT_SHA)
      --dry-run    Print discussions instead of posting them

Authentication:
  GITFLIC_TOKEN    GitFlic access token (required unless --dry-run)

Example (inside GitFlic CI merge request pipeline):
  ocr review --from "origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" --to "$CI_COMMIT_SHA" \
    --format json --audience agent | ocr publish gitflic`)
}
