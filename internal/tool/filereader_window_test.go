// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
)

func TestScanLines_BufferBoundaries(t *testing.T) {
	inputs := []string{
		"",
		"\n",
		strings.Repeat("x", 8192),
		strings.Repeat("x", 8192) + "\n",
		strings.Repeat("x", 4095) + "\r\nlast\r",
		"first\n" + strings.Repeat("x", 8192) + "\r\nlast\n",
		strings.Repeat("x", 8192) + "\nselected\r\n" + strings.Repeat("y", 8192),
		"first\r\n\r\nlast\xff\r\n",
	}
	for i, input := range inputs {
		for _, window := range [][2]int{{1, 10}, {2, 1}, {3, 2}, {20, 1}, {0, 1}, {1, 0}} {
			t.Run(fmt.Sprintf("input%d/start%d/limit%d", i, window[0], window[1]), func(t *testing.T) {
				var all, want []string
				if input != "" {
					all = strings.Split(input, "\n")
				}
				for n, line := range all {
					if n+1 >= window[0] && len(want) < window[1] {
						want = append(want, strings.TrimSuffix(line, "\r"))
					}
				}
				for _, reader := range []io.Reader{strings.NewReader(input), iotest.OneByteReader(strings.NewReader(input))} {
					got, total, err := scanLines(reader, window[0], window[1])
					if err != nil || total != len(all) || !slices.Equal(got, want) {
						t.Fatalf("scanLines() returned %d lines, total %d, err %v; want %d lines, total %d", len(got), total, err, len(want), len(all))
					}
				}
			})
		}
	}
}

func TestScanLines_ReadErrorAfterLongLine(t *testing.T) {
	wantErr := errors.New("read failed")
	for _, limit := range []int{1, 3} {
		reader := io.MultiReader(strings.NewReader("first\n"+strings.Repeat("x", 8192)), iotest.ErrReader(wantErr))
		got, total, err := scanLines(reader, 1, limit)
		if !errors.Is(err, wantErr) || got != nil || total != 0 {
			t.Fatalf("scanLines(limit=%d) = %v, %d, %v; want nil, 0, read error", limit, got, total, err)
		}
	}
}

func BenchmarkFileReaderReadLinesWindow(b *testing.B) {
	shortLine := strings.Repeat("x", 79) + "\n"
	for _, tt := range []struct {
		name      string
		content   string
		startLine int
		maxLines  int
		wantTotal int
	}{
		{"short-file", strings.Repeat(shortLine, 100), 1, 100, 101},
		{"long-selected-line", strings.Repeat("x", 1<<20) + "\n", 1, 1, 2},
		{"large-file-head", strings.Repeat(shortLine, 100000), 1, 500, 100001},
		{"large-file-tail", strings.Repeat(shortLine, 100000), 99501, 500, 100001},
		{"long-unselected-lines", "selected\n" + strings.Repeat(strings.Repeat("x", 65535)+"\n", 128), 1, 1, 130},
	} {
		b.Run(tt.name, func(b *testing.B) {
			dir := b.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(tt.content), 0600); err != nil {
				b.Fatal(err)
			}
			reader := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
			b.ReportAllocs()
			b.SetBytes(int64(len(tt.content)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				lines, total, err := reader.ReadLines(context.Background(), "file.txt", tt.startLine, tt.maxLines)
				if err != nil || total != tt.wantTotal || len(lines) != tt.maxLines {
					b.Fatalf("ReadLines() returned %d lines, total %d, err %v", len(lines), total, err)
				}
			}
		})
	}
}
