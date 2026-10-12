// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

// UseTestSessions redirects session persistence to the "test-sessions"
// subdirectory and raw captures to "test-raw", so that test runs do not
// pollute the real stores.
//
// It must be called from init() in a _test.go file or from TestMain,
// before any test goroutines start. It is NOT safe for concurrent use.
func UseTestSessions() {
	sessionSubDir = "test-sessions"
	rawSubDir = "test-raw"
}
