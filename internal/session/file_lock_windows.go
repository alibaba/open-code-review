//go:build windows

package session

import "os"

// Windows builds rely on per-process mutexes; cross-process JSONL locking is best-effort.
func lockSessionFile(*os.File) error   { return nil }
func unlockSessionFile(*os.File) error { return nil }
