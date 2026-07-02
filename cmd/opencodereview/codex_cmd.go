package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/reviewbundle"
	scanpkg "github.com/open-code-review/open-code-review/internal/scan"
	"github.com/open-code-review/open-code-review/internal/session"
	"github.com/open-code-review/open-code-review/internal/stdout"
)

type codexPrepareOptions struct {
	repoDir        string
	rulePath       string
	from           string
	to             string
	commit         string
	excludes       string
	includes       string
	paths          string
	format         string
	outputPath     string
	maxBundleBytes int
	maxFileBytes   int
	maxTokenBudget int
	maxGitProcs    int
	batchStrategy  string
	batchSize      int
	sessionID      string
	scan           bool
	split          bool
	preview        bool
	showHelp       bool
}

func runCodex(args []string) error {
	return runCodexWithWriter(args, stdout.Writer())
}

func runCodexWithWriter(args []string, writer io.Writer) error {
	return runAgentCommandsWithWriter("codex", args, writer)
}

func runAgent(args []string) error {
	return runAgentWithWriter(args, stdout.Writer())
}

func runAgentWithWriter(args []string, writer io.Writer) error {
	return runAgentCommandsWithWriter("agent", args, writer)
}

func runAgentCommandsWithWriter(command string, args []string, writer io.Writer) error {
	if len(args) == 0 {
		printAgentCommandUsage(writer, command)
		return nil
	}
	switch args[0] {
	case "prepare":
		options, err := parseCodexPrepareFlags(args[1:])
		if err != nil {
			return err
		}
		if options.showHelp {
			printCodexPrepareUsage(writer)
			return nil
		}
		return executeCodexPrepare(context.Background(), options, writer)
	case "validate-comments":
		return runCodexValidateComments(context.Background(), args[1:], writer)
	case "report":
		return runCodexReport(args[1:], writer)
	case "context":
		return runCodexContext(context.Background(), args[1:], writer)
	case "-h", "--help":
		printAgentCommandUsage(writer, command)
		return nil
	default:
		return fmt.Errorf("unknown %s command: %s", command, args[0])
	}
}

func parseCodexPrepareFlags(args []string) (codexPrepareOptions, error) {
	flags := newOcrFlagSet("ocr codex prepare")
	options := codexPrepareOptions{}
	flags.StringVar(&options.repoDir, "repo", "", "root directory of the git repository")
	flags.StringVar(&options.rulePath, "rule", "", "path to a custom review rule file")
	flags.StringVar(&options.from, "from", "", "source ref for a range review")
	flags.StringVar(&options.to, "to", "", "target ref for a range review")
	flags.StringVarP(&options.commit, "commit", "c", "", "single commit to review")
	flags.StringVar(&options.excludes, "exclude", "", "comma-separated path patterns to exclude")
	flags.StringVar(&options.includes, "include", "", "comma-separated path patterns to include")
	flags.StringVar(&options.paths, "path", "", "comma-separated scan files or directories")
	flags.StringVarP(&options.format, "format", "f", "json", "output format: json")
	flags.StringVar(&options.outputPath, "output", "", "explicit bundle output path")
	flags.IntVar(
		&options.maxBundleBytes,
		"max-bundle-bytes",
		int(reviewbundle.DefaultMaxBundleBytes),
		"maximum encoded bundle size",
	)
	flags.IntVar(&options.maxGitProcs, "max-git-procs", 16, "maximum concurrent git subprocesses")
	flags.IntVar(
		&options.maxFileBytes,
		"max-file-size-bytes",
		int(scanpkg.DefaultMaxFileSizeBytes),
		"maximum scan file size",
	)
	flags.IntVar(&options.maxTokenBudget, "max-tokens-budget", 0, "hard scan token estimate budget")
	flags.StringVar(&options.batchStrategy, "batch", "by-language", "scan grouping strategy")
	flags.IntVar(&options.batchSize, "batch-size", 50, "maximum files per scan bundle")
	flags.StringVar(&options.sessionID, "session-id", "", "explicit Codex-owned session ID")
	flags.BoolVar(&options.scan, "scan", false, "prepare full-file scan bundles")
	flags.BoolVar(&options.split, "split", false, "emit a manifest of size-bounded diff bundles")
	flags.BoolVarP(&options.preview, "preview", "p", false, "show the file manifest without patches")
	if err := flags.Parse(args); err != nil {
		return options, fmt.Errorf("parse flags: %w", err)
	}
	options.showHelp = flags.showHelp
	if options.showHelp {
		return options, nil
	}
	if err := validateCodexPrepareOptions(options); err != nil {
		return options, err
	}
	return options, nil
}

func validateCodexPrepareOptions(options codexPrepareOptions) error {
	modeCount := 0
	if options.from != "" || options.to != "" {
		modeCount++
	}
	if options.commit != "" {
		modeCount++
	}
	if modeCount > 1 {
		return fmt.Errorf("only one review mode allowed (--from/--to or --commit)")
	}
	if options.scan && modeCount > 0 {
		return fmt.Errorf("--scan cannot be combined with --from, --to, or --commit")
	}
	if options.scan && options.split {
		return fmt.Errorf("--split is for diff targets; scan mode is already partitioned")
	}
	if options.from != "" && options.to == "" {
		return fmt.Errorf("--to is required when --from is specified")
	}
	if options.to != "" && options.from == "" {
		return fmt.Errorf("--from is required when --to is specified")
	}
	if options.format != "json" {
		return fmt.Errorf("invalid --format value %q: must be json", options.format)
	}
	if options.maxBundleBytes <= 0 {
		return fmt.Errorf("--max-bundle-bytes must be greater than zero")
	}
	if options.maxGitProcs <= 0 {
		return fmt.Errorf("--max-git-procs must be greater than zero")
	}
	if options.maxFileBytes <= 0 || options.maxTokenBudget < 0 || options.batchSize <= 0 {
		return fmt.Errorf("--max-file-size-bytes and --batch-size must be positive; --max-tokens-budget cannot be negative")
	}
	switch options.batchStrategy {
	case "none", "by-language", "by-directory":
	default:
		return fmt.Errorf("--batch must be none, by-language, or by-directory")
	}
	if options.preview && options.outputPath != "" {
		return fmt.Errorf("--output cannot be used with --preview")
	}
	return nil
}

func executeCodexPrepare(
	ctx context.Context,
	options codexPrepareOptions,
	writer io.Writer,
) error {
	started := time.Now()
	repoDir, _, err := resolveWorkingDir(options.repoDir, !options.scan)
	if err != nil {
		return err
	}
	resolver, fileFilter, err := rules.NewResolver(repoDir, options.rulePath)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}
	excludePatterns := splitPaths(options.excludes)
	if len(excludePatterns) > 0 {
		if fileFilter == nil {
			fileFilter = &rules.FileFilter{}
		}
		fileFilter.Exclude = append(fileFilter.Exclude, excludePatterns...)
	}
	includePatterns := splitPaths(options.includes)
	if len(includePatterns) > 0 {
		if fileFilter == nil {
			fileFilter = &rules.FileFilter{}
		}
		fileFilter.Include = append(fileFilter.Include, includePatterns...)
	}
	if options.scan {
		return executeCodexScanPrepare(
			ctx,
			options,
			repoDir,
			resolver,
			fileFilter,
			writer,
		)
	}
	if options.split {
		return executeCodexDiffPartition(
			ctx,
			options,
			repoDir,
			resolver,
			fileFilter,
			writer,
		)
	}

	bundle, encoded, err := reviewbundle.Prepare(ctx, reviewbundle.PrepareOptions{
		RepoDir: repoDir,
		Target: reviewbundle.TargetSpec{
			From:   options.from,
			To:     options.to,
			Commit: options.commit,
		},
		Resolver:      resolver,
		FileFilter:    fileFilter,
		GitRunner:     gitcmd.New(options.maxGitProcs),
		MaxBundleSize: int64(options.maxBundleBytes),
	})
	if err != nil {
		return fmt.Errorf("prepare Codex review bundle: %w", err)
	}
	if err := recordCodexEvent(
		repoDir,
		options.sessionID,
		bundle.BundleID,
		"prepare",
		session.CodexEvent{
			Files:      bundle.Summary.ReviewableFiles,
			Warnings:   len(bundle.Warnings),
			DurationMS: time.Since(started).Milliseconds(),
		},
		false,
	); err != nil {
		return err
	}
	if options.preview {
		writeCodexPreview(writer, bundle)
		return nil
	}
	if options.outputPath != "" {
		return writePrivateFile(options.outputPath, encoded)
	}
	if _, err := writer.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write review bundle: %w", err)
	}
	return nil
}

func executeCodexDiffPartition(
	ctx context.Context,
	options codexPrepareOptions,
	repoDir string,
	resolver rules.Resolver,
	fileFilter *rules.FileFilter,
	writer io.Writer,
) error {
	started := time.Now()
	manifest, encoded, err := reviewbundle.PreparePartitioned(
		ctx,
		reviewbundle.PrepareOptions{
			RepoDir: repoDir,
			Target: reviewbundle.TargetSpec{
				From: options.from, To: options.to, Commit: options.commit,
			},
			Resolver:      resolver,
			FileFilter:    fileFilter,
			GitRunner:     gitcmd.New(options.maxGitProcs),
			MaxBundleSize: int64(options.maxBundleBytes),
		},
	)
	if err != nil {
		return fmt.Errorf("prepare partitioned Codex review: %w", err)
	}
	if err := recordCodexEvent(
		repoDir,
		options.sessionID,
		manifest.ManifestID,
		"prepare.diff_manifest",
		session.CodexEvent{
			Files:      manifest.Summary.ReviewableFiles,
			Warnings:   len(manifest.Warnings),
			DurationMS: time.Since(started).Milliseconds(),
		},
		false,
	); err != nil {
		return err
	}
	if options.preview {
		fmt.Fprintf(
			writer,
			"Codex diff manifest preview: %d files, %d bundle(s)\n",
			manifest.Summary.TotalFiles,
			len(manifest.Bundles),
		)
		return nil
	}
	if options.outputPath != "" {
		return writePrivateFile(options.outputPath, encoded)
	}
	_, err = writer.Write(append(encoded, '\n'))
	return err
}

func executeCodexScanPrepare(
	ctx context.Context,
	options codexPrepareOptions,
	repoDir string,
	resolver rules.Resolver,
	fileFilter *rules.FileFilter,
	writer io.Writer,
) error {
	started := time.Now()
	manifest, encoded, err := reviewbundle.PrepareScan(ctx, reviewbundle.ScanOptions{
		RepoDir:          repoDir,
		Paths:            splitPaths(options.paths),
		Resolver:         resolver,
		FileFilter:       fileFilter,
		GitRunner:        gitcmd.New(options.maxGitProcs),
		MaxFileSizeBytes: int64(options.maxFileBytes),
		MaxTokenBudget:   int64(options.maxTokenBudget),
		MaxBundleSize:    int64(options.maxBundleBytes),
		BatchStrategy:    options.batchStrategy,
		BatchSize:        options.batchSize,
	})
	if err != nil {
		return fmt.Errorf("prepare Codex scan manifest: %w", err)
	}
	if err := recordCodexEvent(
		repoDir,
		options.sessionID,
		manifest.ManifestID,
		"prepare.scan",
		session.CodexEvent{
			Files:      manifest.Summary.ReviewableFiles,
			Warnings:   len(manifest.Warnings),
			Partial:    manifest.Partial,
			DurationMS: time.Since(started).Milliseconds(),
		},
		false,
	); err != nil {
		return err
	}
	if options.preview {
		fmt.Fprintf(
			writer,
			"Codex scan preview: %d files (%d included, %d skipped), %d bundle(s), ~%d tokens\n",
			manifest.Summary.TotalFiles,
			manifest.Summary.ReviewableFiles,
			manifest.Summary.ExcludedFiles,
			len(manifest.Bundles),
			manifest.EstimatedTokens,
		)
		for _, skipped := range manifest.SkippedFiles {
			fmt.Fprintf(writer, "  skip:%-16s %s\n", skipped.Reason, sanitizeTerminal(skipped.Path))
		}
		return nil
	}
	if options.outputPath != "" {
		return writePrivateFile(options.outputPath, encoded)
	}
	_, err = writer.Write(append(encoded, '\n'))
	return err
}

func recordCodexEvent(
	repoDir string,
	sessionID string,
	bundleID string,
	event string,
	details session.CodexEvent,
	finalize bool,
) error {
	if sessionID == "" {
		return nil
	}
	recorder, err := session.OpenCodexRecorder(repoDir, sessionID, bundleID)
	if err != nil {
		return fmt.Errorf("open Codex session: %w", err)
	}
	if finalize {
		return recorder.Finalize(details)
	}
	return recorder.Record(event, details)
}

func writePrivateFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open bundle output %s: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("restrict bundle output %s: %w", path, err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write bundle output %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close bundle output %s: %w", path, err)
	}
	return nil
}

func writeCodexPreview(writer io.Writer, bundle *reviewbundle.Bundle) {
	fmt.Fprintf(
		writer,
		"Codex review bundle preview: %d files (%d reviewable, %d excluded), +%d -%d\n",
		bundle.Summary.TotalFiles,
		bundle.Summary.ReviewableFiles,
		bundle.Summary.ExcludedFiles,
		bundle.Summary.Insertions,
		bundle.Summary.Deletions,
	)
	for _, file := range bundle.Files {
		state := "review"
		if !file.Reviewable {
			state = "exclude:" + string(file.ExcludeReason)
		}
		fmt.Fprintf(
			writer,
			"  %-8s %-10s %s (+%d -%d)\n",
			state,
			file.Status,
			sanitizeTerminal(file.Path),
			file.Insertions,
			file.Deletions,
		)
	}
}

func printCodexUsage(writer io.Writer) {
	printAgentCommandUsage(writer, "codex")
}

func printAgentCommandUsage(writer io.Writer, command string) {
	fmt.Fprintln(writer, `Usage:
  ocr `+command+` prepare [options]
  ocr `+command+` validate-comments --bundle FILE --comments FILE [options]
  ocr `+command+` report --bundle FILE --comments FILE [options]
  ocr `+command+` context read|find|diff|search --bundle FILE [options]

Commands:
  prepare             Build deterministic review input without invoking an OCR LLM
  validate-comments   Validate agent findings against immutable bundle evidence
  report              Render validated agent findings as Markdown, text, or JSON
  context             Read target-aware repository context without an LLM`)
}

func printCodexPrepareUsage(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  ocr codex prepare [--repo PATH] [--from REF --to REF | --commit REF]
                    [--rule PATH] [--exclude PATTERNS] [--preview]
                    [--output PATH] [--max-bundle-bytes N] [--split]
  ocr codex prepare --scan [--repo PATH] [--path PATHS]
                    [--include PATTERNS] [--exclude PATTERNS]
                    [--batch none|by-language|by-directory] [--batch-size N]
                    [--max-tokens-budget N] [--max-file-size-bytes N]`)
}
