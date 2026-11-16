// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package stdout

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestProgressStylePreservesText(test *testing.T) {
	for _, fixture := range []struct {
		line, color string
	}{
		{"[ocr] 2 file(s) changed, reviewing 1\n", "\x1b[2m[ocr]\x1b[0m"},
		{"[ocr]   ▶ file_read start_line=1 end_line=430 file_path=src/main.go\n", "\x1b[36m▶\x1b[0m"},
		{"[ocr]   ▶ code_search\n", "\x1b[1;36mcode_search\x1b[0m"},
		{"[ocr]   ▶ task_done", "\x1b[1;36mtask_done\x1b[0m"},
		{"[ocr]   ✔ file_read (56ms)\n", "\x1b[32m✔\x1b[0m"},
		{"[ocr]   ✘ file_read failed: permission denied\n", "\x1b[31m✘\x1b[0m"},
		{"Report text containing [ocr] stays untouched.\n", ""},
		{"{\"analysis\":\"**Markdown**\"}\n", ""},
	} {
		var buffer bytes.Buffer
		writer := progressWriter(&buffer, true)
		written, err := io.WriteString(writer, fixture.line)
		if err != nil || written != len(fixture.line) {
			test.Fatalf("Write = %d, %v", written, err)
		}
		got := buffer.String()
		if ansi.Strip(got) != fixture.line {
			test.Errorf("content changed: %q -> %q", fixture.line, got)
		}
		if fixture.color == "" {
			if got != fixture.line {
				test.Errorf("non-progress text styled: %q", got)
			}
		} else if !strings.Contains(got, fixture.color) {
			test.Errorf("missing style %q: %q", fixture.color, got)
		}
	}
	var buffer bytes.Buffer
	plain := progressWriter(&buffer, false)
	if plain != &buffer || progressWriter(io.Discard, true) != io.Discard {
		test.Fatal("disabled/discard writer was wrapped")
	}
}

func TestProgressStyleConcurrentWrites(test *testing.T) {
	var buffer bytes.Buffer
	writer := progressWriter(&buffer, true)
	line := "[ocr]   ▶ file_read path=main.go\n[ocr]   ✔ file_read (1ms)\n"
	var workers sync.WaitGroup
	for range 20 {
		workers.Go(func() {
			fmt.Fprint(writer, line)
		})
	}
	workers.Wait()
	if got := ansi.Strip(buffer.String()); got != strings.Repeat(line, 20) {
		test.Fatalf("interleaved or lost progress: %q", got)
	}
}

type progressFailureWriter struct {
	err error
}

func (writer progressFailureWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

func TestProgressStyleWriteErrors(test *testing.T) {
	failure := errors.New("closed progress stream")
	for _, fixture := range []struct{ failure, want error }{{failure, failure}, {nil, io.ErrShortWrite}} {
		writer := progressWriter(progressFailureWriter{err: fixture.failure}, true)
		written, err := io.WriteString(writer, "[ocr] progress\n")
		if written != 0 || !errors.Is(err, fixture.want) {
			test.Errorf("Write = %d, %v; want %v", written, err, fixture.want)
		}
	}
}

func TestStyleProgressRestoresStreams(test *testing.T) {
	if ErrorWriter() != os.Stderr {
		test.Fatal("default error destination changed")
	}
	var buffer bytes.Buffer
	defer Swap(&buffer)()
	restore := StyleProgress(true, true)
	defer restore()
	styled, ok := ErrorWriter().(*styledProgressWriter)
	if !ok || styled.destination != os.Stderr {
		test.Fatal("error stream is not styled stderr")
	}
	fmt.Fprintln(Writer(), "[ocr] progress")
	if !strings.Contains(buffer.String(), "\x1b[2m[ocr]") {
		test.Fatal("progress stream was not styled")
	}
	quietRestore := Quiet()
	if Writer() != io.Discard {
		test.Fatal("quiet did not suppress styled progress")
	}
	quietRestore()
	restore()
	if Writer() != &buffer || ErrorWriter() != os.Stderr {
		test.Fatal("stream restoration failed")
	}
}
