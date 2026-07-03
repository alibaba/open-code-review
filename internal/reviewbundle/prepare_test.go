package reviewbundle

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/model"
)

type detailResolverStub struct{}

func (detailResolverStub) Resolve(path string) string {
	return detailResolverStub{}.ResolveDetail(path).Rule
}

func (detailResolverStub) ResolveDetail(path string) rules.RuleDetail {
	if strings.HasSuffix(path, ".go") {
		return rules.RuleDetail{
			Rule:    "Review Go correctness.",
			Source:  "project",
			Pattern: "**/*.go",
		}
	}
	return rules.RuleDetail{Rule: "Review correctness.", Source: "system", Pattern: "default"}
}

func TestPrepareWorkspaceBuildsDeterministicCompleteBundle(t *testing.T) {
	repository := initPrepareRepository(t)
	writeTargetFile(t, repository, "base.go", "package sample\n\nvar changed = true\n")
	writeTargetFile(t, repository, "staged.go", "package sample\n\nvar staged = true\n")
	runTargetGit(t, repository, "add", "staged.go")
	if err := os.Remove(filepath.Join(repository, "deleted.go")); err != nil {
		t.Fatalf("remove deleted file: %v", err)
	}
	runTargetGit(t, repository, "mv", "old.go", "renamed.go")
	writeTargetFile(t, repository, "binary.bin", "\x00\x01changed")
	writeTargetFile(t, repository, "ignored_test.go", "package sample\n\nfunc TestIgnored() {}\n")
	writeTargetFile(t, repository, "no-newline.go", "package sample")
	if err := os.Symlink("../outside", filepath.Join(repository, "link.go")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	options := PrepareOptions{
		RepoDir:       repository,
		Resolver:      detailResolverStub{},
		FileFilter:    &rules.FileFilter{},
		GitRunner:     gitcmd.New(4),
		MaxBundleSize: DefaultMaxBundleBytes,
	}
	first, encoded, err := Prepare(context.Background(), options)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	second, secondEncoded, err := Prepare(context.Background(), options)
	if err != nil {
		t.Fatalf("second Prepare() error = %v", err)
	}

	if first.BundleID == "" || first.BundleID != second.BundleID {
		t.Fatalf("unstable bundle IDs: %q != %q", first.BundleID, second.BundleID)
	}
	if string(encoded) != string(secondEncoded) {
		t.Fatal("Prepare() output is not deterministic")
	}
	if first.SchemaVersion != BundleSchemaVersion || first.Target.DiffSHA256 == "" {
		t.Fatalf("invalid protocol identity: %+v", first.Target)
	}
	if first.WorkspaceState == nil {
		t.Fatal("workspace_state is nil")
	}
	if first.Summary.TotalFiles != len(first.Files) ||
		first.Summary.ReviewableFiles+first.Summary.ExcludedFiles != first.Summary.TotalFiles {
		t.Fatalf("inconsistent summary: %+v, files=%d", first.Summary, len(first.Files))
	}
	if len(first.Rules) != 2 {
		t.Fatalf("rules = %d, want 2 deduplicated entries", len(first.Rules))
	}

	files := make(map[string]File, len(first.Files))
	for _, file := range first.Files {
		files[file.Path] = file
		if file.ContentSHA256 == "" && file.ExcludeReason != model.ExcludeDeleted {
			t.Errorf("file missing hashes or rule: %+v", file)
		}
		if file.RuleID == "" {
			t.Errorf("file missing rule id: %+v", file)
		}
	}
	assertPreparedStatus(t, files, "base.go", "modified", true, "")
	assertPreparedStatus(t, files, "staged.go", "modified", true, "")
	assertPreparedStatus(t, files, "deleted.go", "deleted", false, "deleted")
	if files["deleted.go"].ContentSHA256 != "" {
		t.Fatalf("deleted.go content hash = %q, want empty", files["deleted.go"].ContentSHA256)
	}
	assertPreparedStatus(t, files, "renamed.go", "renamed", true, "")
	assertPreparedStatus(t, files, "binary.bin", "binary", false, "binary")
	assertPreparedStatus(t, files, "ignored_test.go", "added", false, "default_path")
	assertPreparedStatus(t, files, "no-newline.go", "added", true, "")
	assertPreparedStatus(t, files, "link.go", "added", true, "")
	if len(files["base.go"].Hunks) == 0 || files["base.go"].Patch == "" {
		t.Fatalf("base.go missing patch evidence: %+v", files["base.go"])
	}

	var decoded Bundle
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode prepared JSON: %v", err)
	}
	if decoded.BundleID != first.BundleID {
		t.Fatalf("encoded bundle ID = %q, want %q", decoded.BundleID, first.BundleID)
	}
	if int64(len(encoded)) != first.Contract.BundleSizeBytes {
		t.Fatalf("bundle_size_bytes = %d, actual %d", first.Contract.BundleSizeBytes, len(encoded))
	}
}

func TestPrepareRejectsOversizedBundleWithoutTruncation(t *testing.T) {
	repository := initPrepareRepository(t)
	writeTargetFile(t, repository, "large.go", "package sample\n// "+strings.Repeat("x", 4096)+"\n")

	_, _, err := Prepare(context.Background(), PrepareOptions{
		RepoDir:       repository,
		Resolver:      detailResolverStub{},
		GitRunner:     gitcmd.New(2),
		MaxBundleSize: 128,
	})
	var protocolError *ProtocolError
	if !errors.As(err, &protocolError) || protocolError.Code != "bundle_too_large" {
		t.Fatalf("Prepare() error = %v, want bundle_too_large", err)
	}
}

func TestPreparePartitionedSplitsLargeDiffWithoutDuplicates(t *testing.T) {
	repository := initPrepareRepository(t)
	for _, name := range []string{"one.go", "two.go", "three.go"} {
		writeTargetFile(
			t,
			repository,
			name,
			"package sample\n// "+strings.Repeat(name, 120)+"\n",
		)
	}
	manifest, encoded, err := PreparePartitioned(context.Background(), PrepareOptions{
		RepoDir:       repository,
		Resolver:      detailResolverStub{},
		GitRunner:     gitcmd.New(2),
		MaxBundleSize: 3200,
	})
	if err != nil {
		t.Fatalf("PreparePartitioned() error = %v", err)
	}
	if len(encoded) == 0 || len(manifest.Bundles) < 2 ||
		manifest.BatchStrategy != "diff" {
		t.Fatalf("manifest = %+v", manifest)
	}
	seen := make(map[string]bool)
	for _, bundle := range manifest.Bundles {
		if bundle.Contract.BundleSizeBytes > 3200 {
			t.Errorf("bundle size = %d, want <= 3200", bundle.Contract.BundleSizeBytes)
		}
		for _, file := range bundle.Files {
			if seen[file.Path] {
				t.Errorf("duplicate file %s", file.Path)
			}
			seen[file.Path] = true
		}
	}
	if len(seen) != manifest.Summary.TotalFiles {
		t.Fatalf("covered files = %d, summary = %+v", len(seen), manifest.Summary)
	}
}

func TestPrepareRangeAndCommitUseRequestedTarget(t *testing.T) {
	repository := initPrepareRepository(t)
	base := strings.TrimSpace(runTargetGit(t, repository, "rev-parse", "HEAD"))
	writeTargetFile(t, repository, "base.go", "package sample\n\nvar rangeChange = true\n")
	runTargetGit(t, repository, "add", "base.go")
	runTargetGit(t, repository, "commit", "-m", "range change")
	head := strings.TrimSpace(runTargetGit(t, repository, "rev-parse", "HEAD"))

	baseOptions := PrepareOptions{
		RepoDir:       repository,
		Resolver:      detailResolverStub{},
		GitRunner:     gitcmd.New(2),
		MaxBundleSize: DefaultMaxBundleBytes,
	}
	rangeOptions := baseOptions
	rangeOptions.Target = TargetSpec{From: base, To: head}
	rangeBundle, _, err := Prepare(context.Background(), rangeOptions)
	if err != nil {
		t.Fatalf("Prepare(range) error = %v", err)
	}
	if rangeBundle.Target.Mode != TargetRange || rangeBundle.Target.BaseSHA != base ||
		rangeBundle.Target.HeadSHA != head || len(rangeBundle.Files) != 1 {
		t.Fatalf("unexpected range bundle: target=%+v files=%d", rangeBundle.Target, len(rangeBundle.Files))
	}

	commitOptions := baseOptions
	commitOptions.Target = TargetSpec{Commit: head}
	commitBundle, _, err := Prepare(context.Background(), commitOptions)
	if err != nil {
		t.Fatalf("Prepare(commit) error = %v", err)
	}
	if commitBundle.Target.Mode != TargetCommit || commitBundle.Target.BaseSHA != base ||
		commitBundle.Target.HeadSHA != head || len(commitBundle.Files) != 1 {
		t.Fatalf("unexpected commit bundle: target=%+v files=%d", commitBundle.Target, len(commitBundle.Files))
	}
}

func initPrepareRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	runTargetGit(t, repository, "init", "-q")
	runTargetGit(t, repository, "config", "user.email", "tests@example.com")
	runTargetGit(t, repository, "config", "user.name", "OCR Tests")
	runTargetGit(t, repository, "config", "commit.gpgsign", "false")
	for name, content := range map[string]string{
		"base.go":    "package sample\n\nvar base = true\n",
		"staged.go":  "package sample\n\nvar staged = false\n",
		"deleted.go": "package sample\n\nvar deleted = true\n",
		"old.go":     "package sample\n\nvar renamed = true\n",
		"binary.bin": "\x00\x01original",
	} {
		writeTargetFile(t, repository, name, content)
	}
	runTargetGit(t, repository, "add", ".")
	runTargetGit(t, repository, "commit", "-m", "initial")
	return repository
}

func assertPreparedStatus(
	t *testing.T,
	files map[string]File,
	path string,
	status string,
	reviewable bool,
	excludeReason string,
) {
	t.Helper()
	file, ok := files[path]
	if !ok {
		t.Errorf("prepared files missing %s; got %+v", path, files)
		return
	}
	if file.Status != status || file.Reviewable != reviewable ||
		string(file.ExcludeReason) != excludeReason {
		t.Errorf(
			"%s = status %q, reviewable %v, reason %q; want %q, %v, %q",
			path,
			file.Status,
			file.Reviewable,
			file.ExcludeReason,
			status,
			reviewable,
			excludeReason,
		)
	}
}
