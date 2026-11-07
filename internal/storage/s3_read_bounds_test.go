// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestS3ListKeepsTheServerErrorDetail pins that a rejected list still reports
// what the server said. statusError quotes the response body, and the XML
// decoder also reads it, so checking the status only after decoding would leave
// the error without the detail the server sent.
func TestS3ListKeepsTheServerErrorDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<Error><Code>AccessDenied</Code><Message>no list permission</Message></Error>"))
	}))
	defer server.Close()

	backend, err := newS3Backend(S3Config{
		Endpoint: server.URL, Bucket: "test-bucket", Region: "us-west-2",
		AccessKeyID: "a", SecretAccessKey: "b", PathStyle: true,
	})
	if err != nil {
		t.Fatalf("newS3Backend: %v", err)
	}

	_, err = backend.List(context.Background(), "sessions/")
	if err == nil {
		t.Fatal("List against a rejecting server returned no error")
	}
	if !strings.Contains(err.Error(), "AccessDenied") {
		t.Errorf("the list error lost the server's detail: %v", err)
	}
}

// TestReadBoundedRefusesAnOversizedBody pins the bound itself: a body larger
// than the limit is reported rather than buffered without limit, and a body
// exactly at the limit is still accepted.
func TestReadBoundedRefusesAnOversizedBody(t *testing.T) {
	if _, err := readBounded(strings.NewReader("0123456789"), "sessions/id.jsonl", 4); err == nil {
		t.Fatal("an oversized body was accepted")
	} else if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("the error does not name the bound: %v", err)
	}

	data, err := readBounded(strings.NewReader("0123"), "sessions/id.jsonl", 4)
	if err != nil {
		t.Fatalf("a body exactly at the limit was refused: %v", err)
	}
	if string(data) != "0123" {
		t.Errorf("data = %q, want the whole body", data)
	}
}
