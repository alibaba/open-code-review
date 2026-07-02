package main

import (
	"strings"
	"testing"
)

func TestRunConfig_UnknownSubcommand(t *testing.T) {
	err := runConfig([]string{"delete", "foo"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error = %q, expected to contain 'unknown'", err.Error())
	}
}

func TestRunConfig_InvalidSetMissingValue(t *testing.T) {
	err := runConfig([]string{"set", "provider"})
	if err == nil {
		t.Fatal("expected error for set without value")
	}
}

func TestRunConfig_InvalidUnsetMissingKey(t *testing.T) {
	err := runConfig([]string{"unset"})
	if err == nil {
		t.Fatal("expected error for unset without key")
	}
}
