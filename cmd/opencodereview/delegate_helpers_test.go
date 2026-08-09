// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
)

func TestValidateDelegateOptions(t *testing.T) {
	cases := []struct {
		name    string
		opts    delegateOptions
		wantErr bool
	}{
		{"workspace", delegateOptions{format: "text"}, false},
		{"commit", delegateOptions{commit: "abc", format: "text"}, false},
		{"range", delegateOptions{from: "main", to: "dev", format: "text"}, false},
		{"json format", delegateOptions{format: "json"}, false},
		{"empty format", delegateOptions{}, true},
		{"invalid format", delegateOptions{format: "yaml"}, true},
		{"from without to", delegateOptions{from: "main"}, true},
		{"to without from", delegateOptions{to: "dev"}, true},
		{"commit and range mixed", delegateOptions{commit: "abc", from: "main", to: "dev"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateDelegateOptions(&c.opts)
			if (err != nil) != c.wantErr {
				t.Errorf("validateDelegateOptions() err = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestDelegateFlags_RegisterJSONFormat(t *testing.T) {
	var opts delegateOptions
	cmd := &cobra.Command{
		Use:           "preview",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return validateDelegateOptions(&opts)
		},
	}
	registerDelegateFlags(cmd, &opts)
	cmd.SetArgs([]string{"--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("delegate preview rejected --format json: %v", err)
	}
	if opts.format != "json" {
		t.Fatalf("format = %q, want json", opts.format)
	}

	formatFlag := cmd.Flags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("delegate preview did not register --format")
	}
	if formatFlag.Shorthand != "f" {
		t.Fatalf("format shorthand = %q, want f", formatFlag.Shorthand)
	}

	for _, command := range []*cobra.Command{delegatePreviewCmd, delegateRuleCmd} {
		if command.Flags().Lookup("format") == nil {
			t.Errorf("%s did not register --format", command.Name())
		}
	}
}

func TestDelegateContextReviewMode(t *testing.T) {
	cases := []struct {
		name string
		opts delegateOptions
		want string
	}{
		{"commit", delegateOptions{commit: "abc"}, "commit"},
		{"range", delegateOptions{from: "main", to: "dev"}, "range"},
		{"workspace", delegateOptions{}, "workspace"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dc := &delegateContext{cc: &commonContext{}, opts: c.opts}
			if got := dc.reviewMode(); got != c.want {
				t.Errorf("reviewMode() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDelegateContextMergeBaseEmptyForNonRange(t *testing.T) {
	dc := &delegateContext{cc: &commonContext{}, opts: delegateOptions{commit: "abc"}}
	if got := dc.mergeBase(context.Background()); got != "" {
		t.Errorf("mergeBase(commit mode) = %q, want empty", got)
	}
	dc = &delegateContext{cc: &commonContext{}, opts: delegateOptions{}}
	if got := dc.mergeBase(context.Background()); got != "" {
		t.Errorf("mergeBase(workspace mode) = %q, want empty", got)
	}
}

func TestDelegateContextResolver(t *testing.T) {
	dc := &delegateContext{cc: &commonContext{}, opts: delegateOptions{}}
	if got := dc.resolver(); got != nil {
		t.Errorf("resolver() = %v, want nil", got)
	}
}
