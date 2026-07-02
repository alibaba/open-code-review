package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/reviewbundle"
	"github.com/open-code-review/open-code-review/internal/session"
)

type codexValidateOptions struct {
	repoDir      string
	bundlePath   string
	commentsPath string
	outputPath   string
	maxGitProcs  int
	sessionID    string
	showHelp     bool
}

func runCodexValidateComments(
	ctx context.Context,
	args []string,
	writer io.Writer,
) error {
	return runCodexValidateCommentsForCommand(ctx, "codex", args, writer)
}

func runCodexValidateCommentsForCommand(
	ctx context.Context,
	command string,
	args []string,
	writer io.Writer,
) error {
	started := time.Now()
	options, err := parseCodexValidateFlags(command, args)
	if err != nil {
		return err
	}
	if options.showHelp {
		printCodexValidateUsage(writer, command)
		return nil
	}
	commentsFile, err := os.Open(options.commentsPath)
	if err != nil {
		return fmt.Errorf("open comments: %w", err)
	}
	comments, loadErr := reviewbundle.LoadComments(commentsFile)
	closeErr := commentsFile.Close()
	if loadErr != nil {
		return loadErr
	}
	if closeErr != nil {
		return fmt.Errorf("close comments: %w", closeErr)
	}
	bundle, err := loadCodexBundleByID(options.bundlePath, comments.BundleID)
	if err != nil {
		return err
	}
	repoDir, _, err := resolveWorkingDir(
		options.repoDir,
		bundle.Target.Mode != reviewbundle.TargetScan,
	)
	if err != nil {
		return err
	}
	result := reviewbundle.ValidateComments(
		ctx,
		bundle,
		comments,
		repoDir,
		gitcmd.New(options.maxGitProcs),
	)
	if err := recordCodexEvent(
		repoDir,
		options.sessionID,
		bundle.BundleID,
		"validate",
		session.CodexEvent{
			Files:           comments.Summary.FilesReviewed,
			Findings:        len(comments.Comments),
			Warnings:        len(result.Warnings),
			DurationMS:      time.Since(started).Milliseconds(),
			ValidationValid: &result.Valid,
		},
		false,
	); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode validation result: %w", err)
	}
	if options.outputPath != "" {
		return writePrivateFile(options.outputPath, append(encoded, '\n'))
	}
	_, err = writer.Write(append(encoded, '\n'))
	return err
}

func parseCodexValidateFlags(command string, args []string) (codexValidateOptions, error) {
	flags := newOcrFlagSet("ocr " + command + " validate-comments")
	options := codexValidateOptions{}
	flags.StringVar(&options.repoDir, "repo", "", "root directory of the git repository")
	flags.StringVar(&options.bundlePath, "bundle", "", "review bundle JSON path")
	flags.StringVar(&options.commentsPath, "comments", "", agentCommentsHelp(command))
	flags.StringVar(&options.outputPath, "output", "", "explicit validation output path")
	flags.IntVar(&options.maxGitProcs, "max-git-procs", 16, "maximum concurrent git subprocesses")
	flags.StringVar(&options.sessionID, "session-id", "", agentSessionIDHelp(command))
	if err := flags.Parse(args); err != nil {
		return options, fmt.Errorf("parse flags: %w", err)
	}
	options.showHelp = flags.showHelp
	if options.showHelp {
		return options, nil
	}
	if options.bundlePath == "" || options.commentsPath == "" {
		return options, fmt.Errorf("--bundle and --comments are required")
	}
	if options.maxGitProcs <= 0 {
		return options, fmt.Errorf("--max-git-procs must be greater than zero")
	}
	return options, nil
}

func printCodexValidateUsage(writer io.Writer, command string) {
	fmt.Fprintln(writer, `Usage:
  ocr `+command+` validate-comments --bundle FILE --comments FILE
                              [--repo PATH] [--output FILE]`)
}
