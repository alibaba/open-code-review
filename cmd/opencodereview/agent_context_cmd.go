package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/reviewbundle"
	"github.com/open-code-review/open-code-review/internal/session"
)

type agentContextOptions struct {
	operation     string
	repoDir       string
	bundlePath    string
	path          string
	query         string
	filePatterns  string
	startLine     int
	maxLines      int
	maxGitProcs   int
	caseSensitive bool
	usePerlRegexp bool
	showHelp      bool
	sessionID     string
	bundleIndex   int
}

func runAgentContextForCommand(
	ctx context.Context,
	command string,
	args []string,
	writer io.Writer,
) error {
	started := time.Now()
	if len(args) == 0 {
		printAgentContextUsage(writer, command)
		return nil
	}
	options, err := parseAgentContextFlags(command, args[0], args[1:])
	if err != nil {
		return err
	}
	if options.showHelp {
		printAgentContextUsage(writer, command)
		return nil
	}
	bundleContent, err := reviewbundle.ReadProtocolFile(options.bundlePath)
	if err != nil {
		return fmt.Errorf("open bundle: %w", err)
	}
	bundle, loadErr := reviewbundle.LoadBundle(bytes.NewReader(bundleContent))
	var manifest *reviewbundle.ScanManifest
	if loadErr != nil {
		loadedManifest, manifestErr := reviewbundle.LoadScanManifest(bytes.NewReader(bundleContent))
		if manifestErr != nil {
			return fmt.Errorf("open bundle: %w", manifestErr)
		}
		manifest = loadedManifest
		if options.bundleIndex < 0 && len(manifest.Bundles) == 1 {
			options.bundleIndex = 0
		}
		if options.bundleIndex < 0 || options.bundleIndex >= len(manifest.Bundles) {
			return fmt.Errorf("--bundle-index must select one of %d scan bundles", len(manifest.Bundles))
		}
		bundle = &manifest.Bundles[options.bundleIndex]
	}
	repoDir, _, err := resolveWorkingDir(
		options.repoDir,
		bundle.Target.Mode != reviewbundle.TargetScan,
	)
	if err != nil {
		return err
	}
	if manifest != nil && bundle.Target.Mode == reviewbundle.TargetScan {
		validation := reviewbundle.ValidationResult{Errors: make([]reviewbundle.ValidationNotice, 0)}
		reviewbundle.ValidateScanManifestFreshness(&validation, manifest, bundle.BundleID, repoDir)
		if len(validation.Errors) > 0 {
			return &reviewbundle.ProtocolError{
				Code:    "stale_bundle",
				Message: validation.Errors[0].Message,
			}
		}
	}
	service := reviewbundle.NewContextService(repoDir, bundle, gitcmd.New(options.maxGitProcs))
	result, err := executeContextOperation(ctx, service, options)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode context result: %w", err)
	}
	if _, err := writer.Write(append(encoded, '\n')); err != nil {
		return err
	}
	recordAgentEventBestEffort(
		repoDir,
		options.sessionID,
		bundle.BundleID,
		"context."+options.operation,
		session.AgentEvent{
			ContextCalls: 1,
			DurationMS:   time.Since(started).Milliseconds(),
		},
		false,
	)
	return nil
}

func executeContextOperation(
	ctx context.Context,
	service *reviewbundle.ContextService,
	options agentContextOptions,
) (reviewbundle.ContextResult, error) {
	switch options.operation {
	case "read":
		return service.Read(ctx, options.path, options.startLine, options.maxLines)
	case "find":
		return service.Find(ctx, options.query, options.caseSensitive)
	case "diff":
		return service.Diff(ctx, splitPaths(options.path))
	case "search":
		return service.Search(
			ctx,
			options.query,
			options.caseSensitive,
			options.usePerlRegexp,
			splitPaths(options.filePatterns),
		)
	default:
		return reviewbundle.ContextResult{}, fmt.Errorf("unknown context command: %s", options.operation)
	}
}

func parseAgentContextFlags(command string, operation string, args []string) (agentContextOptions, error) {
	flags := newOcrFlagSet("ocr " + command + " context " + operation)
	options := agentContextOptions{operation: operation, bundleIndex: -1}
	flags.StringVar(&options.repoDir, "repo", "", "repository root")
	flags.StringVar(&options.bundlePath, "bundle", "", "review bundle JSON path")
	flags.StringVar(&options.path, "path", "", "file path or comma-separated paths")
	flags.StringVar(&options.query, "query", "", "file-name or code-search query")
	flags.StringVar(&options.filePatterns, "file-pattern", "", "comma-separated search pathspecs")
	flags.IntVar(&options.startLine, "start-line", 1, "one-based first line")
	flags.IntVar(&options.maxLines, "max-lines", 200, "maximum lines to return")
	flags.IntVar(&options.maxGitProcs, "max-git-procs", 16, "maximum concurrent git subprocesses")
	flags.BoolVar(&options.caseSensitive, "case-sensitive", false, "use case-sensitive matching")
	flags.BoolVar(&options.usePerlRegexp, "perl-regexp", false, "use Perl-compatible search regex")
	flags.StringVar(&options.sessionID, "session-id", "", agentSessionIDHelp(command))
	flags.IntVar(&options.bundleIndex, "bundle-index", -1, "scan manifest bundle index")
	if err := flags.Parse(args); err != nil {
		return options, fmt.Errorf("parse flags: %w", err)
	}
	options.showHelp = flags.showHelp
	if options.showHelp {
		return options, nil
	}
	if options.bundlePath == "" {
		return options, fmt.Errorf("--bundle is required")
	}
	if options.maxGitProcs <= 0 || options.maxLines <= 0 || options.startLine <= 0 {
		return options, fmt.Errorf("--max-git-procs, --start-line, and --max-lines must be positive")
	}
	switch operation {
	case "read", "diff":
		if options.path == "" {
			return options, fmt.Errorf("--path is required for context %s", operation)
		}
	case "find", "search":
		if options.query == "" {
			return options, fmt.Errorf("--query is required for context %s", operation)
		}
	default:
		return options, fmt.Errorf("unknown context command: %s", operation)
	}
	return options, nil
}

func printAgentContextUsage(writer io.Writer, command string) {
	fmt.Fprintln(writer, `Usage:
  ocr `+command+` context read --bundle FILE [--bundle-index N] --path FILE
                   [--repo PATH] [--session-id ID] [--start-line N --max-lines N]
  ocr `+command+` context find --bundle FILE [--bundle-index N] --query NAME
                   [--repo PATH] [--session-id ID]
  ocr `+command+` context diff --bundle FILE [--bundle-index N] --path FILE[,FILE]
                   [--repo PATH] [--session-id ID]
  ocr `+command+` context search --bundle FILE [--bundle-index N] --query TEXT
                   [--repo PATH] [--session-id ID] [--file-pattern PATTERNS]`)
}
