// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"bufio"
	"io"
)

// sessionRecordReader lends each record to an immediate JSON decoder. Decoded
// fields own their storage, so the next read can reuse both the buffered input
// and the overflow buffer used for records larger than bufio.Reader's buffer.
type sessionRecordReader struct {
	reader   *bufio.Reader
	overflow []byte
}

func newSessionRecordReader(reader io.Reader) *sessionRecordReader {
	return &sessionRecordReader{reader: bufio.NewReader(reader)}
}

// readRecord includes the newline when present and preserves partial data on
// read errors. The returned bytes are valid only until the next readRecord.
func (r *sessionRecordReader) readRecord() ([]byte, error) {
	fragment, err := r.reader.ReadSlice('\n')
	if err != bufio.ErrBufferFull {
		return fragment, err
	}
	r.overflow = append(r.overflow[:0], fragment...)
	for err == bufio.ErrBufferFull {
		fragment, err = r.reader.ReadSlice('\n')
		r.overflow = append(r.overflow, fragment...)
	}
	return r.overflow, err
}
