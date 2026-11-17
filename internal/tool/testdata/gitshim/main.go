// Command gitshim is a test double for the git binary in the nil-Runner exec
// tests. It blocks without writing anything, so a cancelled `git grep` leaves
// both streams empty — the shape that used to be reported as "No matches found"
// on Windows, where exec.CommandContext kills the process with exit code 1
// instead of the signal that yields -1 on Unix (#1946).
//
// It lives under testdata so it is excluded from ./... builds and coverage.
package main

import "time"

func main() {
	// Long enough that only cancellation, never this sleep, ends the process.
	time.Sleep(time.Hour)
}
