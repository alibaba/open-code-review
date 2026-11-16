// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/vcs"
)

type callbackReviewSource struct {
	resolve func(context.Context, vcs.ReviewSpec) (vcs.ResolvedReview, error)
	changes func(context.Context, vcs.ResolvedReview) (vcs.Changes, error)
}

func (s *callbackReviewSource) ResolveReview(ctx context.Context, spec vcs.ReviewSpec) (vcs.ResolvedReview, error) {
	return s.resolve(ctx, spec)
}

func (s *callbackReviewSource) Changes(ctx context.Context, review vcs.ResolvedReview) (vcs.Changes, error) {
	return s.changes(ctx, review)
}

type namedReviewSource struct{ *callbackReviewSource }

func (*namedReviewSource) CurrentBranch(context.Context) (string, error) {
	return "provider-trunk", nil
}

func providerRunArgs(repoDir string, source vcs.ReviewSource, client llm.LLMClient) Args {
	return Args{
		RepoDir: repoDir, Repository: source, LLMClient: client, Model: "fake",
		MaxConcurrency: 1,
		Template: template.Template{
			MaxTokens: 100000, MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}},
			},
		},
		MainToolDefs: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "task_done"}}},
	}
}

func TestProviderChangesFailureLeavesAdmissionReadOnly(t *testing.T) {
	for _, operation := range []string{"identity", "preview"} {
		t.Run(operation, func(t *testing.T) {
			homeDir := t.TempDir()
			setTestHome(t, homeDir)
			t.Setenv("PATH", "")
			want := errors.New("provider failed after reading part of the changeset")
			var calls []string
			source := &callbackReviewSource{
				resolve: func(_ context.Context, spec vcs.ReviewSpec) (vcs.ResolvedReview, error) {
					calls = append(calls, "resolve")
					return vcs.ResolvedReview{Provider: "example", Spec: spec, Head: "r42", Content: vcs.ContentRef{Revision: "r42"}}, nil
				},
				changes: func(context.Context, vcs.ResolvedReview) (vcs.Changes, error) {
					calls = append(calls, "changes")
					return vcs.Changes{Included: []model.Diff{{NewPath: "partial.go", Diff: "+partial"}}}, want
				},
			}
			args := Args{RepoDir: t.TempDir(), Repository: source, Commit: "latest"}
			if operation == "identity" {
				sealed, err := ResolveIdentity(context.Background(), args)
				if !errors.Is(err, want) || sealed != nil {
					t.Fatalf("partial input was admitted: %+v, %v", sealed, err)
				}
			} else {
				preview, err := Preview(context.Background(), args)
				if !errors.Is(err, want) || preview != nil {
					t.Fatalf("partial input was previewed as complete: %+v, %v", preview, err)
				}
			}
			if !slices.Equal(calls, []string{"resolve", "changes"}) {
				t.Fatalf("provider calls = %v", calls)
			}
			entries, err := os.ReadDir(homeDir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("admission created persisted state: %v, %v", entries, err)
			}
		})
	}
}

func TestProviderInputFailuresPersistBeforeDispatch(t *testing.T) {
	for _, stage := range []string{"resolve", "changes"} {
		for _, failure := range []struct {
			name  string
			cause error
			class session.RunFailureClass
		}{
			{"unavailable", errors.New("repository unavailable"), session.RunFailureInput},
			{"cancelled", context.Canceled, session.RunFailureCancelled},
			{"timeout", context.DeadlineExceeded, session.RunFailureTimeout},
		} {
			t.Run(stage+"/"+failure.name, func(t *testing.T) {
				setTestHome(t, t.TempDir())
				t.Setenv("PATH", "")
				var calls []string
				source := &callbackReviewSource{
					resolve: func(_ context.Context, spec vcs.ReviewSpec) (vcs.ResolvedReview, error) {
						calls = append(calls, "resolve")
						if stage == "resolve" {
							return vcs.ResolvedReview{}, fmt.Errorf("resolve revision: %w", failure.cause)
						}
						return vcs.ResolvedReview{Provider: "example", Spec: spec, Head: "r42", Content: vcs.ContentRef{Revision: "r42"}}, nil
					},
					changes: func(context.Context, vcs.ResolvedReview) (vcs.Changes, error) {
						calls = append(calls, "changes")
						return vcs.Changes{Included: []model.Diff{{NewPath: "partial.go", Diff: "+partial"}}}, fmt.Errorf("read changes: %w", failure.cause)
					},
				}
				client := &fakeAgentClient{responses: []*llm.ChatResponse{agentTaskDoneResponse()}}
				args := providerRunArgs(t.TempDir(), source, client)
				args.Commit = "latest"
				a := New(args)
				t.Cleanup(func() { _ = a.Session().Finalize() })
				comments, err := a.Run(context.Background())
				if !errors.Is(err, failure.cause) || len(comments) != 0 {
					t.Fatalf("input failure lost: comments=%v, error=%v", comments, err)
				}
				if client.calls != 0 {
					t.Fatalf("LLM called before valid input: %d", client.calls)
				}
				wantCalls := []string{"resolve"}
				if stage == "changes" {
					wantCalls = append(wantCalls, "changes")
				}
				if !slices.Equal(calls, wantCalls) {
					t.Fatalf("provider calls = %v, want %v", calls, wantCalls)
				}
				manifest := a.RunManifest()
				if manifest == nil || manifest.TerminalState != session.StateFailed || manifest.RunFailure == nil || manifest.RunFailure.Classification != failure.class {
					t.Fatalf("input failure misclassified: %+v", manifest)
				}
				if len(manifest.Coverage.Selected) != 0 || len(manifest.Coverage.Completed) != 0 || len(manifest.Coverage.Failed) != 0 {
					t.Fatalf("partial provider result entered coverage: %+v", manifest.Coverage)
				}
				summary, err := session.LoadSummary(args.RepoDir, a.SessionID())
				if err != nil || summary.Aborted || !reflect.DeepEqual(summary.RunManifest, manifest) {
					t.Fatalf("input failure not persisted: %+v, %v", summary, err)
				}
			})
		}
	}
}

func TestProviderFrozenReviewDrivesPreviewRunAndPersistedSession(t *testing.T) {
	setTestHome(t, t.TempDir())
	t.Setenv("PATH", "")
	head := "r42"
	resolves := 0
	changes := vcs.Changes{
		Included: []model.Diff{
			{OldPath: "main.go", NewPath: "main.go", Diff: "@@ -1 +1 @@\n-old\n+reviewed", Insertions: 1, Deletions: 1},
			{OldPath: "former.go", NewPath: "renamed.go", Diff: "@@ -1 +1 @@\n-old\n+renamed", Insertions: 1, Deletions: 1},
			{OldPath: "notes.md", NewPath: "notes.md", Diff: "+notes", Insertions: 1},
			{OldPath: "gone.go", NewPath: "/dev/null", Diff: "-gone", IsDeleted: true, Deletions: 1},
		},
		Excluded: []model.Diff{{OldPath: "vendor/dependency.go", NewPath: "vendor/dependency.go", Diff: "+dependency", Insertions: 1}},
	}
	source := &namedReviewSource{&callbackReviewSource{
		resolve: func(_ context.Context, spec vcs.ReviewSpec) (vcs.ResolvedReview, error) {
			resolves++
			return vcs.ResolvedReview{Provider: "example", RepositoryID: "example/repo", Spec: spec, Base: "r41", Head: head, Content: vcs.ContentRef{Revision: head}}, nil
		},
		changes: func(_ context.Context, review vcs.ResolvedReview) (vcs.Changes, error) {
			if review.Head == "r42" && review.Content.Revision == "r42" {
				return changes, nil
			}
			return vcs.Changes{Included: []model.Diff{{NewPath: "late.go", Diff: "+late", Insertions: 1}}}, nil
		},
	}}
	client := &fakeAgentClient{responses: []*llm.ChatResponse{agentTaskDoneResponse(), agentTaskDoneResponse()}}
	args := providerRunArgs(t.TempDir(), source, client)
	args.From, args.To = "trunk", "feature"
	sealed, err := ResolveIdentity(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	head = "r43"
	moved, err := ResolveIdentity(context.Background(), args)
	if err != nil || moved.Identity == sealed.Identity {
		t.Fatalf("moving provider fixture changed no input: %+v, %v", moved, err)
	}
	args.ResolvedReview = &sealed.Review
	args.From, args.To = "", ""
	resolveCount := resolves
	preview, err := Preview(context.Background(), args)
	if err != nil || preview.TotalFiles != 5 || preview.ReviewableCount != 2 || preview.ExcludedCount != 3 {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	var previewed []string
	for _, entry := range preview.Entries {
		if entry.WillReview {
			previewed = append(previewed, entry.Path)
		}
		if entry.Path == "renamed.go" && entry.Status != "renamed" {
			t.Fatalf("normalized rename lost: %+v", entry)
		}
	}
	a := New(args)
	t.Cleanup(func() { _ = a.Session().Finalize() })
	if _, err := a.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	manifest := a.RunManifest()
	if manifest == nil || manifest.TerminalState != session.StateComplete {
		t.Fatalf("run did not complete: %+v", manifest)
	}
	var selected []string
	for _, item := range manifest.Coverage.Selected {
		selected = append(selected, item.Path)
	}
	slices.Sort(previewed)
	slices.Sort(selected)
	if !slices.Equal(previewed, selected) || len(manifest.Coverage.Completed) != 2 {
		t.Fatalf("preview %v disagrees with coverage %+v", previewed, manifest.Coverage)
	}
	if resolves != resolveCount {
		t.Fatal("preview or run re-resolved the admitted review")
	}
	identity := session.RunIdentity{Mode: manifest.Input.Mode, ResolvedBase: manifest.Input.ResolvedBase, ResolvedHead: manifest.Input.ResolvedHead, RepositorySHA256: manifest.Repository.IdentitySHA256, SourceArtifactSHA256: manifest.Input.SourceArtifactSHA256, RuleConfigSHA256: manifest.Execution.RuleConfigSHA256}
	if identity != sealed.Identity || manifest.Input.ResolvedBase != "r41" || manifest.Input.ResolvedHead != "r42" || manifest.Input.ExactRange != "" {
		t.Fatalf("run lost frozen provider identity: %+v", manifest)
	}
	summary, err := session.LoadSummary(args.RepoDir, a.SessionID())
	if err != nil || summary.GitBranch != "provider-trunk" || summary.DiffFrom != "trunk" || summary.DiffTo != "feature" || summary.Aborted || !reflect.DeepEqual(summary.RunManifest, manifest) {
		t.Fatalf("provider session not persisted: %+v, %v", summary, err)
	}
}
