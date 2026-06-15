package main

import (
	"strings"
	"testing"
)

func TestParseReviewFlagsUpstreamConflictsWithFrom(t *testing.T) {
	_, err := parseReviewFlags([]string{"--upstream", "upstream", "--from", "main"})
	if err == nil || !strings.Contains(err.Error(), "--upstream cannot be combined") {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestParseReviewFlagsUpstreamConflictsWithCommit(t *testing.T) {
	_, err := parseReviewFlags([]string{"--upstream", "upstream", "--commit", "abc123"})
	if err == nil || !strings.Contains(err.Error(), "--upstream cannot be combined") {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestParseReviewFlagsUpstreamBranchRequiresUpstream(t *testing.T) {
	_, err := parseReviewFlags([]string{"--upstream-branch", "develop"})
	if err == nil || !strings.Contains(err.Error(), "--upstream-branch requires --upstream") {
		t.Fatalf("expected requires-upstream error, got: %v", err)
	}
}

func TestParseReviewFlagsNoFetchRequiresUpstream(t *testing.T) {
	_, err := parseReviewFlags([]string{"--no-fetch"})
	if err == nil || !strings.Contains(err.Error(), "--no-fetch requires --upstream") {
		t.Fatalf("expected requires-upstream error, got: %v", err)
	}
}

func TestParseReviewFlagsUpstreamOK(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--upstream", "upstream", "--to", "feature"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.upstream != "upstream" || opts.to != "feature" {
		t.Fatalf("unexpected opts: %+v", opts)
	}
}
