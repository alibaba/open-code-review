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
	warnings       []Event
	err            error
}

type streamHandle struct {
	result <-chan streamResult
	done   <-chan struct{}
	cancel func()
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

func readStderr(r io.Reader, byteLimit, lineLimit int, emit func(Event)) (string, bool, []Event, error) {
	reader := bufio.NewReaderSize(r, 32<<10)
	tail := tailBuffer{limit: byteLimit}
	warned := false
	var warnings []Event
	for {
		var line []byte
		truncated := false
		for {
			part, err := reader.ReadSlice('\n')
			if len(part) > 0 {
				tail.Write(part)
				if len(line) < lineLimit+1 {
					remaining := lineLimit + 1 - len(line)
					if len(part) > remaining {
						part = part[:remaining]
						truncated = true
					}
					line = append(line, part...)
				}
			}
			if err == bufio.ErrBufferFull {
				truncated = true
				continue
			}
			if err != nil && err != io.EOF {
				return tail.String(), tail.cut, warnings, err
			}
			if len(line) > 0 {
				message := strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r")
				if len(message) > lineLimit {
					message = message[:lineLimit]
					truncated = true
				}
				kind := EventDiagnostic
				if strings.HasPrefix(strings.TrimSpace(message), "[ocr]") {
					kind = EventProgress
				}
				emit(Event{Kind: kind, Message: message, Truncated: truncated, OccurredAt: time.Now()})
				if (truncated || tail.cut) && !warned {
					warned = true
					warning := Event{Kind: EventWarning, Message: "OCR stderr was truncated", Truncated: true, OccurredAt: time.Now()}
					warnings = append(warnings, warning)
					emit(warning)
				}
			}
			if err == io.EOF {
				return tail.String(), tail.cut, warnings, nil
			}
			break
		}
	}
}

func consumeStreams(stdout, stderr io.Reader, limits Limits, emit func(Event)) streamHandle {
	result := make(chan streamResult, 1)
	done := make(chan struct{}, 2)
	var stdoutData []byte
	var stdoutExceeded bool
	var stderrTail string
	var stderrCut bool
	var warnings []Event
	var stdoutErr error
	var stderrErr error
	go func() {
		defer func() { done <- struct{}{} }()
		stdoutData, stdoutExceeded, stdoutErr = readStdout(stdout, limits.StdoutBytes)
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		stderrTail, stderrCut, warnings, stderrErr = readStderr(stderr, limits.StderrBytes, limits.StderrLine, emit)
	}()
	allDone := make(chan struct{})
	go func() {
		<-done
		<-done
		close(allDone)
		if stdoutErr != nil {
			result <- streamResult{stdout: stdoutData, stdoutExceeded: stdoutExceeded, stderrTail: stderrTail, stderrCut: stderrCut, warnings: warnings, err: stdoutErr}
			return
		}
		result <- streamResult{stdout: stdoutData, stdoutExceeded: stdoutExceeded, stderrTail: stderrTail, stderrCut: stderrCut, warnings: warnings, err: stderrErr}
	}()
	return streamHandle{result: result, done: allDone, cancel: func() {
		if c, ok := stdout.(io.Closer); ok {
			_ = c.Close()
		}
		if c, ok := stderr.(io.Closer); ok {
			_ = c.Close()
		}
	}}
}
