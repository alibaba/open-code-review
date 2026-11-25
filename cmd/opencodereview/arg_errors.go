// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Positional-argument validators that replace Cobra's raw count message
// ("accepts 2 arg(s), received 1") with an actionable one built from metadata
// the command already declares — the positional signature in Use, plus Example
// and ValidArgs where present — so the guidance cannot drift from the command's
// own help output. Because the root command sets SilenceUsage, everything the
// user sees has to travel in the error itself.

// exactArgs behaves like cobra.ExactArgs(n) but reports a wrong count with the
// command's own usage information.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == n {
			return nil
		}
		return argCountError(cmd, fmt.Sprintf("requires exactly %d argument(s)", n))
	}
}

// minimumArgs behaves like cobra.MinimumNArgs(n) but reports too few arguments
// with the command's own usage information.
func minimumArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) >= n {
			return nil
		}
		return argCountError(cmd, fmt.Sprintf("requires at least %d argument(s)", n))
	}
}

// maximumArgs behaves like cobra.MaximumNArgs(n) but reports too many arguments
// with the command's own usage information.
func maximumArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) <= n {
			return nil
		}
		return argCountError(cmd, fmt.Sprintf("accepts at most %d argument(s)", n))
	}
}

// argCountError assembles the guidance for a wrong argument count. The count the
// user supplied is deliberately not echoed back — it adds nothing they do not
// already know from the line they just typed.
func argCountError(cmd *cobra.Command, requirement string) error {
	var b strings.Builder
	b.Grow(256)

	path := cmd.CommandPath()
	fmt.Fprintf(&b, "%q %s", path, requirement)
	if sig := positionalSignature(cmd); sig != "" {
		fmt.Fprintf(&b, " (%s)", sig)
	}

	if len(cmd.ValidArgs) > 0 {
		b.WriteString("\n\nValid values: ")
		b.WriteString(strings.Join(validArgNames(cmd.ValidArgs), ", "))
	}

	b.WriteString("\n\nUsage:\n  ")
	b.WriteString(cmd.UseLine())

	if example := strings.TrimRight(cmd.Example, "\n"); example != "" {
		b.WriteString("\n\nExample:\n")
		b.WriteString(example)
	}

	b.WriteString("\n\nRun '")
	b.WriteString(path)
	b.WriteString(" --help' for more information.")

	return errors.New(b.String())
}

// positionalSignature extracts the positional-argument part of cmd.Use, e.g.
// "<key> <value>" from "set <key> <value>" and "<path...>" from
// "rule [flags] <path...>". Bracketed fields are not placeholders the user
// types, so "[flags]" and inline enumerations such as "[bash|zsh]" are dropped;
// enumerated values are reported from ValidArgs instead.
func positionalSignature(cmd *cobra.Command) string {
	fields := strings.Fields(cmd.Use)
	if len(fields) < 2 {
		return ""
	}

	parts := make([]string, 0, len(fields)-1)
	for _, f := range fields[1:] {
		if strings.HasPrefix(f, "<") {
			parts = append(parts, f)
		}
	}
	return strings.Join(parts, " ")
}

// validArgNames strips the "name\tdescription" form Cobra allows in ValidArgs,
// keeping only the value the user is expected to type. The split matches the one
// cobra.OnlyValidArgs applies, so the listed values are exactly those accepted.
func validArgNames(valid []string) []string {
	names := make([]string, 0, len(valid))
	for _, v := range valid {
		names = append(names, strings.SplitN(v, "\t", 2)[0])
	}
	return names
}
