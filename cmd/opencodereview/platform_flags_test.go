package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/platform"
)

// clearGitLabEnv zeroes all GitLab-related env vars so resolver tests are deterministic.
func clearGitLabEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"GITLAB_TOKEN", "OCR_GITLAB_TOKEN",
		"OCR_GITLAB_BASE_URL", "CI_SERVER_URL",
		"CI_PROJECT_ID", "CI_MERGE_REQUEST_IID",
	} {
		t.Setenv(k, "")
	}
}

// --- Flag parsing tests ---

func TestParseReviewFlags_GitLabPublish(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--platform", "gitlab", "--publish", "--mr", "123", "--project-id", "456"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.platform != "gitlab" {
		t.Fatalf("expected platform gitlab, got %s", opts.platform)
	}
	if !opts.publish {
		t.Fatal("expected publish to be true")
	}
	if opts.mrIID != 123 {
		t.Fatalf("expected mrIID 123, got %d", opts.mrIID)
	}
	if opts.projectID != "456" {
		t.Fatalf("expected projectID 456, got %s", opts.projectID)
	}
}

func TestParseReviewFlags_ClearFlags(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--clear-inline"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.clearInline {
		t.Fatal("expected clearInline to be true")
	}

	opts, err = parseReviewFlags([]string{"--clear-summary"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.clearSummary {
		t.Fatal("expected clearSummary to be true")
	}
}

func TestParseReviewFlags_MiscFlags(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--pr-summary", "--no-inline", "--no-summary-comment", "--clear-existing"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.prSummary {
		t.Fatal("expected prSummary to be true")
	}
	if !opts.noInline {
		t.Fatal("expected noInline to be true")
	}
	if !opts.noSummaryComment {
		t.Fatal("expected noSummaryComment to be true")
	}
	if !opts.clearExisting {
		t.Fatal("expected clearExisting to be true")
	}
}

func TestParseReviewFlags_InvalidPlatformReturnsError(t *testing.T) {
	_, err := parseReviewFlags([]string{"--platform", "github"})
	if err == nil {
		t.Fatal("expected error for invalid platform")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected error to mention unsupported, got: %v", err)
	}
}

// --- Resolver tests ---

func TestResolveGitLab_ExplicitFlagsWinOverEnv(t *testing.T) {
	clearGitLabEnv(t)
	t.Setenv("CI_PROJECT_ID", "env-project")
	t.Setenv("CI_MERGE_REQUEST_IID", "999")
	t.Setenv("CI_SERVER_URL", "https://ci.example.com")

	opts := reviewOptions{
		platform:  "gitlab",
		publish:   true,
		projectID: "explicit-project",
		mrIID:     42,
		baseURL:   "https://explicit.example.com",
	}
	pubOpts, err := resolveGitLabReviewOptions(opts, "my-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pubOpts.ProjectID != "explicit-project" {
		t.Fatalf("expected explicit project, got %s", pubOpts.ProjectID)
	}
	if pubOpts.MergeRequestIID != 42 {
		t.Fatalf("expected 42, got %d", pubOpts.MergeRequestIID)
	}
	if pubOpts.BaseURL != "https://explicit.example.com" {
		t.Fatalf("expected explicit base URL, got %s", pubOpts.BaseURL)
	}
}

func TestResolveGitLab_CIEnvInference(t *testing.T) {
	clearGitLabEnv(t)
	t.Setenv("CI_PROJECT_ID", "ci-project")
	t.Setenv("CI_MERGE_REQUEST_IID", "77")
	t.Setenv("CI_SERVER_URL", "https://gitlab.ci.local")

	opts := reviewOptions{
		platform: "gitlab",
		publish:  true,
	}
	pubOpts, err := resolveGitLabReviewOptions(opts, "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pubOpts.ProjectID != "ci-project" {
		t.Fatalf("expected ci-project, got %s", pubOpts.ProjectID)
	}
	if pubOpts.MergeRequestIID != 77 {
		t.Fatalf("expected 77, got %d", pubOpts.MergeRequestIID)
	}
	if pubOpts.BaseURL != "https://gitlab.ci.local" {
		t.Fatalf("expected CI_SERVER_URL base, got %s", pubOpts.BaseURL)
	}
}

func TestResolveGitLab_TokenPrecedence(t *testing.T) {
	clearGitLabEnv(t)
	t.Setenv("GITLAB_TOKEN", "gitlab-tok")
	t.Setenv("OCR_GITLAB_TOKEN", "ocr-tok")

	opts := reviewOptions{platform: "gitlab", publish: true, projectID: "p", mrIID: 1}
	pubOpts, err := resolveGitLabReviewOptions(opts, "gitlab-tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pubOpts.Token != "gitlab-tok" {
		t.Fatalf("expected GITLAB_TOKEN to win, got %s", pubOpts.Token)
	}
}

func TestResolveGitLab_OCRBaseURLFallback(t *testing.T) {
	clearGitLabEnv(t)
	t.Setenv("OCR_GITLAB_BASE_URL", "https://ocr.example.com")

	opts := reviewOptions{platform: "gitlab", publish: true, projectID: "p", mrIID: 1}
	pubOpts, err := resolveGitLabReviewOptions(opts, "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pubOpts.BaseURL != "https://ocr.example.com" {
		t.Fatalf("expected OCR_GITLAB_BASE_URL, got %s", pubOpts.BaseURL)
	}
}

func TestResolveGitLab_DefaultBaseURL(t *testing.T) {
	clearGitLabEnv(t)
	opts := reviewOptions{platform: "gitlab", publish: true, projectID: "p", mrIID: 1}
	pubOpts, err := resolveGitLabReviewOptions(opts, "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pubOpts.BaseURL != "https://gitlab.com" {
		t.Fatalf("expected default base URL, got %s", pubOpts.BaseURL)
	}
}

func TestResolveGitLab_MissingTokenForPublish(t *testing.T) {
	clearGitLabEnv(t)
	opts := reviewOptions{platform: "gitlab", publish: true, projectID: "p", mrIID: 1}
	_, err := resolveGitLabReviewOptions(opts, "")
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Fatalf("expected error to mention token, got: %v", err)
	}
}

func TestResolveGitLab_MissingProjectForClear(t *testing.T) {
	clearGitLabEnv(t)
	opts := reviewOptions{platform: "gitlab", clearInline: true, mrIID: 1}
	_, err := resolveGitLabReviewOptions(opts, "tok")
	if err == nil {
		t.Fatal("expected error for missing project")
	}
	if !strings.Contains(err.Error(), "project") {
		t.Fatalf("expected error to mention project, got: %v", err)
	}
}

func TestResolveGitLab_MissingMRForPublish(t *testing.T) {
	clearGitLabEnv(t)
	opts := reviewOptions{platform: "gitlab", publish: true, projectID: "p"}
	_, err := resolveGitLabReviewOptions(opts, "tok")
	if err == nil {
		t.Fatal("expected error for missing MR IID")
	}
	if !strings.Contains(err.Error(), "mr") {
		t.Fatalf("expected error to mention MR, got: %v", err)
	}
}

func TestResolveGitLab_InvalidMRFromEnv(t *testing.T) {
	clearGitLabEnv(t)
	t.Setenv("CI_MERGE_REQUEST_IID", "not-a-number")

	opts := reviewOptions{platform: "gitlab", publish: true, projectID: "p"}
	_, err := resolveGitLabReviewOptions(opts, "tok")
	if err == nil {
		t.Fatal("expected error for invalid MR IID")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected error to mention invalid, got: %v", err)
	}
}

func TestResolveGitLab_ClearInlineRequiresPlatform(t *testing.T) {
	clearGitLabEnv(t)
	opts := reviewOptions{clearInline: true}
	_, err := resolveGitLabReviewOptions(opts, "tok")
	if err == nil {
		t.Fatal("expected error when clear flag set without platform")
	}
}

func TestResolveGitLab_ClearExistingRequiresPublish(t *testing.T) {
	clearGitLabEnv(t)
	opts := reviewOptions{platform: "gitlab", clearExisting: true, projectID: "p", mrIID: 1}
	_, err := resolveGitLabReviewOptions(opts, "tok")
	if err == nil {
		t.Fatal("expected error for --clear-existing without --publish")
	}
	if !strings.Contains(err.Error(), "--clear-existing") {
		t.Fatalf("expected error to mention --clear-existing, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--publish") {
		t.Fatalf("expected error to mention --publish, got: %v", err)
	}
}

func TestResolveGitLab_ClearExistingWithPublishIsAllowed(t *testing.T) {
	clearGitLabEnv(t)
	opts := reviewOptions{platform: "gitlab", clearExisting: true, publish: true, projectID: "p", mrIID: 1}
	pubOpts, err := resolveGitLabReviewOptions(opts, "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pubOpts.ClearExisting {
		t.Fatal("expected ClearExisting to be true")
	}
	if !pubOpts.Publish {
		t.Fatal("expected Publish to be true")
	}
}

func TestResolveGitLab_FlagsMapCorrectly(t *testing.T) {
	clearGitLabEnv(t)
	opts := reviewOptions{
		platform:         "gitlab",
		publish:          true,
		projectID:        "proj",
		mrIID:            10,
		prSummary:        true,
		clearExisting:    true,
		noInline:         true,
		noSummaryComment: true,
	}
	pubOpts, err := resolveGitLabReviewOptions(opts, "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pubOpts.Publish {
		t.Fatal("expected Publish to be mapped")
	}
	if !pubOpts.PRSummary {
		t.Fatal("expected PRSummary to be mapped")
	}
	if !pubOpts.ClearExisting {
		t.Fatal("expected ClearExisting to be mapped")
	}
	if !pubOpts.NoInline {
		t.Fatal("expected NoInline to be mapped")
	}
	if !pubOpts.NoSummaryComment {
		t.Fatal("expected NoSummaryComment to be mapped")
	}
}

func TestHasGitLabOperation(t *testing.T) {
	tests := []struct {
		name string
		opts reviewOptions
		want bool
	}{
		{"none", reviewOptions{}, false},
		{"publish", reviewOptions{publish: true}, true},
		{"clearInline", reviewOptions{clearInline: true}, true},
		{"clearSummary", reviewOptions{clearSummary: true}, true},
		{"clearExisting", reviewOptions{clearExisting: true}, true},
		{"prSummaryOnly", reviewOptions{prSummary: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasGitLabOperation(tt.opts); got != tt.want {
				t.Fatalf("hasGitLabOperation() = %v, want %v", got, tt.want)
			}
		})
	}
}

// captureStdout runs fn and returns whatever was written to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestPrintPublishResult_HumanModePrints(t *testing.T) {
	result := &platform.PublishResult{InlineCreated: 3, SummaryCreated: true}
	out := captureStdout(t, func() {
		printPublishResult(reviewOptions{outputFormat: "text", audience: "human"}, result)
	})
	if !strings.Contains(out, "3 inline") {
		t.Fatalf("expected human mode to print inline count, got: %q", out)
	}
	if !strings.Contains(out, "summary") {
		t.Fatalf("expected human mode to print summary status, got: %q", out)
	}
}

func TestPrintPublishResult_JSONModePrintsNothing(t *testing.T) {
	result := &platform.PublishResult{InlineCreated: 3, SummaryCreated: true}
	out := captureStdout(t, func() {
		printPublishResult(reviewOptions{outputFormat: "json"}, result)
	})
	if out != "" {
		t.Fatalf("expected JSON mode to print nothing, got: %q", out)
	}
}

func TestPrintPublishResult_AgentModePrintsNothing(t *testing.T) {
	result := &platform.PublishResult{InlineCreated: 3, SummaryCreated: true}
	out := captureStdout(t, func() {
		printPublishResult(reviewOptions{outputFormat: "text", audience: "agent"}, result)
	})
	if out != "" {
		t.Fatalf("expected agent mode to print nothing, got: %q", out)
	}
}
