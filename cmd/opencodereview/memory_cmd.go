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
		var scope, prop, review, finding, reason string
		cmd := &cobra.Command{Use: name + " <schema.json>", Args: exactArgs(1), Short: "Extract direct property const and enum constraints from a committed JSON schema"}
		cmd.Flags().StringVar(&scope, "scope", "", "repository-relative file or directory governed by the schema")
		_ = cmd.MarkFlagRequired("scope")
		if name == "decide" {
			cmd.Use = "decide <evidence-file>"
			cmd.Short = "Record a review-local decision backed by a committed file"
			cmd.Flags().StringVar(&review, "review-id", "", "PR, MR or local review identifier")
			cmd.Flags().StringVar(&finding, "finding", "", "review finding that was discussed")
			cmd.Flags().StringVar(&reason, "reason", "", "why this finding needs no change")
			for _, flag := range []string{"review-id", "finding", "reason"} {
				_ = cmd.MarkFlagRequired(flag)
			}
		} else {
			cmd.Flags().StringVar(&prop, "property", "", "select one named schema property")
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			s, tree, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer s.Close()
			var facts []knowledge.Fact
			if name == "decide" {
				var fact knowledge.Fact
				fact, err = tree.Decide(cmd.Context(), args[0], scope, review, finding, reason)
				facts = []knowledge.Fact{fact}
			} else {
				facts, err = tree.Learn(cmd.Context(), args[0], scope, prop)
			}
			if err != nil {
				return err
			}
			if err = s.Save(cmd.Context(), facts); err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(facts)
		}
		root.AddCommand(cmd)
	}
	var review, search string
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, Short: "Show recorded evidence and its status at the selected commit", RunE: func(cmd *cobra.Command, args []string) error {
		s, tree, err := open(cmd, false)
		if err != nil {
			return err
		}
		defer s.Close()
		facts, err := s.List(cmd.Context(), review, search)
		if err != nil {
			return err
		}
		checked, err := tree.Check(cmd.Context(), facts)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(checked)
	}}
	list.Flags().StringVar(&review, "review-id", "", "also include decisions from this review")
	list.Flags().StringVar(&search, "search", "", "literal full-text phrase (at least three characters)")
	root.AddCommand(list, &cobra.Command{Use: "revoke <id>", Args: exactArgs(1), Short: "Withdraw a memory from future reviews", RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := open(cmd, true)
		if err != nil {
			return err
		}
		defer s.Close()
		return s.Revoke(cmd.Context(), args[0])
	}})
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
	facts, err := s.List(ctx, opts.reviewID, "")
	if err != nil {
		return nil, err
	}
	if f := cc.FileFilter; f != nil {
		facts = slices.DeleteFunc(facts, func(item knowledge.Fact) bool { return f.IsUserExcluded(item.Source) || f.IsUserExcluded(item.Scope) })
	}
	tree := knowledge.Tree{Repo: cc.RepoDir, Git: cc.GitRunner, Commit: code.resolution.ResolvedHead, Workspace: code.reader.Mode == tool.ModeWorkspace}
	if tree.Workspace {
		tree.Commit = code.resolution.ResolvedBase
	}
	checked, err := tree.Check(ctx, facts)
	if err != nil {
		return nil, err
	}
	var snapshot knowledge.Snapshot
	for _, item := range checked {
		if item.Status == "verified" {
			snapshot = append(snapshot, item.Fact)
		}
	}
	return snapshot, nil
}
