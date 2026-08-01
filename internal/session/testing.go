package session

// UseTestSessions redirects session persistence to the "test-sessions"
// subdirectory so that test runs do not pollute the real sessions store.
//
// It must be called from init() in a _test.go file or from TestMain,
// before any test goroutines start. It is NOT safe for concurrent use.
func UseTestSessions() {
	sessionSubDir = "test-sessions"
}
