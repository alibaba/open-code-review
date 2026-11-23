// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestSessionRecordReaderPreservesReadBytesResults(t *testing.T) {
	readFailure := errors.New("input failed")
	readers := map[string]func(string) io.Reader{
		"ordinary":     func(s string) io.Reader { return strings.NewReader(s) },
		"one-byte":     func(s string) io.Reader { return iotest.OneByteReader(strings.NewReader(s)) },
		"data-and-EOF": func(s string) io.Reader { return iotest.DataErrReader(strings.NewReader(s)) },
		"read-error":   func(s string) io.Reader { return io.MultiReader(strings.NewReader(s), iotest.ErrReader(readFailure)) },
	}
	for _, content := range []string{
		"", "\n", "one\r\ntwo\nlast", "invalid\xff\x00\nnext\n",
		strings.Repeat("x", 4095) + "\nnext\n",
		strings.Repeat("x", 4096) + "\nnext\n",
		strings.Repeat("x", 4097) + "\r\nnext\n",
		strings.Repeat("a", 64<<10) + "\nshort\n" + strings.Repeat("b", 48<<10),
	} {
		for name, create := range readers {
			t.Run(fmt.Sprintf("bytes=%d/%s", len(content), name), func(t *testing.T) {
				wantReader := bufio.NewReader(create(content))
				gotReader := newSessionRecordReader(create(content))
				for record := 0; ; record++ {
					want, wantErr := wantReader.ReadBytes('\n')
					got, gotErr := gotReader.readRecord()
					if !bytes.Equal(got, want) || !errors.Is(gotErr, wantErr) {
						t.Fatalf("record %d: got %d bytes, %v; want %d bytes, %v", record, len(got), gotErr, len(want), wantErr)
					}
					if wantErr != nil {
						break
					}
				}
			})
		}
	}
}
