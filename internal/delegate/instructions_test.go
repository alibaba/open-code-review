// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package delegate

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

type delegateRepo struct {
	dir, base, commit string
	env               []string
}

func delegateGitEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_ATTR_NOSYSTEM=1")
}

func (r delegateRepo) run(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()
	// Cleanup checks run after the test context has been canceled.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir, cmd.Env = r.dir, r.env
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	return out, err
}

func (r delegateRepo) must(t *testing.T, args ...string) string {
	t.Helper()
	out, err := r.run(t, args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func newDelegateRepo(t *testing.T) delegateRepo {
	t.Helper()
	r := delegateRepo{dir: t.TempDir(), env: append(delegateGitEnv(),
		"GIT_EXTERNAL_DIFF=ocr-forbidden-external-diff", "GIT_PAGER=ocr-forbidden-pager")}
	r.must(t, "init", "-q", "--template=")
	r.must(t, "config", "user.name", "Test User")
	r.must(t, "config", "user.email", "test@example.com")
	r.must(t, "config", "commit.gpgsign", "false")
	r.must(t, "config", "core.autocrlf", "false")
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(r.dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("sample file.txt", "before\n")
	write(".gitattributes", "*.txt diff=ocr-test\n")
	r.must(t, "add", ".")
	r.must(t, "commit", "-qm", "initial")
	r.base = strings.TrimSpace(r.must(t, "rev-parse", "HEAD"))
	write("sample file.txt", "committed\n")
	r.must(t, "add", "sample file.txt")
	r.must(t, "commit", "-qm", "change")
	r.commit = strings.TrimSpace(r.must(t, "rev-parse", "HEAD"))
	write("sample file.txt", "workspace\n")
	r.must(t, "config", "color.ui", "always")
	r.must(t, "config", "diff.ocr-test.textconv", "ocr-forbidden-textconv")
	return r
}

func delegateExamples(t *testing.T) []string {
	t.Helper()
	docs := []string{
		"skills/open-code-review-delegate/SKILL.md",
		"plugins/open-code-review/skills/open-code-review-delegate/SKILL.md",
		"plugins/open-code-review/claude-code/commands/delegate-review.md",
		"plugins/open-code-review/kimi-code/commands/delegate-review.md",
	}
	for _, locale := range []string{"en", "zh", "ja", "ko", "ru"} {
		docs = append(docs, "pages/src/content/docs/"+locale+"/integrations/delegate.md")
	}
	// Go runs package tests from the package directory, including with -trimpath.
	examples, err := readDelegateExamples(os.DirFS(filepath.Join("..", "..")), docs)
	if err != nil {
		t.Fatal(err)
	}
	return examples
}

func readDelegateExamples(root fs.FS, docs []string) ([]string, error) {
	if len(docs) == 0 {
		return nil, fmt.Errorf("no delegate documents to check")
	}
	exampleRE := regexp.MustCompile("git [^`\r\n]*(?: -- \"?<path>\"?|\"?<ref>:<path>\"?)|cat \"?<path>\"?")
	var canonical []string
	for i, doc := range docs {
		content, err := fs.ReadFile(root, doc)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", doc, err)
		}
		set := map[string]bool{}
		for _, example := range exampleRE.FindAllString(string(content), -1) {
			set[example] = true
		}
		var examples []string
		for example := range set {
			examples = append(examples, example)
		}
		slices.Sort(examples)
		if len(examples) == 0 {
			return nil, fmt.Errorf("%s contains no review command examples", doc)
		}
		if !slices.ContainsFunc(examples, func(example string) bool { return strings.HasPrefix(example, "git ") }) {
			return nil, fmt.Errorf("%s contains no Git command examples", doc)
		}
		if i == 0 {
			canonical = examples
		} else if !slices.Equal(examples, canonical) {
			return nil, fmt.Errorf("%s examples differ from %s:\n%q\nwant %q", doc, docs[0], examples, canonical)
		}
	}
	return canonical, nil
}

func TestReadDelegateExamples(t *testing.T) {
	const diff = "git --no-pager diff --no-ext-diff --no-textconv --no-color HEAD -- \"<path>\""
	const cat = "cat \"<path>\""
	for _, tc := range []struct {
		name     string
		contents []string
		want     []string
		wantErr  string
	}{
		{
			name:     "matching sets with duplicates and different order",
			contents: []string{diff + "\n" + cat + "\n" + diff, cat + "\n" + diff},
			want:     []string{cat, diff},
		},
		{name: "empty first document", contents: []string{"# No examples", diff}, wantErr: "doc-0.md contains no review command examples"},
		{name: "empty later document", contents: []string{diff, "# No examples"}, wantErr: "doc-1.md contains no review command examples"},
		{name: "all documents empty", contents: []string{"", ""}, wantErr: "doc-0.md contains no review command examples"},
		{name: "no documents", wantErr: "no delegate documents to check"},
		{name: "no Git commands", contents: []string{cat, cat}, wantErr: "doc-0.md contains no Git command examples"},
		{name: "different command sets", contents: []string{diff, diff + "\n" + cat}, wantErr: "doc-1.md examples differ from doc-0.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fstest.MapFS{}
			var docs []string
			for i, content := range tc.contents {
				name := fmt.Sprintf("doc-%d.md", i)
				docs = append(docs, name)
				root[name] = &fstest.MapFile{Data: []byte(content)}
			}
			got, err := readDelegateExamples(root, docs)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("examples = %q, want %q", got, tc.want)
			}
		})
	}
}

func (r delegateRepo) args(example, patchPath string) []string {
	// Split placeholders before inserting paths containing spaces as single arguments.
	args := strings.Fields(example)[1:]
	replace := strings.NewReplacer("\"", "", "<merge_base>", r.base, "<to>", r.commit,
		"<commit>", r.commit, "<ref>", r.commit, "<path>", "sample file.txt", "<absolute-diff-file>", patchPath)
	for i := range args {
		args[i] = replace.Replace(args[i])
	}
	return args
}

func TestDelegateDiffExamples(t *testing.T) {
	examples := delegateExamples(t)
	r := newDelegateRepo(t)
	modes := map[string]bool{}
	for _, example := range examples {
		t.Run(example, func(t *testing.T) {
			if strings.HasPrefix(example, "cat ") {
				modes["untracked"] = true
				if example != "cat \"<path>\"" {
					t.Fatal("untracked file path must be quoted")
				}
				bash, err := exec.LookPath("bash")
				if err != nil {
					t.Skip("shell execution requires Bash; path quotation was checked")
				}
				name := "untracked sample file.txt"
				if err := os.WriteFile(filepath.Join(r.dir, name), []byte("untracked\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, bash, "-c", strings.ReplaceAll(example, "<path>", "$1"), "ocr-read", name)
				cmd.Dir, cmd.Env = r.dir, r.env
				if out, err := cmd.CombinedOutput(); err != nil || string(out) != "untracked\n" {
					t.Fatalf("read untracked file: %v\n%s", err, out)
				}
				return
			}
			if !strings.Contains(example, " -- \"<path>\"") && !strings.Contains(example, "\"<ref>:<path>\"") {
				t.Fatal("file path placeholder must be quoted")
			}
			patchPath := filepath.Join(t.TempDir(), "review patch.diff")
			args := r.args(example, patchPath)
			if len(args) < 2 || args[0] != "--no-pager" {
				t.Fatal("every Git command must disable the pager before the subcommand")
			}
			if args[1] == "log" || args[1] == "blame" || strings.Contains(example, "<ref>:<path>") {
				modes[args[1]] = true
				want := "workspace"
				if args[1] == "log" {
					want = "change"
				} else if args[1] == "show" {
					want = "committed"
				}
				if out := r.must(t, args...); !strings.Contains(out, want) {
					t.Fatalf("context output missing %q: %s", want, out)
				}
				return
			}
			for _, flag := range []string{"--no-ext-diff", "--no-textconv", "--no-color"} {
				if !slices.Contains(args, flag) {
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
			if strings.Contains(example, "<absolute-diff-file>") {
				modes["output-file"] = true
				if !strings.Contains(example, "--output=\"<absolute-diff-file>\"") {
					t.Fatal("output path placeholder must be quoted")
				}
			} else {
				assertDelegatePatch(t, r.must(t, args...), want)
				args = append(args[:2:2], append([]string{"--output=" + patchPath}, args[2:]...)...)
			}
			r.must(t, args...)
			patch, err := os.ReadFile(patchPath)
			if err != nil {
				t.Fatal(err)
			}
			assertDelegatePatch(t, string(patch), want)
			for _, failure := range []string{"invalid-ref", "output-is-directory"} {
				badArgs := append([]string(nil), args...)
				for i, arg := range badArgs {
					if failure == "invalid-ref" {
						badArgs[i] = strings.NewReplacer(r.base, "ocr-missing-ref", r.commit, "ocr-missing-ref", "HEAD", "ocr-missing-ref").Replace(arg)
					} else if strings.HasPrefix(arg, "--output=") {
						badArgs[i] = "--output=" + t.TempDir()
					}
				}
				if out, err := r.run(t, badArgs...); err == nil {
					t.Errorf("%s unexpectedly succeeded: %s", failure, out)
				} else if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() <= 0 {
					t.Errorf("%s did not return a positive Git exit code: %v", failure, err)
				}
			}
		})
	}
	for _, mode := range []string{"range", "commit", "workspace", "output-file", "untracked", "log", "blame", "show"} {
		if !modes[mode] {
			t.Errorf("missing %s example", mode)
		}
	}
}

func TestDelegateGitIsolation(t *testing.T) {
	ambient := newDelegateRepo(t)
	head := ambient.must(t, "rev-parse", "HEAD")
	status := ambient.must(t, "status", "--porcelain")
	indexPath := filepath.Join(ambient.dir, ".git", "index")
	index, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, name := range []string{"sample file.txt", ".gitattributes"} {
		content, err := os.ReadFile(filepath.Join(ambient.dir, name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = content
	}
	for key, value := range map[string]string{
		"GIT_DIR": filepath.Join(ambient.dir, ".git"), "GIT_WORK_TREE": ambient.dir,
		"GIT_INDEX_FILE": indexPath, "GIT_OBJECT_DIRECTORY": filepath.Join(ambient.dir, ".git", "objects"),
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": filepath.Join(ambient.dir, ".git", "objects"),
		"GIT_COMMON_DIR":                   filepath.Join(ambient.dir, ".git"), "GIT_CONFIG_COUNT": "1",
		"GIT_CONFIG_KEY_0": "core.worktree", "GIT_CONFIG_VALUE_0": ambient.dir,
	} {
		t.Setenv(key, value)
	}
	t.Cleanup(func() {
		if got := ambient.must(t, "rev-parse", "HEAD"); got != head {
			t.Error("ambient repository HEAD changed")
		}
		if got := ambient.must(t, "status", "--porcelain"); got != status {
			t.Error("ambient repository worktree changed")
		}
		if got, err := os.ReadFile(indexPath); err != nil || !bytes.Equal(got, index) {
			t.Error("ambient repository index changed")
		}
		for name, content := range files {
			if got, err := os.ReadFile(filepath.Join(ambient.dir, name)); err != nil || !bytes.Equal(got, content) {
				t.Errorf("ambient worktree file %s changed", name)
			}
		}
	})
	isolated := newDelegateRepo(t)
	if _, err := os.Stat(filepath.Join(isolated.dir, ".git")); err != nil {
		t.Fatal("isolated repository was not initialized:", err)
	}
	got := strings.TrimSpace(isolated.must(t, "rev-parse", "--show-toplevel"))
	actual, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(isolated.dir)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(actual, want) {
		t.Fatalf("repository escaped temporary directory: %s", got)
	}
}

func TestDelegatePagerProcess(t *testing.T) {
	if os.Getenv("OCR_DELEGATE_PAGER_HELPER") != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv("OCR_DELEGATE_PAGER_MARKER"), []byte("started"), 0o600); err != nil {
		os.Exit(2)
	}
	if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestDelegatePagerPTY(t *testing.T) {
	examples := delegateExamples(t)
	unavailable := func(reason string) {
		t.Helper()
		if os.Getenv("OCR_DELEGATE_REQUIRE_PTY") == "1" {
			t.Fatal(reason)
		}
		t.Skip(reason)
	}
	if runtime.GOOS != "linux" {
		unavailable("PTY regression uses util-linux script")
	}
	script, err := exec.LookPath("script")
	if err != nil {
		unavailable("PTY regression requires util-linux script: " + err.Error())
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	version, err := exec.CommandContext(ctx, script, "--version").CombinedOutput()
	if err != nil || !bytes.Contains(version, []byte("util-linux")) {
		unavailable("PTY regression requires the util-linux implementation of script")
	}
	r := newDelegateRepo(t)
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	run := func(t *testing.T, args []string) string {
		t.Helper()
		marker := filepath.Join(t.TempDir(), "pager-started")
		command := []string{"git"}
		for _, arg := range args {
			command = append(command, quote(arg))
		}
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, script, "-q", "-e", "-c", strings.Join(command, " "), os.DevNull)
		cmd.Dir = r.dir
		cmd.Env = append(delegateGitEnv(), "TERM=xterm", "OCR_DELEGATE_PAGER_HELPER=1", "OCR_DELEGATE_PAGER_MARKER="+marker,
			"GIT_PAGER="+quote(os.Args[0])+" -test.run=^TestDelegatePagerProcess$")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("PTY command %v: %v\n%s", args, err, out)
		}
		return marker
	}
	if _, err := os.Stat(run(t, []string{"--paginate", "log", "--oneline"})); err != nil {
		t.Fatal("positive control did not launch the pager:", err)
	}
	for _, example := range examples {
		if !strings.HasPrefix(example, "git ") {
			continue
		}
		t.Run(example, func(t *testing.T) {
			marker := run(t, r.args(example, filepath.Join(t.TempDir(), "review patch.diff")))
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("documented command launched the pager: %v", err)
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
