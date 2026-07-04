package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/reviewbundle"
)

func TestAgentPrepareEmitsBundleWithoutLLMConfiguration(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	t.Setenv("HOME", t.TempDir())

	var output bytes.Buffer
	err := runAgentWithWriter([]string{"prepare", "--repo", repository}, &output)
	if err != nil {
		t.Fatalf("runAgentWithWriter() error = %v", err)
	}
	var bundle reviewbundle.Bundle
	if err := json.Unmarshal(output.Bytes(), &bundle); err != nil {
		t.Fatalf("decode stdout bundle: %v\n%s", err, output.String())
	}
	if bundle.SchemaVersion != reviewbundle.BundleSchemaVersion ||
		bundle.Target.Mode != reviewbundle.TargetWorkspace {
		t.Fatalf("unexpected bundle: %+v", bundle)
	}
}

func TestAgentPrepareWritesOnlyExplicitOutputWithRestrictedMode(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	outputPath := filepath.Join(t.TempDir(), "bundle.json")

	var stdout bytes.Buffer
	err := runAgentWithWriter([]string{
		"prepare",
		"--repo", repository,
		"--output", outputPath,
	}, &stdout)
	if err != nil {
		t.Fatalf("runAgentWithWriter() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty with --output", stdout.String())
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("output mode = %o, want 600", got)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var bundle reviewbundle.Bundle
	if err := json.Unmarshal(content, &bundle); err != nil {
		t.Fatalf("decode output bundle: %v", err)
	}
}

func TestAgentPrepareWritesOutputWhenSessionRecordingFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".opencodereview"), 0o700); err != nil {
		t.Fatalf("create opencodereview dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".opencodereview", "sessions"), []byte("x"), 0o600); err != nil {
		t.Fatalf("block sessions dir: %v", err)
	}
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	outputPath := filepath.Join(t.TempDir(), "bundle.json")

	err := runAgentWithWriter([]string{
		"prepare",
		"--repo", repository,
		"--output", outputPath,
		"--session-id", "run-prepare",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("prepare with broken session store: %v", err)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read bundle output: %v", err)
	}
	var bundle reviewbundle.Bundle
	if err := json.Unmarshal(content, &bundle); err != nil {
		t.Fatalf("decode bundle output: %v\n%s", err, content)
	}
}

func TestAgentPreparePreviewOmitsPatchBodies(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")

	var output bytes.Buffer
	err := runAgentWithWriter(
		[]string{"prepare", "--repo", repository, "--preview"},
		&output,
	)
	if err != nil {
		t.Fatalf("runAgentWithWriter() error = %v", err)
	}
	if !strings.Contains(output.String(), "main.go") {
		t.Fatalf("preview missing file: %s", output.String())
	}
	if strings.Contains(output.String(), "diff --git") {
		t.Fatalf("preview leaked patch body: %s", output.String())
	}
}

func TestAgentPreparePreviewIgnoresBundleSizeLimit(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n// "+strings.Repeat("x", 1024)+"\n")

	var output bytes.Buffer
	err := runAgentWithWriter([]string{
		"prepare",
		"--repo", repository,
		"--preview",
		"--max-bundle-bytes", "128",
	}, &output)
	if err != nil {
		t.Fatalf("runAgentWithWriter() error = %v", err)
	}
	if !strings.Contains(output.String(), "Agent review bundle preview") ||
		!strings.Contains(output.String(), "main.go") {
		t.Fatalf("preview output = %q", output.String())
	}
}

func TestAgentPrepareRejectsConflictingTargets(t *testing.T) {
	var output bytes.Buffer
	err := runAgentWithWriter(
		[]string{"prepare", "--from", "main", "--to", "HEAD", "--commit", "HEAD"},
		&output,
	)
	if err == nil || !strings.Contains(err.Error(), "only one review mode") {
		t.Fatalf("error = %v, want target conflict", err)
	}
}

func TestAgentPrepareRejectsOversizedOutput(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n// "+strings.Repeat("x", 1024)+"\n")

	var output bytes.Buffer
	err := runAgentWithWriter([]string{
		"prepare",
		"--repo", repository,
		"--max-bundle-bytes", "128",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "bundle_too_large") {
		t.Fatalf("error = %v, want bundle_too_large", err)
	}
	if output.Len() != 0 {
		t.Fatalf("partial output emitted: %q", output.String())
	}
}

func TestAgentPrepareSplitEmitsLargeDiffManifest(t *testing.T) {
	repository := initAgentRepository(t)
	for _, name := range []string{"one.go", "two.go"} {
		writeAgentFile(
			t,
			repository,
			name,
			"package sample\n// "+strings.Repeat(name, 120)+"\n",
		)
	}
	var output bytes.Buffer
	err := runAgentWithWriter([]string{
		"prepare",
		"--repo", repository,
		"--split",
		"--max-bundle-bytes", "3600",
	}, &output)
	if err != nil {
		t.Fatalf("split prepare: %v", err)
	}
	var manifest reviewbundle.ScanManifest
	if err := json.Unmarshal(output.Bytes(), &manifest); err != nil {
		t.Fatalf("decode diff manifest: %v", err)
	}
	if manifest.BatchStrategy != "diff" || len(manifest.Bundles) < 2 {
		t.Fatalf("manifest = %+v", manifest)
	}
}

func TestAgentValidateSplitManifestIgnoresWorkingTreeSiblingChanges(t *testing.T) {
	repository := initAgentRepository(t)
	for _, name := range []string{"one.go", "two.go"} {
		writeAgentFile(
			t,
			repository,
			name,
			"package sample\n// "+strings.Repeat(name, 120)+"\n",
		)
	}
	runAgentGit(t, repository, "add", ".")
	runAgentGit(t, repository, "commit", "-m", "add split files")

	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := runAgentWithWriter([]string{
		"prepare",
		"--repo", repository,
		"--from", "HEAD~1",
		"--to", "HEAD",
		"--split",
		"--max-bundle-bytes", "3600",
		"--output", manifestPath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare split manifest: %v", err)
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest reviewbundle.ScanManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.BatchStrategy != "diff" || len(manifest.Bundles) < 2 {
		t.Fatalf("manifest = %+v, want split diff manifest", manifest)
	}

	stalePath := manifest.Bundles[1].Files[0].Path
	writeAgentFile(t, repository, stalePath, "package sample\n\nfunc ChangedInWorkingTree() {}\n")

	commentsPath := filepath.Join(t.TempDir(), "comments.json")
	writeAgentJSON(t, commentsPath, reviewbundle.Comments{
		SchemaVersion: reviewbundle.CommentsSchemaVersion,
		BundleID:      manifest.Bundles[0].BundleID,
		Summary:       reviewbundle.CommentsSummary{FilesReviewed: 0, IssuesFound: 0},
		Comments:      []reviewbundle.ReviewComment{},
	})
	err = runAgentWithWriter([]string{
		"validate-comments",
		"--repo", repository,
		"--bundle", manifestPath,
		"--comments", commentsPath,
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("validate split manifest with dirty sibling: %v", err)
	}
}

func TestAgentContextSplitManifestIgnoresWorkingTreeSiblingChanges(t *testing.T) {
	repository := initAgentRepository(t)
	for _, name := range []string{"one.go", "two.go"} {
		writeAgentFile(
			t,
			repository,
			name,
			"package sample\n// "+strings.Repeat(name, 120)+"\n",
		)
	}
	runAgentGit(t, repository, "add", ".")
	runAgentGit(t, repository, "commit", "-m", "add split files")

	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := runAgentWithWriter([]string{
		"prepare",
		"--repo", repository,
		"--from", "HEAD~1",
		"--to", "HEAD",
		"--split",
		"--max-bundle-bytes", "3600",
		"--output", manifestPath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare split manifest: %v", err)
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest reviewbundle.ScanManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.BatchStrategy != "diff" || len(manifest.Bundles) < 2 {
		t.Fatalf("manifest = %+v, want split diff manifest", manifest)
	}

	stalePath := manifest.Bundles[1].Files[0].Path
	writeAgentFile(t, repository, stalePath, "package sample\n\nfunc ChangedInWorkingTree() {}\n")

	var output bytes.Buffer
	err = runAgentWithWriter([]string{
		"context", "read",
		"--repo", repository,
		"--bundle", manifestPath,
		"--bundle-index", "0",
		"--path", manifest.Bundles[0].Files[0].Path,
	}, &output)
	if err != nil {
		t.Fatalf("context read split manifest with dirty sibling: %v", err)
	}
	if !strings.Contains(output.String(), manifest.Bundles[0].Files[0].Path) {
		t.Fatalf("context output missing selected file:\n%s", output.String())
	}
}

func TestAgentUnknownSubcommand(t *testing.T) {
	var output bytes.Buffer
	err := runAgentWithWriter([]string{"unknown"}, &output)
	if err == nil || !strings.Contains(err.Error(), "unknown agent command") {
		t.Fatalf("error = %v, want unknown command", err)
	}
}

func TestAgentAliasPrepareEmitsBundle(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	t.Setenv("HOME", t.TempDir())

	var output bytes.Buffer
	err := runAgentWithWriter([]string{"prepare", "--repo", repository}, &output)
	if err != nil {
		t.Fatalf("runAgentWithWriter() error = %v", err)
	}
	var bundle reviewbundle.Bundle
	if err := json.Unmarshal(output.Bytes(), &bundle); err != nil {
		t.Fatalf("decode stdout bundle: %v\n%s", err, output.String())
	}
	if bundle.SchemaVersion != reviewbundle.BundleSchemaVersion ||
		bundle.Target.Mode != reviewbundle.TargetWorkspace {
		t.Fatalf("unexpected bundle: %+v", bundle)
	}
}

func TestAgentAliasUnknownSubcommand(t *testing.T) {
	var output bytes.Buffer
	err := runAgentWithWriter([]string{"unknown"}, &output)
	if err == nil || !strings.Contains(err.Error(), "unknown agent command") {
		t.Fatalf("error = %v, want unknown agent command", err)
	}
}

func TestAgentAliasHelpUsesAgentCommandName(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"prepare", []string{"prepare", "--help"}, "ocr agent prepare"},
		{"validate", []string{"validate-comments", "--help"}, "ocr agent validate-comments"},
		{"report", []string{"report", "--help"}, "ocr agent report"},
		{"context", []string{"context"}, "ocr agent context read"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := runAgentWithWriter(tc.args, &output); err != nil {
				t.Fatalf("runAgentWithWriter() error = %v", err)
			}
			if !strings.Contains(output.String(), tc.want) {
				t.Fatalf("help output missing %q:\n%s", tc.want, output.String())
			}
			if strings.Contains(output.String(), "ocr codex") {
				t.Fatalf("agent help leaked codex command name:\n%s", output.String())
			}
		})
	}
}

func TestAgentValidateCommentsEmitsStructuredResult(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	bundlePath := filepath.Join(t.TempDir(), "bundle.json")
	if err := runAgentWithWriter([]string{
		"prepare", "--repo", repository, "--output", bundlePath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare bundle: %v", err)
	}
	bundleContent, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	var bundle reviewbundle.Bundle
	if err := json.Unmarshal(bundleContent, &bundle); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	comments := reviewbundle.Comments{
		SchemaVersion: reviewbundle.CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       reviewbundle.CommentsSummary{FilesReviewed: 1, IssuesFound: 0},
		Comments:      []reviewbundle.ReviewComment{},
	}
	commentsPath := filepath.Join(t.TempDir(), "comments.json")
	commentsContent, err := json.Marshal(comments)
	if err != nil {
		t.Fatalf("marshal comments: %v", err)
	}
	if err := os.WriteFile(commentsPath, commentsContent, 0o600); err != nil {
		t.Fatalf("write comments: %v", err)
	}

	var output bytes.Buffer
	err = runAgentWithWriter([]string{
		"validate-comments",
		"--repo", repository,
		"--bundle", bundlePath,
		"--comments", commentsPath,
	}, &output)
	if err != nil {
		t.Fatalf("validate comments: %v", err)
	}
	var result reviewbundle.ValidationResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode validation: %v\n%s", err, output.String())
	}
	if !result.Valid || result.BundleID != bundle.BundleID {
		t.Fatalf("validation = %+v, want valid result", result)
	}
	if result.CommentsSHA256 == "" {
		t.Fatalf("validation comments hash is empty: %+v", result)
	}
}

func TestAgentReportEmitsMarkdown(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	directory := t.TempDir()
	bundlePath := filepath.Join(directory, "bundle.json")
	if err := runAgentWithWriter([]string{
		"prepare", "--repo", repository, "--output", bundlePath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare bundle: %v", err)
	}
	bundleFile, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	bundle, loadErr := reviewbundle.LoadBundle(bundleFile)
	closeErr := bundleFile.Close()
	if loadErr != nil {
		t.Fatalf("load bundle: %v", loadErr)
	}
	if closeErr != nil {
		t.Fatalf("close bundle: %v", closeErr)
	}
	comments := reviewbundle.Comments{
		SchemaVersion: reviewbundle.CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       reviewbundle.CommentsSummary{FilesReviewed: 1, IssuesFound: 0},
		Comments:      []reviewbundle.ReviewComment{},
	}
	commentsPath := filepath.Join(directory, "comments.json")
	writeAgentJSON(t, commentsPath, comments)
	validationPath := filepath.Join(directory, "validation.json")
	if err := runAgentWithWriter([]string{
		"validate-comments",
		"--repo", repository,
		"--bundle", bundlePath,
		"--comments", commentsPath,
		"--output", validationPath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("validate comments: %v", err)
	}

	var output bytes.Buffer
	err = runAgentWithWriter([]string{
		"report",
		"--bundle", bundlePath,
		"--comments", commentsPath,
		"--validation", validationPath,
		"--format", "markdown",
	}, &output)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if !strings.Contains(output.String(), "# Agent Code Review") ||
		!strings.Contains(output.String(), "No findings.") {
		t.Fatalf("unexpected report:\n%s", output.String())
	}
}

func TestAgentReportRejectsReformattedValidatedComments(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	directory := t.TempDir()
	bundlePath := filepath.Join(directory, "bundle.json")
	if err := runAgentWithWriter([]string{
		"prepare", "--repo", repository, "--output", bundlePath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare bundle: %v", err)
	}
	bundleFile, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	bundle, loadErr := reviewbundle.LoadBundle(bundleFile)
	closeErr := bundleFile.Close()
	if loadErr != nil {
		t.Fatalf("load bundle: %v", loadErr)
	}
	if closeErr != nil {
		t.Fatalf("close bundle: %v", closeErr)
	}
	comments := reviewbundle.Comments{
		SchemaVersion: reviewbundle.CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       reviewbundle.CommentsSummary{FilesReviewed: 1, IssuesFound: 0},
		Comments:      []reviewbundle.ReviewComment{},
	}
	commentsPath := filepath.Join(directory, "comments.json")
	writeAgentJSON(t, commentsPath, comments)
	validationPath := filepath.Join(directory, "validation.json")
	if err := runAgentWithWriter([]string{
		"validate-comments",
		"--repo", repository,
		"--bundle", bundlePath,
		"--comments", commentsPath,
		"--output", validationPath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("validate comments: %v", err)
	}
	reformatted, err := json.MarshalIndent(comments, "", "  ")
	if err != nil {
		t.Fatalf("marshal comments: %v", err)
	}
	reformattedPath := filepath.Join(directory, "comments-pretty.json")
	if err := os.WriteFile(reformattedPath, reformatted, 0o600); err != nil {
		t.Fatalf("write reformatted comments: %v", err)
	}

	err = runAgentWithWriter([]string{
		"report",
		"--bundle", bundlePath,
		"--comments", reformattedPath,
		"--validation", validationPath,
		"--format", "markdown",
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "comments_sha256 mismatch") {
		t.Fatalf("report error = %v, want comments_sha256 mismatch", err)
	}
}

func TestAgentContextReadReturnsBundleEnvelope(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	bundlePath := filepath.Join(t.TempDir(), "bundle.json")
	if err := runAgentWithWriter([]string{
		"prepare", "--repo", repository, "--output", bundlePath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare bundle: %v", err)
	}
	var output bytes.Buffer
	err := runAgentWithWriter([]string{
		"context", "read",
		"--repo", repository,
		"--bundle", bundlePath,
		"--path", "main.go",
		"--start-line", "1",
		"--max-lines", "5",
	}, &output)
	if err != nil {
		t.Fatalf("context read: %v", err)
	}
	var result reviewbundle.ContextResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode context result: %v\n%s", err, output.String())
	}
	if result.Operation != "read" || !strings.Contains(result.Result, "var changed = true") {
		t.Fatalf("context result = %+v", result)
	}
}

func TestAgentContextReadRejectsPathEscape(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	bundlePath := filepath.Join(t.TempDir(), "bundle.json")
	if err := runAgentWithWriter([]string{
		"prepare", "--repo", repository, "--output", bundlePath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare bundle: %v", err)
	}
	var output bytes.Buffer
	err := runAgentWithWriter([]string{
		"context", "read",
		"--repo", repository,
		"--bundle", bundlePath,
		"--path", "../etc/passwd",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "path_escape") {
		t.Fatalf("error = %v, want path_escape", err)
	}
}

func TestAgentValidateCommentsRejectsBundleIDMismatch(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	directory := t.TempDir()
	bundlePath := filepath.Join(directory, "bundle.json")
	if err := runAgentWithWriter([]string{
		"prepare", "--repo", repository, "--output", bundlePath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare bundle: %v", err)
	}
	bundleContent, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	var bundle reviewbundle.Bundle
	if err := json.Unmarshal(bundleContent, &bundle); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	comments := reviewbundle.Comments{
		SchemaVersion: reviewbundle.CommentsSchemaVersion,
		BundleID:      "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Summary:       reviewbundle.CommentsSummary{FilesReviewed: 1, IssuesFound: 0},
		Comments:      []reviewbundle.ReviewComment{},
	}
	commentsPath := filepath.Join(directory, "comments.json")
	writeAgentJSON(t, commentsPath, comments)

	var output bytes.Buffer
	err = runAgentWithWriter([]string{
		"validate-comments",
		"--repo", repository,
		"--bundle", bundlePath,
		"--comments", commentsPath,
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "comments require") {
		t.Fatalf("error = %v, want bundle_id mismatch", err)
	}
}

func TestAgentReportRejectsBundleIDMismatch(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	directory := t.TempDir()
	bundlePath := filepath.Join(directory, "bundle.json")
	if err := runAgentWithWriter([]string{
		"prepare", "--repo", repository, "--output", bundlePath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare bundle: %v", err)
	}
	comments := reviewbundle.Comments{
		SchemaVersion: reviewbundle.CommentsSchemaVersion,
		BundleID:      "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Summary:       reviewbundle.CommentsSummary{FilesReviewed: 1, IssuesFound: 0},
		Comments:      []reviewbundle.ReviewComment{},
	}
	commentsPath := filepath.Join(directory, "comments.json")
	writeAgentJSON(t, commentsPath, comments)
	validationPath := filepath.Join(directory, "validation.json")
	writeAgentJSON(t, validationPath, reviewbundle.ValidationResult{
		SchemaVersion: "agent-review-validation/v1",
		BundleID:      "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Valid:         true,
	})

	var output bytes.Buffer
	err := runAgentWithWriter([]string{
		"report",
		"--bundle", bundlePath,
		"--comments", commentsPath,
		"--validation", validationPath,
		"--format", "markdown",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "comments require") {
		t.Fatalf("error = %v, want bundle_id mismatch", err)
	}
}

func TestAgentPrepareScanWorksWithoutGitOrLLMConfiguration(t *testing.T) {
	directory := t.TempDir()
	writeAgentFile(t, directory, "main.go", "package sample\n\nfunc Main() {}\n")
	writeAgentFile(t, directory, "README.md", "# Sample\n")
	t.Setenv("HOME", t.TempDir())

	var output bytes.Buffer
	err := runAgentWithWriter([]string{
		"prepare",
		"--scan",
		"--repo", directory,
		"--path", "main.go,README.md",
		"--batch", "by-language",
	}, &output)
	if err != nil {
		t.Fatalf("prepare scan: %v", err)
	}
	var manifest reviewbundle.ScanManifest
	if err := json.Unmarshal(output.Bytes(), &manifest); err != nil {
		t.Fatalf("decode manifest: %v\n%s", err, output.String())
	}
	if manifest.SchemaVersion != reviewbundle.ScanManifestSchemaVersion ||
		manifest.Summary.TotalFiles != 2 ||
		manifest.Summary.ReviewableFiles != 1 ||
		manifest.Summary.ExcludedFiles != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, output.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	err = runAgentWithWriter([]string{
		"context", "read",
		"--repo", directory,
		"--bundle", manifestPath,
		"--path", "main.go",
	}, &output)
	if err != nil {
		t.Fatalf("scan context read: %v", err)
	}
	if !strings.Contains(output.String(), "func Main") {
		t.Fatalf("scan context output:\n%s", output.String())
	}
	commentsPath := filepath.Join(t.TempDir(), "scan-comments.json")
	writeAgentJSON(t, commentsPath, reviewbundle.Comments{
		SchemaVersion: reviewbundle.CommentsSchemaVersion,
		BundleID:      manifest.Bundles[0].BundleID,
		Summary:       reviewbundle.CommentsSummary{FilesReviewed: 1, IssuesFound: 0},
		Comments:      []reviewbundle.ReviewComment{},
	})
	output.Reset()
	err = runAgentWithWriter([]string{
		"validate-comments",
		"--repo", directory,
		"--bundle", manifestPath,
		"--comments", commentsPath,
	}, &output)
	if err != nil {
		t.Fatalf("validate scan comments: %v", err)
	}
	if !strings.Contains(output.String(), `"valid": true`) {
		t.Fatalf("scan validation output:\n%s", output.String())
	}
}

func TestAgentPrepareScanOutputInsideRepoIsNotScanned(t *testing.T) {
	directory := t.TempDir()
	writeAgentFile(t, directory, "a.go", "package sample\n\nfunc A() {}\n")
	outputPath := filepath.Join(directory, "manifest.json")

	err := runAgentWithWriter([]string{
		"prepare",
		"--scan",
		"--repo", directory,
		"--batch", "none",
		"--batch-size", "1",
		"--output", outputPath,
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("prepare scan: %v", err)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest reviewbundle.ScanManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("decode manifest: %v\n%s", err, content)
	}
	for _, bundle := range manifest.Bundles {
		for _, file := range bundle.Files {
			if file.Path == "manifest.json" {
				t.Fatalf("scan output file was included in manifest: %+v", manifest)
			}
		}
	}
	if manifest.Summary.TotalFiles != 1 || manifest.Summary.ReviewableFiles != 1 {
		t.Fatalf("manifest summary = %+v, want only a.go", manifest.Summary)
	}
}

func TestAgentValidateScanManifestRejectsStaleSiblingBundleFile(t *testing.T) {
	directory := t.TempDir()
	writeAgentFile(t, directory, "a.go", "package sample\n\nfunc A() {}\n")
	writeAgentFile(t, directory, "b.go", "package sample\n\nfunc B() {}\n")
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := runAgentWithWriter([]string{
		"prepare",
		"--scan",
		"--repo", directory,
		"--batch", "none",
		"--batch-size", "1",
		"--output", manifestPath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare scan: %v", err)
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest reviewbundle.ScanManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.Bundles) != 2 {
		t.Fatalf("bundles = %d, want two single-file bundles", len(manifest.Bundles))
	}
	commentsPath := filepath.Join(t.TempDir(), "comments.json")
	writeAgentJSON(t, commentsPath, reviewbundle.Comments{
		SchemaVersion: reviewbundle.CommentsSchemaVersion,
		BundleID:      manifest.Bundles[0].BundleID,
		Summary:       reviewbundle.CommentsSummary{FilesReviewed: 0, IssuesFound: 0},
		Comments:      []reviewbundle.ReviewComment{},
	})
	stalePath := manifest.Bundles[1].Files[0].Path
	writeAgentFile(t, directory, stalePath, "package sample\n\nfunc Changed() {}\n")

	var output bytes.Buffer
	err = runAgentWithWriter([]string{
		"validate-comments",
		"--repo", directory,
		"--bundle", manifestPath,
		"--comments", commentsPath,
	}, &output)
	if _, ok := err.(validationFailedError); !ok {
		t.Fatalf("validate comments error = %T(%v), want validation failure", err, err)
	}
	if !strings.Contains(output.String(), `"stale_bundle"`) ||
		!strings.Contains(output.String(), stalePath) {
		t.Fatalf("validation output did not report stale sibling file %q:\n%s", stalePath, output.String())
	}
}

func TestAgentContextRejectsStaleSiblingBundleFile(t *testing.T) {
	directory := t.TempDir()
	writeAgentFile(t, directory, "a.go", "package sample\n\nfunc A() {}\n")
	writeAgentFile(t, directory, "b.go", "package sample\n\nfunc B() {}\n")
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := runAgentWithWriter([]string{
		"prepare",
		"--scan",
		"--repo", directory,
		"--batch", "none",
		"--batch-size", "1",
		"--output", manifestPath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare scan: %v", err)
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest reviewbundle.ScanManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.Bundles) != 2 {
		t.Fatalf("bundles = %d, want two single-file bundles", len(manifest.Bundles))
	}
	stalePath := manifest.Bundles[1].Files[0].Path
	writeAgentFile(t, directory, stalePath, "package sample\n\nfunc Changed() {}\n")

	err = runAgentWithWriter([]string{
		"context", "read",
		"--repo", directory,
		"--bundle", manifestPath,
		"--bundle-index", "0",
		"--path", manifest.Bundles[0].Files[0].Path,
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "stale_bundle") {
		t.Fatalf("context read error = %v, want stale_bundle", err)
	}
}

func TestAgentPrepareScanValidateWithSharedSessionID(t *testing.T) {
	directory := t.TempDir()
	writeAgentFile(t, directory, "main.go", "package sample\n\nfunc Main() {}\n")
	t.Setenv("HOME", t.TempDir())

	var output bytes.Buffer
	err := runAgentWithWriter([]string{
		"prepare",
		"--scan",
		"--repo", directory,
		"--path", "main.go",
		"--session-id", "scan-run-1",
	}, &output)
	if err != nil {
		t.Fatalf("prepare scan: %v", err)
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, output.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	var manifest reviewbundle.ScanManifest
	if err := json.Unmarshal(output.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	commentsPath := filepath.Join(t.TempDir(), "scan-comments.json")
	writeAgentJSON(t, commentsPath, reviewbundle.Comments{
		SchemaVersion: reviewbundle.CommentsSchemaVersion,
		BundleID:      manifest.Bundles[0].BundleID,
		Summary:       reviewbundle.CommentsSummary{FilesReviewed: 1, IssuesFound: 0},
		Comments:      []reviewbundle.ReviewComment{},
	})
	output.Reset()
	err = runAgentWithWriter([]string{
		"validate-comments",
		"--repo", directory,
		"--bundle", manifestPath,
		"--comments", commentsPath,
		"--session-id", "scan-run-1",
	}, &output)
	if err != nil {
		t.Fatalf("validate scan comments with session id: %v", err)
	}
}

func TestAgentDispatchIsRegistered(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"ocr", "agent", "prepare", "--from", "main"}
	t.Cleanup(func() {
		os.Args = originalArgs
	})

	err := dispatch()
	if err == nil || !strings.Contains(err.Error(), "--to is required") {
		t.Fatalf("dispatch() error = %v, want agent prepare validation error", err)
	}
}

func TestAgentDispatchIsNotRegistered(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"ocr", "codex", "prepare"}
	t.Cleanup(func() {
		os.Args = originalArgs
	})

	err := dispatch()
	if err == nil || !strings.Contains(err.Error(), "unknown command: codex") {
		t.Fatalf("dispatch() error = %v, want unknown command", err)
	}
}

func TestAgentSkillsUseHostAgentWorkflow(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	paths := []string{
		filepath.Join(repositoryRoot, "skills", "open-code-review", "SKILL.md"),
		filepath.Join(
			repositoryRoot,
			"plugins",
			"open-code-review",
			"skills",
			"open-code-review",
			"SKILL.md",
		),
	}
	required := []string{
		"The host agent owns the review",
		"ocr agent prepare",
		"ocr agent validate-comments",
		"ocr agent report",
		"ocr agent context",
		"agent-review-comments/v1",
		"second-pass",
		"deduplicate",
		"project summary",
		"untrusted data",
		"explicitly requested fixes",
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		for _, fragment := range required {
			if !strings.Contains(text, fragment) {
				t.Errorf("%s missing %q", path, fragment)
			}
		}
		for _, forbidden := range []string{
			"ocr llm test",
			"ocr codex prepare",
			"ocr review --audience agent",
			"Requires a configured LLM",
		} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s contains legacy default %q", path, forbidden)
			}
		}
	}
}

func TestAgentValidateCommentsFailsWhenInvalid(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	directory := t.TempDir()
	bundlePath := filepath.Join(directory, "bundle.json")
	if err := runAgentWithWriter([]string{
		"prepare", "--repo", repository, "--output", bundlePath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare bundle: %v", err)
	}
	bundleFile, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	bundle, loadErr := reviewbundle.LoadBundle(bundleFile)
	closeErr := bundleFile.Close()
	if loadErr != nil {
		t.Fatalf("load bundle: %v", loadErr)
	}
	if closeErr != nil {
		t.Fatalf("close bundle: %v", closeErr)
	}
	commentsPath := filepath.Join(directory, "comments.json")
	if err := os.WriteFile(commentsPath, []byte(fmt.Sprintf(`{
  "schema_version": "agent-review-comments/v1",
  "bundle_id": %q,
  "summary": {"files_reviewed": 2, "issues_found": 0},
  "comments": []
}`, bundle.BundleID)), 0o600); err != nil {
		t.Fatalf("write comments: %v", err)
	}

	var output bytes.Buffer
	err = runAgentWithWriter([]string{
		"validate-comments",
		"--repo", repository,
		"--bundle", bundlePath,
		"--comments", commentsPath,
	}, &output)
	if err == nil {
		t.Fatalf("validate comments: nil, want validation failure exit")
	}
	if _, ok := err.(validationFailedError); !ok {
		t.Fatalf("error = %T(%v), want validationFailedError", err, err)
	}
	if !strings.Contains(output.String(), `"valid": false`) {
		t.Fatalf("expected invalid validation JSON:\n%s", output.String())
	}
}

func TestAgentValidateWritesOutputWhenSessionRecordingFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	if err := os.MkdirAll(filepath.Join(home, ".opencodereview"), 0o700); err != nil {
		t.Fatalf("create opencodereview dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".opencodereview", "sessions"), []byte("x"), 0o600); err != nil {
		t.Fatalf("block sessions dir: %v", err)
	}
	directory := t.TempDir()
	bundlePath := filepath.Join(directory, "bundle.json")
	if err := runAgentWithWriter([]string{
		"prepare", "--repo", repository, "--output", bundlePath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare bundle: %v", err)
	}
	bundleFile, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	bundle, loadErr := reviewbundle.LoadBundle(bundleFile)
	closeErr := bundleFile.Close()
	if loadErr != nil {
		t.Fatalf("load bundle: %v", loadErr)
	}
	if closeErr != nil {
		t.Fatalf("close bundle: %v", closeErr)
	}
	commentsPath := filepath.Join(directory, "comments.json")
	writeAgentJSON(t, commentsPath, reviewbundle.Comments{
		SchemaVersion: reviewbundle.CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       reviewbundle.CommentsSummary{FilesReviewed: 2, IssuesFound: 0},
		Comments:      []reviewbundle.ReviewComment{},
	})
	validationPath := filepath.Join(directory, "validation.json")
	err = runAgentValidateCommentsForCommand(context.Background(), "agent", []string{
		"--repo", repository,
		"--bundle", bundlePath,
		"--comments", commentsPath,
		"--output", validationPath,
		"--session-id", "run-validate",
	}, &bytes.Buffer{})
	if _, ok := err.(validationFailedError); !ok {
		t.Fatalf("error = %T(%v), want validationFailedError", err, err)
	}
	encoded, readErr := os.ReadFile(validationPath)
	if readErr != nil {
		t.Fatalf("read validation output: %v", readErr)
	}
	if !strings.Contains(string(encoded), `"valid": false`) {
		t.Fatalf("validation output missing valid:false:\n%s", encoded)
	}
}

func TestAgentReportRequiresValidation(t *testing.T) {
	repository := initAgentRepository(t)
	writeAgentFile(t, repository, "main.go", "package sample\n\nvar changed = true\n")
	directory := t.TempDir()
	bundlePath := filepath.Join(directory, "bundle.json")
	if err := runAgentWithWriter([]string{
		"prepare", "--repo", repository, "--output", bundlePath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("prepare bundle: %v", err)
	}
	bundleFile, err := os.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	bundle, loadErr := reviewbundle.LoadBundle(bundleFile)
	closeErr := bundleFile.Close()
	if loadErr != nil {
		t.Fatalf("load bundle: %v", loadErr)
	}
	if closeErr != nil {
		t.Fatalf("close bundle: %v", closeErr)
	}
	commentsPath := filepath.Join(directory, "comments.json")
	writeAgentJSON(t, commentsPath, reviewbundle.Comments{
		SchemaVersion: reviewbundle.CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       reviewbundle.CommentsSummary{FilesReviewed: 1, IssuesFound: 0},
		Comments:      []reviewbundle.ReviewComment{},
	})

	err = runAgentWithWriter([]string{
		"report",
		"--bundle", bundlePath,
		"--comments", commentsPath,
		"--format", "markdown",
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--validation is required") {
		t.Fatalf("error = %v, want missing validation requirement", err)
	}
}

func initAgentRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	runAgentGit(t, repository, "init", "-q")
	runAgentGit(t, repository, "config", "user.email", "tests@example.com")
	runAgentGit(t, repository, "config", "user.name", "OCR Tests")
	runAgentGit(t, repository, "config", "commit.gpgsign", "false")
	writeAgentFile(t, repository, "main.go", "package sample\n")
	runAgentGit(t, repository, "add", "main.go")
	runAgentGit(t, repository, "commit", "-m", "initial")
	return repository
}

func writeAgentFile(t *testing.T, repository, name, content string) {
	t.Helper()
	path := filepath.Join(repository, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func writeAgentJSON(t *testing.T, path string, value any) {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write JSON: %v", err)
	}
}

func runAgentGit(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}
