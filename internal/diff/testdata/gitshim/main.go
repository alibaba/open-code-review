// Command gitshim is a test double for the git binary. It stands in for a real
// git in the nil-Runner exec tests: it can stream content forever (so a
// cancellation lands mid-write) or fail with content on both streams (so the
// error-splitting behavior is exercised).
//
// It lives under testdata so it is excluded from ./... builds and coverage.
package main

import (
	"fmt"
	"os"
)

func main() {
	secret := os.Getenv("GITSHIM_SECRET")
	switch os.Getenv("GITSHIM_MODE") {
	case "refuse":
		// Fails like a git command that hits a repository error: plausible
		// diff content on stdout, a diagnosis on stderr, non-zero exit.
		fmt.Println("+" + secret)
		fmt.Fprintln(os.Stderr, "fatal: shim refused")
		os.Exit(128)
	case "stream":
		// Streams diff content forever, so cancellation always lands inside
		// the write window rather than depending on timing.
		for {
			fmt.Println("+" + secret)
		}
	default:
		fmt.Fprintln(os.Stderr, "gitshim: unknown GITSHIM_MODE")
		os.Exit(2)
	}
}
