package gitlab

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/platform"
)

// --- Client method tests ---

func TestListDiscussions_DecodesDiscussionAndNotes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/merge_requests/5/discussions") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]Discussion{
			{
				ID: "abc123",
				Notes: []Note{
					{ID: 101, Body: "first note", System: false},
					{ID: 102, Body: "second note", System: true},
				},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	discs, err := client.ListDiscussions(context.Background(), "proj", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(discs) != 1 {
		t.Fatalf("expected 1 discussion, got %d", len(discs))
	}
	if discs[0].ID != "abc123" {
		t.Fatalf("expected discussion ID abc123, got %s", discs[0].ID)
	}
	if len(discs[0].Notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(discs[0].Notes))
	}
	if discs[0].Notes[0].ID != 101 {
		t.Fatalf("expected note ID 101, got %d", discs[0].Notes[0].ID)
	}
}

func TestCreateNote_SendsBodyToNotes(t *testing.T) {
	var gotBody string
	var gotMethod string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		var req struct {
			Body string `json:"body"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		gotBody = req.Body
		json.NewEncoder(w).Encode(Note{ID: 99, Body: req.Body})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	note, err := client.CreateNote(context.Background(), "proj", 3, "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/merge_requests/3/notes") {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody != "hello world" {
		t.Fatalf("expected body 'hello world', got %q", gotBody)
	}
	if note.ID != 99 {
		t.Fatalf("expected note ID 99, got %d", note.ID)
	}
}

func TestUpdateNote_SendsPUTToNoteID(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		var req struct {
			Body string `json:"body"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		gotBody = req.Body
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Note{ID: 42, Body: req.Body})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	err := client.UpdateNote(context.Background(), "proj", 3, 42, "updated")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("expected PUT, got %s", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/merge_requests/3/notes/42") {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody != "updated" {
		t.Fatalf("expected body 'updated', got %q", gotBody)
	}
}

func TestDeleteNote_SendsDELETEToNoteID(t *testing.T) {
	var gotMethod string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	err := client.DeleteNote(context.Background(), "proj", 3, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/merge_requests/3/notes/42") {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestCreateInlineDiscussion_SendsBodyAndPosition(t *testing.T) {
	var gotBody string
	var gotPosition Position
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/discussions") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req struct {
			Body     string   `json:"body"`
			Position Position `json:"position"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		gotBody = req.Body
		gotPosition = req.Position
		json.NewEncoder(w).Encode(Discussion{
			ID:    "disc1",
			Notes: []Note{{ID: 1, Body: req.Body}},
		})
	}))
	defer srv.Close()

	pos := Position{
		PositionType: "text",
		BaseSHA:      "base123",
		StartSHA:     "start123",
		HeadSHA:      "head123",
		NewPath:      "main.go",
		NewLine:      42,
	}
	client := NewClient(srv.URL, "tok")
	disc, err := client.CreateInlineDiscussion(context.Background(), "proj", 3, "inline comment", pos)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody != "inline comment" {
		t.Fatalf("expected body 'inline comment', got %q", gotBody)
	}
	if gotPosition.PositionType != "text" {
		t.Fatalf("expected position_type text, got %s", gotPosition.PositionType)
	}
	if gotPosition.BaseSHA != "base123" {
		t.Fatalf("expected base_sha base123, got %s", gotPosition.BaseSHA)
	}
	if gotPosition.StartSHA != "start123" {
		t.Fatalf("expected start_sha start123, got %s", gotPosition.StartSHA)
	}
	if gotPosition.HeadSHA != "head123" {
		t.Fatalf("expected head_sha head123, got %s", gotPosition.HeadSHA)
	}
	if gotPosition.NewPath != "main.go" {
		t.Fatalf("expected new_path main.go, got %s", gotPosition.NewPath)
	}
	if gotPosition.NewLine != 42 {
		t.Fatalf("expected new_line 42, got %d", gotPosition.NewLine)
	}
	if disc.ID != "disc1" {
		t.Fatalf("expected discussion ID disc1, got %s", disc.ID)
	}
}

// --- Publisher clear tests ---

func TestClearInline_DeletesOnlyInlineMarkerNotes(t *testing.T) {
	inlineBody := "some comment\n\n" + platform.InlineMarker
	summaryBody := "summary\n\n" + platform.SummaryMarker
	userBody := "user comment without marker"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/discussions") && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode([]Discussion{
				{ID: "d1", Notes: []Note{{ID: 10, Body: inlineBody, System: false}}},
				{ID: "d2", Notes: []Note{{ID: 20, Body: summaryBody, System: false}}},
				{ID: "d3", Notes: []Note{{ID: 30, Body: userBody, System: false}}},
			})
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	result, err := pub.ClearInline()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.InlineDeleted != 1 {
		t.Fatalf("expected 1 inline deleted, got %d", result.InlineDeleted)
	}
	if result.SummaryDeleted != 0 {
		t.Fatalf("expected 0 summary deleted, got %d", result.SummaryDeleted)
	}
}

func TestClearSummary_DeletesOnlySummaryAndRequestChangesMarkerNotes(t *testing.T) {
	inlineBody := "some comment\n\n" + platform.InlineMarker
	summaryBody := "summary\n\n" + platform.SummaryMarker
	requestBody := "request changes\n\n" + platform.RequestChangesMarker
	userBody := "user comment"

	var mu sync.Mutex
	var deletedIDs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/discussions") && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode([]Discussion{
				{ID: "d1", Notes: []Note{{ID: 10, Body: inlineBody, System: false}}},
				{ID: "d2", Notes: []Note{{ID: 20, Body: summaryBody, System: false}}},
				{ID: "d3", Notes: []Note{{ID: 30, Body: requestBody, System: false}}},
				{ID: "d4", Notes: []Note{{ID: 40, Body: userBody, System: false}}},
			})
			return
		}
		if r.Method == http.MethodDelete {
			parts := strings.Split(r.URL.Path, "/")
			noteID := parts[len(parts)-1]
			mu.Lock()
			deletedIDs = append(deletedIDs, noteID)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	result, err := pub.ClearSummary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SummaryDeleted != 2 {
		t.Fatalf("expected 2 summary deleted, got %d", result.SummaryDeleted)
	}
	if result.InlineDeleted != 0 {
		t.Fatalf("expected 0 inline deleted, got %d", result.InlineDeleted)
	}
	if len(deletedIDs) != 2 {
		t.Fatalf("expected 2 delete calls, got %d: %v", len(deletedIDs), deletedIDs)
	}
	for _, id := range deletedIDs {
		if id != "20" && id != "30" {
			t.Fatalf("unexpected note deleted: %s (expected only 20 and 30)", id)
		}
	}
}

func TestClear_DoesNotDeleteUnmarkedUserNotes(t *testing.T) {
	userBody := "user comment without any marker"

	var deleteCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/discussions") && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode([]Discussion{
				{ID: "d1", Notes: []Note{{ID: 10, Body: userBody, System: false}}},
			})
			return
		}
		if r.Method == http.MethodDelete {
			deleteCount++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})

	result, err := pub.ClearInline()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleteCount != 0 {
		t.Fatalf("expected 0 deletes for unmarked notes, got %d", deleteCount)
	}
	if result.InlineDeleted != 0 {
		t.Fatalf("expected 0 inline deleted, got %d", result.InlineDeleted)
	}

	result, err = pub.ClearSummary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleteCount != 0 {
		t.Fatalf("expected 0 deletes for unmarked notes, got %d", deleteCount)
	}
	if result.SummaryDeleted != 0 {
		t.Fatalf("expected 0 summary deleted, got %d", result.SummaryDeleted)
	}
}

func TestClear_ContinuesOnDeleteFailure(t *testing.T) {
	inlineBody1 := "comment 1\n\n" + platform.InlineMarker
	inlineBody2 := "comment 2\n\n" + platform.InlineMarker

	var mu sync.Mutex
	deletedIDs := make(map[int]bool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/discussions") && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode([]Discussion{
				{ID: "d1", Notes: []Note{
					{ID: 10, Body: inlineBody1, System: false},
					{ID: 20, Body: inlineBody2, System: false},
				}},
			})
			return
		}
		if r.Method == http.MethodDelete {
			// Extract note ID from path suffix
			parts := strings.Split(r.URL.Path, "/")
			noteIDStr := parts[len(parts)-1]
			var noteID int
			for _, c := range noteIDStr {
				noteID = noteID*10 + int(c-'0')
			}
			mu.Lock()
			deletedIDs[noteID] = true
			mu.Unlock()
			// First delete fails
			if noteID == 10 {
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, `{"message":"internal error"}`)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	result, err := pub.ClearInline()

	// Should return an error summarizing the failure
	if err == nil {
		t.Fatal("expected error when delete fails")
	}
	if !strings.Contains(err.Error(), "1") {
		t.Fatalf("expected error to mention failure count, got: %v", err)
	}
	// Should still attempt to delete both
	if result.InlineDeleted != 1 {
		t.Fatalf("expected 1 successful delete, got %d", result.InlineDeleted)
	}
	if len(deletedIDs) != 2 {
		t.Fatalf("expected 2 delete attempts, got %d", len(deletedIDs))
	}
}

// --- Publish tests ---

// newPublishServer returns a test server that handles the full publish flow:
// versions, discussions (list + create inline), notes (create/update summary).
func newPublishServer(t *testing.T) (*httptest.Server, *publishRecorder) {
	t.Helper()
	rec := &publishRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.paths = append(rec.paths, r.Method+" "+r.URL.Path)
		rec.mu.Unlock()

		switch {
		// Diff versions
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{
				{ID: 1, BaseCommitSHA: "base1", StartCommitSHA: "start1", HeadCommitSHA: "head1"},
			})
			return

		// Inline discussions (POST)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/discussions"):
			var req struct {
				Body     string   `json:"body"`
				Position Position `json:"position"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			rec.mu.Lock()
			rec.inlineBodies = append(rec.inlineBodies, req.Body)
			rec.positions = append(rec.positions, req.Position)
			rec.mu.Unlock()
			json.NewEncoder(w).Encode(Discussion{ID: "new-disc", Notes: []Note{{ID: 100, Body: req.Body}}})
			return

		// List discussions
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			rec.mu.Lock()
			existing := rec.existingDiscussions
			rec.mu.Unlock()
			if existing == nil {
				existing = []Discussion{}
			}
			json.NewEncoder(w).Encode(existing)
			return

		// Create summary note
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			var req struct {
				Body string `json:"body"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			rec.mu.Lock()
			rec.summaryBodies = append(rec.summaryBodies, req.Body)
			rec.mu.Unlock()
			json.NewEncoder(w).Encode(Note{ID: 200, Body: req.Body})
			return

		// Update summary note
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/notes/"):
			var req struct {
				Body string `json:"body"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			rec.mu.Lock()
			rec.summaryBodies = append(rec.summaryBodies, req.Body)
			rec.updatedNoteIDs = append(rec.updatedNoteIDs, r.URL.Path)
			rec.mu.Unlock()
			json.NewEncoder(w).Encode(Note{Body: req.Body})
			return

		// Delete note
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return srv, rec
}

type publishRecorder struct {
	mu                  sync.Mutex
	paths               []string
	inlineBodies        []string
	positions           []Position
	summaryBodies       []string
	updatedNoteIDs      []string
	existingDiscussions []Discussion
}

func TestPublish_CreatesInlineDiscussionsWithMarkerAndPosition(t *testing.T) {
	srv, rec := newPublishServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	comments := []model.LlmComment{
		{Path: "main.go", Content: "fix this", StartLine: 10},
		{Path: "util.go", Content: "check that", StartLine: 20},
	}
	result, err := pub.Publish(comments)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.InlineCreated != 2 {
		t.Fatalf("expected 2 inline created, got %d", result.InlineCreated)
	}
	if len(rec.inlineBodies) != 2 {
		t.Fatalf("expected 2 inline bodies, got %d", len(rec.inlineBodies))
	}
	for _, body := range rec.inlineBodies {
		if !strings.Contains(body, platform.InlineMarker) {
			t.Fatalf("inline body missing marker: %q", body)
		}
	}
	if len(rec.positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(rec.positions))
	}
	if rec.positions[0].NewPath != "main.go" || rec.positions[0].NewLine != 10 {
		t.Fatalf("unexpected first position: %+v", rec.positions[0])
	}
	if rec.positions[1].NewPath != "util.go" || rec.positions[1].NewLine != 20 {
		t.Fatalf("unexpected second position: %+v", rec.positions[1])
	}
	if rec.positions[0].BaseSHA != "base1" {
		t.Fatalf("expected base_sha from diff version, got %s", rec.positions[0].BaseSHA)
	}
}

func TestPublish_ContinuesWhenInlineFails(t *testing.T) {
	var mu sync.Mutex
	inlineAttempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{
				{ID: 1, BaseCommitSHA: "b", StartCommitSHA: "s", HeadCommitSHA: "h"},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/discussions"):
			mu.Lock()
			inlineAttempts++
			n := inlineAttempts
			mu.Unlock()
			if n == 1 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				io.WriteString(w, `{"message":"cannot create"}`)
				return
			}
			var req struct{ Body string }
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(Discussion{ID: "d", Notes: []Note{{ID: 1, Body: req.Body}}})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			json.NewEncoder(w).Encode([]Discussion{})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			var req struct{ Body string }
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(Note{ID: 1, Body: req.Body})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	comments := []model.LlmComment{
		{Path: "a.go", Content: "first", StartLine: 1},
		{Path: "b.go", Content: "second", StartLine: 2},
	}
	result, err := pub.Publish(comments)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.InlineCreated != 1 {
		t.Fatalf("expected 1 inline created, got %d", result.InlineCreated)
	}
	if result.InlineFailed != 1 {
		t.Fatalf("expected 1 inline failed, got %d", result.InlineFailed)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
	if result.Warnings[0].Type != "inline_failed" {
		t.Fatalf("expected warning type inline_failed, got %s", result.Warnings[0].Type)
	}
}

func TestPublish_CreatesSummaryNoteWhenNoneExists(t *testing.T) {
	srv, rec := newPublishServer(t)
	defer srv.Close()
	rec.existingDiscussions = nil

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	result, err := pub.Publish([]model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.SummaryCreated {
		t.Fatal("expected summary created")
	}
	if result.SummaryUpdated {
		t.Fatal("expected summary not updated")
	}
	if len(rec.summaryBodies) != 1 {
		t.Fatalf("expected 1 summary body, got %d", len(rec.summaryBodies))
	}
	if !strings.Contains(rec.summaryBodies[0], platform.SummaryMarker) {
		t.Fatalf("summary body missing marker: %q", rec.summaryBodies[0])
	}
}

func TestPublish_UpdatesExistingSummaryMarkerNote(t *testing.T) {
	srv, rec := newPublishServer(t)
	defer srv.Close()
	rec.existingDiscussions = []Discussion{
		{ID: "d1", Notes: []Note{{ID: 50, Body: "old\n\n" + platform.SummaryMarker, System: false}}},
	}

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	result, err := pub.Publish([]model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SummaryCreated {
		t.Fatal("expected summary not created")
	}
	if !result.SummaryUpdated {
		t.Fatal("expected summary updated")
	}
	if len(rec.updatedNoteIDs) != 1 {
		t.Fatalf("expected 1 update, got %d", len(rec.updatedNoteIDs))
	}
	if !strings.Contains(rec.updatedNoteIDs[0], "/notes/50") {
		t.Fatalf("expected update to note 50, got %s", rec.updatedNoteIDs[0])
	}
}

func TestPublish_SummaryFailureIsFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{
				{ID: 1, BaseCommitSHA: "b", StartCommitSHA: "s", HeadCommitSHA: "h"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			json.NewEncoder(w).Encode([]Discussion{})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/discussions"):
			json.NewEncoder(w).Encode(Discussion{ID: "d", Notes: []Note{{ID: 1, Body: "ok"}}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"message":"internal error"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	_, err := pub.Publish([]model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1}})
	if err == nil {
		t.Fatal("expected fatal error for summary failure")
	}
	if !strings.Contains(err.Error(), "create summary note") {
		t.Fatalf("expected error context 'create summary note', got: %v", err)
	}
}

func TestPublish_RespectsNoInline(t *testing.T) {
	srv, rec := newPublishServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{
		ProjectID:       "proj",
		MergeRequestIID: 1,
		NoInline:        true,
	})
	result, err := pub.Publish([]model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.InlineCreated != 0 {
		t.Fatalf("expected 0 inline created, got %d", result.InlineCreated)
	}
	if len(rec.inlineBodies) != 0 {
		t.Fatalf("expected no inline discussions, got %d", len(rec.inlineBodies))
	}
}

func TestPublish_RespectsNoSummaryComment(t *testing.T) {
	srv, rec := newPublishServer(t)
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{
		ProjectID:        "proj",
		MergeRequestIID:  1,
		NoSummaryComment: true,
	})
	result, err := pub.Publish([]model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SummaryCreated || result.SummaryUpdated {
		t.Fatal("expected no summary created/updated")
	}
	if len(rec.summaryBodies) != 0 {
		t.Fatalf("expected no summary bodies, got %d", len(rec.summaryBodies))
	}
}

func TestPublish_ClearExistingClearsBeforePublishing(t *testing.T) {
	inlineBody := "old inline\n\n" + platform.InlineMarker
	summaryBody := "old summary\n\n" + platform.SummaryMarker

	var mu sync.Mutex
	var deleteIDs []string
	listCallCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{
				{ID: 1, BaseCommitSHA: "b", StartCommitSHA: "s", HeadCommitSHA: "h"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			mu.Lock()
			listCallCount++
			call := listCallCount
			mu.Unlock()
			// First two calls: clear inline + clear summary, return existing
			// Third call: findExistingSummaryNote, return empty
			if call <= 2 {
				json.NewEncoder(w).Encode([]Discussion{
					{ID: "d1", Notes: []Note{{ID: 10, Body: inlineBody}}},
					{ID: "d2", Notes: []Note{{ID: 20, Body: summaryBody}}},
				})
			} else {
				json.NewEncoder(w).Encode([]Discussion{})
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/discussions"):
			var req struct{ Body string }
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(Discussion{ID: "new", Notes: []Note{{ID: 100, Body: req.Body}}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			var req struct{ Body string }
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(Note{ID: 200, Body: req.Body})
		case r.Method == http.MethodDelete:
			parts := strings.Split(r.URL.Path, "/")
			mu.Lock()
			deleteIDs = append(deleteIDs, parts[len(parts)-1])
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{
		ProjectID:       "proj",
		MergeRequestIID: 1,
		ClearExisting:   true,
	})
	result, err := pub.Publish([]model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deleteIDs) != 2 {
		t.Fatalf("expected 2 deletes (clear), got %d: %v", len(deleteIDs), deleteIDs)
	}
	if result.InlineDeleted != 1 {
		t.Fatalf("expected 1 inline deleted, got %d", result.InlineDeleted)
	}
	if result.SummaryDeleted != 1 {
		t.Fatalf("expected 1 summary deleted, got %d", result.SummaryDeleted)
	}
	if result.InlineCreated != 1 {
		t.Fatalf("expected 1 inline created after clear, got %d", result.InlineCreated)
	}
}

func TestPublish_EmptyDiffVersionsMarksInlineFailedAndPublishesSummary(t *testing.T) {
	var mu sync.Mutex
	var inlineDiscussionCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			json.NewEncoder(w).Encode([]Discussion{})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/discussions"):
			mu.Lock()
			inlineDiscussionCalls++
			mu.Unlock()
			json.NewEncoder(w).Encode(Discussion{ID: "d", Notes: []Note{{ID: 1}}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			var req struct{ Body string }
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(Note{ID: 200, Body: req.Body})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	comments := []model.LlmComment{
		{Path: "a.go", Content: "fix1", StartLine: 1},
		{Path: "b.go", Content: "fix2", StartLine: 2},
	}
	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	result, err := pub.Publish(comments)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inlineDiscussionCalls != 0 {
		t.Fatalf("expected 0 inline discussion calls, got %d", inlineDiscussionCalls)
	}
	if result.InlineCreated != 0 {
		t.Fatalf("expected 0 inline created, got %d", result.InlineCreated)
	}
	if result.InlineFailed != len(comments) {
		t.Fatalf("expected %d inline failed, got %d", len(comments), result.InlineFailed)
	}
	if len(result.Warnings) != len(comments) {
		t.Fatalf("expected %d warnings, got %d", len(comments), len(result.Warnings))
	}
	for _, w := range result.Warnings {
		if w.Type != "inline_failed" {
			t.Fatalf("expected warning type inline_failed, got %s", w.Type)
		}
		if !strings.Contains(w.Message, "diff version") {
			t.Fatalf("expected warning to mention diff version, got: %s", w.Message)
		}
	}
	if !result.SummaryCreated {
		t.Fatal("expected summary created")
	}
}

// --- Task 5 tests: MR description, request changes ---

func TestGetMergeRequest_DecodesDescription(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/merge_requests/7") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(MergeRequest{IID: 7, Description: "user wrote this"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	mr, err := client.GetMergeRequest(context.Background(), "proj", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mr.Description != "user wrote this" {
		t.Fatalf("expected description 'user wrote this', got %q", mr.Description)
	}
}

func TestUpdateDescription_SendsPUTWithDescription(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(MergeRequest{IID: 3})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	err := client.UpdateDescription(context.Background(), "proj", 3, "new desc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("expected PUT, got %s", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/merge_requests/3") {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody["description"] != "new desc" {
		t.Fatalf("expected description 'new desc', got %q", gotBody["description"])
	}
}

func TestUnapprove_SendsPOSTToUnapprove(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	err := client.Unapprove(context.Background(), "proj", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/merge_requests/5/unapprove") {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestPublish_PRSummary_AppendsManagedBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			json.NewEncoder(w).Encode([]Discussion{})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/merge_requests/"):
			json.NewEncoder(w).Encode(MergeRequest{IID: 1, Description: "User wrote this"})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/merge_requests/"):
			var req map[string]string
			json.NewDecoder(r.Body).Decode(&req)
			desc := req["description"]
			if !strings.Contains(desc, "User wrote this") {
				t.Errorf("user text lost in description: %q", desc)
			}
			if !strings.Contains(desc, platform.PRSummaryStartMarker) {
				t.Errorf("missing PR summary start marker: %q", desc)
			}
			if !strings.Contains(desc, platform.SummaryMarker) {
				t.Errorf("missing summary marker: %q", desc)
			}
			json.NewEncoder(w).Encode(MergeRequest{IID: 1})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			var req struct{ Body string }
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(Note{ID: 200, Body: req.Body})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{
		ProjectID:       "proj",
		MergeRequestIID: 1,
		PRSummary:       true,
	})
	result, err := pub.Publish([]model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.DescriptionUpdated {
		t.Fatal("expected description updated")
	}
}

func TestPublish_PRSummary_ReplacesExistingBlockPreservesUserText(t *testing.T) {
	existingDesc := "Title\n\n---\n" + platform.PRSummaryStartMarker + "\nold summary\n" + platform.PRSummaryEndMarker + "\n\nTrailing"
	var updatedDesc string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			json.NewEncoder(w).Encode([]Discussion{})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/merge_requests/"):
			json.NewEncoder(w).Encode(MergeRequest{IID: 1, Description: existingDesc})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/merge_requests/"):
			var req map[string]string
			json.NewDecoder(r.Body).Decode(&req)
			updatedDesc = req["description"]
			json.NewEncoder(w).Encode(MergeRequest{IID: 1})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			var req struct{ Body string }
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(Note{ID: 200, Body: req.Body})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{
		ProjectID:       "proj",
		MergeRequestIID: 1,
		PRSummary:       true,
	})
	_, err := pub.Publish([]model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(updatedDesc, "old summary") {
		t.Fatalf("old summary remained: %q", updatedDesc)
	}
	if !strings.Contains(updatedDesc, "Title") {
		t.Fatalf("leading user text lost: %q", updatedDesc)
	}
	if !strings.Contains(updatedDesc, "Trailing") {
		t.Fatalf("trailing user text lost: %q", updatedDesc)
	}
	if strings.Count(updatedDesc, platform.PRSummaryStartMarker) != 1 {
		t.Fatalf("expected exactly one managed block, got %d", strings.Count(updatedDesc, platform.PRSummaryStartMarker))
	}
}

func TestPublish_PRSummaryFalse_DoesNotTouchDescription(t *testing.T) {
	var mrGetCalled, mrPutCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			json.NewEncoder(w).Encode([]Discussion{})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/merge_requests/"):
			mrGetCalled = true
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/merge_requests/"):
			mrPutCalled = true
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			var req struct{ Body string }
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(Note{ID: 200, Body: req.Body})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{
		ProjectID:       "proj",
		MergeRequestIID: 1,
		PRSummary:       false,
	})
	_, err := pub.Publish([]model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mrGetCalled {
		t.Fatal("GET /merge_requests should not be called when PRSummary is false")
	}
	if mrPutCalled {
		t.Fatal("PUT /merge_requests should not be called when PRSummary is false")
	}
}

func TestRequestChanges_PostsNoteWithMarker(t *testing.T) {
	var noteBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/unapprove"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			var req struct{ Body string }
			json.NewDecoder(r.Body).Decode(&req)
			noteBody = req.Body
			json.NewEncoder(w).Encode(Note{ID: 300, Body: req.Body})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	comments := []model.LlmComment{
		{Path: "main.go", Content: "critical issue", StartLine: 10},
	}
	result, err := pub.RequestChanges(comments)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(noteBody, platform.RequestChangesMarker) {
		t.Fatalf("request-changes note missing marker: %q", noteBody)
	}
	if !strings.Contains(noteBody, "main.go:10") {
		t.Fatalf("request-changes note missing file:line: %q", noteBody)
	}
	_ = result
}

func TestRequestChanges_ContinuesOnUnapprove403(t *testing.T) {
	var noteCreated bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/unapprove"):
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"message":"403 Forbidden"}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			noteCreated = true
			json.NewEncoder(w).Encode(Note{ID: 300})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	result, err := pub.RequestChanges([]model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !noteCreated {
		t.Fatal("request-changes note should be created even when unapprove 403s")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning for unapprove 403, got %d", len(result.Warnings))
	}
	if !strings.Contains(result.Warnings[0].Message, "403") {
		t.Fatalf("warning should mention 403, got: %s", result.Warnings[0].Message)
	}
}

func TestRequestChanges_FatalWhenNoteFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/unapprove"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"message":"internal error"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	_, err := pub.RequestChanges([]model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1}})
	if err == nil {
		t.Fatal("expected fatal error when request-changes note fails")
	}
	if !strings.Contains(err.Error(), "request-changes note") {
		t.Fatalf("expected error context 'request-changes note', got: %v", err)
	}
}

func TestRequestChanges_ContinuesOnUnapproveNon403(t *testing.T) {
	var noteCreated bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/unapprove"):
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"message":"internal server error"}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			noteCreated = true
			json.NewEncoder(w).Encode(Note{ID: 300})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	result, err := pub.RequestChanges([]model.LlmComment{{Path: "a.go", Content: "fix", StartLine: 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !noteCreated {
		t.Fatal("request-changes note should be created even when unapprove returns 500")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
	if result.Warnings[0].Type != "unapprove_failed" {
		t.Fatalf("expected warning type unapprove_failed, got %s", result.Warnings[0].Type)
	}
	if !strings.Contains(result.Warnings[0].Message, "unapprove") {
		t.Fatalf("warning should mention unapprove, got: %s", result.Warnings[0].Message)
	}
	if !strings.Contains(result.Warnings[0].Message, "500") {
		t.Fatalf("warning should mention HTTP status 500, got: %s", result.Warnings[0].Message)
	}
}

// --- diffAddedLines tests ---

func TestDiffAddedLines_SimpleAddition(t *testing.T) {
	diff := "@@ -1,3 +1,5 @@\n context\n+added1\n+added2\n unchanged"
	added := diffAddedLines(diff)
	if !added[2] {
		t.Fatal("expected line 2 to be added")
	}
	if !added[3] {
		t.Fatal("expected line 3 to be added")
	}
	if added[1] {
		t.Fatal("line 1 should not be added (context line)")
	}
	if added[4] {
		t.Fatal("line 4 should not be added (unchanged line)")
	}
}

func TestDiffAddedLines_MultipleHunks(t *testing.T) {
	diff := "@@ -1,3 +1,4 @@\n context\n+first\n unchanged\n@@ -10,3 +11,4 @@\n before\n+second\n after"
	added := diffAddedLines(diff)
	if !added[2] {
		t.Fatal("expected line 2 (first hunk) to be added")
	}
	if !added[12] {
		t.Fatal("expected line 12 (second hunk) to be added")
	}
}

func TestDiffAddedLines_DeletionsOnly(t *testing.T) {
	diff := "@@ -1,3 +1,2 @@\n-deleted\n remaining\n context"
	added := diffAddedLines(diff)
	if len(added) != 0 {
		t.Fatalf("expected no added lines, got %d", len(added))
	}
}

func TestDiffAddedLines_EmptyDiff(t *testing.T) {
	added := diffAddedLines("")
	if len(added) != 0 {
		t.Fatalf("expected no added lines for empty diff, got %d", len(added))
	}
}

func TestDiffAddedLines_IgnoresFileHeaders(t *testing.T) {
	diff := "@@ -1,3 +1,4 @@\n--- a/file.go\n+++ b/file.go\n context\n+added\n rest"
	added := diffAddedLines(diff)
	// The "---" and "+++" lines should NOT be counted as added.
	if added[2] {
		t.Fatal("line 2 (from +++ header) should not be counted as added")
	}
}

// --- isAddedLine tests ---

func TestIsAddedLine_ReturnsTrueForAddedLine(t *testing.T) {
	diff := "@@ -1,3 +1,5 @@\n context\n+added1\n+added2\n unchanged"
	if !isAddedLine(diff, 2) {
		t.Fatal("expected line 2 to be added")
	}
}

func TestIsAddedLine_ReturnsFalseForContextLine(t *testing.T) {
	diff := "@@ -1,3 +1,5 @@\n context\n+added1\n+added2\n unchanged"
	if isAddedLine(diff, 1) {
		t.Fatal("line 1 should not be added")
	}
}

func TestIsAddedLine_ReturnsFalseForZeroLine(t *testing.T) {
	diff := "@@ -1,3 +1,5 @@\n+added"
	if isAddedLine(diff, 0) {
		t.Fatal("line 0 should not be valid")
	}
}

func TestIsAddedLine_ReturnsFalseForNegativeLine(t *testing.T) {
	diff := "@@ -1,3 +1,5 @@\n+added"
	if isAddedLine(diff, -1) {
		t.Fatal("negative line should not be valid")
	}
}

func TestIsAddedLine_PermissiveOnEmptyDiff(t *testing.T) {
	// Empty diff means no info available; be permissive.
	if !isAddedLine("", 5) {
		t.Fatal("should be permissive when diff is empty")
	}
}

// --- Position validation in Publish ---

func TestPublish_SkipsCommentWithZeroStartLine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{
				{BaseCommitSHA: "b", StartCommitSHA: "s", HeadCommitSHA: "h"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/diffs"):
			json.NewEncoder(w).Encode([]ChangedFile{
				{NewPath: "a.go", Diff: "@@ -1,3 +1,4 @@\n context\n+added\n rest"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			json.NewEncoder(w).Encode([]Discussion{})
		default:
			// Should not be called for inline discussion.
			if strings.HasSuffix(r.URL.Path, "/discussions") && r.Method == http.MethodPost {
				t.Fatal("should NOT create inline discussion for zero start line")
			}
			json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1, NoSummaryComment: true})
	result, err := pub.Publish([]model.LlmComment{
		{Path: "a.go", Content: "issue", StartLine: 0},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.InlineCreated != 0 {
		t.Fatalf("expected 0 inline created, got %d", result.InlineCreated)
	}
	if result.InlineSkipped != 1 {
		t.Fatalf("expected 1 inline skipped, got %d", result.InlineSkipped)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
	if result.Warnings[0].Type != "inline_skipped" {
		t.Fatalf("expected warning type inline_skipped, got %s", result.Warnings[0].Type)
	}
}

func TestPublish_SkipsCommentWithLineNotInDiff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{
				{BaseCommitSHA: "b", StartCommitSHA: "s", HeadCommitSHA: "h"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/diffs"):
			json.NewEncoder(w).Encode([]ChangedFile{
				{NewPath: "a.go", Diff: "@@ -1,3 +1,4 @@\n context\n+added\n rest"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			json.NewEncoder(w).Encode([]Discussion{})
		default:
			if strings.HasSuffix(r.URL.Path, "/discussions") && r.Method == http.MethodPost {
				t.Fatal("should NOT create inline discussion for line not in diff")
			}
			json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1, NoSummaryComment: true})
	// Line 1 is a context line (not added), line 2 is the added line.
	result, err := pub.Publish([]model.LlmComment{
		{Path: "a.go", Content: "issue", StartLine: 1},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.InlineCreated != 0 {
		t.Fatalf("expected 0 inline created, got %d", result.InlineCreated)
	}
	if result.InlineSkipped != 1 {
		t.Fatalf("expected 1 inline skipped, got %d", result.InlineSkipped)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
	if result.Warnings[0].Type != "inline_skipped" {
		t.Fatalf("expected warning type inline_skipped, got %s", result.Warnings[0].Type)
	}
	if !strings.Contains(result.Warnings[0].Message, "no added line in range") {
		t.Fatalf("expected 'no added line in range' in message, got: %s", result.Warnings[0].Message)
	}
}

func TestPublish_CreatesInlineForAddedLine(t *testing.T) {
	var discussionCreated bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{
				{BaseCommitSHA: "b", StartCommitSHA: "s", HeadCommitSHA: "h"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/diffs"):
			json.NewEncoder(w).Encode([]ChangedFile{
				{NewPath: "a.go", Diff: "@@ -1,3 +1,4 @@\n context\n+added\n rest"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			json.NewEncoder(w).Encode([]Discussion{})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/discussions"):
			discussionCreated = true
			json.NewEncoder(w).Encode(Discussion{ID: "disc-1"})
		default:
			json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1, NoSummaryComment: true})
	// Line 2 is the added line in the diff.
	result, err := pub.Publish([]model.LlmComment{
		{Path: "a.go", Content: "issue", StartLine: 2},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !discussionCreated {
		t.Fatal("expected inline discussion to be created for added line")
	}
	if result.InlineCreated != 1 {
		t.Fatalf("expected 1 inline created, got %d", result.InlineCreated)
	}
	if result.InlineSkipped != 0 {
		t.Fatalf("expected 0 inline skipped, got %d", result.InlineSkipped)
	}
}

// --- selectAddedLineInRange tests ---

func TestSelectAddedLineInRange_StartLineAdded(t *testing.T) {
	diff := "@@ -1,3 +1,5 @@\n context\n+added1\n+added2\n unchanged"
	line, ok := selectAddedLineInRange(diff, 2, 2)
	if !ok {
		t.Fatal("expected ok=true when startLine is an added line")
	}
	if line != 2 {
		t.Fatalf("expected line 2, got %d", line)
	}
}

func TestSelectAddedLineInRange_ContextStartAddedLineLaterInRange(t *testing.T) {
	// Line 1 is context, line 2 is added, line 3 is context.
	diff := "@@ -1,3 +1,5 @@\n context\n+added\n unchanged"
	line, ok := selectAddedLineInRange(diff, 1, 3)
	if !ok {
		t.Fatal("expected ok=true when range contains an added line")
	}
	if line != 2 {
		t.Fatalf("expected line 2 (first added in range), got %d", line)
	}
}

func TestSelectAddedLineInRange_NoAddedLineInRange(t *testing.T) {
	// All lines in range are context lines.
	diff := "@@ -1,3 +1,5 @@\n context\n+added\n unchanged"
	_, ok := selectAddedLineInRange(diff, 1, 1)
	if ok {
		t.Fatal("expected ok=false when range has no added lines")
	}
}

func TestSelectAddedLineInRange_EndBeforeStartTreatsSingleLine(t *testing.T) {
	diff := "@@ -1,3 +1,5 @@\n context\n+added\n unchanged"
	// endLine < startLine: should treat as single-line range (startLine only).
	line, ok := selectAddedLineInRange(diff, 2, 0)
	if !ok {
		t.Fatal("expected ok=true when endLine < startLine and startLine is added")
	}
	if line != 2 {
		t.Fatalf("expected line 2, got %d", line)
	}
}

func TestSelectAddedLineInRange_StartLineZero(t *testing.T) {
	diff := "@@ -1,3 +1,5 @@\n context\n+added\n unchanged"
	_, ok := selectAddedLineInRange(diff, 0, 3)
	if ok {
		t.Fatal("expected ok=false for startLine <= 0")
	}
}

func TestSelectAddedLineInRange_EmptyDiffFallback(t *testing.T) {
	// Empty diff: permissive fallback returns startLine.
	line, ok := selectAddedLineInRange("", 5, 10)
	if !ok {
		t.Fatal("expected ok=true for empty diff (permissive fallback)")
	}
	if line != 5 {
		t.Fatalf("expected fallback to startLine=5, got %d", line)
	}
}

// --- Publish integration tests for range-based anchor selection ---

func TestPublish_ContextStartRangeAnchorsToAddedLine(t *testing.T) {
	var capturedPosition Position
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{
				{BaseCommitSHA: "b", StartCommitSHA: "s", HeadCommitSHA: "h"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/diffs"):
			// Line 1: context, Line 2: added, Line 3: context
			json.NewEncoder(w).Encode([]ChangedFile{
				{NewPath: "service/user.go", Diff: "@@ -1,3 +1,5 @@\n context\n+added\n unchanged"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			json.NewEncoder(w).Encode([]Discussion{})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/discussions"):
			var req struct {
				Body     string   `json:"body"`
				Position Position `json:"position"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			capturedPosition = req.Position
			json.NewEncoder(w).Encode(Discussion{ID: "d1", Notes: []Note{{ID: 1, Body: req.Body}}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			var req struct{ Body string }
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(Note{ID: 200, Body: req.Body})
		default:
			json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	// StartLine=1 is context, EndLine=3 includes added line 2.
	result, err := pub.Publish([]model.LlmComment{
		{Path: "service/user.go", Content: "issue in range", StartLine: 1, EndLine: 3},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.InlineCreated != 1 {
		t.Fatalf("expected 1 inline created, got %d", result.InlineCreated)
	}
	if result.InlineSkipped != 0 {
		t.Fatalf("expected 0 inline skipped, got %d", result.InlineSkipped)
	}
	if capturedPosition.NewLine != 2 {
		t.Fatalf("expected position.new_line=2 (first added in range), got %d", capturedPosition.NewLine)
	}
	if capturedPosition.NewPath != "service/user.go" {
		t.Fatalf("expected position.new_path=service/user.go, got %s", capturedPosition.NewPath)
	}
}

func TestPublish_NoAddedLineInRangeSkipsInline(t *testing.T) {
	var inlineDiscussionCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode([]DiffVersion{
				{BaseCommitSHA: "b", StartCommitSHA: "s", HeadCommitSHA: "h"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/diffs"):
			// All context lines, no additions.
			json.NewEncoder(w).Encode([]ChangedFile{
				{NewPath: "a.go", Diff: "@@ -1,3 +1,3 @@\n line1\n line2\n line3"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discussions"):
			json.NewEncoder(w).Encode([]Discussion{})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/discussions"):
			inlineDiscussionCalled = true
			json.NewEncoder(w).Encode(Discussion{ID: "d1", Notes: []Note{{ID: 1}}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes"):
			var req struct{ Body string }
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(Note{ID: 200, Body: req.Body})
		default:
			json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	pub := NewPublisher(client, platform.PublishOptions{ProjectID: "proj", MergeRequestIID: 1})
	// Range 1..3 but diff has no added lines.
	result, err := pub.Publish([]model.LlmComment{
		{Path: "a.go", Content: "issue", StartLine: 1, EndLine: 3},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inlineDiscussionCalled {
		t.Fatal("should NOT call inline discussion API when no added line in range")
	}
	if result.InlineSkipped != 1 {
		t.Fatalf("expected InlineSkipped=1, got %d", result.InlineSkipped)
	}
	if result.InlineCreated != 0 {
		t.Fatalf("expected 0 inline created, got %d", result.InlineCreated)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
	if result.Warnings[0].Type != "inline_skipped" {
		t.Fatalf("expected warning type inline_skipped, got %s", result.Warnings[0].Type)
	}
}
