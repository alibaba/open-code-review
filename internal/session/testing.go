package session

// UseTestSessions redirects session persistence to the "test-sessions"
// subdirectory so that test runs do not pollute the real sessions store.
//
// It also redirects the dismissal store to a "test-dismissals" subdirectory so
// dismissal tests stay offline and hermetic alongside session tests.
//
// It must be called from init() in a _test.go file or from TestMain,
// before any test goroutines start. It is NOT safe for concurrent use.
func UseTestSessions() {
	sessionSubDir = "test-sessions"
	dismissalSubDir = "test-dismissals"
}
