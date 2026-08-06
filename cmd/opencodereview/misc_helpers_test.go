// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
	"github.com/spf13/cobra"
)

func TestReviewModeFromOptions(t *testing.T) {
	cases := []struct {
		name string
		opts reviewOptions
		want string
	}{
		{"commit", reviewOptions{commit: "abc"}, session.ReviewModeCommit},
		{"range", reviewOptions{from: "main", to: "dev"}, session.ReviewModeRange},
		{"workspace", reviewOptions{}, session.ReviewModeWorkspace},
		{"from only falls back to workspace", reviewOptions{from: "main"}, session.ReviewModeWorkspace},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reviewModeFromOptions(c.opts); got != c.want {
				t.Errorf("reviewModeFromOptions() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSanitizeEndpointHost(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace", "   ", ""},
		{"strips credentials and path", "https://user:pass@API.example.com:8080/v1/chat?k=1#frag", "api.example.com:8080"},
		{"lowercases host", "https://Example.COM", "example.com"},
		{"no host yields empty", "mailto:foo@bar.com", ""},
		{"unparseable yields empty", "://:::", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sanitizeEndpointHost(c.in); got != c.want {
				t.Errorf("sanitizeEndpointHost(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestShortSessionID(t *testing.T) {
	if got := shortSessionID("0123456789abcdef"); got != "01234567" {
		t.Errorf("shortSessionID(long) = %q, want %q", got, "01234567")
	}
	if got := shortSessionID("short"); got != "short" {
		t.Errorf("shortSessionID(short) = %q, want %q", got, "short")
	}
	if got := shortSessionID("12345678"); got != "12345678" {
		t.Errorf("shortSessionID(exactly8) = %q, want %q", got, "12345678")
	}
}

func TestCompleteSessionIDs(t *testing.T) {
	t.Run("with args returns no completions", func(t *testing.T) {
		cmd := &cobra.Command{Use: "x"}
		comps, directive := completeSessionIDs(cmd, []string{"already"}, "")
		if comps != nil {
			t.Errorf("expected nil completions, got %v", comps)
		}
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want NoFileComp", directive)
		}
	})

	t.Run("fresh repo yields empty completions", func(t *testing.T) {
		dir := initTestGitRepo(t)
		cmd := &cobra.Command{Use: "x"}
		cmd.Flags().String("repo", dir, "")
		comps, directive := completeSessionIDs(cmd, nil, "")
		if len(comps) != 0 {
			t.Errorf("expected no completions for fresh repo, got %v", comps)
		}
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v, want NoFileComp", directive)
		}
	})
}
