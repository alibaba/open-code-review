// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadJSONLLinesRetainedRecords(t *testing.T) {
	want := []string{"first\n", strings.Repeat("long", 32768) + "\n", "third\r\n", "unterminated"}
	var got [][]byte
	if err := readJSONLLines(strings.NewReader(strings.Join(want, "")), func(line []byte) {
		got = append(got, line)
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("record count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], []byte(want[i])) {
			t.Fatalf("retained record %d changed after subsequent reads", i)
		}
	}
}

func BenchmarkListSessionsResponseRecords(b *testing.B) {
	for _, size := range []struct {
		name  string
		bytes int
	}{
		{"1KiB", 1024},
		{"64KiB", 64 * 1024},
		{"256KiB", 256 * 1024},
	} {
		b.Run(size.name, func(b *testing.B) {
			root := b.TempDir()
			repo := filepath.Join(root, "repo")
			if err := os.Mkdir(repo, 0700); err != nil {
				b.Fatal(err)
			}
			var data strings.Builder
			data.WriteString("{\"type\":\"session_start\",\"timestamp\":\"2026-11-17T00:00:00Z\",\"cwd\":\"/repo\"}\n")
			response := "{\"type\":\"llm_response\",\"filePath\":\"main.go\",\"content\":\"" + strings.Repeat("x", size.bytes) + "\"}\n"
			for i := 0; i < 64; i++ {
				data.WriteString(response)
			}
			data.WriteString("{\"type\":\"review_item_done\",\"filePath\":\"main.go\",\"comments\":[{\"content\":\"finding\"}]}\n")
			data.WriteString("{\"type\":\"session_end\",\"duration_seconds\":45,\"files_reviewed\":[\"main.go\"]}\n")
			if err := os.WriteFile(filepath.Join(repo, "session.jsonl"), []byte(data.String()), 0600); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(data.Len()))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				summaries, err := ListSessions(root, "repo")
				if err != nil {
					b.Fatal(err)
				}
				if len(summaries) != 1 || summaries[0].Aborted || summaries[0].FileCount != 1 || summaries[0].CommentCount != 1 || summaries[0].DurationSec != 45 {
					b.Fatalf("unexpected summary: %+v", summaries)
				}
			}
		})
	}
}
