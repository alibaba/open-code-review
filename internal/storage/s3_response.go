// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package storage

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// statusError converts a non-success response into an error that names the
// status and quotes a bounded prefix of the server's explanation, which is where
// S3-compatible servers put the actual reason (bad signature, missing bucket).
func statusError(action, key string, resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	detail := strings.TrimSpace(string(excerpt))
	if detail == "" {
		return fmt.Errorf("storage: %s %q: unexpected status %s", action, key, resp.Status)
	}
	return fmt.Errorf("storage: %s %q: unexpected status %s: %s", action, key, resp.Status, detail)
}

// listBucketResult is the subset of the ListObjectsV2 response this package
// reads: the keys, and whether another page follows.
type listBucketResult struct {
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
	Contents              []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
}
