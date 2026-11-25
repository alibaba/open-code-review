// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
	"github.com/spf13/cobra"
)

func writeCompletionTranscript(test testing.TB, repository, id string, payloadBytes int, malformed bool) {
	test.Helper()
	directory, err := session.SessionsDir(repository)
	if err != nil {
		test.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		test.Fatal(err)
	}
	header, err := json.Marshal(map[string]string{"type": "session_start", "sessionId": id, "cwd": repository})
	if err != nil {
		test.Fatal(err)
	}
	content := string(header) + "\n"
	if malformed {
		content = "{malformed-session\n"
	}
	if payloadBytes > 0 {
		content += `{"type":"llm_response","content":"` + strings.Repeat("x", payloadBytes) + "\"}\n"
	}
	if err := os.WriteFile(filepath.Join(directory, id+".jsonl"), []byte(content), 0600); err != nil {
		test.Fatal(err)
	}
}

func TestSessionCompletionPrefixPreservesRepositoryDescriptions(t *testing.T) {
	setTestHome(t, t.TempDir())
	first, second := t.TempDir(), t.TempDir()
	writeCompletionTranscript(t, first, "match-first", 0, false)
	writeCompletionTranscript(t, second, "match-second", 0, true)
	writeCompletionTranscript(t, first, "other-large", 2<<20, false)
	writeCompletionTranscript(t, second, "other-corrupt", 0, true)
	command := &cobra.Command{}
	command.Flags().String("repo", "", "")
	for _, prefix := range []string{"", "match-", "missing-"} {
		got, directive := completeSessionIDsAnywhere(command, nil, prefix)
		want := []string{}
		if prefix == "" || prefix == "match-" {
			want = []string{"match-first\t" + first, "match-second\t(repository not recorded)"}
		}
		if prefix == "" {
			want = append(want, "other-corrupt\t(repository not recorded)", "other-large\t"+first)
		}
		if !slices.Equal(got, want) || directive != cobra.ShellCompDirectiveNoFileComp {
			t.Fatalf("prefix %q: completions=%q directive=%v, want %q", prefix, got, directive, want)
		}
	}
}

func BenchmarkSessionCompletionWithUnrelatedTranscripts(bench *testing.B) {
	for _, size := range []int{0, 1 << 20} {
		bench.Run(fmt.Sprintf("unrelated-bytes=%d", size), func(bench *testing.B) {
			home := bench.TempDir()
			bench.Setenv("HOME", home)
			bench.Setenv("USERPROFILE", home)
			repository := bench.TempDir()
			writeCompletionTranscript(bench, repository, "match-session", 0, false)
			for index := range 32 {
				writeCompletionTranscript(bench, repository, fmt.Sprintf("other-%02d", index), size, false)
			}
			command := &cobra.Command{}
			command.Flags().String("repo", "", "")
			want := []string{"match-session\t" + repository}
			bench.ReportAllocs()
			bench.ResetTimer()
			for range bench.N {
				got, directive := completeSessionIDsAnywhere(command, nil, "match-")
				if !slices.Equal(got, want) || directive != cobra.ShellCompDirectiveNoFileComp {
					bench.Fatalf("unexpected completion: %q, %v", got, directive)
				}
			}
		})
	}
}
