// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package stdout

import (
	"io"
	"os"
	"strings"
	"sync"
)

var errorWriter io.Writer

func ErrorWriter() io.Writer {
	mu.RLock()
	defer mu.RUnlock()
	if errorWriter != nil {
		return errorWriter
	}
	return os.Stderr
}

func StyleProgress(progressColor, errorColor bool) func() {
	mu.Lock()
	previous, previousErrors := w, errorWriter
	w = progressWriter(w, progressColor)
	errorWriter = progressWriter(os.Stderr, errorColor)
	mu.Unlock()
	return func() {
		mu.Lock()
		w, errorWriter = previous, previousErrors
		mu.Unlock()
	}
}

func progressWriter(destination io.Writer, enabled bool) io.Writer {
	if !enabled || destination == io.Discard {
		return destination
	}
	return &styledProgressWriter{destination: destination}
}

type styledProgressWriter struct {
	mu          sync.Mutex
	destination io.Writer
}

func (writer *styledProgressWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	lines := strings.SplitAfter(string(data), "\n")
	for index, line := range lines {
		lines[index] = styleProgressLine(line)
	}
	styled := strings.Join(lines, "")
	written, err := io.WriteString(writer.destination, styled)
	if err != nil {
		return 0, err
	}
	if written != len(styled) {
		return 0, io.ErrShortWrite
	}
	return len(data), nil
}

func styleProgressLine(line string) string {
	rest, ok := strings.CutPrefix(line, "[ocr]")
	if !ok {
		return line
	}
	prefix := "\x1b[2m[ocr]\x1b[0m"
	for _, status := range []struct{ marker, color string }{
		{"▶", "36"},
		{"✔", "32"},
		{"✘", "31"},
	} {
		if tool, found := strings.CutPrefix(rest, "   "+status.marker+" "); found {
			end := strings.IndexAny(tool, " \t\r\n")
			if end < 0 {
				end = len(tool)
			}
			return prefix + "   \x1b[" + status.color + "m" + status.marker + "\x1b[0m \x1b[1;36m" + tool[:end] + "\x1b[0m" + tool[end:]
		}
	}
	return prefix + rest
}
