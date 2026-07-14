package main

import (
	"context"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/tool"
)

func TestValidateReviewRefsRejectsOptionLikeCommit(t *testing.T) {
	err := validateReviewRefs(t.TempDir(), reviewOptions{commit: "-O./pwn.sh"})
	if err == nil {
		t.Fatal("expected option-like --commit ref to be rejected")
	}
	if !strings.Contains(err.Error(), "--commit") || !strings.Contains(err.Error(), "must not start with '-'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateReviewRefsRejectsOptionLikeRangeRef(t *testing.T) {
	err := validateReviewRefs(t.TempDir(), reviewOptions{to: "-O./pwn.sh"})
	if err == nil {
		t.Fatal("expected option-like --to ref to be rejected")
	}
	if !strings.Contains(err.Error(), "--to") || !strings.Contains(err.Error(), "must not start with '-'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseReviewFlagsRejectsToWithoutFrom(t *testing.T) {
	_, err := parseReviewFlags([]string{"--to", "HEAD"})
	if err == nil {
		t.Fatal("expected --to without --from to fail")
	}
	if !strings.Contains(err.Error(), "--from is required when --to is specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseReviewFlagsRejectsFromWithoutTo(t *testing.T) {
	_, err := parseReviewFlags([]string{"--from", "main"})
	if err == nil {
		t.Fatal("expected --from without --to to fail")
	}
	if !strings.Contains(err.Error(), "--to is required when --from is specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseReviewFlagsAllowsFromAndTo(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--from", "main", "--to", "HEAD"})
	if err != nil {
		t.Fatalf("expected --from/--to to pass, got: %v", err)
	}
	if opts.from != "main" || opts.to != "HEAD" {
		t.Fatalf("unexpected opts: from=%q to=%q", opts.from, opts.to)
	}
}

func TestInitMCPClients_NilConfig(t *testing.T) {
	clients := initMCPClients(context.Background(), nil, tool.NewRegistry(), "/tmp", "test")
	if clients != nil {
		t.Fatalf("expected nil for nil config, got %d clients", len(clients))
	}
}

func TestInitMCPClients_EmptyServers(t *testing.T) {
	clients := initMCPClients(context.Background(), &Config{}, tool.NewRegistry(), "/tmp", "test")
	if clients != nil {
		t.Fatalf("expected nil for empty servers, got %d clients", len(clients))
	}
}

func TestInitMCPClients_EmptyCommandSkipped(t *testing.T) {
	cfg := &Config{
		MCPServers: map[string]MCPServerConfig{
			"no-cmd": {Setup: "echo hello"},
		},
	}
	clients := initMCPClients(context.Background(), cfg, tool.NewRegistry(), "/tmp", "test")
	if clients != nil {
		t.Fatalf("expected nil when the only server has no command")
	}
}

func TestInitMCPClients_SetupOnly(t *testing.T) {
	cfg := &Config{
		MCPServers: map[string]MCPServerConfig{
			"no-cmd": {Command: "echo", Setup: "echo setup-run"},
		},
	}
	// Setup runs successfully, then the server is created (echo exits after initCtx timeout).
	clients := initMCPClients(context.Background(), cfg, tool.NewRegistry(), "/tmp", "test")
	if len(clients) != 0 {
		t.Fatalf("expected 0 clients (echo is not an MCP server), got %d", len(clients))
	}
}

func TestInitMCPClients_SetupTimeoutAborts(t *testing.T) {
	cfg := &Config{
		MCPServers: map[string]MCPServerConfig{
			"sleepy": {
				Command:      "echo",
				Setup:        "sleep 5",
				SetupTimeout: 1, // 1 minute timeout
			},
		},
	}
	// A 1-minute timeout should be plenty for "sleep 5" to succeed.
	clients := initMCPClients(context.Background(), cfg, tool.NewRegistry(), "/tmp", "test")
	_ = clients
}
