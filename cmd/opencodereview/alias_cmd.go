// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Manage command aliases",
	Long: `Command aliases let you shorten frequently used ocr commands.

Examples:
  ocr alias set cp 'config provider'
  ocr alias set rq 'review --audience agent'
  ocr alias list
  ocr alias rm cp`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var aliasSetCmd = &cobra.Command{
	Use:     "set <name> <command>",
	Short:   "Set a command alias",
	Long:    "Set a command alias. The command argument must be quoted as a single string.",
	Example: "  ocr alias set cp 'config provider'\n  ocr alias set rq 'review --audience agent'",
	Args:    exactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAliasSet(args[0], args[1])
	},
}

var aliasRmCmd = &cobra.Command{
	Use:     "rm <name>",
	Short:   "Remove a command alias",
	Example: "  ocr alias rm cp",
	Args:    exactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAliasRm(args[0])
	},
}

var aliasListCmd = &cobra.Command{
	Use:   "list",
	Short: "List command aliases",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAliasList()
	},
}

func init() {
	aliasCmd.AddCommand(aliasSetCmd)
	aliasCmd.AddCommand(aliasRmCmd)
	aliasCmd.AddCommand(aliasListCmd)
}

func runAliasSet(name, command string) error {
	if err := validateAliasName(name); err != nil {
		return err
	}
	if err := validateAliasTarget(command); err != nil {
		return err
	}

	configPath, err := defaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Aliases == nil {
		cfg.Aliases = make(map[string]string)
	}
	cfg.Aliases[name] = command

	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Printf("Alias %q set to %q\n", name, command)
	return nil
}

func runAliasRm(name string) error {
	if err := validateAliasName(name); err != nil {
		return err
	}

	configPath, err := defaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if _, ok := cfg.Aliases[name]; !ok {
		return fmt.Errorf("alias %q not found", name)
	}

	delete(cfg.Aliases, name)
	if len(cfg.Aliases) == 0 {
		cfg.Aliases = nil
	}

	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Printf("Removed alias %q.\n", name)
	return nil
}

func runAliasList() error {
	configPath, err := defaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if len(cfg.Aliases) == 0 {
		fmt.Println("No aliases configured.")
		return nil
	}

	names := make([]string, 0, len(cfg.Aliases))
	for name := range cfg.Aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("%s = %s\n", name, cfg.Aliases[name])
	}
	return nil
}

func validateAliasName(name string) error {
	if !validAliasName(name) {
		return fmt.Errorf("invalid alias name %q: use letters, digits, '-' or '_' and start with a letter or digit", name)
	}
	if isReservedAliasName(name) {
		return fmt.Errorf("alias name %q conflicts with a built-in command", name)
	}
	return nil
}

func validAliasName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return false
	}
	if name[0] == '-' || name[0] == '_' {
		return false
	}
	return true
}

func isReservedAliasName(name string) bool {
	if name == "help" || strings.HasPrefix(name, "__") {
		return true
	}
	cmd, _, err := rootCmd.Find([]string{name})
	return err == nil && cmd != rootCmd
}

func validateAliasTarget(command string) error {
	tokens, err := parseAliasTarget(command)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return fmt.Errorf("alias command cannot be empty")
	}
	for _, token := range tokens {
		if token == "" {
			return fmt.Errorf("alias command cannot contain an empty argument")
		}
	}
	if strings.HasPrefix(tokens[0], "-") {
		return fmt.Errorf("alias command must start with an ocr command")
	}
	if tokens[0] == "ocr" {
		return fmt.Errorf("alias command must not include the leading 'ocr' command")
	}

	path := aliasCommandPath(tokens)
	cmd, _, err := rootCmd.Find(path)
	if err != nil {
		return fmt.Errorf("unknown alias command %q: %w", command, err)
	}
	if cmd == rootCmd {
		return fmt.Errorf("unknown alias command %q", command)
	}
	return nil
}

// aliasCommandPath returns the non-flag prefix of an alias command. It is used
// to validate that the alias points to a registered command before the
// command's flags and positional arguments are considered.
func aliasCommandPath(tokens []string) []string {
	for i, token := range tokens {
		if strings.HasPrefix(token, "-") {
			return tokens[:i]
		}
	}
	return tokens
}

// parseAliasTarget splits a stored alias command into argv tokens using simple
// shell-like rules: whitespace separates tokens, single quotes preserve
// everything until the next single quote, double quotes preserve everything
// except escaped double quotes, and backslash escapes the next character
// outside single quotes.
func parseAliasTarget(command string) ([]string, error) {
	tokens := make([]string, 0, strings.Count(command, " ")+1)
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	started := false

	flush := func() {
		if started {
			tokens = append(tokens, current.String())
			current.Reset()
			started = false
		}
	}

	for i := 0; i < len(command); i++ {
		c := command[i]
		if escaped {
			current.WriteByte(c)
			escaped = false
			started = true
			continue
		}
		if c == '\\' && !inSingleQuote {
			escaped = true
			started = true
			continue
		}
		if inSingleQuote {
			if c == '\'' {
				inSingleQuote = false
			} else {
				current.WriteByte(c)
			}
			started = true
			continue
		}
		if inDoubleQuote {
			if c == '"' {
				inDoubleQuote = false
			} else {
				current.WriteByte(c)
			}
			started = true
			continue
		}

		switch c {
		case '\'':
			inSingleQuote = true
			started = true
		case '"':
			inDoubleQuote = true
			started = true
		case ' ', '\t', '\r', '\n':
			flush()
		default:
			current.WriteByte(c)
			started = true
		}
	}

	if escaped {
		return nil, fmt.Errorf("alias command has a trailing backslash")
	}
	if inSingleQuote || inDoubleQuote {
		return nil, fmt.Errorf("alias command has an unclosed quote")
	}
	flush()
	return tokens, nil
}

func expandConfiguredAliases(args []string) ([]string, error) {
	configPath, err := defaultConfigPath()
	if err != nil {
		return args, nil
	}
	cfg, err := LoadAppConfig(configPath)
	if err != nil || cfg == nil {
		return args, nil
	}
	return expandAliases(args, cfg.Aliases)
}

func expandAliases(args []string, aliases map[string]string) ([]string, error) {
	if len(args) == 0 || len(aliases) == 0 {
		return args, nil
	}

	target, ok := aliases[args[0]]
	if !ok {
		return args, nil
	}

	tokens, err := parseAliasTarget(target)
	if err != nil {
		return nil, fmt.Errorf("expand alias %q: %w", args[0], err)
	}
	if err := validateAliasTarget(target); err != nil {
		return nil, fmt.Errorf("invalid alias %q: %w", args[0], err)
	}

	expanded := make([]string, 0, len(tokens)+len(args)-1)
	expanded = append(expanded, tokens...)
	expanded = append(expanded, args[1:]...)
	return expanded, nil
}
