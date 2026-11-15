// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/spf13/cobra"
)

var rulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Inspect and debug review rules",
	Long:  "Inspect and debug review rules.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var rulesCheckRepoDir string
var rulesCheckRulePath string

var rulesCheckCmd = &cobra.Command{
	Use:   "check [flags] <file-path>",
	Short: "Show which review rule applies to a given file path",
	Long:  "Show which review rule applies to the given file path, including its source layer and matched pattern.",
	Example: `  ocr rules check src/main/java/com/example/Foo.java
  ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml`,
	Args: exactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRulesCheck(args[0])
	},
}

func init() {
	addRepoFlag(rulesCheckCmd, &rulesCheckRepoDir)
	rulesCheckCmd.Flags().StringVar(&rulesCheckRulePath, "rule", "", "path to a custom rule JSON file")
	rulesCmd.AddCommand(rulesCheckCmd)
}

// ruleLookupPath rewrites a file argument into the form rule patterns are
// written against: relative to the repository, with forward slashes. Without
// this, "./src/a.go", "src\a.go" (PowerShell tab completion) and an absolute
// path all miss project and global rules and silently fall through to the
// system layer. A relative argument stays relative to the repository, as
// before; an absolute one outside the repository is left as given.
func ruleLookupPath(repoDir, p string) string {
	if filepath.IsAbs(p) {
		if rel, ok := localRel(repoDir, p); ok {
			p = rel
		} else if rel, ok := localRel(canonicalPath(repoDir), canonicalPath(p)); ok {
			// The repository root comes from git, which resolves symlinks
			// (macOS /var -> /private/var) and Windows 8.3 short names
			// (RUNNER~1), so a path spelled through either needs the same.
			p = rel
		}
	}
	return path.Clean(filepath.ToSlash(p))
}

// localRel returns target relative to base when target lies inside base.
func localRel(base, target string) (string, bool) {
	rel, err := filepath.Rel(base, target)
	return rel, err == nil && filepath.IsLocal(rel)
}

// canonicalPath resolves symlinks and short names in the longest existing
// prefix of p. The file being checked does not have to exist.
func canonicalPath(p string) string {
	dir, rest := p, ""
	for {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return p
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
	}
}

func runRulesCheck(filePath string) error {
	resolvedRepo, err := resolveRepoDir(rulesCheckRepoDir)
	if err != nil {
		return err
	}

	resolver, _, err := rules.NewResolver(resolvedRepo, rulesCheckRulePath, rules.ResolverOptions{})
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}

	dr, ok := resolver.(rules.DetailResolver)
	if !ok {
		return fmt.Errorf("resolver does not support detail inspection")
	}

	filePath = ruleLookupPath(resolvedRepo, filePath)
	detail := dr.ResolveDetail(filePath)

	sourceLabel := map[string]string{
		"custom":  "Custom (--rule)",
		"project": "Project (.opencodereview/rule.json)",
		"global":  "Global (~/.opencodereview/rule.json)",
		"system":  "System built-in",
	}

	fmt.Printf("File: %s\n", filePath)
	fmt.Printf("Source: %s\n", sourceLabel[detail.Source])
	fmt.Printf("Pattern: %s\n", detail.Pattern)
	if detail.SniffedAs != "" {
		fmt.Printf("Note:    rule selected by file content (%s), not by path alone\n", detail.SniffedAs)
	}
	fmt.Println("Rule:")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println(detail.Rule)
	fmt.Println(strings.Repeat("─", 40))

	return nil
}
