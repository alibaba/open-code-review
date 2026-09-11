// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"time"
)

type streamResult struct {
	stdout         []byte
	stdoutExceeded bool
	stderrTail     string
	stderrCut      bool
	err            error
}

type tailBuffer struct {
	limit int
	data  []byte
	cut   bool
}

func (b *tailBuffer) Write(p []byte) {
	if len(p) >= b.limit {
		b.data = append(b.data[:0], p[len(p)-b.limit:]...)
		b.cut = true
		return
	}
	if len(b.data)+len(p) > b.limit {
		drop := len(b.data) + len(p) - b.limit
		copy(b.data, b.data[drop:])
		b.data = b.data[:len(b.data)-drop]
		b.cut = true
	}
	b.data = append(b.data, p...)
}

func (b *tailBuffer) String() string { return string(b.data) }

func readStdout(r io.Reader, limit int) ([]byte, bool, error) {
	var out bytes.Buffer
	buf := make([]byte, 32<<10)
	exceeded := false
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if remaining := limit - out.Len(); remaining > 0 {
				if n <= remaining {
					_, _ = out.Write(buf[:n])
				} else {
					_, _ = out.Write(buf[:remaining])
					exceeded = true
				}
			} else {
				exceeded = true
			}
		}
		if err == io.EOF {
			return out.Bytes(), exceeded, nil
		}
		if err != nil {
			return out.Bytes(), exceeded, err
		}
	}
}

func readStderr(r io.Reader, byteLimit, lineLimit int, emit func(Event)) (string, bool, error) {
	reader := bufio.NewReaderSize(r, 32<<10)
	tail := tailBuffer{limit: byteLimit}
	warned := false
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			tail.Write([]byte(line))
			message, truncated := truncateLine(line, lineLimit)
			kind := EventDiagnostic
			if strings.HasPrefix(strings.TrimSpace(message), "[ocr]") {
				kind = EventProgress
			}
			emit(Event{Kind: kind, Message: message, Truncated: truncated, OccurredAt: time.Now()})
			if (truncated || tail.cut) && !warned {
				warned = true
				emit(Event{Kind: EventWarning, Message: "OCR stderr was truncated", Truncated: true, OccurredAt: time.Now()})
			}
		}
		if err == io.EOF {
			return tail.String(), tail.cut, nil
		}
		if err != nil {
			return tail.String(), tail.cut, err
		}
	}
}

func truncateLine(line string, limit int) (string, bool) {
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	if len(line) <= limit {
		return line, false
	}
	return line[:limit], true
}

func consumeStreams(stdout, stderr io.Reader, limits Limits, emit func(Event)) <-chan streamResult {
	result := make(chan streamResult, 1)
	done := make(chan struct{}, 2)
	var stdoutData []byte
	var stdoutExceeded bool
	var stderrTail string
	var stderrCut bool
	var stdoutErr error
	var stderrErr error
	go func() {
		defer func() { done <- struct{}{} }()
		stdoutData, stdoutExceeded, stdoutErr = readStdout(stdout, limits.StdoutBytes)
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		stderrTail, stderrCut, stderrErr = readStderr(stderr, limits.StderrBytes, limits.StderrLine, emit)
	}()
	go func() {
		<-done
		<-done
		if stdoutErr != nil {
			result <- streamResult{stdout: stdoutData, stdoutExceeded: stdoutExceeded, stderrTail: stderrTail, stderrCut: stderrCut, err: stdoutErr}
			return
		}
		result <- streamResult{stdout: stdoutData, stdoutExceeded: stdoutExceeded, stderrTail: stderrTail, stderrCut: stderrCut, err: stderrErr}
	}()
	return result
}
