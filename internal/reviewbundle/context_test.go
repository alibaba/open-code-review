package reviewbundle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/rules"
	"github.com/open-code-review/open-code-review/internal/gitcmd"
)

func TestContextReadAndDiffAreBoundToBundle(t *testing.T) {
	repository := initPrepareRepository(t)
	writeTargetFile(t, repository, "base.go", "package sample\n\nvar changed = true\n")
	bundle, _, err := Prepare(context.Background(), PrepareOptions{
		RepoDir:       repository,
		Resolver:      detailResolverStub{},
		FileFilter:    &rules.FileFilter{},
		GitRunner:     gitcmd.New(2),
		MaxBundleSize: DefaultMaxBundleBytes,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	service := NewContextService(repository, bundle, gitcmd.New(2))

	read, err := service.Read(context.Background(), "base.go", 1, 3)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if read.BundleID != bundle.BundleID || !strings.Contains(read.Result, "var changed = true") {
		t.Fatalf("Read() = %+v", read)
	}
	diffResult, err := service.Diff(context.Background(), []string{"base.go"})
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	if !strings.Contains(diffResult.Result, "diff --git") {
		t.Fatalf("Diff() = %+v", diffResult)
	}
}

func TestContextRejectsStaleWorkspaceAndPathEscape(t *testing.T) {
	repository := initPrepareRepository(t)
	writeTargetFile(t, repository, "base.go", "package sample\n\nvar changed = true\n")
	bundle, _, err := Prepare(context.Background(), PrepareOptions{
		RepoDir:       repository,
		Resolver:      detailResolverStub{},
		GitRunner:     gitcmd.New(2),
		MaxBundleSize: DefaultMaxBundleBytes,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	service := NewContextService(repository, bundle, gitcmd.New(2))
	if _, err := service.Read(context.Background(), "../secret", 1, 3); err == nil {
		t.Fatal("Read(path escape) error = nil")
	}
	writeTargetFile(t, repository, "base.go", "package sample\n\nvar changedAgain = true\n")
	staleService := NewContextService(repository, bundle, gitcmd.New(2))
	_, err = staleService.Search(context.Background(), "changed", false, false, nil)
	var protocolError *ProtocolError
	if !errors.As(err, &protocolError) || protocolError.Code != "stale_bundle" {
		t.Fatalf("Search(stale) error = %v, want stale_bundle", err)
	}
}

func TestContextReadyHonorsCancelledContext(t *testing.T) {
	repository := initPrepareRepository(t)
	writeTargetFile(t, repository, "base.go", "package sample\n\nvar changed = true\n")
	bundle, _, err := Prepare(context.Background(), PrepareOptions{
		RepoDir:       repository,
		Resolver:      detailResolverStub{},
		GitRunner:     gitcmd.New(2),
		MaxBundleSize: DefaultMaxBundleBytes,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	service := NewContextService(repository, bundle, gitcmd.New(2))
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Read(cancelled, "base.go", 1, 3); err == nil {
		t.Fatal("Read(cancelled) error = nil")
	}
	if _, err := service.Read(context.Background(), "base.go", 1, 3); err != nil {
		t.Fatalf("Read(fresh context) error = %v, want success after cancelled call", err)
	}
}

func TestContextFindAndSearchUseTargetAwareTools(t *testing.T) {
	repository := initPrepareRepository(t)
	base := strings.TrimSpace(runTargetGit(t, repository, "rev-parse", "HEAD"))
	writeTargetFile(t, repository, "base.go", "package sample\n\nfunc TargetSymbol() {}\n")
	runTargetGit(t, repository, "add", "base.go")
	runTargetGit(t, repository, "commit", "-m", "target")
	head := strings.TrimSpace(runTargetGit(t, repository, "rev-parse", "HEAD"))
	bundle, _, err := Prepare(context.Background(), PrepareOptions{
		RepoDir:       repository,
		Target:        TargetSpec{From: base, To: head},
		Resolver:      detailResolverStub{},
		GitRunner:     gitcmd.New(2),
		MaxBundleSize: DefaultMaxBundleBytes,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	service := NewContextService(repository, bundle, gitcmd.New(2))
	found, err := service.Find(context.Background(), "base.go", true)
	if err != nil || !strings.Contains(found.Result, "base.go") {
		t.Fatalf("Find() = %+v, %v", found, err)
	}
	searched, err := service.Search(context.Background(), "TargetSymbol", true, false, []string{"*.go"})
	if err != nil || !strings.Contains(searched.Result, "base.go") {
		t.Fatalf("Search() = %+v, %v", searched, err)
	}
}

func TestScanContextStaysInsideBundle(t *testing.T) {
	repository := initPrepareRepository(t)
	writeTargetFile(t, repository, "only.go", "package sample\n\nfunc InBundle() {}\n")
	writeTargetFile(t, repository, "other.go", "package sample\n\nfunc OutsideBundle() {}\n")
	bundle := &Bundle{
		SchemaVersion: BundleSchemaVersion,
		BundleID:      "sha256:scan",
		Target:        Target{Mode: TargetScan},
		Files: []File{{
			Path:          "only.go",
			Reviewable:    true,
			Content:       "package sample\n\nfunc InBundle() {}\n",
			ContentSHA256: hashFields([]byte("package sample\n\nfunc InBundle() {}\n")),
		}},
		Contract: DefaultContract(),
	}
	service := NewContextService(repository, bundle, gitcmd.New(2))

	read, err := service.Read(context.Background(), "only.go", 1, 5)
	if err != nil || !strings.Contains(read.Result, "InBundle") {
		t.Fatalf("Read(in bundle) = %+v, %v", read, err)
	}
	if _, err := service.Read(context.Background(), "other.go", 1, 5); err == nil {
		t.Fatal("Read(outside scan bundle) error = nil")
	}
	found, err := service.Find(context.Background(), "other.go", true)
	if err != nil || strings.Contains(found.Result, "other.go") {
		t.Fatalf("Find(outside scan bundle) = %+v, %v", found, err)
	}
	searched, err := service.Search(context.Background(), "OutsideBundle", true, false, nil)
	if err != nil || strings.Contains(searched.Result, "other.go") || strings.Contains(searched.Result, "OutsideBundle") {
		t.Fatalf("Search(outside scan bundle) = %+v, %v", searched, err)
	}
}

func TestScanContextReadReportsRequestedRangeTruncation(t *testing.T) {
	repository := t.TempDir()
	content := strings.Join([]string{"line 1", "line 2", "line 3"}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(repository, "only.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := &Bundle{
		SchemaVersion: BundleSchemaVersion,
		BundleID:      "sha256:scan",
		Target:        Target{Mode: TargetScan},
		Files: []File{{
			Path:          "only.txt",
			Reviewable:    true,
			Content:       content,
			ContentSHA256: hashFields([]byte(content)),
		}},
		Contract: DefaultContract(),
	}
	service := NewContextService(repository, bundle, gitcmd.New(2))

	read, err := service.Read(context.Background(), "only.txt", 1, 1)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !strings.Contains(read.Result, "IS_TRUNCATED: true") {
		t.Fatalf("Read() = %q, want requested range truncation", read.Result)
	}
}

func TestScanContextSearchRejectsPerlRegexp(t *testing.T) {
	repository := t.TempDir()
	content := "package sample\n\nfunc InBundle() {}\n"
	if err := os.WriteFile(filepath.Join(repository, "only.go"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := &Bundle{
		SchemaVersion: BundleSchemaVersion,
		BundleID:      "sha256:scan",
		Target:        Target{Mode: TargetScan},
		Files: []File{{
			Path:          "only.go",
			Reviewable:    true,
			Content:       content,
			ContentSHA256: hashFields([]byte(content)),
		}},
		Contract: DefaultContract(),
	}
	service := NewContextService(repository, bundle, gitcmd.New(2))

	_, err := service.Search(context.Background(), `func (?=InBundle)`, true, true, nil)
	var protocolError *ProtocolError
	if !errors.As(err, &protocolError) || protocolError.Code != "unsupported_search_regex" {
		t.Fatalf("Search(perl regexp in scan bundle) error = %v, want unsupported_search_regex", err)
	}
}

func TestScanContextSearchMatchesDirectoryPattern(t *testing.T) {
	repository := t.TempDir()
	content := "package reviewbundle\n\nfunc scanSearchMatcher() {}\n"
	if err := os.MkdirAll(filepath.Join(repository, "internal", "reviewbundle"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "internal", "reviewbundle", "context.go"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := &Bundle{
		SchemaVersion: BundleSchemaVersion,
		BundleID:      "sha256:scan",
		Target:        Target{Mode: TargetScan},
		Files: []File{{
			Path:          "internal/reviewbundle/context.go",
			Reviewable:    true,
			Content:       content,
			ContentSHA256: hashFields([]byte(content)),
		}},
		Contract: DefaultContract(),
	}
	service := NewContextService(repository, bundle, gitcmd.New(2))

	searched, err := service.Search(
		context.Background(),
		"scanSearchMatcher",
		true,
		false,
		[]string{"internal/reviewbundle"},
	)
	if err != nil || !strings.Contains(searched.Result, "internal/reviewbundle/context.go") {
		t.Fatalf("Search(directory pattern in scan bundle) = %+v, %v", searched, err)
	}
}

func TestScanContextDiffReturnsEmbeddedContent(t *testing.T) {
	repository := t.TempDir()
	content := "package sample\n\nfunc ScanDiffTarget() {}\n"
	if err := os.WriteFile(filepath.Join(repository, "scan.go"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := &Bundle{
		SchemaVersion: BundleSchemaVersion,
		BundleID:      "sha256:scan",
		Target:        Target{Mode: TargetScan},
		Files: []File{{
			Path:          "scan.go",
			Reviewable:    true,
			Content:       content,
			ContentSHA256: hashFields([]byte(content)),
		}},
		Contract: DefaultContract(),
	}
	service := NewContextService(repository, bundle, gitcmd.New(2))

	diffResult, err := service.Diff(context.Background(), []string{"scan.go"})
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	if strings.Contains(diffResult.Result, "diff --git") {
		t.Fatalf("Diff(scan bundle) = %q, want embedded content not patch", diffResult.Result)
	}
	if !strings.Contains(diffResult.Result, "ScanDiffTarget") {
		t.Fatalf("Diff(scan bundle) = %q, want file content", diffResult.Result)
	}
}
