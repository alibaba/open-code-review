// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package storage

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestSignMatchesAWSDocumentedExample pins the whole signing pipeline — canonical
// request, string to sign, signing key derivation and the Authorization header —
// against the worked example published in the S3 SigV4 documentation
// ("Example: GET Object" under "Signature Calculations for the Authorization
// Header"). The request carries a Range header, which the signer covers because
// the example signs it; every other part of the request is built the same way the
// client builds its own.
func TestSignMatchesAWSDocumentedExample(t *testing.T) {
	const (
		documentedSignature = "f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
		documentedScope     = "20130524/us-east-1/s3/aws4_request"
	)

	s := newSigner("AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "", "us-east-1")
	s.now = func() time.Time { return time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC) }

	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Range", "bytes=0-9")

	s.sign(req, emptyHash, "range")

	authorization := req.Header.Get("Authorization")
	if !strings.Contains(authorization, "SignedHeaders=host;range;x-amz-content-sha256;x-amz-date") {
		t.Errorf("SignedHeaders does not cover the request's headers: %q", authorization)
	}
	if !strings.Contains(authorization, "Credential=AKIAIOSFODNN7EXAMPLE/"+documentedScope) {
		t.Errorf("credential scope is wrong: %q", authorization)
	}
	if !strings.HasSuffix(authorization, "Signature="+documentedSignature) {
		t.Errorf("signature = %q, want the documented %s", authorization, documentedSignature)
	}
	if got := req.Header.Get("x-amz-content-sha256"); got != emptyHash {
		t.Errorf("x-amz-content-sha256 = %q, want the empty-body hash", got)
	}
}

func TestSignOverridesCallerValues(t *testing.T) {
	s := newSigner("key", "secret", "", "us-west-2")
	s.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

	req, err := http.NewRequest(http.MethodPut, "https://s3.example.com/bucket/key.jsonl", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	// A caller-supplied date would silently produce an expired signature, so the
	// signer always replaces it.
	req.Header.Set("x-amz-date", "19990101T000000Z")

	s.sign(req, hexSHA256([]byte("body")))

	if got := req.Header.Get("x-amz-date"); got != "20260102T030405Z" {
		t.Errorf("x-amz-date = %q, want the signer's own timestamp", got)
	}
	if !strings.Contains(req.Header.Get("Authorization"), "SignedHeaders=host;x-amz-content-sha256;x-amz-date") {
		t.Errorf("unexpected signed header set: %q", req.Header.Get("Authorization"))
	}
}

func TestCanonicalQuerySortsAndEscapes(t *testing.T) {
	parsed, err := url.Parse("https://s3.example.com/bucket?list-type=2&prefix=sessions%2FMy%20Repo%2F&continuation-token=a%2Fb")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := canonicalQuery(parsed)
	want := "continuation-token=a%2Fb&list-type=2&prefix=sessions%2FMy%20Repo%2F"
	if got != want {
		t.Errorf("canonicalQuery = %q, want %q", got, want)
	}
	if strings.Contains(got, "+") {
		t.Error("a space must be %20 in a canonical query, not +")
	}
	if canonicalQuery(&url.URL{}) != "" {
		t.Error("an absent query string must stay empty")
	}
}

func TestAWSEscape(t *testing.T) {
	// Unreserved characters stay literal; everything else is percent-encoded, a
	// space included. The '+' case matters: an S3 key containing one must not be
	// sent as an unescaped plus, which a server reads as a space.
	cases := map[string]string{
		"abcXYZ019":        "abcXYZ019",
		"-_.~":             "-_.~",
		"sessions/":        "sessions%2F",
		"My Repo":          "My%20Repo",
		"a+b":              "a%2Bb",
		"C_Users-dev":      "C_Users-dev",
		"oldsession.jsonl": "oldsession.jsonl",
		"caf\u00e9":        "caf%C3%A9",
	}
	for input, want := range cases {
		if got := awsEscape(input); got != want {
			t.Errorf("awsEscape(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestHexSHA256(t *testing.T) {
	if got := hexSHA256([]byte("abc")); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("hexSHA256(abc) = %q", got)
	}
	if emptyHash != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("emptyHash = %q, want the SHA-256 of an empty body", emptyHash)
	}
}
