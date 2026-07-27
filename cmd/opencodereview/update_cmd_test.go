package main

import (
	"testing"
)

func TestDisplayVersionDev(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()
	Version = "dev"
	if got := displayVersion(); got != "dev" {
		t.Fatalf("displayVersion() = %q, want %q", got, "dev")
	}
}

func TestDisplayVersionEmpty(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()
	Version = ""
	if got := displayVersion(); got != "dev" {
		t.Fatalf("displayVersion() = %q, want %q", got, "dev")
	}
}

func TestDisplayVersionSet(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()
	Version = "v1.7.17"
	if got := displayVersion(); got != "v1.7.17" {
		t.Fatalf("displayVersion() = %q, want %q", got, "v1.7.17")
	}
}

func TestRunUpdateHelp(t *testing.T) {
	// --help should not return an error.
	if err := runUpdate([]string{"--help"}); err != nil {
		t.Fatalf("runUpdate(--help) returned error: %v", err)
	}
}

func TestRunUpdateShortHelp(t *testing.T) {
	// -h should also work (ocrFlagSet handles this).
	if err := runUpdate([]string{"-h"}); err != nil {
		t.Fatalf("runUpdate(-h) returned error: %v", err)
	}
}

func TestRunUpdateCheckNoUpdate(t *testing.T) {
	// With version "dev" and a check, it should either succeed or fail
	// gracefully (network may not be available in test env).
	// We just verify it doesn't panic.
	_ = runUpdate([]string{"--check"})
}

func TestNpmUpdateHint(t *testing.T) {
	if err := npmUpdateHint("v1.7.17"); err != nil {
		t.Fatalf("npmUpdateHint returned error: %v", err)
	}
}
