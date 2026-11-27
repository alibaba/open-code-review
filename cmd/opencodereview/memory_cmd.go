// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/spf13/cobra"
)

func openMemory(ctx context.Context, database, repo string, runner *gitcmd.Runner, write bool) (*knowledge.Store, error) {
	identity := diff.NewWorkspaceProvider(repo, runner).RemoteIdentity(ctx)
	if identity == "" {
		common, err := runner.Output(ctx, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if err != nil {
			return nil, err
		}
		identity = "local:" + filepath.Clean(strings.TrimSpace(string(common)))
	}
	return knowledge.Open(ctx, database, identity, write)
}

type memoryOpener func(*cobra.Command, bool) (*knowledge.Store, knowledge.Tree, error)

func newMemoryCommand() *cobra.Command {
	var database, repoDir, ref string
	root := &cobra.Command{Use: "memory", Short: "Manage versioned repository evidence for review"}
	root.PersistentFlags().StringVar(&database, "memory-db", "", "trusted SQLite memory database path")
	root.PersistentFlags().StringVar(&repoDir, "repo", "", "Git repository (default: current directory)")
	root.PersistentFlags().StringVar(&ref, "ref", "HEAD", "commit at which to read the evidence")
	_ = root.MarkPersistentFlagRequired("memory-db")
	open := func(cmd *cobra.Command, write bool) (*knowledge.Store, knowledge.Tree, error) {
		var tree knowledge.Tree
		repo, err := resolveRepoDir(repoDir)
		if err != nil {
			return nil, tree, err
		}
		runner := gitcmd.New(1)
		oid, err := runner.Output(cmd.Context(), repo, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
		if err != nil {
			return nil, tree, fmt.Errorf("resolve memory revision %q: %w", ref, err)
		}
		tree = knowledge.Tree{Repo: repo, Commit: strings.TrimSpace(string(oid)), Git: runner}
		s, err := openMemory(cmd.Context(), database, repo, runner, write)
		return s, tree, err
	}
	for _, name := range []string{"learn", "decide"} {
		var scope, prop, review, finding, reason, lines, expires string
		cmd := &cobra.Command{Use: name + " <schema.json>", Args: exactArgs(1), Short: "Extract direct property const and enum constraints from a committed JSON schema"}
		if name == "decide" {
			cmd.Use = "decide <evidence-file>"
			cmd.Short = "Record a review-local decision backed by a committed file"
			cmd.Flags().StringVar(&scope, "scope", "", "reviewed file relative to the repository")
			cmd.Flags().StringVar(&lines, "evidence-lines", "", "source lines supporting the decision, e.g. 15-20")
			cmd.Flags().StringVar(&expires, "expires-at", "", "expire this review decision at an RFC3339 timestamp")
			cmd.Flags().StringVar(&review, "review-id", "", "PR, MR or local review identifier")
			cmd.Flags().StringVar(&finding, "finding", "", "review finding that was discussed")
			cmd.Flags().StringVar(&reason, "reason", "", "why this finding needs no change")
			for _, flag := range []string{"review-id", "finding", "reason"} {
				_ = cmd.MarkFlagRequired(flag)
			}
		} else {
			cmd.Flags().StringVar(&scope, "scope", "", "repository-relative file or directory governed by the schema")
			cmd.Flags().StringVar(&prop, "property", "", "select one named schema property")
		}
		_ = cmd.MarkFlagRequired("scope")
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			s, tree, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer s.Close()
			if name == "learn" {
				learned, err := s.Learn(cmd.Context(), tree, args[0], scope, prop)
				if err != nil {
					return err
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(learned)
			}
			start, end, err := parseEvidenceLines(lines)
			if err != nil {
				return err
			}
			fact, err := tree.DecideWithCitation(cmd.Context(), args[0], scope, review, finding, reason, start, end)
			if err != nil {
				return err
			}
			if expires != "" {
				until, err := time.Parse(time.RFC3339, expires)
				if err != nil {
					return fmt.Errorf("parse --expires-at: %w", err)
				}
				fact.ExpiresAt = until.UTC().Format(time.RFC3339)
			}
			facts := []knowledge.Fact{fact}
			if err = s.Save(cmd.Context(), facts); err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(facts)
		}
		root.AddCommand(cmd)
	}
	var review, search, asOf string
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, Short: "Show recorded evidence and its status at the selected commit", RunE: func(cmd *cobra.Command, args []string) error {
		s, tree, err := open(cmd, false)
		if err != nil {
			return err
		}
		defer s.Close()
		tree.Now, err = parseMemoryTime(asOf)
		if err != nil {
			return err
		}
		facts, err := s.ListAt(cmd.Context(), review, "", tree.Now)
		if err != nil {
			return err
		}
		checked, err := tree.Check(cmd.Context(), facts)
		if err != nil {
			return err
		}
		if search != "" {
			// Compute conflicts before narrowing output to the matching text.
			matches, err := s.ListAt(cmd.Context(), review, search, tree.Now)
			if err != nil {
				return err
			}
			ids := make(map[string]bool, len(matches))
			for _, f := range matches {
				ids[f.ID] = true
			}
			checked = slices.DeleteFunc(checked, func(c knowledge.Checked) bool {
				if c.Category == knowledge.CategoryDiscussionSource {
					return !c.MatchesDiscussionSearch(search)
				}
				return !ids[c.ID]
			})
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(checked)
	}}
	list.Flags().StringVar(&review, "review-id", "", "also include decisions from this review")
	list.Flags().StringVar(&search, "search", "", "literal full-text phrase (at least three characters)")
	list.Flags().StringVar(&asOf, "as-of", "", "evaluate evidence and feedback at this RFC3339 timestamp")
	root.AddCommand(list, &cobra.Command{Use: "revoke <id>", Args: exactArgs(1), Short: "Withdraw a memory from future reviews", RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := open(cmd, true)
		if err != nil {
			return err
		}
		defer s.Close()
		return s.Revoke(cmd.Context(), args[0])
	}})
	root.AddCommand(newMemorySync(open), newMemoryFeedback(open))
	return root
}

func reviewMemory(ctx context.Context, cc *commonContext, opts reviewOptions, code *reviewCodeTools) (knowledge.Snapshot, error) {
	if opts.memoryDB == "" {
		return nil, nil
	}
	s, err := openMemory(ctx, opts.memoryDB, cc.RepoDir, cc.GitRunner, false)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	facts, err := s.ListAt(ctx, opts.reviewID, "", opts.memoryTime)
	if err != nil {
		return nil, err
	}
	tree := reviewMemoryTree(cc, opts, code)
	if cc.FileFilter != nil {
		facts = slices.DeleteFunc(facts, func(f knowledge.Fact) bool {
			return cc.FileFilter.IsUserExcluded(f.Scope) || slices.ContainsFunc(f.EvidencePaths(), cc.FileFilter.IsUserExcluded)
		})
	}
	checked, err := tree.CheckReview(ctx, code.resolution.ResolvedBase, facts)
	if err != nil {
		return nil, err
	}
	snapshot := knowledge.ReviewSnapshot(checked)
	base := tree
	base.Commit, base.Workspace = code.resolution.ResolvedBase, false
	if err := snapshot.DescribeReviewChanges(ctx, base, tree, checked); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func reviewMemoryTree(cc *commonContext, opts reviewOptions, code *reviewCodeTools) knowledge.Tree {
	tree := knowledge.Tree{Repo: cc.RepoDir, Git: cc.GitRunner, Commit: code.resolution.ResolvedHead, Workspace: code.reader.Mode == tool.ModeWorkspace, Now: opts.memoryTime}
	if tree.Workspace {
		tree.Commit = code.resolution.ResolvedBase
	}
	if cc.FileFilter != nil {
		tree.AllowPath = func(p string) bool { return !cc.FileFilter.IsUserExcluded(p) }
	}
	return tree.WithoutDiscussionExports(opts.memoryDiscussions)
}
