// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package delegate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Check the published examples with external diff, textconv, and color configured.
func TestDelegateDiffExamples(t *testing.T) {
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	repo := t.TempDir()
	runGit := func(args ...string) ([]byte, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		return out, err
	}
	mustGit := func(t *testing.T, args ...string) string {
		t.Helper()
		out, err := runGit(args...)
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustGit(t, "init", "-q")
	mustGit(t, "config", "user.name", "Test User")
	mustGit(t, "config", "user.email", "test@example.com")
	mustGit(t, "config", "commit.gpgsign", "false")
	mustGit(t, "config", "core.autocrlf", "false")
	const file = "sample file.txt"
	write(file, "before\n")
	write(".gitattributes", "*.txt diff=ocr-test\n")
	mustGit(t, "add", ".")
	mustGit(t, "commit", "-qm", "initial")
	base := strings.TrimSpace(mustGit(t, "rev-parse", "HEAD"))
	write(file, "committed\n")
	mustGit(t, "add", file)
	mustGit(t, "commit", "-qm", "change")
	commit := strings.TrimSpace(mustGit(t, "rev-parse", "HEAD"))
	write(file, "workspace\n")
	mustGit(t, "config", "color.ui", "always")
	mustGit(t, "config", "diff.ocr-test.textconv", "ocr-forbidden-textconv")
	t.Setenv("GIT_EXTERNAL_DIFF", "ocr-forbidden-external-diff")
	t.Setenv("GIT_PAGER", "ocr-forbidden-pager")

	// Go runs package tests from the package directory, including with -trimpath.
	root := filepath.Join("..", "..")
	docs := []string{
		"skills/open-code-review-delegate/SKILL.md",
		"plugins/open-code-review/skills/open-code-review-delegate/SKILL.md",
		"plugins/open-code-review/claude-code/commands/delegate-review.md",
		"plugins/open-code-review/kimi-code/commands/delegate-review.md",
	}
	for _, locale := range []string{"en", "zh", "ja", "ko", "ru"} {
		docs = append(docs, "pages/src/content/docs/"+locale+"/integrations/delegate.md")
	}
	// The placeholders contain no spaces; split them before inserting concrete
	// paths to preserve a path containing spaces as one argument.
	exampleRE := regexp.MustCompile("git [^`\r\n]* -- \"?<path>\"?")
	for _, doc := range docs {
		t.Run(doc, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc)))
			if err != nil {
				t.Fatal(err)
			}
			examples := exampleRE.FindAllString(string(content), -1)
			modes := map[string]bool{}
			for _, example := range examples {
				t.Run(example, func(t *testing.T) {
					if !strings.Contains(example, " -- \"<path>\"") {
						t.Fatal("file path placeholder must be quoted for paths containing spaces")
					}
					if strings.Contains(example, "<absolute-diff-file>") && !strings.Contains(example, "--output=\"<absolute-diff-file>\"") {
						t.Fatal("output path placeholder must be quoted for paths containing spaces")
					}
					args := strings.Fields(example)[1:]
					if len(args) < 2 || args[0] != "--no-pager" {
						t.Fatalf("example must disable the pager before the Git subcommand: %s", example)
					}
					for _, flag := range []string{"--no-ext-diff", "--no-textconv", "--no-color"} {
						if !strings.Contains(" "+example+" ", " "+flag+" ") {
							t.Errorf("missing %s", flag)
						}
					}
					mode, want := "workspace", "+workspace\n"
					if strings.Contains(example, "<merge_base>") {
						mode, want = "range", "+committed\n"
					} else if strings.Contains(example, "<commit>") {
						mode, want = "commit", "+committed\n"
					}
					modes[mode] = true
					patchPath := filepath.Join(t.TempDir(), "review patch.diff")
					replace := strings.NewReplacer(
						"\"", "", "<merge_base>", base, "<to>", commit,
						"<commit>", commit, "<path>", file, "<absolute-diff-file>", patchPath,
					)
					hasOutput := false
					for i := range args {
						args[i] = replace.Replace(args[i])
						hasOutput = hasOutput || strings.HasPrefix(args[i], "--output=")
					}
					if hasOutput {
						modes["output-file"] = true
					}
					if !hasOutput {
						assertDelegatePatch(t, mustGit(t, args...), want)
						// The instructions allow --output for all three modes.
						args = append(args[:2:2], append([]string{"--output=" + patchPath}, args[2:]...)...)
					}
					mustGit(t, args...)
					patch, err := os.ReadFile(patchPath)
					if err != nil {
						t.Fatal(err)
					}
					assertDelegatePatch(t, string(patch), want)
					// Invalid refs and unwritable destinations must produce Git exit errors.
					for _, failure := range []string{"invalid-ref", "output-is-directory"} {
						badArgs := append([]string(nil), args...)
						for i, arg := range badArgs {
							if failure == "invalid-ref" {
								badArgs[i] = strings.NewReplacer(base, "ocr-missing-ref", commit, "ocr-missing-ref", "HEAD", "ocr-missing-ref").Replace(arg)
							} else if strings.HasPrefix(arg, "--output=") {
								badArgs[i] = "--output=" + t.TempDir()
							}
						}
						if out, err := runGit(badArgs...); err == nil {
							t.Errorf("%s unexpectedly succeeded: %s", failure, out)
						} else if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() <= 0 {
							t.Errorf("%s did not return a positive Git exit code: %v", failure, err)
						}
					}
				})
			}
			for _, mode := range []string{"range", "commit", "workspace", "output-file"} {
				if !modes[mode] {
					t.Errorf("missing %s example", mode)
				}
			}
		})
	}
}

func assertDelegatePatch(t *testing.T, patch, want string) {
	t.Helper()
	for _, part := range []string{"diff --git ", "@@ -1 +1 @@", want} {
		if !strings.Contains(patch, part) {
			t.Errorf("patch missing %q:\n%s", part, patch)
		}
	}
	if strings.Contains(patch, "\x1b[") {
		t.Error("patch contains ANSI color escapes")
	}
}
