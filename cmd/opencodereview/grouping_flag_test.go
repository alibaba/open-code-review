package main

import (
	"strings"
	"testing"
)

func TestGroupingFlag(t *testing.T) {
	for _, fixture := range []struct {
		args  []string
		valid bool
	}{
		{[]string{}, true},
		{[]string{"--grouping", "per-file"}, true},
		{[]string{"--grouping", "bundle-all"}, true},
		{[]string{"--grouping", "llm"}, true},
		{[]string{"--grouping", "everything"}, false},
	} {
		opts, err := parseReviewFlags(fixture.args)
		if err == nil {
			err = validateReviewOptions(&opts)
		}
		if (err == nil) != fixture.valid {
			t.Fatalf("args %v: valid=%v err=%v", fixture.args, fixture.valid, err)
		}
		if err != nil && !strings.Contains(err.Error(), "--grouping") {
			t.Fatalf("args %v: error %q does not name the flag", fixture.args, err)
		}
	}
}
