package reviewbundle

import (
	"context"
	"errors"
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
