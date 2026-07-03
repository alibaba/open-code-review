package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/open-code-review/open-code-review/internal/reviewbundle"
	"github.com/open-code-review/open-code-review/internal/session"
)

type codexReportOptions struct {
	bundlePath     string
	commentsPath   string
	validationPath string
	outputPath     string
	format         string
	repoDir        string
	sessionID      string
	showHelp       bool
}

func runCodexReportForCommand(command string, args []string, writer io.Writer) error {
	started := time.Now()
	options, err := parseCodexReportFlags(command, args)
	if err != nil {
		return err
	}
	if options.showHelp {
		printCodexReportUsage(writer, command)
		return nil
	}
	bundle, comments, err := loadCodexInputs(options.bundlePath, options.commentsPath)
	if err != nil {
		return err
	}
	validation, err := loadValidationResult(options.validationPath)
	if err != nil {
		return err
	}
	report, err := reviewbundle.RenderReport(bundle, comments, reviewbundle.ReportOptions{
		Format:     options.format,
		Validation: validation,
	})
	if err != nil {
		return err
	}
	if options.sessionID != "" {
		repoDir, _, resolveErr := resolveWorkingDir(options.repoDir, false)
		if resolveErr != nil {
			return resolveErr
		}
		paths := make([]string, 0, len(bundle.Files))
		for _, file := range bundle.Files {
			if file.Reviewable {
				paths = append(paths, file.Path)
			}
		}
		valid := validation == nil || validation.Valid
		if err := recordCodexEvent(
			repoDir,
			options.sessionID,
			bundle.BundleID,
			"report",
			session.CodexEvent{
				Files:           comments.Summary.FilesReviewed,
				Findings:        len(comments.Comments),
				Warnings:        len(comments.Warnings),
				DurationMS:      time.Since(started).Milliseconds(),
				ValidationValid: &valid,
				FilesReviewed:   paths,
			},
			true,
		); err != nil {
			return err
		}
	}
	if options.outputPath != "" {
		return writePrivateFile(options.outputPath, report)
	}
	_, err = writer.Write(report)
	return err
}

func parseCodexReportFlags(command string, args []string) (codexReportOptions, error) {
	flags := newOcrFlagSet("ocr " + command + " report")
	options := codexReportOptions{}
	flags.StringVar(&options.bundlePath, "bundle", "", "review bundle JSON path")
	flags.StringVar(&options.commentsPath, "comments", "", agentCommentsHelp(command))
	flags.StringVar(&options.validationPath, "validation", "", "optional validation result JSON path")
	flags.StringVar(&options.outputPath, "output", "", "explicit report output path")
	flags.StringVarP(&options.format, "format", "f", "markdown", "markdown, text, or json")
	flags.StringVar(&options.repoDir, "repo", "", "repository root for session persistence")
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
	switch options.format {
	case "markdown", "text", "json":
	default:
		return options, fmt.Errorf("--format must be markdown, text, or json")
	}
	return options, nil
}

func loadCodexInputs(bundlePath, commentsPath string) (*reviewbundle.Bundle, *reviewbundle.Comments, error) {
	commentsFile, err := os.Open(commentsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open comments: %w", err)
	}
	comments, loadErr := reviewbundle.LoadComments(commentsFile)
	closeErr := commentsFile.Close()
	if loadErr != nil {
		return nil, nil, loadErr
	}
	if closeErr != nil {
		return nil, nil, fmt.Errorf("close comments: %w", closeErr)
	}
	bundle, err := loadCodexBundleByID(bundlePath, comments.BundleID)
	if err != nil {
		return nil, nil, err
	}
	return bundle, comments, nil
}

func loadCodexBundleByID(path, bundleID string) (*reviewbundle.Bundle, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bundle: %w", err)
	}
	bundle, bundleErr := reviewbundle.LoadBundle(bytes.NewReader(content))
	if bundleErr == nil {
		if bundle.BundleID != bundleID {
			return nil, fmt.Errorf(
				"bundle at %q has bundle_id %q, comments require %q",
				path,
				bundle.BundleID,
				bundleID,
			)
		}
		return bundle, nil
	}
	manifest, manifestErr := reviewbundle.LoadScanManifest(bytes.NewReader(content))
	if manifestErr != nil {
		return nil, bundleErr
	}
	for index := range manifest.Bundles {
		if manifest.Bundles[index].BundleID == bundleID {
			return &manifest.Bundles[index], nil
		}
	}
	return nil, fmt.Errorf("bundle_id %q is not present in scan manifest", bundleID)
}

func loadValidationResult(path string) (*reviewbundle.ValidationResult, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open validation result: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var result reviewbundle.ValidationResult
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode validation result: %w", err)
	}
	return &result, nil
}

func printCodexReportUsage(writer io.Writer, command string) {
	fmt.Fprintln(writer, `Usage:
  ocr `+command+` report --bundle FILE --comments FILE
                   [--validation FILE] [--format markdown|text|json]
                   [--output FILE] [--repo PATH] [--session-id ID]`)
}
