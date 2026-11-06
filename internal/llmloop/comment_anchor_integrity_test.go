// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// These tests characterize issue #746: a comment whose quoted code belongs to a
// different file than the one it was filed against is emitted verbatim with the
// wrong path at start_line 0 / end_line 0 once every resolution stage declines.

const pomXMLPath = "pom.xml"
const javaPath = "src/main/java/com/qz/onboard/pojo/DevOpsResource.java"

const pomXMLContent = `<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>com.qz.onboard</groupId>
</project>
`

const pomXMLDiff = `--- a/pom.xml
+++ b/pom.xml
@@ -1,3 +1,4 @@
 <project>
+  <modelVersion>4.0.0</modelVersion>
   <groupId>com.qz.onboard</groupId>
 </project>
`

const javaContent = `package com.qz.onboard.pojo;

public class DevOpsResource {
    private Long id;
    private String resourceId;

    public String toString() {
        System.out.println("id=" + id);
        System.out.println("resourceId=" + resourceId);
        System.out.println("name=" + name);
        return "DevOpsResource";
    }
}
`

// driftedJavaQuote quotes javaContent with one internal-whitespace drift
// (`id );` instead of `id);`) — the shape reported in issue #746.
const driftedJavaQuote = `    public String toString() {
        System.out.println("id=" + id );
        System.out.println("resourceId=" + resourceId);
        System.out.println("name=" + name);
`

func anchorTestDiff(path, diffText, content string) model.Diff {
	return model.Diff{OldPath: path, NewPath: path, Diff: diffText, NewFileContent: content}
}

func anchorTestRunner(collector *tool.CommentCollector, diffs []model.Diff, client llm.LLMClient, reLocation bool) *Runner {
	byPath := make(map[string]*model.Diff, len(diffs))
	for i := range diffs {
		byPath[diffs[i].NewPath] = &diffs[i]
	}
	tpl := template.Template{MaxTokens: 100000, MaxToolRequestTimes: 10}
	if reLocation {
		tpl.ReLocationTask = &template.LlmConversation{
			Messages: []template.ChatMessage{{Role: "user", Content: "locate {existing_code} in {diff}"}},
		}
	}
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()
	return NewRunner(Deps{
		LLMClient:        client,
		Model:            "fake",
		Template:         tpl,
		Tools:            reg,
		CommentCollector: collector,
		Session:          session.New("/tmp/test-repo", "main", "fake", session.SessionOptions{}),
		DiffLookup: func(path string) *model.Diff {
			return byPath[path]
		},
		AllDiffs: func() []model.Diff {
			return diffs
		},
	})
}

func submitCodeComment(t *testing.T, r *Runner, newPath string, comments []map[string]any) {
	t.Helper()
	args, err := json.Marshal(map[string]any{"comments": comments})
	if err != nil {
		t.Fatal(err)
	}
	cp := r.executeToolCall(context.Background(), newPath, llm.ToolCall{
		Function: llm.FunctionCall{Name: "code_comment", Arguments: string(args)},
	}, nil, "")
	if cp.Data != tool.CommentSucceed {
		t.Fatalf("code_comment result = %q, want %q", cp.Data, tool.CommentSucceed)
	}
}

// TestAnchorIntegrity_DriftedQuoteRefilesToRealFile is the grouped-review
// reproducer from issue #746: the model files Java advice against pom.xml and
// quotes the Java file with one internal-whitespace drift. The eliding tier
// lets the cross-file search place it: the comment is re-filed onto the Java
// file with real lines (the pre-fix behavior was emission under pom.xml at
// 0/0).
func TestAnchorIntegrity_DriftedQuoteRefilesToRealFile(t *testing.T) {
	collector := tool.NewCommentCollector()
	r := anchorTestRunner(collector, []model.Diff{
		anchorTestDiff(pomXMLPath, pomXMLDiff, pomXMLContent),
		anchorTestDiff(javaPath, "", javaContent),
	}, &fakeClient{}, false)

	submitCodeComment(t, r, pomXMLPath+","+javaPath, []map[string]any{{
		"path":          pomXMLPath,
		"content":       "toString() should return, not print",
		"existing_code": driftedJavaQuote,
	}})

	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("collected %d comments, want 1", len(comments))
	}
	if comments[0].Path != javaPath || comments[0].StartLine != 7 || comments[0].EndLine != 10 {
		t.Fatalf("comment = %s:%d-%d; want re-filed onto the Java file at 7..10",
			comments[0].Path, comments[0].StartLine, comments[0].EndLine)
	}
}

// TestAnchorIntegrity_ZeroOverlapQuoteSkipsRelocation verifies the stage-3
// plausibility gate: with the quoted code present nowhere in the run, the LLM
// re-location step is NOT invoked (it would only be able to fabricate a
// location from the wrong file's diff), and the comment stays unlocated with
// its evidence quote intact.
func TestAnchorIntegrity_ZeroOverlapQuoteSkipsRelocation(t *testing.T) {
	collector := tool.NewCommentCollector()
	relocator := &fakeClient{responses: []*llm.ChatResponse{reLocationCodeBlockResponse(pomXMLSnippet)}}
	r := anchorTestRunner(collector, []model.Diff{
		anchorTestDiff(pomXMLPath, pomXMLDiff, pomXMLContent),
	}, relocator, true)

	quote := strings.ReplaceAll(driftedJavaQuote, "id );", "id);")
	submitCodeComment(t, r, pomXMLPath, []map[string]any{{
		"path":          pomXMLPath,
		"content":       "toString() should return, not print",
		"existing_code": quote,
	}})

	if relocator.calls != 0 {
		t.Fatalf("re-location LLM invoked %d times, want 0 (zero-overlap quote)", relocator.calls)
	}
	if got := collector.Comments(); len(got) != 0 {
		t.Fatalf("collected %d comments, want 0 (unresolvable quote dropped)", len(got))
	}
	warnings := r.Warnings()
	if len(warnings) != 1 || warnings[0].Type != "comment_unresolved" || warnings[0].File != pomXMLPath {
		t.Fatalf("warnings = %+v, want one comment_unresolved on %s", warnings, pomXMLPath)
	}
}

// TestAnchorIntegrity_PlausibleQuoteStillRescued verifies the LLM rescue stays
// available for its intended case: a quote that shares lines with the claimed
// file but cannot be matched deterministically (a paraphrased line alongside a
// real one) still gets the re-location attempt, which here locates it.
func TestAnchorIntegrity_PlausibleQuoteStillRescued(t *testing.T) {
	collector := tool.NewCommentCollector()
	relocator := &fakeClient{responses: []*llm.ChatResponse{reLocationCodeBlockResponse(pomXMLSnippet)}}
	r := anchorTestRunner(collector, []model.Diff{
		anchorTestDiff(pomXMLPath, pomXMLDiff, pomXMLContent),
	}, relocator, true)

	submitCodeComment(t, r, pomXMLPath, []map[string]any{{
		"path":    pomXMLPath,
		"content": "the project coordinates are wrong",
		// First line exists in pom.xml (non-trivial, so the quote is
		// plausible); the second does not, so deterministic matching fails.
		"existing_code": "<project>\n  <artifactId>onboard-parent</artifactId>",
	}})

	if relocator.calls != 1 {
		t.Fatalf("re-location LLM invoked %d times, want 1 (plausible quote)", relocator.calls)
	}
	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("collected %d comments, want 1", len(comments))
	}
	if comments[0].StartLine != 1 || comments[0].EndLine != 3 {
		t.Fatalf("comment = %s:%d-%d; want rescued onto pom.xml lines 1..3",
			comments[0].Path, comments[0].StartLine, comments[0].EndLine)
	}
}

// TestAnchorIntegrity_ScanPathKeepsUnconditionalRelocation verifies scan-mode
// wiring (AllDiffs nil) keeps the LLM rescue unconditional: even a
// zero-overlap quote gets the attempt, exactly as before the gate existed.
func TestAnchorIntegrity_ScanPathKeepsUnconditionalRelocation(t *testing.T) {
	collector := tool.NewCommentCollector()
	relocator := &fakeClient{responses: []*llm.ChatResponse{reLocationCodeBlockResponse(pomXMLSnippet)}}
	r := anchorTestRunner(collector, []model.Diff{
		anchorTestDiff(pomXMLPath, pomXMLDiff, pomXMLContent),
	}, relocator, true)
	r.deps.AllDiffs = nil

	submitCodeComment(t, r, pomXMLPath, []map[string]any{{
		"path":          pomXMLPath,
		"content":       "toString() should return, not print",
		"existing_code": strings.ReplaceAll(driftedJavaQuote, "id );", "id);"),
	}})

	if relocator.calls != 1 {
		t.Fatalf("re-location LLM invoked %d times, want 1 (scan keeps unconditional rescue)", relocator.calls)
	}
}

// TestAnchorIntegrity_GroupKeyPathDropped verifies INV-2: a path-less comment
// under a grouped review is bound to the comma-joined group key by the parser
// fallback; the run holds no diff for that pseudo-path, so the emission gate
// drops it with a warning instead of emitting it under a path that can never
// resolve.
func TestAnchorIntegrity_GroupKeyPathDropped(t *testing.T) {
	collector := tool.NewCommentCollector()
	r := anchorTestRunner(collector, []model.Diff{
		anchorTestDiff(pomXMLPath, pomXMLDiff, pomXMLContent),
	}, &fakeClient{}, false)

	groupKey := pomXMLPath + "," + javaPath
	submitCodeComment(t, r, groupKey, []map[string]any{{
		"content": "general advice about the group",
	}})

	if got := collector.Comments(); len(got) != 0 {
		t.Fatalf("collected %d comments, want 0 (group-key path dropped)", len(got))
	}
	warnings := r.Warnings()
	if len(warnings) != 1 || warnings[0].Type != "comment_unresolved" || warnings[0].File != groupKey {
		t.Fatalf("warnings = %+v, want one comment_unresolved on the group key", warnings)
	}
}

// TestAnchorIntegrity_UnresolvableQuoteDroppedWhenRealFileAbsent is the issue
// #746 end shape: Java advice filed against pom.xml with the Java file not
// part of the run. No stage can place it, so it is dropped with a warning
// rather than emitted under pom.xml at 0/0.
func TestAnchorIntegrity_UnresolvableQuoteDroppedWhenRealFileAbsent(t *testing.T) {
	collector := tool.NewCommentCollector()
	r := anchorTestRunner(collector, []model.Diff{
		anchorTestDiff(pomXMLPath, pomXMLDiff, pomXMLContent),
	}, &fakeClient{}, false)

	submitCodeComment(t, r, pomXMLPath, []map[string]any{{
		"path":          pomXMLPath,
		"content":       "toString() should return, not print",
		"existing_code": strings.ReplaceAll(driftedJavaQuote, "id );", "id);"),
	}})

	if got := collector.Comments(); len(got) != 0 {
		t.Fatalf("collected %d comments, want 0 (wrong-file comment dropped)", len(got))
	}
	warnings := r.Warnings()
	if len(warnings) != 1 || warnings[0].Type != "comment_unresolved" {
		t.Fatalf("warnings = %+v, want one comment_unresolved", warnings)
	}
}

// TestAnchorIntegrity_DegenerateQuoteKeptAsGeneralAdvice verifies INV-4's
// degenerate edge: a quote that normalizes to zero lines (a bare code fence)
// is no evidence at all, so on a reviewed file the comment is kept as general
// advice exactly like an empty quote.
func TestAnchorIntegrity_DegenerateQuoteKeptAsGeneralAdvice(t *testing.T) {
	collector := tool.NewCommentCollector()
	r := anchorTestRunner(collector, []model.Diff{
		anchorTestDiff(pomXMLPath, pomXMLDiff, pomXMLContent),
	}, &fakeClient{}, false)

	submitCodeComment(t, r, pomXMLPath, []map[string]any{{
		"path":          pomXMLPath,
		"content":       "keep the POM tidy",
		"existing_code": "\n   \n",
	}})

	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("collected %d comments, want 1 (degenerate quote kept as general advice)", len(comments))
	}
	if comments[0].StartLine != 0 {
		t.Fatalf("comment lines = %d..%d, want 0/0 general advice", comments[0].StartLine, comments[0].EndLine)
	}
	if len(r.Warnings()) != 0 {
		t.Fatalf("warnings = %+v, want none", r.Warnings())
	}
}

// TestAnchorIntegrity_ScanPathCollectsUnchanged verifies the emission gate is
// review-only: with AllDiffs nil (scan wiring), even an unresolvable
// quote-less group-key comment is collected exactly as before, with no
// warnings.
func TestAnchorIntegrity_ScanPathCollectsUnchanged(t *testing.T) {
	collector := tool.NewCommentCollector()
	r := anchorTestRunner(collector, []model.Diff{
		anchorTestDiff(pomXMLPath, pomXMLDiff, pomXMLContent),
	}, &fakeClient{}, false)
	r.deps.AllDiffs = nil

	groupKey := pomXMLPath + "," + javaPath
	submitCodeComment(t, r, groupKey, []map[string]any{{
		"content": "general advice about the group",
	}})

	comments := collector.Comments()
	if len(comments) != 1 || comments[0].Path != groupKey {
		t.Fatalf("comments = %+v, want 1 collected under the group key (scan unchanged)", comments)
	}
	if len(r.Warnings()) != 0 {
		t.Fatalf("warnings = %+v, want none (scan unchanged)", r.Warnings())
	}
}

// TestAnchorIntegrity_AmbiguousSnippetDroppedNotGuessed covers the ambiguous
// cross-file edge: when the quoted snippet exists verbatim in TWO files of the
// run, RelocateAcrossFiles declines rather than guessing, so the comment ends
// resolution unlocated and the emission gate drops it with a warning. The
// single-copy subtest is the contrast that keeps this test honest: the same
// snippet with one copy removed must re-file, proving the two-copy drop is the
// ambiguity decline and not an absence-of-match artifact.
func TestAnchorIntegrity_AmbiguousSnippetDroppedNotGuessed(t *testing.T) {
	const dupA = "src/main/java/com/qz/onboard/pojo/DupA.java"
	const dupB = "src/main/java/com/qz/onboard/pojo/DupB.java"
	contentA := "package pojo;\n\npublic class DupA {\n" + toStringBlock + "\treturn \"DupA\";\n}\n"
	contentB := "package pojo;\n\npublic class DupB {\n" + toStringBlock + "\treturn \"DupB\";\n}\n"

	// toStringBlock sits at lines 4..6 of both files.
	run := func(t *testing.T, diffs []model.Diff) (*Runner, *tool.CommentCollector, *fakeClient) {
		collector := tool.NewCommentCollector()
		// The relocator is armed: if the ambiguity decline let the comment fall
		// through to stage 3, the LLM would be invoked (and could fabricate a
		// pom.xml anchor). Asserting zero calls pins the drop on the decline.
		relocator := &fakeClient{responses: []*llm.ChatResponse{reLocationCodeBlockResponse(pomXMLSnippet)}}
		return anchorTestRunner(collector, diffs, relocator, true), collector, relocator
	}

	t.Run("two copies decline and drop", func(t *testing.T) {
		r, collector, relocator := run(t, []model.Diff{
			anchorTestDiff(pomXMLPath, pomXMLDiff, pomXMLContent),
			anchorTestDiff(dupA, "", contentA),
			anchorTestDiff(dupB, "", contentB),
		})
		submitCodeComment(t, r, pomXMLPath, []map[string]any{{
			"path":          pomXMLPath,
			"content":       "toString() should return, not print",
			"existing_code": toStringBlock,
		}})

		if relocator.calls != 0 {
			t.Fatalf("re-location LLM invoked %d times, want 0 (quote implausible for pom.xml)", relocator.calls)
		}
		if got := collector.Comments(); len(got) != 0 {
			t.Fatalf("collected %d comments, want 0 (ambiguous snippet must not be guessed onto either file)", len(got))
		}
		warnings := r.Warnings()
		if len(warnings) != 1 || warnings[0].Type != "comment_unresolved" || warnings[0].File != pomXMLPath {
			t.Fatalf("warnings = %+v, want one comment_unresolved on %s", warnings, pomXMLPath)
		}
	})

	t.Run("single copy re-files", func(t *testing.T) {
		r, collector, relocator := run(t, []model.Diff{
			anchorTestDiff(pomXMLPath, pomXMLDiff, pomXMLContent),
			anchorTestDiff(dupA, "", contentA),
		})
		submitCodeComment(t, r, pomXMLPath, []map[string]any{{
			"path":          pomXMLPath,
			"content":       "toString() should return, not print",
			"existing_code": toStringBlock,
		}})

		if relocator.calls != 0 {
			t.Fatalf("re-location LLM invoked %d times, want 0 (unique cross-file hit needs no rescue)", relocator.calls)
		}
		comments := collector.Comments()
		if len(comments) != 1 {
			t.Fatalf("collected %d comments, want 1 re-filed", len(comments))
		}
		if comments[0].Path != dupA || comments[0].StartLine != 4 || comments[0].EndLine != 6 {
			t.Fatalf("comment = %s:%d-%d; want re-filed onto %s at 4..6 (proves the snippet is findable)",
				comments[0].Path, comments[0].StartLine, comments[0].EndLine, dupA)
		}
	})
}

// toStringBlock is the snippet shared by the ambiguity test's two candidate
// files: non-trivial lines (so it is anchorable evidence) that exist verbatim
// in both.
const toStringBlock = `    public String toString() {
        System.out.println("id=" + id);
        System.out.println("resourceId=" + resourceId);
`

// submitCodeCommentViaPool drives code_comment through the CommentWorkerPool
// branch and proves it: that branch records "(async)" on the task record
// synchronously while the work runs on a worker under context.WithoutCancel.
func submitCodeCommentViaPool(t *testing.T, r *Runner, newPath string, comments []map[string]any) {
	t.Helper()
	args, err := json.Marshal(map[string]any{"comments": comments})
	if err != nil {
		t.Fatal(err)
	}
	rec := &session.TaskRecord{}
	cp := r.executeToolCall(context.Background(), newPath, llm.ToolCall{
		Function: llm.FunctionCall{Name: "code_comment", Arguments: string(args)},
	}, rec, "")
	if cp.Data != tool.CommentSucceed {
		t.Fatalf("code_comment result = %q, want %q", cp.Data, tool.CommentSucceed)
	}
	if len(rec.ToolResults) != 1 || rec.ToolResults[0].Result != "(async)" {
		t.Fatalf("recorded results = %+v, want one (async) entry — pool branch not taken", rec.ToolResults)
	}
}

// TestAnchorIntegrity_AsyncPoolAppliesSameGate covers grouped reviews' real
// wiring: comments arrive through the CommentWorkerPool, and the emission gate
// must behave there exactly as on the synchronous path — a located comment is
// collected with its lines, an unresolvable one is dropped with a
// comment_unresolved warning. CollectPendingComments drains the pool before
// any assertion, so the test is deterministic.
func TestAnchorIntegrity_AsyncPoolAppliesSameGate(t *testing.T) {
	t.Run("located comment collected with lines", func(t *testing.T) {
		collector := tool.NewCommentCollector()
		r := anchorTestRunner(collector, []model.Diff{
			anchorTestDiff(pomXMLPath, pomXMLDiff, pomXMLContent),
		}, &fakeClient{}, false)
		r.deps.CommentWorkerPool = NewCommentWorkerPool(2)

		submitCodeCommentViaPool(t, r, pomXMLPath, []map[string]any{{
			"path":          pomXMLPath,
			"content":       "the coordinates are wrong",
			"existing_code": pomXMLSnippet,
		}})

		comments := r.CollectPendingComments()
		if len(comments) != 1 {
			t.Fatalf("collected %d comments, want 1", len(comments))
		}
		if comments[0].Path != pomXMLPath || comments[0].StartLine != 1 || comments[0].EndLine != 3 {
			t.Fatalf("comment = %s:%d-%d; want %s at 1..3 after pool drain",
				comments[0].Path, comments[0].StartLine, comments[0].EndLine, pomXMLPath)
		}
		if len(r.Warnings()) != 0 {
			t.Fatalf("warnings = %+v, want none", r.Warnings())
		}
	})

	t.Run("unresolvable quote dropped with warning", func(t *testing.T) {
		collector := tool.NewCommentCollector()
		r := anchorTestRunner(collector, []model.Diff{
			anchorTestDiff(pomXMLPath, pomXMLDiff, pomXMLContent),
		}, &fakeClient{}, false)
		r.deps.CommentWorkerPool = NewCommentWorkerPool(2)

		submitCodeCommentViaPool(t, r, pomXMLPath, []map[string]any{{
			"path":          pomXMLPath,
			"content":       "toString() should return, not print",
			"existing_code": strings.ReplaceAll(driftedJavaQuote, "id );", "id);"),
		}})

		if got := r.CollectPendingComments(); len(got) != 0 {
			t.Fatalf("collected %d comments, want 0 (wrong-file quote dropped via pool)", len(got))
		}
		warnings := r.Warnings()
		if len(warnings) != 1 || warnings[0].Type != "comment_unresolved" || warnings[0].File != pomXMLPath {
			t.Fatalf("warnings = %+v, want one comment_unresolved on %s", warnings, pomXMLPath)
		}
	})
}

const pomXMLSnippet = `<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>com.qz.onboard</groupId>
`

func reLocationCodeBlockResponse(snippet string) *llm.ChatResponse {
	content := "The code is:\n```xml\n" + snippet + "\n```\n"
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
		Model:   "fake",
	}
}
