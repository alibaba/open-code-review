//go:build e2e

package gitlab_e2e

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/platform"
	"github.com/open-code-review/open-code-review/internal/platform/gitlab"
)

type e2eConfig struct {
	baseURL   string
	token     string
	projectID string
	mrIID     int
}

func loadGitLabE2EConfig(t *testing.T) e2eConfig {
	t.Helper()
	if os.Getenv("OCR_E2E_GITLAB") != "1" {
		t.Skip("skipping GitLab e2e: OCR_E2E_GITLAB != 1")
	}
	baseURL := os.Getenv("OCR_E2E_GITLAB_URL")
	token := os.Getenv("OCR_E2E_GITLAB_TOKEN")
	projectID := os.Getenv("OCR_E2E_GITLAB_PROJECT_ID")
	mrIIDStr := os.Getenv("OCR_E2E_GITLAB_MR_IID")
	if baseURL == "" || token == "" || projectID == "" || mrIIDStr == "" {
		t.Skip("skipping GitLab e2e: missing required env vars (OCR_E2E_GITLAB_URL, OCR_E2E_GITLAB_TOKEN, OCR_E2E_GITLAB_PROJECT_ID, OCR_E2E_GITLAB_MR_IID)")
	}
	mrIID, err := strconv.Atoi(mrIIDStr)
	if err != nil {
		t.Fatalf("invalid OCR_E2E_GITLAB_MR_IID %q: %v", mrIIDStr, err)
	}
	return e2eConfig{baseURL: baseURL, token: token, projectID: projectID, mrIID: mrIID}
}

func newE2EPublisher(t *testing.T, cfg e2eConfig, opts platform.PublishOptions) (*gitlab.Client, *gitlab.Publisher) {
	t.Helper()
	client := gitlab.NewClient(cfg.baseURL, cfg.token)
	opts.ProjectID = cfg.projectID
	opts.MergeRequestIID = cfg.mrIID
	pub := gitlab.NewPublisher(client, opts)
	return client, pub
}

// findFirstNewLine parses a unified diff and returns the first added line number.
func findFirstNewLine(diff string) int {
	re := regexp.MustCompile(`@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)
	currentLine := 0
	for _, line := range strings.Split(diff, "\n") {
		if m := re.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			currentLine = n
			continue
		}
		if currentLine > 0 {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				return currentLine
			}
			if !strings.HasPrefix(line, "-") {
				currentLine++
			}
		}
	}
	return 0
}

func uniqueSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func listAllNotes(t *testing.T, client *gitlab.Client, cfg e2eConfig) []gitlab.Note {
	t.Helper()
	ctx := context.Background()
	discs, err := client.ListDiscussions(ctx, cfg.projectID, cfg.mrIID)
	if err != nil {
		t.Fatalf("list discussions: %v", err)
	}
	var notes []gitlab.Note
	for _, d := range discs {
		notes = append(notes, d.Notes...)
	}
	return notes
}

func findNotesWithMarker(notes []gitlab.Note, marker string) []gitlab.Note {
	var found []gitlab.Note
	for _, n := range notes {
		if !n.System && strings.Contains(n.Body, marker) {
			found = append(found, n)
		}
	}
	return found
}

// clearOCRNotes removes all OCR inline and summary notes from the MR.
func clearOCRNotes(t *testing.T, pub *gitlab.Publisher) {
	t.Helper()
	_, err := pub.ClearInline()
	if err != nil {
		t.Fatalf("clearOCRNotes clear inline: %v", err)
	}
	_, err = pub.ClearSummary()
	if err != nil {
		t.Fatalf("clearOCRNotes clear summary: %v", err)
	}
}

func TestGitLabE2E_CreateAndClearSummary(t *testing.T) {
	cfg := loadGitLabE2EConfig(t)
	client, pub := newE2EPublisher(t, cfg, platform.PublishOptions{})
	ctx := context.Background()

	// Start clean.
	clearOCRNotes(t, pub)
	t.Cleanup(func() { clearOCRNotes(t, pub) })

	comments := []model.LlmComment{
		{Path: "test.go", Content: "e2e summary test", StartLine: 1},
	}

	// Publish summary.
	result, err := pub.Publish(comments)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !result.SummaryCreated {
		t.Fatal("expected summary created")
	}

	// Verify summary note exists with marker and expected content.
	notes := listAllNotes(t, client, cfg)
	marked := findNotesWithMarker(notes, platform.SummaryMarker)
	if len(marked) == 0 {
		t.Fatal("no summary marker note found after publish")
	}
	// RenderSummaryComment renders "## OpenCodeReview Summary" + comment count.
	if !strings.Contains(marked[0].Body, "OpenCodeReview Summary") {
		t.Fatalf("summary note missing header: %q", marked[0].Body[:100])
	}

	// Clear summary.
	clearResult, err := pub.ClearSummary()
	if err != nil {
		t.Fatalf("clear summary: %v", err)
	}
	if clearResult.SummaryDeleted < 1 {
		t.Fatalf("expected at least 1 summary deleted, got %d", clearResult.SummaryDeleted)
	}

	// Verify summary note is gone.
	notes = listAllNotes(t, client, cfg)
	marked = findNotesWithMarker(notes, platform.SummaryMarker)
	if len(marked) != 0 {
		t.Fatalf("summary marker notes still exist after clear: %d", len(marked))
	}
	_ = ctx
}

func TestGitLabE2E_CreateAndClearInline(t *testing.T) {
	cfg := loadGitLabE2EConfig(t)
	// Use NoSummaryComment to avoid polluting shared MR with summary notes.
	client, pub := newE2EPublisher(t, cfg, platform.PublishOptions{NoSummaryComment: true})
	ctx := context.Background()

	clearOCRNotes(t, pub)
	t.Cleanup(func() { clearOCRNotes(t, pub) })

	suffix := uniqueSuffix()

	// Find a valid file and line from the MR diff.
	files, err := client.ListChangedFiles(ctx, cfg.projectID, cfg.mrIID)
	if err != nil {
		t.Fatalf("list changed files: %v", err)
	}
	if len(files) == 0 {
		t.Skip("MR has no changed files; cannot test inline comments")
	}
	var targetFile string
	var targetLine int
	for _, f := range files {
		if !f.DeletedFile && f.NewPath != "" && f.Diff != "" {
			line := findFirstNewLine(f.Diff)
			if line > 0 {
				targetFile = f.NewPath
				targetLine = line
				break
			}
		}
	}
	if targetFile == "" {
		t.Skip("no suitable non-deleted file with a valid new line in MR diff")
	}

	comments := []model.LlmComment{
		{Path: targetFile, Content: "e2e inline test " + suffix, StartLine: targetLine},
	}

	// Publish inline.
	result, err := pub.Publish(comments)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if result.InlineCreated < 1 {
		if len(result.Warnings) > 0 {
			t.Skipf("inline publish skipped: %s", result.Warnings[0].Message)
		}
		t.Fatal("expected at least 1 inline created")
	}

	// Verify inline note exists with marker.
	notes := listAllNotes(t, client, cfg)
	marked := findNotesWithMarker(notes, platform.InlineMarker)
	found := false
	for _, n := range marked {
		if strings.Contains(n.Body, suffix) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("inline note with suffix %s not found", suffix)
	}

	// Clear inline.
	clearResult, err := pub.ClearInline()
	if err != nil {
		t.Fatalf("clear inline: %v", err)
	}
	if clearResult.InlineDeleted < 1 {
		t.Fatalf("expected at least 1 inline deleted, got %d", clearResult.InlineDeleted)
	}

	// Verify inline note is gone.
	notes = listAllNotes(t, client, cfg)
	marked = findNotesWithMarker(notes, platform.InlineMarker)
	for _, n := range marked {
		if strings.Contains(n.Body, suffix) {
			t.Fatalf("inline note with suffix %s still exists after clear", suffix)
		}
	}
}

func TestGitLabE2E_ClearDoesNotDeleteUnmarkedNotes(t *testing.T) {
	cfg := loadGitLabE2EConfig(t)
	client, pub := newE2EPublisher(t, cfg, platform.PublishOptions{})
	ctx := context.Background()

	// Create an unmarked note.
	unmarkedBody := "e2e unmarked note " + uniqueSuffix()
	note, err := client.CreateNote(ctx, cfg.projectID, cfg.mrIID, unmarkedBody)
	if err != nil {
		t.Fatalf("create unmarked note: %v", err)
	}
	t.Cleanup(func() {
		_ = client.DeleteNote(context.Background(), cfg.projectID, cfg.mrIID, note.ID)
	})

	// Run clear operations - must not fail.
	_, err = pub.ClearInline()
	if err != nil {
		t.Fatalf("clear inline: %v", err)
	}
	_, err = pub.ClearSummary()
	if err != nil {
		t.Fatalf("clear summary: %v", err)
	}

	// Verify unmarked note still exists.
	notes := listAllNotes(t, client, cfg)
	for _, n := range notes {
		if n.ID == note.ID {
			return // found, good
		}
	}
	t.Fatalf("unmarked note %d was deleted by clear operations", note.ID)
}

func TestGitLabE2E_AppendAndReplacePRSummary(t *testing.T) {
	cfg := loadGitLabE2EConfig(t)
	// NoInline: keep summary note creation, skip inline to avoid position issues.
	client, pub := newE2EPublisher(t, cfg, platform.PublishOptions{
		PRSummary: true,
		NoInline:  true,
	})
	ctx := context.Background()

	clearOCRNotes(t, pub)

	// Save original description for cleanup.
	mr, err := client.GetMergeRequest(ctx, cfg.projectID, cfg.mrIID)
	if err != nil {
		t.Fatalf("get MR: %v", err)
	}
	originalDesc := mr.Description
	t.Cleanup(func() {
		_ = client.UpdateDescription(context.Background(), cfg.projectID, cfg.mrIID, originalDesc)
		clearOCRNotes(t, pub)
	})

	// First publish with 1 comment -> summary shows "Review comments: 1".
	comments1 := []model.LlmComment{
		{Path: "test.go", Content: "e2e pr-summary test", StartLine: 1},
	}
	result, err := pub.Publish(comments1)
	if err != nil {
		t.Fatalf("publish 1: %v", err)
	}
	if !result.DescriptionUpdated {
		t.Fatal("expected description updated on first publish")
	}

	// Verify managed block was appended with markers and summary header.
	mr, err = client.GetMergeRequest(ctx, cfg.projectID, cfg.mrIID)
	if err != nil {
		t.Fatalf("get MR after publish 1: %v", err)
	}
	if !strings.Contains(mr.Description, platform.PRSummaryStartMarker) {
		t.Fatal("description missing PR summary start marker after publish 1")
	}
	if !strings.Contains(mr.Description, platform.PRSummaryEndMarker) {
		t.Fatal("description missing PR summary end marker after publish 1")
	}
	if !strings.Contains(mr.Description, "OpenCodeReview Summary") {
		t.Fatal("description missing summary header after publish 1")
	}
	if !strings.Contains(mr.Description, "Review comments: 1") {
		t.Fatal("description missing 'Review comments: 1' after publish 1")
	}
	if originalDesc != "" && !strings.Contains(mr.Description, originalDesc) {
		t.Fatal("original user description was lost after publish 1")
	}

	// Second publish with 2 comments -> summary should show "Review comments: 2".
	comments2 := []model.LlmComment{
		{Path: "test.go", Content: "e2e pr-summary replacement", StartLine: 1},
		{Path: "main.go", Content: "e2e pr-summary second", StartLine: 2},
	}
	result, err = pub.Publish(comments2)
	if err != nil {
		t.Fatalf("publish 2: %v", err)
	}
	if !result.DescriptionUpdated {
		t.Fatal("expected description updated on second publish")
	}

	mr, err = client.GetMergeRequest(ctx, cfg.projectID, cfg.mrIID)
	if err != nil {
		t.Fatalf("get MR after publish 2: %v", err)
	}
	if strings.Count(mr.Description, platform.PRSummaryStartMarker) != 1 {
		t.Fatal("expected exactly one managed block after second publish")
	}
	if !strings.Contains(mr.Description, "Review comments: 2") {
		t.Fatal("description missing 'Review comments: 2' after second publish")
	}
	if strings.Contains(mr.Description, "Review comments: 1") {
		t.Fatal("old 'Review comments: 1' should have been replaced")
	}
	if !strings.Contains(mr.Description, "OpenCodeReview Summary") {
		t.Fatal("description missing summary header after replacement")
	}
	if originalDesc != "" && !strings.Contains(mr.Description, originalDesc) {
		t.Fatal("original user description was lost after second publish")
	}
}

func TestGitLabE2E_InlineCommentRendering(t *testing.T) {
	cfg := loadGitLabE2EConfig(t)
	client, pub := newE2EPublisher(t, cfg, platform.PublishOptions{NoSummaryComment: true})

	clearOCRNotes(t, pub)
	t.Cleanup(func() { clearOCRNotes(t, pub) })

	ctx := context.Background()
	suffix := uniqueSuffix()

	// Find a valid file and new line from the MR diff.
	files, err := client.ListChangedFiles(ctx, cfg.projectID, cfg.mrIID)
	if err != nil {
		t.Fatalf("list changed files: %v", err)
	}
	if len(files) == 0 {
		t.Skip("MR has no changed files; cannot test inline comments")
	}

	// Pick the first non-deleted file with a valid new line from the diff.
	var targetFile string
	var targetLine int
	for _, f := range files {
		if !f.DeletedFile && f.NewPath != "" && f.Diff != "" {
			line := findFirstNewLine(f.Diff)
			if line > 0 {
				targetFile = f.NewPath
				targetLine = line
				break
			}
		}
	}
	if targetFile == "" {
		t.Skip("no suitable non-deleted file with a valid new line in MR diff")
	}

	// Build a comment with all rendering features.
	comment := model.LlmComment{
		Path:           targetFile,
		Content:        "e2e rendering test " + suffix,
		StartLine:      targetLine,
		SuggestionCode: "fixed code here",
		ExistingCode:   "old code here",
		Thinking:       "reasoning about the fix",
	}

	result, err := pub.Publish([]model.LlmComment{comment})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if result.InlineCreated < 1 {
		if len(result.Warnings) > 0 {
			t.Skipf("inline publish skipped (line not in diff): %s", result.Warnings[0].Message)
		}
		t.Fatal("expected at least 1 inline created")
	}

	// Verify the rendered note body.
	notes := listAllNotes(t, client, cfg)
	marked := findNotesWithMarker(notes, platform.InlineMarker)
	var foundNote *gitlab.Note
	for i, n := range marked {
		if strings.Contains(n.Body, suffix) {
			foundNote = &marked[i]
			break
		}
	}
	if foundNote == nil {
		t.Fatalf("inline note with suffix %s not found", suffix)
	}

	body := foundNote.Body
	wantContains := []struct {
		label string
		text  string
	}{
		{"badge", "img.shields.io/badge/OpenCodeReview"},
		{"suggested change label", "**Suggested change:**"},
		{"language fence for go file", "```go"},
		{"details block", "<details>"},
		{"existing code content", "old code here"},
		{"thinking label", "Reviewer notes:"},
		{"thinking content", "reasoning about the fix"},
		{"inline marker", platform.InlineMarker},
	}
	for _, w := range wantContains {
		if !strings.Contains(body, w.text) {
			t.Errorf("missing %s (%q) in note body", w.label, w.text)
		}
	}
}

func TestGitLabE2E_PublishUpdatesSummaryNote(t *testing.T) {
	cfg := loadGitLabE2EConfig(t)
	client, pub := newE2EPublisher(t, cfg, platform.PublishOptions{})

	// Start clean so first publish creates (not updates).
	clearOCRNotes(t, pub)
	t.Cleanup(func() { clearOCRNotes(t, pub) })

	suffix := uniqueSuffix()
	comments := []model.LlmComment{
		{Path: "test.go", Content: "e2e rerun test " + suffix, StartLine: 1},
	}

	// First publish.
	result1, err := pub.Publish(comments)
	if err != nil {
		t.Fatalf("publish 1: %v", err)
	}
	if !result1.SummaryCreated {
		t.Fatal("expected summary created on first publish")
	}

	// Second publish with different content.
	suffix2 := uniqueSuffix()
	comments2 := []model.LlmComment{
		{Path: "test.go", Content: "e2e rerun updated " + suffix2, StartLine: 1},
	}
	result2, err := pub.Publish(comments2)
	if err != nil {
		t.Fatalf("publish 2: %v", err)
	}
	if !result2.SummaryUpdated {
		t.Fatal("expected summary updated (not created) on second publish")
	}

	// Verify exactly one summary marker note exists.
	notes := listAllNotes(t, client, cfg)
	marked := findNotesWithMarker(notes, platform.SummaryMarker)
	if len(marked) != 1 {
		t.Fatalf("expected exactly 1 summary marker note, got %d", len(marked))
	}
}
